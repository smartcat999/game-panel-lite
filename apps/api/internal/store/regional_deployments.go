package store

import (
	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// stageRegionalDeployment runs inside asset completion, with its task locked.
// It stores desired state only: even a new placement epoch needs independent
// execution authorization before any allocation or runtime mutation.
func stageRegionalDeployment(tx *gorm.DB, snapshot regional.RevisionSnapshot) error {
	event := snapshot.Event
	// An already superseded revision may finish downloading for historical work,
	// but must not become a new deployment's desired configuration.
	if snapshot.CurrentSpecGeneration != event.SpecGeneration {
		return nil
	}
	row := regional.Deployment{ID: uuid.NewString(), OrganizationID: event.OrganizationID, ServerID: event.ServerID, PlacementEpoch: event.PlacementEpoch, RevisionID: event.RevisionID, RevisionOperationID: event.OperationID, SpecGeneration: event.SpecGeneration, IntentVersion: snapshot.IntentVersion, DesiredState: snapshot.DesiredState, Status: "awaiting_authority"}
	if err := tx.Table("regional_deployments").Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "server_id"}, {Name: "placement_epoch"}}, DoNothing: true}).Create(&row).Error; err != nil {
		return err
	}
	var current regional.Deployment
	if err := tx.Table("regional_deployments").Where("server_id = ? AND placement_epoch = ?", event.ServerID, event.PlacementEpoch).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&current).Error; err != nil {
		return err
	}
	if current.OrganizationID != event.OrganizationID || (current.SpecGeneration == event.SpecGeneration && current.RevisionID != event.RevisionID) || (current.IntentVersion == snapshot.IntentVersion && current.DesiredState != snapshot.DesiredState) {
		return regional.ErrDeploymentConflict
	}
	values := map[string]any{}
	if current.SpecGeneration < event.SpecGeneration {
		values["revision_id"] = event.RevisionID
		values["revision_operation_id"] = event.OperationID
		values["spec_generation"] = event.SpecGeneration
	}
	if current.IntentVersion < snapshot.IntentVersion {
		values["intent_version"] = snapshot.IntentVersion
		values["desired_state"] = snapshot.DesiredState
	}
	if len(values) == 0 {
		return nil
	}
	values["scheduling_status"] = "pending"
	values["scheduling_token"] = ""
	values["scheduling_until_ms"] = 0
	values["scheduling_next_ms"] = 0
	if err := tx.Table("regional_node_tasks").Where("deployment_id = ? AND status = ?", current.ID, "awaiting_authority").Update("status", "superseded").Error; err != nil {
		return err
	}
	return tx.Table("regional_deployments").Where("id = ?", current.ID).Updates(values).Error
}
