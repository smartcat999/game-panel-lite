package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ backup.PreparationTasks = (*RegionalStore)(nil)

type backupPreparationRow struct {
	OperationID, BackupID, EventID, Payload, Status, LeaseToken string
	LeaseUntilMS                                                int64
}

func (r backupPreparationRow) request(region string) (backup.Requested, error) {
	var event backup.Requested
	if json.Unmarshal([]byte(r.Payload), &event) != nil || event.Validate() != nil || event.OperationID != r.OperationID || event.BackupID != r.BackupID || event.EventID != r.EventID || event.RegionID != region {
		return backup.Requested{}, backup.ErrInvalidRequest
	}
	return event, nil
}

func (s *RegionalStore) ClaimBackupPreparation(ctx context.Context, lease time.Duration) (*backup.PreparationClaim, error) {
	if lease < time.Millisecond || lease > time.Hour {
		return nil, backup.ErrInvalidRequest
	}
	var claim *backup.PreparationClaim
	var invalid error
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		var row backupPreparationRow
		err = tx.Table("regional_backup_requests").Where("status = ? AND next_attempt_ms <= ? AND lease_until_ms <= ?", "awaiting_authority", now, now).Order("next_attempt_ms,created_at,operation_id").Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		event, err := row.request(s.regionID)
		if err != nil {
			invalid = err
			return tx.Table("regional_backup_requests").Where("operation_id = ?", row.OperationID).Update("status", "rejected").Error
		}
		now, err = outboxNow(tx)
		if err != nil {
			return err
		}
		token := uuid.NewString()
		if err := tx.Table("regional_backup_requests").Where("operation_id = ?", row.OperationID).Updates(map[string]any{"lease_token": token, "lease_until_ms": now + lease.Milliseconds(), "attempts": gorm.Expr("attempts + 1")}).Error; err != nil {
			return err
		}
		claim = &backup.PreparationClaim{Request: event, Token: token}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claim, invalid
}

// checkBackupPreparation must run after acquiring the request row lock. Using
// database time after lock acquisition prevents an expired waiter committing.
func checkBackupPreparation(tx *gorm.DB, region string, row backupPreparationRow, claim backup.PreparationClaim) error {
	event, err := row.request(region)
	if err != nil || claim.Token == "" || event != claim.Request || row.LeaseToken != claim.Token || row.Status != "awaiting_authority" {
		return backup.ErrPreparationClaimLost
	}
	now, err := outboxNow(tx)
	if err != nil {
		return err
	}
	if row.LeaseUntilMS <= now {
		return backup.ErrPreparationClaimLost
	}
	return nil
}

func (s *RegionalStore) RetryBackupPreparation(ctx context.Context, claim backup.PreparationClaim, delay time.Duration) error {
	if delay < time.Millisecond || delay > 24*time.Hour {
		return backup.ErrInvalidRequest
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row backupPreparationRow
		err := tx.Table("regional_backup_requests").Where("operation_id = ?", claim.Request.OperationID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return backup.ErrPreparationClaimLost
		}
		if err != nil {
			return err
		}
		if err := checkBackupPreparation(tx, s.regionID, row, claim); err != nil {
			return err
		}
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		return tx.Table("regional_backup_requests").Where("operation_id = ?", row.OperationID).Updates(map[string]any{"lease_token": "", "lease_until_ms": 0, "next_attempt_ms": now + delay.Milliseconds()}).Error
	})
}
