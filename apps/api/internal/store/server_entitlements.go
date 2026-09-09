package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/entitlements"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type entitlementChangeRow struct {
	entitlements.Record
	ActorID, RequestID, RequestHash, Reason string
}

// ChangeOperatorEntitlement is an audited platform-admin operation. It must not
// be used to synthesize paid orders. Replays return the original receipt without
// rolling back the current policy; payment-driven issuance remains separate.
func (s *Store) ChangeOperatorEntitlement(ctx context.Context, actor string, change entitlements.OperatorChange) (entitlements.Record, error) {
	if actor == "" {
		return entitlements.Record{}, entitlements.ErrOperatorRequired
	}
	if change.Validate() != nil {
		return entitlements.Record{}, entitlements.ErrInvalid
	}
	encoded, err := json.Marshal(change)
	if err != nil {
		return entitlements.Record{}, entitlements.ErrInvalid
	}
	digest := sha256.Sum256(encoded)
	hash := hex.EncodeToString(digest[:])
	var result entitlements.Record
	err = s.Transaction(ctx, func(tx *Store) error {
		var account struct {
			Role         domain.Role
			PlatformRole domain.PlatformRole
		}
		query := tx.db.WithContext(ctx).Table("admin_accounts").Select("role", "platform_role").Where("id = ?", actor)
		if tx.db.Dialector.Name() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "SHARE"})
		}
		err := query.Take(&account).Error
		if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && domain.NormalizePlatformRole(account.PlatformRole, account.Role) != domain.PlatformRoleAdmin) {
			return entitlements.ErrOperatorRequired
		}
		if err != nil {
			return err
		}
		var server instances.Server
		query = tx.db.Table("logical_servers").Where("id = ? AND organization_id = ?", change.ServerID, change.OrganizationID)
		if tx.db.Dialector.Name() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.Take(&server).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entitlements.ErrUnavailable
			}
			return err
		}
		if server.DesiredState == "deleted" {
			return entitlements.ErrUnavailable
		}
		var previous entitlementChangeRow
		err = tx.db.Table("global_entitlement_changes").Where("actor_id = ? AND request_id = ?", actor, change.RequestID).Take(&previous).Error
		if err == nil {
			if previous.RequestHash != hash {
				return entitlements.ErrRequestConflict
			}
			result = previous.Record
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var current entitlements.Record
		err = tx.db.Table("global_server_entitlements").Where("server_id = ?", change.ServerID).Take(&current).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if current.Version != change.ExpectedVersion {
			return entitlements.ErrVersionConflict
		}
		now, err := outboxNow(tx.db)
		if err != nil {
			return err
		}
		if change.Status == "active" && change.EndsAtMS <= now {
			return entitlements.ErrInvalid
		}
		result = entitlements.Record{Policy: change.Policy, Version: change.ExpectedVersion + 1, SourceKind: "operator", SourceID: uuid.NewString()}
		if err := tx.db.Table("global_server_entitlements").Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "server_id"}}, UpdateAll: true}).Create(&result).Error; err != nil {
			return err
		}
		row := entitlementChangeRow{Record: result, ActorID: actor, RequestID: change.RequestID, RequestHash: hash, Reason: change.Reason}
		return tx.db.Table("global_entitlement_changes").Create(&row).Error
	})
	if err != nil {
		return entitlements.Record{}, err
	}
	return result, nil
}

// RegionalRunEntitlement verifies current intent, placement and policy in one
// global read snapshot. authenticatedRegion must come from mTLS identity. This
// point-in-time record is not an execution lease and cannot fence a Node.
func (s *Store) RegionalRunEntitlement(ctx context.Context, authenticatedRegion string, event instances.RevisionAvailable, intentVersion int64) (entitlements.Record, error) {
	var result entitlements.Record
	err := s.withRegionalRevision(ctx, authenticatedRegion, event, func(tx *Store, snapshot regional.RevisionSnapshot) error {
		if snapshot.CurrentSpecGeneration != event.SpecGeneration || snapshot.IntentVersion != intentVersion || snapshot.DesiredState != "running" {
			return entitlements.ErrUnavailable
		}
		if err := tx.db.Table("global_server_entitlements").Where("server_id = ? AND organization_id = ?", event.ServerID, event.OrganizationID).Take(&result).Error; err != nil {
			return err
		}
		now, err := outboxNow(tx.db)
		if err != nil {
			return err
		}
		needed := snapshot.Revision.Specification.Resources
		if result.Policy.Validate() != nil || result.Status != "active" || result.Version < 1 || result.StartsAtMS > now || result.EndsAtMS <= now || result.CPU < needed.CPU || result.MemoryMB < needed.MemoryMB {
			return entitlements.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, regional.ErrRevisionUnavailable) {
			err = entitlements.ErrUnavailable
		}
		return entitlements.Record{}, err
	}
	return result, nil
}

func migrateSQLiteServerEntitlements(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("gamepanel_sqlite_migrations").Where("version = 10").Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		if err := tx.Exec(serverEntitlementsSQL).Error; err != nil {
			return err
		}
		for _, statement := range []string{
			"CREATE TRIGGER global_entitlement_changes_no_update BEFORE UPDATE ON global_entitlement_changes BEGIN SELECT RAISE(ABORT, 'entitlement change history is immutable'); END",
			"CREATE TRIGGER global_entitlement_changes_no_delete BEFORE DELETE ON global_entitlement_changes BEGIN SELECT RAISE(ABORT, 'entitlement change history is immutable'); END",
			"INSERT INTO gamepanel_sqlite_migrations(version) VALUES(10)",
		} {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
