package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/commerce"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/entitlements"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ClaimExpiredSubscriptions safely scans and claims active subscriptions whose entitlement has expired.
func (s *Store) ClaimExpiredSubscriptions(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var serverIDs []string
	err := s.readSnapshot(ctx, func(tx *Store) error {
		now, err := outboxNow(tx.db)
		if err != nil {
			return err
		}
		q := tx.db.Table("global_server_entitlements").
			Select("server_id").
			Where("status = ? AND ends_at_ms <= ?", "active", now).
			Order("ends_at_ms asc").
			Limit(limit)
		if tx.db.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		return q.Pluck("server_id", &serverIDs).Error
	})
	return serverIDs, err
}

// ExpireSubscriptionAndStopServer atomically sets the entitlement to 'suspended',
// marks the subscription as 'expired', transitions the server to 'stopped' if it was running,
// and records an audit activity event.
func (s *Store) ExpireSubscriptionAndStopServer(ctx context.Context, serverID string) error {
	if serverID == "" {
		return errors.New("server ID is required")
	}
	return s.Transaction(ctx, func(tx *Store) error {
		now, err := outboxNow(tx.db)
		if err != nil {
			return err
		}
		var ent entitlements.Record
		q := tx.db.Table("global_server_entitlements").Where("server_id = ?", serverID)
		if tx.db.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := q.Take(&ent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		// If entitlement was already suspended/revoked or renewed with ends_at_ms in the future, skip.
		if ent.Status != "active" || ent.EndsAtMS > now {
			return nil
		}

		if err := tx.lockWorkspace(ctx, ent.OrganizationID); err != nil {
			return err
		}

		// Update global_server_entitlements to suspended
		if err := tx.db.Table("global_server_entitlements").
			Where("server_id = ? AND version = ?", serverID, ent.Version).
			Updates(map[string]any{
				"status":  "suspended",
				"version": ent.Version + 1,
			}).Error; err != nil {
			return err
		}

		// Update service_subscriptions to expired
		_ = tx.db.Table("service_subscriptions").
			Where("server_id = ? AND status = ?", serverID, "active").
			Update("status", "expired").Error

		// Update game_servers & logical_servers
		var server domain.GameServer
		if err := tx.db.Table("game_servers").Where("id = ?", serverID).Take(&server).Error; err == nil {
			if server.Spec.DesiredState == domain.DesiredRunning {
				server.Spec.DesiredState = domain.DesiredStopped
				server.Spec.Generation++
				server.Status.Phase = domain.PhaseStopped
				server.UpdatedAt = time.Now().UTC()
				_ = tx.db.Table("game_servers").Where("id = ?", serverID).Save(&server).Error
			}
		}
		_ = tx.db.Table("logical_servers").
			Where("id = ? AND desired_state = ?", serverID, "running").
			Update("desired_state", "stopped").Error

		// Record activity
		event := domain.ActivityEvent{
			ID:             uuid.NewString(),
			InstanceID:     serverID,
			OrganizationID: ent.OrganizationID,
			Type:           "server.subscription.expired",
			Message:        fmt.Sprintf("Subscription expired for server %s; server automatically stopped to preserve data during grace period.", serverID),
			CreatedAt:      time.Now().UTC(),
		}
		_ = tx.CreateActivity(ctx, &event)

		return nil
	})
}

// CheckServerStartable verifies whether a server is permitted to start commercially.
// If an entitlement exists and is not active or has passed ends_at_ms, ErrSubscriptionExpired is returned.
func (s *Store) CheckServerStartable(ctx context.Context, serverID string) error {
	var ent entitlements.Record
	err := s.readSnapshot(ctx, func(tx *Store) error {
		return tx.db.Table("global_server_entitlements").Where("server_id = ?", serverID).Take(&ent).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Servers without commercial entitlements (self-hosted / unbilled) are not blocked.
		return nil
	}
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	if ent.Status != "active" || ent.EndsAtMS <= now {
		return commerce.ErrSubscriptionExpired
	}
	return nil
}
