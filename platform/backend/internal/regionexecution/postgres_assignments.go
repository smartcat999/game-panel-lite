package regionexecution

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
)

const assignmentSelect = `SELECT id, regional_task_id, regional_deployment_id, node_id, action, payload, fencing_token, status, COALESCE(claimed_by, ''), claim_lease_until, attempts, created_at, completed_at FROM work_assignments`

func (p *Postgres) PollAssignments(ctx context.Context, nodeID contract.NodeID, limit int, now time.Time) []WorkAssignment {
	if limit < 1 {
		return nil
	}
	if limit > 100 {
		limit = 100
	}
	node, err := nodeByID(ctx, p.db, nodeID, false)
	if err != nil || node.RegionID != p.regionID || node.State != NodeReady || !node.LeaseUntil.After(now) {
		return nil
	}
	rows, err := p.db.QueryContext(ctx, assignmentSelect+` WHERE node_id = $1 AND (status = $2 OR (status = $3 AND claim_lease_until <= $4)) ORDER BY created_at, id LIMIT $5`, nodeID, AssignmentAvailable, AssignmentClaimed, now, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []WorkAssignment
	for rows.Next() {
		assignment, err := scanAssignment(rows)
		if err == nil {
			result = append(result, assignment)
		}
	}
	return result
}

func (p *Postgres) ClaimAssignment(ctx context.Context, assignmentID contract.WorkAssignmentID, nodeID contract.NodeID, leaseUntil, now time.Time) (WorkAssignment, error) {
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return WorkAssignment{}, err
	}
	defer tx.Rollback()
	assignment, err := assignmentByID(ctx, tx, assignmentID, true)
	if err != nil {
		return WorkAssignment{}, err
	}
	node, err := nodeByID(ctx, tx, nodeID, true)
	if err != nil {
		return WorkAssignment{}, err
	}
	reservation, found, err := activeReservation(ctx, tx, assignment.RegionalDeploymentID)
	if err != nil {
		return WorkAssignment{}, err
	}
	if assignment.NodeID != nodeID || node.RegionID != p.regionID || node.State != NodeReady || !node.LeaseUntil.After(now) || !found || reservation.FencingToken != assignment.FencingToken {
		return WorkAssignment{}, errors.New("work assignment claim rejected")
	}
	if assignment.Status == AssignmentClaimed && assignment.ClaimLeaseUntil.After(now) {
		return WorkAssignment{}, errors.New("work assignment already claimed")
	}
	if assignment.Status != AssignmentAvailable && assignment.Status != AssignmentClaimed {
		return WorkAssignment{}, errors.New("work assignment is terminal")
	}
	assignment.Status, assignment.ClaimedBy, assignment.ClaimLeaseUntil = AssignmentClaimed, nodeID, leaseUntil
	assignment.Attempts++
	if _, err := tx.ExecContext(ctx, `UPDATE work_assignments SET status = $1, claimed_by = $2, claim_lease_until = $3, attempts = attempts + 1 WHERE id = $4`, assignment.Status, assignment.ClaimedBy, assignment.ClaimLeaseUntil, assignment.ID); err != nil {
		return WorkAssignment{}, err
	}
	if err := tx.Commit(); err != nil {
		return WorkAssignment{}, err
	}
	return assignment, nil
}

func (p *Postgres) CompleteAssignment(ctx context.Context, assignmentID contract.WorkAssignmentID, nodeID contract.NodeID, fencingToken int64, succeeded bool, now time.Time) (bool, error) {
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	assignment, err := assignmentByID(ctx, tx, assignmentID, true)
	if err != nil {
		return false, err
	}
	if assignment.Status == AssignmentSucceeded || assignment.Status == AssignmentFailed {
		return false, nil
	}
	node, err := nodeByID(ctx, tx, nodeID, true)
	if err != nil {
		return false, err
	}
	reservation, found, err := activeReservation(ctx, tx, assignment.RegionalDeploymentID)
	if err != nil {
		return false, err
	}
	if assignment.Status != AssignmentClaimed || assignment.ClaimedBy != nodeID || !assignment.ClaimLeaseUntil.After(now) || node.State != NodeReady || !node.LeaseUntil.After(now) || !found || reservation.FencingToken != fencingToken || assignment.FencingToken != fencingToken {
		return false, errors.New("stale work assignment")
	}
	status, taskStatus := AssignmentFailed, "failed"
	if succeeded {
		status, taskStatus = AssignmentSucceeded, "succeeded"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE work_assignments SET status = $1, completed_at = $2 WHERE id = $3`, status, now, assignmentID); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE regional_tasks SET status = $1, attempts = $2 WHERE id = $3`, taskStatus, assignment.Attempts, assignment.RegionalTaskID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (p *Postgres) Assignments(ctx context.Context) []WorkAssignment {
	rows, err := p.db.QueryContext(ctx, assignmentSelect+` ORDER BY created_at, id LIMIT 100`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []WorkAssignment
	for rows.Next() {
		assignment, err := scanAssignment(rows)
		if err == nil {
			result = append(result, assignment)
		}
	}
	return result
}

func assignmentByID(ctx context.Context, query interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, assignmentID contract.WorkAssignmentID, lock bool) (WorkAssignment, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	return scanAssignment(query.QueryRowContext(ctx, assignmentSelect+` WHERE id = $1`+suffix, assignmentID))
}

func scanAssignment(row scanner) (WorkAssignment, error) {
	var assignment WorkAssignment
	var payload []byte
	var claimLease, completed sql.NullTime
	err := row.Scan(&assignment.ID, &assignment.RegionalTaskID, &assignment.RegionalDeploymentID, &assignment.NodeID, &assignment.Action, &payload, &assignment.FencingToken, &assignment.Status, &assignment.ClaimedBy, &claimLease, &assignment.Attempts, &assignment.CreatedAt, &completed)
	if err != nil {
		return WorkAssignment{}, err
	}
	if err := json.Unmarshal(payload, &assignment.Payload); err != nil {
		return WorkAssignment{}, err
	}
	if claimLease.Valid {
		assignment.ClaimLeaseUntil = claimLease.Time
	}
	if completed.Valid {
		assignment.CompletedAt = &completed.Time
	}
	return assignment, nil
}
