package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ regional.SchedulingTasks = (*RegionalStore)(nil)

type regionalSchedulingRow struct {
	regional.Deployment
	SchedulingStatus, SchedulingToken string
	SchedulingUntilMS                 int64
}

func (s *RegionalStore) ClaimScheduling(ctx context.Context, lease time.Duration) (*regional.SchedulingClaim, error) {
	if lease < time.Millisecond || lease > time.Hour {
		return nil, regional.ErrSchedulingClaimLost
	}
	var claim *regional.SchedulingClaim
	var invalid error
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		var row regionalSchedulingRow
		err = tx.Table("regional_deployments").Where("scheduling_status = ? AND desired_state = ? AND scheduling_next_ms <= ? AND scheduling_until_ms <= ?", "pending", "running", now, now).Order("scheduling_next_ms,created_at,id").Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		// Plain MVCC read while holding Deployment: do not invert asset completion's
		// revision-task -> Deployment lock order by locking the revision task here.
		var task struct{ Status, Snapshot string }
		err = tx.Table("regional_revision_tasks").Select("status,CASE WHEN octet_length(snapshot) <= ? THEN snapshot ELSE '' END AS snapshot", 4<<20).Where("operation_id = ?", row.RevisionOperationID).Take(&task).Error
		if err != nil {
			return err
		}
		var snapshot regional.RevisionSnapshot
		if json.Unmarshal([]byte(task.Snapshot), &snapshot) != nil || snapshot.ValidateFor(snapshot.Event) != nil || task.Status != "assets_prepared" ||
			snapshot.CurrentSpecGeneration != row.SpecGeneration || row.IntentVersion < snapshot.IntentVersion || (row.IntentVersion == snapshot.IntentVersion && row.DesiredState != snapshot.DesiredState) ||
			snapshot.Event.RegionID != s.regionID || snapshot.Event.OrganizationID != row.OrganizationID || snapshot.Event.ServerID != row.ServerID ||
			snapshot.Event.OperationID != row.RevisionOperationID || snapshot.Event.RevisionID != row.RevisionID || snapshot.Event.SpecGeneration != row.SpecGeneration || snapshot.Event.PlacementEpoch != row.PlacementEpoch || row.Status != "awaiting_authority" {
			invalid = regional.ErrDeploymentConflict
			return tx.Table("regional_deployments").Where("id = ?", row.ID).Update("scheduling_status", "rejected").Error
		}
		// Intent is mutable regional desired state, distinct from immutable revision.
		snapshot.IntentVersion, snapshot.DesiredState = row.IntentVersion, row.DesiredState
		snapshot.CurrentSpecGeneration = row.SpecGeneration
		now, err = outboxNow(tx)
		if err != nil {
			return err
		}
		token := uuid.NewString()
		if err := tx.Table("regional_deployments").Where("id = ?", row.ID).Updates(map[string]any{"scheduling_token": token, "scheduling_until_ms": now + lease.Milliseconds(), "scheduling_attempts": gorm.Expr("scheduling_attempts + 1")}).Error; err != nil {
			return err
		}
		claim = &regional.SchedulingClaim{Token: token, Deployment: row.Deployment, Snapshot: snapshot}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claim, invalid
}

func lockSchedulingClaim(tx *gorm.DB, claim regional.SchedulingClaim) (regionalSchedulingRow, error) {
	var row regionalSchedulingRow
	err := tx.Table("regional_deployments").Where("id = ?", claim.Deployment.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, regional.ErrSchedulingClaimLost
	}
	if err != nil {
		return row, err
	}
	if claim.Token == "" || row.SchedulingToken != claim.Token || row.SchedulingStatus != "pending" || row.Deployment != claim.Deployment {
		return row, regional.ErrSchedulingClaimLost
	}
	now, err := outboxNow(tx)
	if err != nil {
		return row, err
	}
	if row.SchedulingUntilMS <= now {
		return row, regional.ErrSchedulingClaimLost
	}
	return row, nil
}

func (s *RegionalStore) RetryScheduling(ctx context.Context, claim regional.SchedulingClaim, delay time.Duration) error {
	if delay < time.Millisecond || delay > 24*time.Hour {
		return regional.ErrSchedulingClaimLost
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := lockSchedulingClaim(tx, claim)
		if err != nil {
			return err
		}
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		return tx.Table("regional_deployments").Where("id = ?", row.ID).Updates(map[string]any{"scheduling_token": "", "scheduling_until_ms": 0, "scheduling_next_ms": now + delay.Milliseconds()}).Error
	})
}

func (s *RegionalStore) CompleteScheduling(ctx context.Context, claim regional.SchedulingClaim, allocation regional.Allocation) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := lockSchedulingClaim(tx, claim)
		if err != nil {
			return err
		}
		if allocation.RegionID != s.regionID || allocation.DeploymentID != row.ID || allocation.OrganizationID != row.OrganizationID || allocation.ServerID != row.ServerID || allocation.PlacementEpoch != row.PlacementEpoch || allocation.RevisionID != row.RevisionID || allocation.SpecGeneration != row.SpecGeneration || allocation.IntentVersion != row.IntentVersion || allocation.Status != "reserved" {
			return regional.ErrAllocationConflict
		}
		var stored regionalAllocationRow
		err = tx.Table("regional_allocations").Where("id = ? AND deployment_id = ? AND status = ?", allocation.ID, row.ID, "reserved").Take(&stored).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return regional.ErrAllocationConflict
		}
		if err != nil {
			return err
		}
		receipt, err := stored.allocation()
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(receipt, allocation) {
			return regional.ErrAllocationConflict
		}
		if err := checkRegionalPortReceipt(tx, receipt, allocation.Ports); err != nil {
			return err
		}
		// Recheck database time after all reads before completing the claim.
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		if row.SchedulingUntilMS <= now {
			return regional.ErrSchedulingClaimLost
		}
		return tx.Table("regional_deployments").Where("id = ?", row.ID).Updates(map[string]any{"scheduling_status": "reserved", "scheduling_token": "", "scheduling_until_ms": 0}).Error
	})
}
