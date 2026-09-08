package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/internal/workload"
	"gorm.io/gorm"
)

// PublishWorkloadAssignment serializes publication against instance writes and
// rejects work built from a superseded configuration or placement.
func (s *Store) PublishWorkloadAssignment(ctx context.Context, before domain.GameServer, assignment *domain.WorkloadAssignment) error {
	if assignment.ServerID != before.ID || assignment.NodeID != before.NodeID || assignment.Generation != before.Spec.Generation || assignment.DesiredState != before.Spec.DesiredState || assignment.UID == "" {
		return ErrReconciliationSuperseded
	}
	if assignment.DesiredState == domain.DesiredRunning {
		if _, err := workload.ResolvePortBindings(assignment.Spec.Network); err != nil {
			return err
		}
	}
	spec, err := json.Marshal(before.Spec)
	if err != nil {
		return err
	}
	return s.Transaction(ctx, func(tx *Store) error {
		if len(assignment.Spec.Options.Artifacts) > 0 {
			if before.OrganizationID == "" {
				return ErrInvalidModLibrary
			}
			if err := tx.lockWorkspace(ctx, before.OrganizationID); err != nil {
				return err
			}
			if err := tx.validatePublishedArtifacts(ctx, before, *assignment); err != nil {
				return err
			}
		}
		locked := tx.db.WithContext(ctx).Model(&domain.GameServer{}).
			Where("id = ? AND spec = ? AND COALESCE(node_id, '') = ? AND COALESCE(organization_id, '') = ?", before.ID, string(spec), before.NodeID, before.OrganizationID).
			UpdateColumn("updated_at", gorm.Expr("updated_at"))
		if locked.Error != nil {
			return locked.Error
		}
		if locked.RowsAffected == 0 {
			return ErrReconciliationSuperseded
		}
		current, err := tx.GetWorkloadAssignmentByServer(ctx, before.ID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if err == nil {
			if current.Generation > assignment.Generation {
				return ErrReconciliationSuperseded
			}
			if current.NodeID == assignment.NodeID && current.UID != assignment.UID {
				return ErrReconciliationSuperseded
			}
			if current.Generation == assignment.Generation {
				currentSpec, err := json.Marshal(current.Spec)
				if err != nil {
					return err
				}
				nextSpec, err := json.Marshal(assignment.Spec)
				if err != nil {
					return err
				}
				if current.UID != assignment.UID || current.NodeID != assignment.NodeID || current.DesiredState != assignment.DesiredState || string(currentSpec) != string(nextSpec) {
					return ErrReconciliationSuperseded
				}
				*assignment = current
				return nil
			}
			if current.NodeID != assignment.NodeID && current.UID == assignment.UID {
				return ErrReconciliationSuperseded
			}
		}
		if err := tx.upsertWorkloadAssignment(ctx, assignment); err != nil {
			return err
		}
		return tx.replaceArtifactReferences(ctx, before.OrganizationID, *assignment)
	})
}
