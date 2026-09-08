package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ regional.RevisionTasks = (*RegionalStore)(nil)

func (s *RegionalStore) ClaimRevision(ctx context.Context, lease time.Duration) (*regional.RevisionClaim, error) {
	if lease < time.Millisecond || lease > time.Hour {
		return nil, errors.New("invalid revision lease")
	}
	var result *regional.RevisionClaim
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		var row struct{ OperationID, Payload string }
		err = tx.Table("regional_revision_tasks").Select("operation_id,payload").Where("status = ? AND next_attempt_ms <= ? AND lease_until_ms <= ?", "awaiting_revision", now, now).Order("next_attempt_ms,created_at,operation_id").Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		event, err := regional.DecodeRevisionNotification([]byte(row.Payload))
		if err != nil || event.RegionID != s.regionID || event.OperationID != row.OperationID {
			return regional.ErrInvalidNotification
		}
		now, err = outboxNow(tx)
		if err != nil {
			return err
		}
		token := uuid.NewString()
		if err := tx.Table("regional_revision_tasks").Where("operation_id = ?", row.OperationID).Updates(map[string]any{"lease_token": token, "lease_until_ms": now + lease.Milliseconds(), "attempts": gorm.Expr("attempts + 1")}).Error; err != nil {
			return err
		}
		result = &regional.RevisionClaim{Event: event, Token: token}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *RegionalStore) SaveRevision(ctx context.Context, claim regional.RevisionClaim, snapshot regional.RevisionSnapshot) error {
	if err := snapshot.ValidateFor(claim.Event); err != nil {
		return err
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return s.finishRevision(ctx, claim, 0, string(encoded))
}

func (s *RegionalStore) RetryRevision(ctx context.Context, claim regional.RevisionClaim, delay time.Duration) error {
	if delay < time.Millisecond || delay > 24*time.Hour {
		return errors.New("invalid revision retry delay")
	}
	return s.finishRevision(ctx, claim, delay, "")
}

func (s *RegionalStore) finishRevision(ctx context.Context, claim regional.RevisionClaim, delay time.Duration, snapshot string) error {
	if claim.Token == "" || claim.Event.RegionID != s.regionID {
		return regional.ErrRevisionClaimLost
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct {
			Payload      string
			LeaseUntilMS int64
		}
		err := tx.Table("regional_revision_tasks").Select("payload,lease_until_ms").Where("operation_id = ? AND lease_token = ? AND status = ?", claim.Event.OperationID, claim.Token, "awaiting_revision").Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return regional.ErrRevisionClaimLost
		}
		if err != nil {
			return err
		}
		event, err := regional.DecodeRevisionNotification([]byte(row.Payload))
		if err != nil || event != claim.Event {
			return regional.ErrRevisionClaimLost
		}
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		if row.LeaseUntilMS <= now {
			return regional.ErrRevisionClaimLost
		}
		values := map[string]any{"lease_token": "", "lease_until_ms": 0, "next_attempt_ms": now + delay.Milliseconds()}
		if snapshot != "" {
			values["snapshot"] = snapshot
			values["status"] = "revision_fetched"
			values["asset_lease_token"] = ""
			values["asset_lease_until_ms"] = 0
			values["asset_next_attempt_ms"] = 0
		}
		return tx.Table("regional_revision_tasks").Where("operation_id = ?", event.OperationID).Updates(values).Error
	})
}
