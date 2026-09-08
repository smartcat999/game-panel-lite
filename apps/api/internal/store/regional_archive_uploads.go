package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type archiveUploadRow struct {
	OperationID, RequestEventID                                 string
	ID, StorageID, ObjectKey, Plan, Status, Receipt, LeaseToken string
	LeaseUntilMS                                                int64
}

var _ backup.UploadTasks = (*RegionalStore)(nil)

// PrepareArchiveUpload persists transport identity before network I/O. Only a
// trusted coordinator may call it after authorizing and preparing the snapshot.
func (s *RegionalStore) PrepareArchiveUpload(ctx context.Context, claim backup.PreparationClaim, plan backup.UploadPlan) error {
	if plan.Validate() != nil || plan.RegionID != s.regionID {
		return backup.ErrUploadPlan
	}
	payload, err := json.Marshal(plan)
	if err != nil || len(payload) > 16384 {
		return backup.ErrUploadPlan
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := archiveUploadRow{ID: plan.ID, OperationID: plan.OperationID, RequestEventID: plan.RequestEventID, StorageID: plan.StorageID, ObjectKey: plan.ObjectKey, Plan: string(payload), Status: "pending"}
		if err := tx.Table("regional_archive_uploads").Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		var stored archiveUploadRow
		if err := tx.Table("regional_archive_uploads").Where("id = ?", plan.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&stored).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return backup.ErrUploadPlan
			}
			return err
		}
		var original backup.UploadPlan
		if json.Unmarshal([]byte(stored.Plan), &original) != nil || original != plan || stored.StorageID != plan.StorageID || stored.ObjectKey != plan.ObjectKey || stored.OperationID != plan.OperationID || stored.RequestEventID != plan.RequestEventID {
			return backup.ErrUploadPlan
		}
		requestRow, err := lockArchiveRequest(tx, plan)
		if err != nil {
			return err
		}
		status := requestRow.Status
		request, err := requestRow.request(s.regionID)
		if err != nil || request != claim.Request || claim.Token == "" {
			return backup.ErrPreparationClaimLost
		}
		if stored.Status == "uploaded" && status == "uploaded" {
			return nil
		}
		if stored.Status != "pending" || (status != "awaiting_authority" && status != "preparing") {
			return backup.ErrUploadPlan
		}
		if status == "preparing" {
			return nil
		} // Exact persisted plan replay after a lost response.
		if err := checkBackupPreparation(tx, s.regionID, requestRow, claim); err != nil {
			return err
		}
		return tx.Table("regional_backup_requests").Where("operation_id = ?", plan.OperationID).Updates(map[string]any{"status": "preparing", "lease_token": "", "lease_until_ms": 0}).Error
	})
}

// Correlate the persisted command; this is bookkeeping, not execution authority.
// All upload mutations lock upload before request to keep the lock order stable.
func lockArchiveRequest(tx *gorm.DB, plan backup.UploadPlan) (backupPreparationRow, error) {
	var row backupPreparationRow
	err := tx.Table("regional_backup_requests").Where("operation_id = ?", plan.OperationID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return backupPreparationRow{}, backup.ErrUploadPlan
	}
	if err != nil {
		return backupPreparationRow{}, err
	}
	var request backup.Requested
	if json.Unmarshal([]byte(row.Payload), &request) != nil || request.Validate() != nil || request.BackupID != row.BackupID || request.EventID != row.EventID || request.OperationID != plan.OperationID || request.EventID != plan.RequestEventID || request.RegionID != plan.RegionID || request.ServerID != plan.ServerID || request.OrganizationID != plan.Asset.OrganizationID || request.PlacementEpoch != plan.PlacementEpoch {
		return backupPreparationRow{}, backup.ErrUploadPlan
	}
	return row, nil
}

