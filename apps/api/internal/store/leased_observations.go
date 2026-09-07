package store

import (
	"context"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
)

// SaveAgentWorkloadObservation checks authority and writes the report in one
// transaction. Reporting does not renew the lease or weaken observation-token CAS.
func (s *Store) SaveAgentWorkloadObservation(ctx context.Context, request ExecutionLeaseRequest, fence int64, observation *domain.WorkloadObservation) error {
	if fence <= 0 || request.HolderID == "" || len(request.HolderID) > 128 || request.NodeToken == "" || request.NodeID == "" || request.AssignmentUID == "" || request.Generation <= 0 || observation.AssignmentUID != request.AssignmentUID || observation.NodeID != request.NodeID || observation.ObservedGeneration != request.Generation {
		return ErrExecutionLeaseUnavailable
	}
	return s.Transaction(ctx, func(tx *Store) error {
		assignment, now, err := tx.lockExecutionAssignment(ctx, request)
		if err != nil {
			return err
		}
		if observation.ServerID != assignment.ServerID {
			return ErrExecutionLeaseUnavailable
		}
		locked := tx.db.WithContext(ctx).Model(&ExecutionLease{}).Where("server_id = ? AND assignment_uid = ? AND node_id = ? AND generation = ? AND holder_id = ? AND fence = ? AND expires_at_ms > ?", assignment.ServerID, request.AssignmentUID, request.NodeID, request.Generation, request.HolderID, fence, now).UpdateColumn("fence", gorm.Expr("fence"))
		if locked.Error != nil {
			return locked.Error
		}
		if locked.RowsAffected != 1 {
			return ErrExecutionLeaseUnavailable
		}
		return tx.UpsertWorkloadObservation(ctx, observation)
	})
}
