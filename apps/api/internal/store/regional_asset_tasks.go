package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ regional.AssetTasks = (*RegionalStore)(nil)

type regionalAssetTaskRow struct {
	OperationID, Payload, Snapshot string
	AssetLeaseUntilMS              int64
}

func (s *RegionalStore) ClaimAssets(ctx context.Context, lease time.Duration) (*regional.AssetClaim, error) {
	if lease < time.Millisecond || lease > time.Hour {
		return nil, errors.New("invalid asset lease")
	}
	var result *regional.AssetClaim
	var invalid error
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		var row regionalAssetTaskRow
		err = tx.Table("regional_revision_tasks").Select("operation_id,payload,CASE WHEN octet_length(snapshot) <= ? THEN snapshot ELSE '' END AS snapshot", 4<<20).Where("status = ? AND asset_next_attempt_ms <= ? AND asset_lease_until_ms <= ?", "revision_fetched", now, now).Order("asset_next_attempt_ms,created_at,operation_id").Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		event, err := regional.DecodeRevisionNotification([]byte(row.Payload))
		var snapshot regional.RevisionSnapshot
		valid := err == nil && event.RegionID == s.regionID && event.OperationID == row.OperationID && json.Unmarshal([]byte(row.Snapshot), &snapshot) == nil && snapshot.ValidateFor(event) == nil
		now, err = outboxNow(tx)
		if err != nil {
			return err
		}
		if !valid {
			// Persist a cooldown so one legacy/corrupt row cannot starve the queue.
			// Operators repair it; this does not reject or authorize execution.
			invalid = regional.ErrInvalidNotification
			return tx.Table("regional_revision_tasks").Where("operation_id = ?", row.OperationID).Update("asset_next_attempt_ms", now+lease.Milliseconds()).Error
		}
		token := uuid.NewString()
		if err := tx.Table("regional_revision_tasks").Where("operation_id = ?", row.OperationID).Updates(map[string]any{"asset_lease_token": token, "asset_lease_until_ms": now + lease.Milliseconds(), "asset_attempts": gorm.Expr("asset_attempts + 1")}).Error; err != nil {
			return err
		}
		result = &regional.AssetClaim{Token: token, Snapshot: snapshot}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, invalid
}

func (s *RegionalStore) CompleteAssets(ctx context.Context, claim regional.AssetClaim) error {
	return s.finishAssets(ctx, claim, 0)
}
func (s *RegionalStore) RetryAssets(ctx context.Context, claim regional.AssetClaim, delay time.Duration) error {
	if delay < time.Millisecond || delay > 24*time.Hour {
		return errors.New("invalid asset retry delay")
	}
	return s.finishAssets(ctx, claim, delay)
}
func (s *RegionalStore) finishAssets(ctx context.Context, claim regional.AssetClaim, delay time.Duration) error {
	event := claim.Snapshot.Event
	if claim.Token == "" || event.RegionID != s.regionID || claim.Snapshot.ValidateFor(event) != nil {
		return regional.ErrAssetClaimLost
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row regionalAssetTaskRow
		err := tx.Table("regional_revision_tasks").Select("operation_id,payload,snapshot,asset_lease_until_ms").Where("operation_id = ? AND asset_lease_token = ? AND status = ?", event.OperationID, claim.Token, "revision_fetched").Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return regional.ErrAssetClaimLost
		}
		if err != nil {
			return err
		}
		storedEvent, err := regional.DecodeRevisionNotification([]byte(row.Payload))
		var stored regional.RevisionSnapshot
		if err != nil || storedEvent != event || json.Unmarshal([]byte(row.Snapshot), &stored) != nil {
			return regional.ErrAssetClaimLost
		}
		a, _ := json.Marshal(stored)
		b, _ := json.Marshal(claim.Snapshot)
		if !bytes.Equal(a, b) {
			return regional.ErrAssetClaimLost
		}
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		if row.AssetLeaseUntilMS <= now {
			return regional.ErrAssetClaimLost
		}
		values := map[string]any{"asset_lease_token": "", "asset_lease_until_ms": 0, "asset_next_attempt_ms": now + delay.Milliseconds()}
		if delay == 0 {
			values["status"] = "assets_prepared"
		}
		return tx.Table("regional_revision_tasks").Where("operation_id = ?", event.OperationID).Updates(values).Error
	})
}
