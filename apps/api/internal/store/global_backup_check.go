package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"gorm.io/gorm"
)

// CheckRegionalBackup checks current intent in one read snapshot. A successful
// check is not a lease, a Node execution grant, or proof of a consistent snapshot.
// authenticatedRegion must come from the verified service identity.
func (s *Store) CheckRegionalBackup(ctx context.Context, authenticatedRegion string, request backup.Requested) error {
	if request.Validate() != nil || !validRegion(authenticatedRegion) || authenticatedRegion != request.RegionID {
		return backup.ErrRequestUnavailable
	}
	err := s.readSnapshot(ctx, func(tx *Store) error {
		var task globalBackupTaskRow
		if err := tx.db.Table("global_backup_tasks").Where("operation_id = ? AND region_id = ?", request.OperationID, authenticatedRegion).Take(&task).Error; err != nil {
			return err
		}
		var original backup.Requested
		if json.Unmarshal([]byte(task.Command), &original) != nil || original != request || task.ID != request.BackupID || task.OrganizationID != request.OrganizationID || task.ServerID != request.ServerID || (task.Status != "requested" && task.Status != "running") {
			return backup.ErrRequestUnavailable
		}
		var operation globalOperationRow
		if err := tx.db.Table("server_operations").Where("id = ?", request.OperationID).Take(&operation).Error; err != nil {
			return err
		}
		if operation.Kind != "backup" || operation.OrganizationID != request.OrganizationID || operation.ServerID != request.ServerID || operation.RevisionID != request.RevisionID || (operation.Status != "pending" && operation.Status != "running") {
			return backup.ErrRequestUnavailable
		}
		var server instances.Server
		if err := tx.db.Table("logical_servers").Where("id = ? AND organization_id = ?", request.ServerID, request.OrganizationID).Take(&server).Error; err != nil {
			return err
		}
		if server.DesiredState == "deleted" || server.CurrentRevisionID != request.RevisionID || server.SpecGeneration != request.SpecGeneration || server.IntentVersion != request.IntentVersion {
			return backup.ErrRequestUnavailable
		}
		var placement instances.Placement
		if err := tx.db.Table("server_placements").Where("server_id = ?", request.ServerID).Take(&placement).Error; err != nil {
			return err
		}
		if placement.RegionID != authenticatedRegion || placement.PlacementEpoch != request.PlacementEpoch {
			return backup.ErrRequestUnavailable
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return backup.ErrRequestUnavailable
	}
	return err
}