func (s *RegionalStore) ClaimArchiveUpload(ctx context.Context, lease time.Duration) (*backup.UploadClaim, error) {
	if lease < time.Millisecond || lease > time.Hour {
		return nil, backup.ErrUploadPlan
	}
	var claim *backup.UploadClaim
	var invalid error
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		var row archiveUploadRow
		err = tx.Table("regional_archive_uploads").Where("status = ? AND next_attempt_ms <= ? AND lease_until_ms <= ?", "pending", now, now).Order("next_attempt_ms,created_at,id").Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var plan backup.UploadPlan
		if json.Unmarshal([]byte(row.Plan), &plan) != nil || plan.Validate() != nil || plan.RegionID != s.regionID || plan.ID != row.ID || plan.StorageID != row.StorageID || plan.ObjectKey != row.ObjectKey || plan.OperationID != row.OperationID || plan.RequestEventID != row.RequestEventID {
			invalid = backup.ErrUploadPlan
			return tx.Table("regional_archive_uploads").Where("id = ?", row.ID).Update("status", "invalid").Error
		}
		now, err = outboxNow(tx)
		if err != nil {
			return err
		}
		token := uuid.NewString()
		if err := tx.Table("regional_archive_uploads").Where("id = ?", row.ID).Updates(map[string]any{"lease_token": token, "lease_until_ms": now + lease.Milliseconds(), "attempts": gorm.Expr("attempts + 1")}).Error; err != nil {
			return err
		}
		claim = &backup.UploadClaim{Token: token, Plan: plan}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claim, invalid
}

// CompleteArchiveUpload records verified storage bytes, not a user-visible
// published backup or permission to restore. Publication requires separate auth.
func (s *RegionalStore) CompleteArchiveUpload(ctx context.Context, claim backup.UploadClaim, receipt backup.StoredArchive) error {
	if receipt.StorageID != claim.Plan.StorageID || receipt.ObjectKey != claim.Plan.ObjectKey || receipt.Asset != claim.Plan.Asset || len(receipt.ObjectVersion) > 1024 || strings.ContainsAny(receipt.ObjectVersion, "\x00\r\n") {
		return backup.ErrUploadPlan
	}
	data, err := json.Marshal(receipt)
	if err != nil || len(data) > 16384 {
		return backup.ErrUploadPlan
	}
	return s.finishArchiveUpload(ctx, claim, string(data), 0)
}

func (s *RegionalStore) RetryArchiveUpload(ctx context.Context, claim backup.UploadClaim, delay time.Duration) error {
	if delay < time.Millisecond || delay > 24*time.Hour {
		return backup.ErrUploadPlan
	}
	return s.finishArchiveUpload(ctx, claim, "", delay)
}

func (s *RegionalStore) finishArchiveUpload(ctx context.Context, claim backup.UploadClaim, receipt string, delay time.Duration) error {
	if claim.Token == "" || claim.Plan.Validate() != nil || claim.Plan.RegionID != s.regionID {
		return backup.ErrUploadClaimLost
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row archiveUploadRow
		err := tx.Table("regional_archive_uploads").Where("id = ? AND lease_token = ? AND status = ?", claim.Plan.ID, claim.Token, "pending").Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return backup.ErrUploadClaimLost
		}
		if err != nil {
			return err
		}
		var original backup.UploadPlan
		if json.Unmarshal([]byte(row.Plan), &original) != nil || original != claim.Plan || row.StorageID != claim.Plan.StorageID || row.ObjectKey != claim.Plan.ObjectKey || row.OperationID != claim.Plan.OperationID || row.RequestEventID != claim.Plan.RequestEventID {
			return backup.ErrUploadClaimLost
		}
		if receipt != "" {
			requestRow, err := lockArchiveRequest(tx, original)
			if err != nil {
				return err
			}
			if requestRow.Status != "preparing" {
				return backup.ErrUploadPlan
			}
		}
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		if row.LeaseUntilMS <= now {
			return backup.ErrUploadClaimLost
		}
		values := map[string]any{"lease_token": "", "lease_until_ms": 0, "next_attempt_ms": now + delay.Milliseconds()}
		if receipt != "" {
			if err := tx.Table("regional_backup_requests").Where("operation_id = ?", original.OperationID).Update("status", "uploaded").Error; err != nil {
				return err
			}
			var archive backup.StoredArchive
			if err := json.Unmarshal([]byte(receipt), &archive); err != nil {
				return err
			}
			event := backup.ArchiveUploaded{SchemaVersion: 1, EventID: uuid.NewString(), Plan: original, Receipt: archive}
			payload, err := json.Marshal(event)
			if err != nil {
				return err
			}
			result := map[string]any{"id": event.EventID, "upload_id": row.ID, "operation_id": original.OperationID, "event_type": "backup.archive.uploaded", "payload": string(payload)}
			if err := tx.Table("regional_backup_result_outbox").Create(result).Error; err != nil {
				return err
			}
			values["status"] = "uploaded"
			values["receipt"] = receipt
		}
		return tx.Table("regional_archive_uploads").Where("id = ?", row.ID).Updates(values).Error
	})
}
