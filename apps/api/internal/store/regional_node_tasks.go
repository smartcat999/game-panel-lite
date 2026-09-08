package store

import (
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// stageRegionalNodeTask runs with the Deployment locked, inside scheduling
// completion. One initial run intent per immutable allocation; no payload or
// secrets are persisted. Retries cannot retarget the task or revive superseded work.
func stageRegionalNodeTask(tx *gorm.DB, allocation regional.Allocation) error {
	task := regional.NodeTask{ID: allocation.ID, AllocationID: allocation.ID, CapacityRequest: allocation.CapacityRequest, Kind: "run", Status: "awaiting_authority"}
	if err := tx.Table("regional_node_tasks").Clauses(clause.OnConflict{DoNothing: true}).Create(&task).Error; err != nil {
		return err
	}
	var existing regional.NodeTask
	if err := tx.Table("regional_node_tasks").Where("allocation_id = ?", allocation.ID).Take(&existing).Error; err != nil {
		return err
	}
	if existing != task {
		return regional.ErrNodeTaskConflict
	}
	return nil
}
