package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

var ErrReconciliationSuperseded = errors.New("server changed during reconciliation")

// SaveReconciledGameServer cannot create an instance or overwrite user-owned
// fields. The observed result is accepted only for the intent it reconciled.
func (s *Store) SaveReconciledGameServer(ctx context.Context, before, after domain.GameServer) error {
	spec, err := json.Marshal(before.Spec)
	if err != nil {
		return err
	}
	update := domain.GameServer{Status: after.Status, UpdatedAt: time.Now().UTC()}
	columns := []string{"status", "updated_at"}
	if before.Spec.Runtime.ModSyncMode != after.Spec.Runtime.ModSyncMode {
		update.Spec = before.Spec
		update.Spec.Runtime.ModSyncMode = after.Spec.Runtime.ModSyncMode
		columns = append(columns, "spec")
	}
	result := s.db.WithContext(ctx).Model(&domain.GameServer{}).
		Where("id = ? AND spec = ? AND COALESCE(node_id, '') = ? AND COALESCE(organization_id, '') = ?", before.ID, string(spec), before.NodeID, before.OrganizationID).
		Select(columns).Updates(&update)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrReconciliationSuperseded
	}
	return nil
}

// MigrateGameServer atomically changes placement and retires the source
// assignment. It does not prove source fencing or transfer world data.
func (s *Store) MigrateGameServer(ctx context.Context, before, after domain.GameServer) error {
	if before.ID != after.ID || before.OrganizationID != after.OrganizationID || after.NodeID == "" || after.NodeID == before.NodeID || after.Spec.Generation <= before.Spec.Generation {
		return ErrReconciliationSuperseded
	}
	spec, err := json.Marshal(before.Spec)
	if err != nil {
		return err
	}
	return s.Transaction(ctx, func(tx *Store) error {
		update := domain.GameServer{NodeID: after.NodeID, Spec: after.Spec, Status: after.Status, UpdatedAt: time.Now().UTC()}
		// Acquire the instance before the assignment, matching publication and lease
		// acquisition. The intent comparison prevents a stale migration winning.
		result := tx.db.WithContext(ctx).Model(&domain.GameServer{}).
			Where("id = ? AND spec = ? AND COALESCE(node_id, '') = ? AND COALESCE(organization_id, '') = ?", before.ID, string(spec), before.NodeID, before.OrganizationID).
			Select("node_id", "spec", "status", "updated_at").Updates(&update)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrReconciliationSuperseded
		}
		return tx.DeleteWorkloadAssignment(ctx, before.ID)
	})
}
