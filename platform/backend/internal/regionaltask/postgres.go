package regionaltask

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instanceaction"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/persistence"
)

var (
	ErrInvalidTask = errors.New("invalid regional instance task")
	ErrTaskLease   = errors.New("regional task lease is not owned")
)

type Task struct {
	ID                string
	MessageID         string
	WorkspaceID       string
	LogicalInstanceID string
	RegionID          string
	OperationID       string
	Kind              string
	Payload           json.RawMessage
	Status            string
	ClaimOwner        string
	ClaimUntil        time.Time
	AttemptCount      int
	FencingToken      int64
}

type BackupResult struct {
	Status      string
	ObjectKey   string
	SizeBytes   int64
	Checksums   map[string]string
	FailureCode string
}

type Postgres struct {
	database     *sql.DB
	regionID     string
	authorityKey []byte
	lease        time.Duration
}

func NewPostgres(database *sql.DB, regionID string, authorityKey []byte) *Postgres {
	return &Postgres{database: database, regionID: regionID, authorityKey: append([]byte(nil), authorityKey...), lease: 30 * time.Second}
}

func (p *Postgres) ReceiveConsole(ctx context.Context, messageID string, payload instanceaction.ConsolePayload, now time.Time) (bool, error) {
	if payload.RegionID != p.regionID || !instanceaction.VerifyConsole(payload, p.authorityKey, now) {
		return false, ErrInvalidTask
	}
	return p.receive(ctx, messageID, payload.WorkspaceID, payload.LogicalInstanceID, payload.OperationID, "console", payload, now)
}

func (p *Postgres) ReceiveBackup(ctx context.Context, messageID string, payload instanceaction.BackupPayload, now time.Time) (bool, error) {
	if payload.RegionID != p.regionID || payload.Kind != "backup" && payload.Kind != "restore" || !instanceaction.VerifyBackup(payload, p.authorityKey, now) {
		return false, ErrInvalidTask
	}
	return p.receive(ctx, messageID, payload.WorkspaceID, payload.LogicalInstanceID, payload.OperationID, payload.Kind, payload, now)
}

