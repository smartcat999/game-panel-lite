package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ManifestRepairBatch struct {
	Next       string   `json:"next"`
	Scanned    int      `json:"scanned"`
	Candidates []string `json:"candidates"`
	Invalid    []string `json:"invalid"`
	Applied    bool     `json:"applied"`
}

// RepairMissingAssetManifests is a trusted regional upgrade operation. It only
// requeues fetched snapshots missing their entire manifest. Runtime/materialized
// tasks are never changed. Existing snapshots are retained until authorized fetch
// succeeds. Call repeatedly by Next; use a new scan to include concurrent inserts.
func (s *RegionalStore) RepairMissingAssetManifests(ctx context.Context, after string, limit int, apply bool) (ManifestRepairBatch, error) {
	if limit < 1 || limit > 100 {
		return ManifestRepairBatch{}, errors.New("invalid manifest repair batch size")
	}
	result := ManifestRepairBatch{Next: after, Applied: apply}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []struct {
			OperationID, Payload string
			Snapshot             *string
		}
		// Oversized or corrupt historical snapshots are reported, never repaired
		// from partial bytes. Bound each row before it crosses the database wire.
		query := tx.Table("regional_revision_tasks").Select("operation_id,payload,CASE WHEN octet_length(snapshot) <= ? THEN snapshot ELSE NULL END AS snapshot", 4<<20).Where("status = ? AND operation_id > ?", "revision_fetched", after).Order("operation_id").Limit(limit)
		if apply {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			result.Scanned++
			result.Next = row.OperationID
			event, err := regional.DecodeRevisionNotification([]byte(row.Payload))
			var snapshot regional.RevisionSnapshot
			if err != nil || event.RegionID != s.regionID || event.OperationID != row.OperationID || row.Snapshot == nil || json.Unmarshal([]byte(*row.Snapshot), &snapshot) != nil {
				result.Invalid = append(result.Invalid, row.OperationID)
				continue
			}
			if !snapshot.NeedsAssetManifest(event) {
				if snapshot.ValidateFor(event) != nil {
					result.Invalid = append(result.Invalid, row.OperationID)
				}
				continue
			}
			result.Candidates = append(result.Candidates, row.OperationID)
			if apply {
				updated := tx.Table("regional_revision_tasks").Where("operation_id = ? AND status = ?", row.OperationID, "revision_fetched").Updates(map[string]any{"status": "awaiting_revision", "lease_token": "", "lease_until_ms": 0, "next_attempt_ms": 0})
				if updated.Error != nil {
					return updated.Error
				}
				if updated.RowsAffected != 1 {
					return errors.New("manifest repair task changed")
				}
			}
		}
		return nil
	})
	if err != nil {
		return ManifestRepairBatch{}, err
	}
	return result, nil
}
