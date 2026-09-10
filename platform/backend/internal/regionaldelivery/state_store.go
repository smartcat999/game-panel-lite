package regionaldelivery

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/persistence"
)

const stateSelect = `SELECT id,workspace_id,logical_instance_id,region_id,placement_version,instance_revision_id,operation_id,desired_state,provider_release_id,game_version,apply_behavior,cpu_milli,memory_mib,disk_gib,configuration,mod_lock,listener_requirements,phase,COALESCE(node_id,''),fencing_token,COALESCE(reconcile_owner,''),reconcile_lease_until,COALESCE(assignment_owner,''),assignment_lease_until,assignment_attempts,observation_sequence,telemetry_sequence,COALESCE(failure_code,''),residual_cleanup_required,updated_at FROM regional_delivery_states`

func stateByID(ctx context.Context, query persistence.DBTX, id string, lock bool) (State, error) {
	suffix := ` WHERE id=$1`
	if lock {
		suffix += ` FOR UPDATE`
	}
	return scanState(query.QueryRowContext(ctx, stateSelect+suffix, id))
}

func stateByInstance(ctx context.Context, query persistence.DBTX, id string, lock bool) (State, error) {
	suffix := ` WHERE logical_instance_id=$1`
	if lock {
		suffix += ` FOR UPDATE`
	}
	return scanState(query.QueryRowContext(ctx, stateSelect+suffix, id))
}

func scanState(row rowScanner) (State, error) {
	var state State
	var configuration, modLock, listeners []byte
	var reconcileLease, assignmentLease sql.NullTime
	err := row.Scan(&state.ID, &state.WorkspaceID, &state.LogicalInstanceID, &state.RegionID, &state.PlacementVersion, &state.InstanceRevisionID, &state.OperationID, &state.DesiredState, &state.ProviderReleaseID, &state.GameVersion, &state.ApplyBehavior, &state.ResourceSpec.CPUMilli, &state.ResourceSpec.MemoryMiB, &state.ResourceSpec.DiskGiB, &configuration, &modLock, &listeners, &state.Phase, &state.NodeID, &state.FencingToken, &state.ReconcileOwner, &reconcileLease, &state.AssignmentOwner, &assignmentLease, &state.AssignmentAttempts, &state.ObservationSequence, &state.TelemetrySequence, &state.FailureCode, &state.ResidualCleanupRequired, &state.UpdatedAt)
	if err != nil {
		return State{}, err
	}
	state.ReconcileLeaseUntil = reconcileLease.Time
	state.AssignmentLeaseUntil = assignmentLease.Time
	if err = json.Unmarshal(configuration, &state.Configuration); err != nil {
		return State{}, err
	}
	if err = json.Unmarshal(modLock, &state.ModLock); err != nil {
		return State{}, err
	}
	err = json.Unmarshal(listeners, &state.ListenerRequirements)
	return state, err
}

type rowScanner interface{ Scan(...any) error }
