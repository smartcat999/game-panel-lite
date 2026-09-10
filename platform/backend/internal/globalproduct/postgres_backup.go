package globalproduct

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/backupcontrol"
	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/persistence"
)

func (p *Postgres) RequestBackup(ctx context.Context, command backupcontrol.CreateCommand, now time.Time) (backupcontrol.Request, error) {
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return backupcontrol.Request{}, err
	}
	defer tx.Rollback()
	if command.Identity.IdempotencyKey == "" || command.WorkspaceID == "" || command.LogicalInstanceID == "" || command.RegionID == "" || (command.Kind != backupcontrol.KindBackup && command.Kind != backupcontrol.KindRestore) {
		return backupcontrol.Request{}, backupcontrol.ErrInvalidRequest
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, command.Identity.IdempotencyKey); err != nil {
		return backupcontrol.Request{}, err
	}
	var previousID contract.BackupRequestID
	err = tx.QueryRowContext(ctx, `SELECT backup_request_id FROM backup_command_results WHERE idempotency_key = $1`, command.Identity.IdempotencyKey).Scan(&previousID)
	if err == nil {
		return backupRequestByID(ctx, tx, previousID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return backupcontrol.Request{}, err
	}
	detail, err := p.instances.Detail(ctx, tx, &command.WorkspaceID, command.LogicalInstanceID)
	if err != nil || detail.Placement.RegionID != command.RegionID {
		return backupcontrol.Request{}, backupcontrol.ErrInvalidRequest
	}
	objectKey := "regions/" + string(command.RegionID) + "/instances/" + string(command.LogicalInstanceID) + "/backups/" + string(command.Identity.IdempotencyKey) + ".tar.gz"
	if command.Kind == backupcontrol.KindRestore {
		source, err := backupRequestByID(ctx, tx, command.SourceBackupRequestID)
		if err != nil || source.Kind != backupcontrol.KindBackup || source.Status != backupcontrol.StatusCompleted || source.WorkspaceID != command.WorkspaceID || source.LogicalInstanceID != command.LogicalInstanceID || source.RegionID != command.RegionID {
			return backupcontrol.Request{}, backupcontrol.ErrCrossRegionRestore
		}
		objectKey = source.ObjectKey
	}
	id, err := persistence.NewID("bkr")
	if err != nil {
		return backupcontrol.Request{}, err
	}
	request := backupcontrol.Request{ID: contract.BackupRequestID(id), WorkspaceID: command.WorkspaceID, LogicalInstanceID: command.LogicalInstanceID, RegionID: command.RegionID, Kind: command.Kind, Status: backupcontrol.StatusQueued, ObjectKey: objectKey, CreatedAt: now, UpdatedAt: now}
	if _, err := tx.ExecContext(ctx, `INSERT INTO backup_requests (id, workspace_id, logical_instance_id, region_id, kind, status, sequence, object_key, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, 0, $7, $8, $8)`, request.ID, request.WorkspaceID, request.LogicalInstanceID, request.RegionID, request.Kind, request.Status, request.ObjectKey, now); err != nil {
		return backupcontrol.Request{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO backup_command_results (idempotency_key, backup_request_id, created_at) VALUES ($1, $2, $3)`, command.Identity.IdempotencyKey, request.ID, now); err != nil {
		return backupcontrol.Request{}, err
	}
	eventID, err := persistence.NewID("evt")
	if err != nil {
		return backupcontrol.Request{}, err
	}
	payload := map[string]any{"backupRequestId": request.ID, "logicalInstanceId": request.LogicalInstanceID, "regionId": request.RegionID, "kind": request.Kind, "objectKey": request.ObjectKey, "transferUrl": "object://" + request.ObjectKey, "relativePath": "instances/" + string(request.LogicalInstanceID) + "/world"}
	if err := p.messages.InsertOutbox(ctx, tx, messaging.OutboxMessage{SchemaVersion: 1, ID: contract.EventID(eventID), MessageType: "backup.requested.v1", IdempotencyKey: command.Identity.IdempotencyKey, Payload: payload, CreatedAt: now}); err != nil {
		return backupcontrol.Request{}, err
	}
	if err := tx.Commit(); err != nil {
		return backupcontrol.Request{}, err
	}
	return request, nil
}

func (p *Postgres) Backups(ctx context.Context, workspaceID contract.WorkspaceID) ([]backupcontrol.Request, error) {
	rows, err := p.db.QueryContext(ctx, backupRequestSelect+` WHERE workspace_id = $1 ORDER BY created_at DESC, id LIMIT 100`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []backupcontrol.Request
	for rows.Next() {
		request, err := scanBackupRequest(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, request)
	}
	return result, rows.Err()
}

func (p *Postgres) ApplyBackupObservation(ctx context.Context, observation backupcontrol.Observation) (bool, error) {
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO global_inbox (message_id, message_type, received_at, handled_at) VALUES ($1, 'backup.observed.v1', $2, $2) ON CONFLICT (message_id) DO NOTHING`, observation.MessageID, observation.ObservedAt)
	if err != nil {
		return false, err
	}
	inserted, err := result.RowsAffected()
	if err != nil || inserted == 0 {
		return false, err
	}
	result, err = tx.ExecContext(ctx, `UPDATE backup_requests SET sequence = $1, status = $2, object_key = COALESCE(NULLIF($3, ''), object_key), size_bytes = $4, checksum = $5, updated_at = $6 WHERE id = $7 AND sequence < $1`, observation.Sequence, observation.Status, observation.ObjectKey, observation.SizeBytes, observation.Checksum, observation.ObservedAt, observation.BackupRequestID)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return changed > 0, tx.Commit()
}

const backupRequestSelect = `SELECT id, workspace_id, logical_instance_id, region_id, kind, status, sequence, COALESCE(object_key, ''), COALESCE(size_bytes, 0), COALESCE(checksum, ''), created_at, updated_at FROM backup_requests`

func backupRequestByID(ctx context.Context, query interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id contract.BackupRequestID) (backupcontrol.Request, error) {
	return scanBackupRequest(query.QueryRowContext(ctx, backupRequestSelect+` WHERE id = $1`, id))
}

func scanBackupRequest(row interface{ Scan(...any) error }) (backupcontrol.Request, error) {
	var request backupcontrol.Request
	err := row.Scan(&request.ID, &request.WorkspaceID, &request.LogicalInstanceID, &request.RegionID, &request.Kind, &request.Status, &request.Sequence, &request.ObjectKey, &request.SizeBytes, &request.Checksum, &request.CreatedAt, &request.UpdatedAt)
	return request, err
}
