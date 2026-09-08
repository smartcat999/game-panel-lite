package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrNotificationConflict = regional.ErrNotificationConflict

// RecordRevisionNotification saves a notification and a task to fetch its
// authorized revision. It does NOT accept a deployment or grant execution.
// The caller must be the authenticated regional broker ingress, not a tenant API.
func (s *RegionalStore) RecordRevisionNotification(ctx context.Context, event instances.RevisionAvailable) error {
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
	// Operation identity is stable even if a repaired publisher assigns a new
	// envelope ID. Different contents under either identity must fail closed.
	operationEvent := event
	operationEvent.EventID = ""
	operationPayload, err := json.Marshal(operationEvent)
	if err != nil {
		return err
	}
	operationHash := fmt.Sprintf("%x", sha256.Sum256(operationPayload))
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		inbox := struct{ EventID, OperationID, PayloadHash string }{event.EventID, event.OperationID, eventHash}
		if err := tx.Table("regional_inbox").Clauses(clause.OnConflict{DoNothing: true}).Create(&inbox).Error; err != nil {
			return err
		}
		var received struct{ PayloadHash string }
		if err := tx.Table("regional_inbox").Where("event_id = ?", event.EventID).Take(&received).Error; err != nil {
			return err
		}
		if received.PayloadHash != eventHash {
			return ErrNotificationConflict
		}
		task := struct{ OperationID, EventID, PayloadHash, Payload string }{event.OperationID, event.EventID, operationHash, string(payload)}
		if err := tx.Table("regional_revision_tasks").Clauses(clause.OnConflict{DoNothing: true}).Create(&task).Error; err != nil {
			return err
		}
		var existing struct{ PayloadHash string }
		if err := tx.Table("regional_revision_tasks").Where("operation_id = ?", event.OperationID).Take(&existing).Error; err != nil {
			return err
		}
		if existing.PayloadHash != operationHash {
			return ErrNotificationConflict
		}
		return nil
	})
}
