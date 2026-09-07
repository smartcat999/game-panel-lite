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
