package regionexecution

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

func (p *Postgres) RecordWorkloadResult(ctx context.Context, assignment WorkAssignment, state, _ string, now time.Time) (bool, error) {
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	messageID := workloadEventID(assignment.ID)
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM regional_observations WHERE message_id = $1)`, messageID).Scan(&exists); err != nil || exists {
		return false, err
	}
	current, err := assignmentByID(ctx, tx, assignment.ID, true)
	if err != nil {
		return false, err
	}
	node, err := nodeByID(ctx, tx, assignment.NodeID, true)
	if err != nil {
		return false, err
	}
	reservation, found, err := activeReservation(ctx, tx, assignment.RegionalDeploymentID)
	if err != nil {
		return false, err
	}
	if current.Status != AssignmentClaimed || current.ClaimedBy != assignment.NodeID || !current.ClaimLeaseUntil.After(now) || !found || reservation.FencingToken != assignment.FencingToken || node.State != NodeReady || !node.LeaseUntil.After(now) {
		return false, errors.New("stale workload result")
	}
	observedState := ObservedState(state)
	if observedState != ObservedRunning && observedState != ObservedStopped && observedState != ObservedFailed {
		return false, errors.New("invalid workload observation")
	}
	deployment, err := deploymentByID(ctx, tx, assignment.RegionalDeploymentID, true)
	if err != nil {
		return false, err
	}
	sequence := deployment.ObservationSequence + 1
	if _, err := tx.ExecContext(ctx, `INSERT INTO regional_observations (message_id, regional_deployment_id, sequence, observed_state, observed_at) VALUES ($1, $2, $3, $4, $5)`, messageID, deployment.ID, sequence, observedState, now); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE regional_deployments SET observed_state = $1, observation_sequence = $2, updated_at = $3 WHERE id = $4`, observedState, sequence, now, deployment.ID); err != nil {
		return false, err
	}
	payload, err := json.Marshal(map[string]any{"logicalInstanceId": deployment.LogicalInstanceID, "regionalDeploymentId": deployment.ID, "regionId": p.regionID, "sequence": sequence, "observedState": observedState, "observedAt": now})
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO regional_outbox (id, message_type, schema_version, idempotency_key, payload, created_at) VALUES ($1, 'deployment.observed.v1', 1, $2, $3, $4) ON CONFLICT (message_type, idempotency_key) DO NOTHING`, messageID, assignment.ID, payload, now); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
