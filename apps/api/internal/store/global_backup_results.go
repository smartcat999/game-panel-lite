package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"gorm.io/gorm"
)

// RecordBackupResult is called only by a receiver bound to an authenticated
// source Region. A result is evidence from that trusted executor, not a grant
// to execute or restore. Receipt, asset metadata and task completion are atomic.
func (s *Store) RecordBackupResult(ctx context.Context, source string, event backup.ArchiveUploaded) error {
	if err := event.Validate(); err != nil {
		return err
	}
	if source == "" || source != event.Plan.RegionID {
		return ErrRegionMismatch
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	stable := event
	stable.EventID = ""
	canonical, err := json.Marshal(stable)
	if err != nil {
		return err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(canonical))
	return s.Transaction(ctx, func(tx *Store) error {
		// The first statement reserves the SQLite writer and locks this task on PG.
		locked := tx.db.Table("global_backup_tasks").Where("operation_id = ? AND region_id = ?", event.Plan.OperationID, source).UpdateColumn("status", gorm.Expr("status"))
		if locked.Error != nil {
			return locked.Error
		}
		if locked.RowsAffected != 1 {
			return ErrNotificationConflict
		}
		var task globalBackupTaskRow
		if err := tx.db.Table("global_backup_tasks").Where("operation_id = ?", event.Plan.OperationID).Take(&task).Error; err != nil {
			return err
		}
		var request backup.Requested
		if json.Unmarshal([]byte(task.Command), &request) != nil || request.Validate() != nil || request.BackupID != task.ID || request.OperationID != task.OperationID || request.OrganizationID != task.OrganizationID || request.ServerID != task.ServerID || request.RegionID != task.RegionID || request.EventID != event.Plan.RequestEventID || request.ServerID != event.Plan.ServerID || request.PlacementEpoch != event.Plan.PlacementEpoch || request.OrganizationID != event.Plan.Asset.OrganizationID {
			return ErrNotificationConflict
		}
		var identity struct{ OperationID string }
		identityErr := tx.db.Table("global_backup_results").Where("event_id = ?", event.EventID).Take(&identity).Error
		if identityErr == nil && identity.OperationID != task.OperationID {
			return ErrNotificationConflict
		}
		if identityErr != nil && !errors.Is(identityErr, gorm.ErrRecordNotFound) {
			return identityErr
		}
		var previous struct{ PayloadHash string }
		err := tx.db.Table("global_backup_results").Where("operation_id = ?", task.OperationID).Take(&previous).Error
		if err == nil {
			if previous.PayloadHash != hash {
				return ErrNotificationConflict
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.db.Table("server_operations").Where("id = ?", task.OperationID).UpdateColumn("status", gorm.Expr("status")).Error; err != nil {
			return err
		}
		var operation globalOperationRow
		if err := tx.db.Table("server_operations").Where("id = ?", task.OperationID).Take(&operation).Error; err != nil {
			return err
		}
		if operation.Kind != "backup" || operation.OrganizationID != task.OrganizationID || operation.ServerID != task.ServerID || operation.RevisionID != request.RevisionID {
			return ErrNotificationConflict
		}
		disposition := "published"
		if task.Status == "cancelled" || task.Status == "failed" || operation.Status == "cancelled" || operation.Status == "failed" {
			disposition = "discarded"
		} else {
			if (task.Status != "requested" && task.Status != "running") || (operation.Status != "pending" && operation.Status != "running") {
				return ErrNotificationConflict
			}
			if err := tx.PublishAssetVersion(ctx, event.Receipt.Asset); err != nil {
				return err
			}
			if err := tx.db.Table("global_backup_tasks").Where("id = ?", task.ID).Update("status", "succeeded").Error; err != nil {
				return err
			}
			if err := tx.db.Table("server_operations").Where("id = ?", task.OperationID).Update("status", "succeeded").Error; err != nil {
				return err
			}
		}
		row := map[string]any{"operation_id": task.OperationID, "event_id": event.EventID, "payload_hash": hash, "payload": string(payload), "disposition": disposition, "created_at": time.Now().UTC()}
		return tx.db.Table("global_backup_results").Create(row).Error
	})
}

func migrateSQLiteGlobalBackupResults(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("gamepanel_sqlite_migrations").Where("version = 9").Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		if err := tx.Exec(globalBackupResultsSQL).Error; err != nil {
			return err
		}
		return tx.Exec("INSERT INTO gamepanel_sqlite_migrations(version) VALUES(9)").Error
	})
}