func (p *Postgres) receive(ctx context.Context, messageID, workspaceID, instanceID, operationID, kind string, payload any, now time.Time) (bool, error) {
	if messageID == "" || workspaceID == "" || instanceID == "" || operationID == "" {
		return false, ErrInvalidTask
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	tx, err := p.database.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO regional_inbox (message_id,message_type,received_at) VALUES ($1,$2,$3) ON CONFLICT (message_id) DO NOTHING`, messageID, kind+".requested.v1", now)
	if err != nil {
		return false, err
	}
	inserted, _ := result.RowsAffected()
	if inserted == 0 {
		return false, nil
	}
	var fencingToken int64
	if err := tx.QueryRowContext(ctx, `SELECT fencing_token FROM regional_delivery_states WHERE logical_instance_id=$1 AND workspace_id=$2 AND region_id=$3`, instanceID, workspaceID, p.regionID).Scan(&fencingToken); err != nil {
		return false, err
	}
	id, err := persistence.NewID("tsk")
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO regional_instance_tasks (id,message_id,workspace_id,logical_instance_id,region_id,operation_id,kind,payload,status,fencing_token,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'queued',$9,$10,$10)`, id, messageID, workspaceID, instanceID, p.regionID, operationID, kind, encoded, fencingToken, now); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE regional_inbox SET handled_at=$2 WHERE message_id=$1`, messageID, now); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (p *Postgres) Claim(ctx context.Context, worker string, now time.Time) (Task, bool, error) {
	tx, err := p.database.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, false, err
	}
	defer tx.Rollback()
	task, err := scanTask(tx.QueryRowContext(ctx, taskSelect+` WHERE status='queued' AND (claim_until IS NULL OR claim_until<=$1) ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1`, now))
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, false, nil
	}
	if err != nil {
		return Task{}, false, err
	}
	task.ClaimOwner, task.ClaimUntil, task.AttemptCount = worker, now.Add(p.lease), task.AttemptCount+1
	if _, err := tx.ExecContext(ctx, `UPDATE regional_instance_tasks SET claim_owner=$2,claim_until=$3,attempt_count=$4,updated_at=$5 WHERE id=$1`, task.ID, worker, task.ClaimUntil, task.AttemptCount, now); err != nil {
		return Task{}, false, err
	}
	return task, true, tx.Commit()
}

func (p *Postgres) Complete(ctx context.Context, taskID, worker string, fencingToken int64, result BackupResult, now time.Time) (bool, error) {
	if result.Status != "completed" && result.Status != "failed" {
		return false, ErrInvalidTask
	}
	tx, err := p.database.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	task, err := scanTask(tx.QueryRowContext(ctx, taskSelect+` WHERE id=$1 FOR UPDATE`, taskID))
	if err != nil {
		return false, err
	}
	if task.Status == "completed" || task.Status == "failed" {
		return false, nil
	}
	if task.ClaimOwner != worker || !task.ClaimUntil.After(now) || task.FencingToken != fencingToken {
		return false, ErrTaskLease
	}
	status := "completed"
	if result.Status == "failed" {
		status = "failed"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE regional_instance_tasks SET status=$2,claim_owner=NULL,claim_until=NULL,failure_code=NULLIF($3,''),updated_at=$4 WHERE id=$1`, task.ID, status, result.FailureCode, now); err != nil {
		return false, err
	}
	if task.Kind == "backup" {
		var payload instanceaction.BackupPayload
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			return false, err
		}
		messageID, err := persistence.NewID("msg")
		if err != nil {
			return false, err
		}
		var providerReleaseID, gameVersion, revisionID string
		var modLock json.RawMessage
		if err := tx.QueryRowContext(ctx, `SELECT provider_release_id,game_version,instance_revision_id,mod_lock FROM regional_delivery_states WHERE logical_instance_id=$1`, task.LogicalInstanceID).Scan(&providerReleaseID, &gameVersion, &revisionID, &modLock); err != nil {
			return false, err
		}
		observed := map[string]any{"workspaceId": task.WorkspaceID, "backupRequestId": payload.BackupRequestID, "operationId": task.OperationID, "logicalInstanceId": task.LogicalInstanceID, "regionId": task.RegionID, "sequence": task.AttemptCount, "status": result.Status, "providerReleaseId": providerReleaseID, "gameVersion": gameVersion, "configurationRevisionId": revisionID, "modLock": modLock, "checksums": result.Checksums, "objectKey": result.ObjectKey, "sizeBytes": result.SizeBytes, "failureCode": result.FailureCode, "observedAt": now}
		if err := messaging.NewPostgres(p.database).InsertOutboxTo(ctx, tx, messaging.RegionOutbox, messaging.OutboxMessage{SchemaVersion: 1, ID: contract.EventID(messageID), MessageType: "backup.observed.v1", IdempotencyKey: contract.IdempotencyKey("backup:" + task.ID), Payload: observed, CreatedAt: now}); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

const taskSelect = `SELECT id,message_id,workspace_id,logical_instance_id,region_id,operation_id,kind,payload,status,COALESCE(claim_owner,''),claim_until,attempt_count,fencing_token FROM regional_instance_tasks`

func scanTask(row interface{ Scan(...any) error }) (Task, error) {
	var task Task
	var claimUntil sql.NullTime
	err := row.Scan(&task.ID, &task.MessageID, &task.WorkspaceID, &task.LogicalInstanceID, &task.RegionID, &task.OperationID, &task.Kind, &task.Payload, &task.Status, &task.ClaimOwner, &claimUntil, &task.AttemptCount, &task.FencingToken)
	task.ClaimUntil = claimUntil.Time
	return task, err
}
