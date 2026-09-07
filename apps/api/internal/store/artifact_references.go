package store

import (
	"context"
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/internal/workload"
	"gorm.io/gorm"
)

// Caller holds the workspace writer lock, shared with library updates/deletes.
// A manifest pins metadata revision as well as bytes: dependency names and the
// provider's enabled-file content were derived from that exact source snapshot.
func (s *Store) validatePublishedArtifacts(ctx context.Context, target domain.GameServer, assignment domain.WorkloadAssignment) error {
	if err := workload.ValidateArtifacts(assignment.Spec.Options); err != nil {
		return ErrInvalidModLibrary
	}
	for _, ref := range assignment.Spec.Options.Artifacts {
		var item domain.ModFile
		err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND provider_key = ? AND instance_id = ? AND source = ?", ref.ID, target.OrganizationID, target.ProviderKey, "unassigned", "upload").Take(&item).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInvalidModLibrary
		}
		if err != nil {
			return err
		}
		if item.Revision != ref.Revision || item.ContentHash != ref.SHA256 || item.SizeBytes != ref.SizeBytes {
			return ErrReconciliationSuperseded
		}
	}
	return nil
}

// Keep every persisted assignment's references until it is replaced or removed,
// including an older generation that may still be executing on a disconnected
// node. Desired-state changes alone do not prove that files are no longer used.
// Caller holds the same workspace lock as artifact publication.
func (s *Store) artifactReferences(ctx context.Context, orgID, modID string) error {
	var assignments []domain.WorkloadAssignment
	err := s.db.WithContext(ctx).Model(&domain.WorkloadAssignment{}).
		Joins("JOIN game_servers ON game_servers.id = workload_assignments.server_id").
		Where("game_servers.organization_id = ?", orgID).
		Select("workload_assignments.spec").Find(&assignments).Error
	if err != nil {
		return err
	}
	for _, assignment := range assignments {
		for _, ref := range assignment.Spec.Options.Artifacts {
			if ref.ID == modID {
				return ErrInvalidModLibrary
			}
		}
	}
	return nil
}
