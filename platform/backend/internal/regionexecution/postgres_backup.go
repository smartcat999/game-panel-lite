package regionexecution

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/persistence"
)

func (p *Postgres) ReceiveBackup(ctx context.Context, requested BackupRequested, now time.Time) (WorkAssignment, bool, error) {
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return WorkAssignment{}, false, err
	}
	defer tx.Rollback()
	if requested.RegionID != p.regionID {
		return WorkAssignment{}, false, ErrWrongRegion
	}
	if requested.Kind != "backup" && requested.Kind != "restore" {
		return WorkAssignment{}, false, errors.New("invalid backup task kind")
	}
	var handled bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM regional_inbox WHERE message_id = $1)`, requested.MessageID).Scan(&handled); err != nil {
		return WorkAssignment{}, false, err
	}
	if handled {
		var assignmentID contract.WorkAssignmentID
		err := tx.QueryRowContext(ctx, `SELECT id FROM work_assignments WHERE payload ->> 'backupRequestId' = $1 ORDER BY created_at LIMIT 1`, requested.BackupRequestID).Scan(&assignmentID)
		if errors.Is(err, sql.ErrNoRows) {
			return WorkAssignment{}, false, nil
		}
		if err != nil {
			return WorkAssignment{}, false, err
		}
		assignment, err := assignmentByID(ctx, tx, assignmentID, false)
		return assignment, false, err
	}
	var deploymentID contract.RegionalDeploymentID
	if err := tx.QueryRowContext(ctx, `SELECT id FROM regional_deployments WHERE logical_instance_id = $1`, requested.LogicalInstanceID).Scan(&deploymentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WorkAssignment{}, false, ErrDeploymentNotFound
		}
		return WorkAssignment{}, false, err
	}
	reservation, found, err := activeReservation(ctx, tx, deploymentID)
	if err != nil {
		return WorkAssignment{}, false, err
	}
	if !found {
		return WorkAssignment{}, false, ErrOverrideUnavailable
	}
	taskID, err := persistence.NewID("rtk")
	if err != nil {
		return WorkAssignment{}, false, err
	}
	assignmentID, err := persistence.NewID("was")
	if err != nil {
		return WorkAssignment{}, false, err
	}
	task := RegionalTask{ID: contract.RegionalTaskID(taskID), RegionalDeploymentID: deploymentID, Kind: requested.Kind, Status: "pending", CreatedAt: now}
	assignment := WorkAssignment{ID: contract.WorkAssignmentID(assignmentID), RegionalTaskID: task.ID, RegionalDeploymentID: deploymentID, NodeID: reservation.NodeID, Action: requested.Kind, Payload: map[string]string{"backupRequestId": string(requested.BackupRequestID), "objectKey": requested.ObjectKey, "transferUrl": requested.TransferURL, "relativePath": requested.RelativePath}, FencingToken: reservation.FencingToken, Status: AssignmentAvailable, CreatedAt: now}
	payload, err := json.Marshal(assignment.Payload)
	if err != nil {
		return WorkAssignment{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO regional_tasks (id, regional_deployment_id, kind, status, attempts, created_at) VALUES ($1, $2, $3, $4, 0, $5)`, task.ID, task.RegionalDeploymentID, task.Kind, task.Status, now); err != nil {
		return WorkAssignment{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO work_assignments (id, regional_task_id, regional_deployment_id, node_id, action, payload, fencing_token, status, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, assignment.ID, assignment.RegionalTaskID, assignment.RegionalDeploymentID, assignment.NodeID, assignment.Action, payload, assignment.FencingToken, assignment.Status, now); err != nil {
		return WorkAssignment{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO regional_inbox (message_id, message_type, received_at, handled_at) VALUES ($1, 'backup.requested.v1', $2, $2)`, requested.MessageID, now); err != nil {
		return WorkAssignment{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return WorkAssignment{}, false, err
	}
	return assignment, true, nil
}

func (p *Postgres) RecordBackupResult(ctx context.Context, assignment WorkAssignment, result BackupResult) (bool, error) {
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
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
	if current.Status != AssignmentClaimed || current.ClaimedBy != assignment.NodeID || !current.ClaimLeaseUntil.After(result.ObservedAt) || !found || reservation.FencingToken != assignment.FencingToken || node.State != NodeReady || !node.LeaseUntil.After(result.ObservedAt) {
		return false, errors.New("stale backup result")
	}
	requestID := contract.BackupRequestID(current.Payload["backupRequestId"])
	if requestID == "" || result.BackupRequestID != requestID {
		return false, errors.New("backup result does not match assignment")
	}
	var logicalInstanceID contract.LogicalInstanceID
	if err := tx.QueryRowContext(ctx, `SELECT logical_instance_id FROM regional_deployments WHERE id = $1`, assignment.RegionalDeploymentID).Scan(&logicalInstanceID); err != nil {
		return false, err
	}
	result.LogicalInstanceID, result.RegionID = logicalInstanceID, p.regionID
	databaseResult, err := tx.ExecContext(ctx, `INSERT INTO region_backup_results (backup_request_id, regional_task_id, logical_instance_id, sequence, status, object_key, size_bytes, checksum, observed_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT (backup_request_id) DO UPDATE SET sequence = EXCLUDED.sequence, status = EXCLUDED.status, object_key = EXCLUDED.object_key, size_bytes = EXCLUDED.size_bytes, checksum = EXCLUDED.checksum, observed_at = EXCLUDED.observed_at WHERE region_backup_results.sequence < EXCLUDED.sequence`, result.BackupRequestID, assignment.RegionalTaskID, logicalInstanceID, result.Sequence, result.Status, result.ObjectKey, result.SizeBytes, result.Checksum, result.ObservedAt)
	if err != nil {
		return false, err
	}
	changed, err := databaseResult.RowsAffected()
	if err != nil || changed == 0 {
		return false, err
	}
	eventID, err := persistence.NewID("evt")
	if err != nil {
		return false, err
	}
	payload, err := json.Marshal(map[string]any{"backupRequestId": result.BackupRequestID, "logicalInstanceId": logicalInstanceID, "regionId": p.regionID, "sequence": result.Sequence, "status": result.Status, "observedAt": result.ObservedAt, "objectKey": result.ObjectKey, "sizeBytes": result.SizeBytes})
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO regional_outbox (id, message_type, schema_version, idempotency_key, payload, created_at) VALUES ($1, 'backup.observed.v1', 1, $2, $3, $4) ON CONFLICT (message_type, idempotency_key) DO NOTHING`, eventID, result.BackupRequestID, payload, result.ObservedAt); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
