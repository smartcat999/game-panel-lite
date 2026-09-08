package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RecordBackupRequest must be called by the authenticated regional broker
// receiver. Inbox receipt and pending work commit together; neither grants
// execution authority or claims that a consistent Node snapshot exists.
func (s *RegionalStore) RecordBackupRequest(ctx context.Context, event backup.Requested) error {
	if err := event.Validate(); err != nil {
		return err
	}
	if event.RegionID != s.regionID {
		return ErrRegionMismatch
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	eventHash := fmt.Sprintf("%x", sha256.Sum256(payload))
	stable := event
	stable.EventID = ""
	encoded, err := json.Marshal(stable)
	if err != nil {
		return err
	}
	operationHash := fmt.Sprintf("%x", sha256.Sum256(encoded))
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		inbox := map[string]any{"event_id": event.EventID, "operation_id": event.OperationID, "payload_hash": eventHash}
		if err := tx.Table("regional_backup_inbox").Clauses(clause.OnConflict{DoNothing: true}).Create(inbox).Error; err != nil {
			return err
		}
		var received struct{ OperationID, PayloadHash string }
		if err := tx.Table("regional_backup_inbox").Where("event_id = ?", event.EventID).Take(&received).Error; err != nil {
			return err
		}
		if received.PayloadHash != eventHash || received.OperationID != event.OperationID {
			return ErrNotificationConflict
		}
		request := map[string]any{"operation_id": event.OperationID, "backup_id": event.BackupID, "event_id": event.EventID, "payload_hash": operationHash, "payload": string(payload)}
		if err := tx.Table("regional_backup_requests").Clauses(clause.OnConflict{DoNothing: true}).Create(request).Error; err != nil {
			return err
		}
		var stored struct{ PayloadHash string }
		if err := tx.Table("regional_backup_requests").Where("operation_id = ?", event.OperationID).Take(&stored).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotificationConflict
			}
			return err
		}
		if stored.PayloadHash != operationHash {
			return ErrNotificationConflict
		}
		return nil
	})
}
