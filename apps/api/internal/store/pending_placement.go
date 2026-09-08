package store

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

// AssignPendingGameServer reserves capacity and records a first placement. An
// unassigned row with an existing workload is not a fresh instance: it requires
// recovery evidence, and must not be silently placed as new work.
func (s *Store) AssignPendingGameServer(ctx context.Context, before, after domain.GameServer) error {
	if before.NodeID != "" || after.NodeID == "" || before.ID != after.ID || before.OrganizationID != after.OrganizationID || before.Spec.DesiredState != domain.DesiredRunning || before.Spec.Generation == math.MaxInt64 {
		return ErrReconciliationSuperseded
	}
	expected := before.Spec
	expected.Generation++
	want, err := json.Marshal(expected)
	if err != nil {
		return err
	}
	got, err := json.Marshal(after.Spec)
	if err != nil {
		return err
	}
	if string(want) != string(got) {
		return ErrReconciliationSuperseded
	}
	spec, err := json.Marshal(before.Spec)
	if err != nil {
		return err
	}
	return s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockNodeAllocation(ctx, after); err != nil {
			return err
		}
		update := domain.GameServer{NodeID: after.NodeID, Spec: after.Spec, Status: after.Status, UpdatedAt: time.Now().UTC()}
		result := tx.db.WithContext(ctx).Model(&domain.GameServer{}).
			Where("id = ? AND spec = ? AND COALESCE(node_id, '') = '' AND COALESCE(organization_id, '') = ?", before.ID, string(spec), before.OrganizationID).
			Select("node_id", "spec", "status", "updated_at").Updates(&update)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrReconciliationSuperseded
		}
		var existing int64
		if err := tx.db.WithContext(ctx).Model(&domain.WorkloadAssignment{}).Where("server_id = ?", before.ID).Count(&existing).Error; err != nil {
			return err
		}
		if existing != 0 {
			return ErrReconciliationSuperseded
		}
		if err := tx.db.WithContext(ctx).Model(&ExecutionLease{}).Where("server_id = ?", before.ID).Count(&existing).Error; err != nil {
			return err
		}
		if existing != 0 {
			return ErrReconciliationSuperseded
		}
		return nil
	})
}
