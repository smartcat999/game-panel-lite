package store

import (
	"context"
	"encoding/json"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

// SaveModInstallationIntent shares the workspace lock with library deletion and
// membership changes. It writes only mod IDs and generation, and requires the
// exact instance and immutable library snapshot used by the application.
func (s *Store) SaveModInstallationIntent(ctx context.Context, userID string, before, after domain.GameServer, source domain.ModFile) error {
	if userID == "" || before.OrganizationID == "" || source.OrganizationID != before.OrganizationID || source.InstanceID != "unassigned" || source.ProviderKey != before.ProviderKey {
		return ErrInvalidModLibrary
	}
	return s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockWorkspaceWriter(ctx, before.OrganizationID, userID); err != nil {
			return err
		}
		var count int64
		if err := tx.libraryModSnapshot(ctx, source).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return ErrReconciliationSuperseded
		}
		update := before
		update.Spec.ModIDs = after.Spec.ModIDs
		update.Spec.Generation = after.Spec.Generation
		update.UpdatedAt = after.UpdatedAt
		if err := tx.validateServerModReferences(ctx, update); err != nil {
			return err
		}
		spec, err := json.Marshal(before.Spec)
		if err != nil {
			return err
		}
		status, err := json.Marshal(before.Status)
		if err != nil {
			return err
		}
		result := tx.db.WithContext(ctx).Model(&domain.GameServer{}).Where("id = ? AND spec = ? AND status = ? AND provider_key = ? AND COALESCE(node_id, '') = ? AND organization_id = ?", before.ID, string(spec), string(status), before.ProviderKey, before.NodeID, before.OrganizationID).Select("spec", "updated_at").Updates(&update)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrReconciliationSuperseded
		}
		return nil
	})
}
