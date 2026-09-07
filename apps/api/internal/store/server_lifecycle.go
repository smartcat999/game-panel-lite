package store

import (
	"context"
	"encoding/json"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

// SaveServerLifecycle writes only lifecycle-owned fields. The authorization,
// configuration and runtime state used to decide a transition must still hold
// when it commits. In particular, deleting a stopped snapshot cannot delete an
// instance that started after the snapshot was read.
func (s *Store) SaveServerLifecycle(ctx context.Context, userID string, before, after domain.GameServer) error {
	return s.Transaction(ctx, func(tx *Store) error {
		if before.OrganizationID != "" {
			if err := tx.lockWorkspaceWriter(ctx, before.OrganizationID, userID); err != nil {
				return err
			}
		} else if userID != "" {
			return ErrWorkspaceWriteDenied
		}
		spec, err := json.Marshal(before.Spec)
		if err != nil {
			return err
		}
		status, err := json.Marshal(before.Status)
		if err != nil {
			return err
		}
		update := domain.GameServer{Spec: before.Spec, Status: before.Status, UpdatedAt: after.UpdatedAt}
		update.Spec.DesiredState = after.Spec.DesiredState
		update.Spec.Generation = after.Spec.Generation
		update.Spec.Runtime.ModSyncMode = after.Spec.Runtime.ModSyncMode
		update.Status.Phase = after.Status.Phase
		update.Status.LastTransitionAt = after.Status.LastTransitionAt
		result := tx.db.WithContext(ctx).Model(&domain.GameServer{}).
			Where("id = ? AND spec = ? AND status = ? AND COALESCE(node_id, '') = ? AND COALESCE(organization_id, '') = ?", before.ID, string(spec), string(status), before.NodeID, before.OrganizationID).
			Select("spec", "status", "updated_at").Updates(&update)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrReconciliationSuperseded
		}
		return nil
	})
}
