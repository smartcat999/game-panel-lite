package instanceaction

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliverycontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/persistence"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
)

var (
	ErrInvalidAction      = errors.New("invalid instance action")
	ErrCrossRegionRestore = errors.New("backup restore must remain in its owning region and instance")
)

type InstanceReader interface {
	Instance(context.Context, string, string) (deliverycontrol.Instance, error)
	Revision(context.Context, string, string, string) (deliverycontrol.Revision, error)
}

type ProviderRegistry interface {
	Verified(context.Context, string) (providercontract.Manifest, error)
}

type Postgres struct {
	database     *sql.DB
	instances    InstanceReader
	providers    ProviderRegistry
	authorityKey []byte
}

type Grant struct {
	Issuer    string    `json:"issuer"`
	Action    string    `json:"action"`
	RegionID  string    `json:"regionId"`
	ExpiresAt time.Time `json:"expiresAt"`
	Signature string    `json:"signature"`
}

type ConsolePayload struct {
	WorkspaceID       string `json:"workspaceId"`
	LogicalInstanceID string `json:"logicalInstanceId"`
	RegionID          string `json:"regionId"`
	OperationID       string `json:"operationId"`
	Command           string `json:"command"`
	AuthorityGrant    Grant  `json:"authorityGrant"`
}

type BackupPayload struct {
	WorkspaceID       string                  `json:"workspaceId"`
	BackupRequestID   string                  `json:"backupRequestId"`
	OperationID       string                  `json:"operationId"`
	LogicalInstanceID string                  `json:"logicalInstanceId"`
	RegionID          string                  `json:"regionId"`
	Kind              string                  `json:"kind"`
	ObjectKey         string                  `json:"objectKey"`
	TransferURL       string                  `json:"transferUrl"`
	DataScope         string                  `json:"dataScope"`
	AuthorityGrant    Grant                   `json:"authorityGrant"`
	ProviderReleaseID string                  `json:"-"`
	GameVersion       string                  `json:"-"`
	RevisionID        string                  `json:"-"`
	ModLock           []contract.ModLockEntry `json:"-"`
}

type Backup struct {
	ID                      string                  `json:"id"`
	WorkspaceID             string                  `json:"-"`
	LogicalInstanceID       string                  `json:"logicalInstanceId"`
	RegionID                string                  `json:"regionId"`
	OperationID             string                  `json:"operationId"`
	ProviderReleaseID       string                  `json:"providerReleaseId"`
	GameVersion             string                  `json:"gameVersion"`
	ConfigurationRevisionID string                  `json:"configurationRevisionId"`
	ModLock                 []contract.ModLockEntry `json:"modLock"`
	Checksums               map[string]string       `json:"checksums"`
	ObjectKey               string                  `json:"objectKey,omitempty"`
	SizeBytes               int64                   `json:"sizeBytes,omitempty"`
	Status                  string                  `json:"status"`
	Sequence                int64                   `json:"-"`
	CreatedAt               time.Time               `json:"createdAt"`
	UpdatedAt               time.Time               `json:"updatedAt"`
}

type BackupObservation struct {
	MessageID               string                  `json:"-"`
	WorkspaceID             string                  `json:"workspaceId"`
	BackupID                string                  `json:"backupRequestId"`
	OperationID             string                  `json:"operationId"`
	LogicalInstanceID       string                  `json:"logicalInstanceId"`
	RegionID                string                  `json:"regionId"`
	ProviderReleaseID       string                  `json:"providerReleaseId"`
	GameVersion             string                  `json:"gameVersion"`
	ConfigurationRevisionID string                  `json:"configurationRevisionId"`
	ModLock                 []contract.ModLockEntry `json:"modLock"`
	Sequence                int64                   `json:"sequence"`
	Status                  string                  `json:"status"`
	ObjectKey               string                  `json:"objectKey"`
	SizeBytes               int64                   `json:"sizeBytes"`
	Checksums               map[string]string       `json:"checksums"`
	FailureCode             string                  `json:"failureCode"`
	ObservedAt              time.Time               `json:"observedAt"`
}

func NewPostgres(database *sql.DB, instances InstanceReader, providers ProviderRegistry, authorityKey []byte) *Postgres {
	return &Postgres{database: database, instances: instances, providers: providers, authorityKey: append([]byte(nil), authorityKey...)}
}

func (p *Postgres) Console(ctx context.Context, workspaceID, instanceID, command, idempotencyKey string, now time.Time) (deliverycontrol.Operation, error) {
	if command == "" || len(command) > 4096 || len(idempotencyKey) < 8 {
		return deliverycontrol.Operation{}, ErrInvalidAction
	}
	instance, manifest, err := p.instanceCapability(ctx, workspaceID, instanceID, "console")
	if err != nil {
		return deliverycontrol.Operation{}, err
	}
	_ = manifest
	payload := ConsolePayload{WorkspaceID: workspaceID, LogicalInstanceID: instanceID, RegionID: instance.RegionID, Command: command}
	return p.writeOperation(ctx, "instance.console", instance, idempotencyKey, "console.command.requested.v1", &payload, "console.execute", now)
}

func (p *Postgres) RequestBackup(ctx context.Context, workspaceID, instanceID, idempotencyKey string, now time.Time) (Backup, deliverycontrol.Operation, error) {
	if len(idempotencyKey) < 8 || now.IsZero() {
		return Backup{}, deliverycontrol.Operation{}, ErrInvalidAction
	}
	instance, _, err := p.instanceCapability(ctx, workspaceID, instanceID, "backup")
	if err != nil {
		return Backup{}, deliverycontrol.Operation{}, err
	}
	revision, err := p.instances.Revision(ctx, workspaceID, instanceID, instance.InstanceRevisionID)
	if err != nil {
		return Backup{}, deliverycontrol.Operation{}, err
	}
	tx, err := p.database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Backup{}, deliverycontrol.Operation{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, workspaceID+"|"+idempotencyKey); err != nil {
		return Backup{}, deliverycontrol.Operation{}, err
	}
	if previous, err := backupByKey(ctx, tx, workspaceID, idempotencyKey); err == nil {
		operation, operationErr := operationByID(ctx, tx, previous.OperationID)
		return previous, operation, operationErr
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Backup{}, deliverycontrol.Operation{}, err
	}
	backupID, operationID, messageID, err := actionIDs("bkr")
	if err != nil {
		return Backup{}, deliverycontrol.Operation{}, err
	}
	now = now.UTC()
	operation := newOperation(operationID, workspaceID, "instance.backup", instanceID, idempotencyKey, now)
	objectKey := "regions/" + instance.RegionID + "/instances/" + instanceID + "/backups/" + backupID + ".tar"
	backup := Backup{ID: backupID, WorkspaceID: workspaceID, LogicalInstanceID: instanceID, RegionID: instance.RegionID, OperationID: operationID, ProviderReleaseID: instance.ProviderReleaseID, GameVersion: instance.GameVersion, ConfigurationRevisionID: revision.ID, ModLock: append([]contract.ModLockEntry(nil), revision.ModLock...), Checksums: map[string]string{}, Status: "queued", CreatedAt: now, UpdatedAt: now}
	payload := BackupPayload{WorkspaceID: workspaceID, BackupRequestID: backupID, OperationID: operationID, LogicalInstanceID: instanceID, RegionID: instance.RegionID, Kind: "backup", ObjectKey: objectKey, TransferURL: "object://" + objectKey, DataScope: "instance"}
	if err := signGrant(&payload, "backup.execute", p.authorityKey, now.Add(30*time.Minute)); err != nil {
		return Backup{}, deliverycontrol.Operation{}, err
	}
	if err := insertOperation(ctx, tx, operation); err != nil {
		return Backup{}, deliverycontrol.Operation{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO managed_backups (id,workspace_id,logical_instance_id,region_id,operation_id,provider_release_id,game_version,configuration_revision_id,mod_lock,checksums,status,sequence,idempotency_key,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,0,$12,$13,$13)`, backup.ID, backup.WorkspaceID, backup.LogicalInstanceID, backup.RegionID, backup.OperationID, backup.ProviderReleaseID, backup.GameVersion, backup.ConfigurationRevisionID, jsonValue(backup.ModLock), jsonValue(backup.Checksums), backup.Status, idempotencyKey, now); err != nil {
		return Backup{}, deliverycontrol.Operation{}, err
	}
	if err := messaging.NewPostgres(p.database).InsertOutbox(ctx, tx, messaging.OutboxMessage{SchemaVersion: 1, ID: contract.EventID(messageID), MessageType: "backup.requested.v1", IdempotencyKey: contract.IdempotencyKey(idempotencyKey), Payload: payload, CreatedAt: now}); err != nil {
		return Backup{}, deliverycontrol.Operation{}, err
	}
	if err := tx.Commit(); err != nil {
		return Backup{}, deliverycontrol.Operation{}, err
	}
	return backup, operation, nil
}

func (p *Postgres) Restore(ctx context.Context, workspaceID, instanceID, backupID, idempotencyKey string, now time.Time) (deliverycontrol.Operation, error) {
	instance, _, err := p.instanceCapability(ctx, workspaceID, instanceID, "backup")
	if err != nil {
		return deliverycontrol.Operation{}, err
	}
	backup, err := backupByID(ctx, p.database, backupID)
	if err != nil || backup.WorkspaceID != workspaceID || backup.LogicalInstanceID != instanceID || backup.RegionID != instance.RegionID || backup.Status != "completed" {
		return deliverycontrol.Operation{}, ErrCrossRegionRestore
	}
	payload := BackupPayload{WorkspaceID: workspaceID, BackupRequestID: backup.ID, LogicalInstanceID: instanceID, RegionID: instance.RegionID, Kind: "restore", ObjectKey: backup.ObjectKey, TransferURL: "object://" + backup.ObjectKey, DataScope: "instance"}
	return p.writeOperation(ctx, "instance.restore", instance, idempotencyKey, "backup.requested.v1", &payload, "backup.execute", now)
}

func (p *Postgres) ApplyBackupObservation(ctx context.Context, observation BackupObservation) (bool, error) {
	if observation.MessageID == "" || observation.BackupID == "" || observation.Sequence < 1 || observation.ObservedAt.IsZero() {
		return false, ErrInvalidAction
	}
	return messaging.NewPostgres(p.database).HandleOnce(ctx, contract.EventID(observation.MessageID), "backup.observed.v1", observation.ObservedAt, func(query persistence.DBTX) error {
		backup, err := backupByID(ctx, query, observation.BackupID)
		if err != nil {
			return err
		}
		if backup.WorkspaceID != observation.WorkspaceID || backup.LogicalInstanceID != observation.LogicalInstanceID || backup.RegionID != observation.RegionID || backup.OperationID != observation.OperationID || backup.ProviderReleaseID != observation.ProviderReleaseID || backup.GameVersion != observation.GameVersion || backup.ConfigurationRevisionID != observation.ConfigurationRevisionID || !sameModLock(backup.ModLock, observation.ModLock) {
			return ErrInvalidAction
		}
		result, err := query.ExecContext(ctx, `UPDATE managed_backups SET sequence=$2,status=$3,object_key=NULLIF($4,''),size_bytes=$5,checksums=$6,updated_at=$7 WHERE id=$1 AND operation_id=$8 AND sequence<$2`, observation.BackupID, observation.Sequence, observation.Status, observation.ObjectKey, observation.SizeBytes, jsonValue(observation.Checksums), observation.ObservedAt, observation.OperationID)
		if err != nil {
			return err
		}
		changed, _ := result.RowsAffected()
		if changed == 0 {
			return nil
		}
		status := "running"
		if observation.Status == "completed" {
			status = "succeeded"
		} else if observation.Status == "failed" {
			status = "failed"
		}
		_, err = query.ExecContext(ctx, `UPDATE operations SET status=$2,failure_code=NULLIF($3,''),updated_at=$4 WHERE id=$1`, observation.OperationID, status, observation.FailureCode, observation.ObservedAt)
		return err
	})
}

func (p *Postgres) ListBackups(ctx context.Context, workspaceID string, limit int) ([]Backup, error) {
	if limit < 1 || limit > 100 {
		limit = 100
	}
	rows, err := p.database.QueryContext(ctx, backupSelect+` WHERE workspace_id=$1 ORDER BY created_at DESC,id LIMIT $2`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Backup, 0)
	for rows.Next() {
		backup, err := scanBackup(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, backup)
	}
	return result, rows.Err()
}

func (p *Postgres) instanceCapability(ctx context.Context, workspaceID, instanceID, capability string) (deliverycontrol.Instance, providercontract.Manifest, error) {
	instance, err := p.instances.Instance(ctx, workspaceID, instanceID)
	if err != nil {
		return deliverycontrol.Instance{}, providercontract.Manifest{}, err
	}
	manifest, err := p.providers.Verified(ctx, instance.ProviderReleaseID)
	if err != nil || !slices.Contains(manifest.Capabilities, capability) {
		return deliverycontrol.Instance{}, providercontract.Manifest{}, ErrInvalidAction
	}
	return instance, manifest, nil
}

func (p *Postgres) writeOperation(ctx context.Context, kind string, instance deliverycontrol.Instance, key, messageType string, payload any, action string, now time.Time) (deliverycontrol.Operation, error) {
	if len(key) < 8 || now.IsZero() {
		return deliverycontrol.Operation{}, ErrInvalidAction
	}
	tx, err := p.database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return deliverycontrol.Operation{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, instance.WorkspaceID+"|"+key); err != nil {
		return deliverycontrol.Operation{}, err
	}
	if previous, err := operationByKey(ctx, tx, instance.WorkspaceID, key); err == nil {
		return previous, tx.Commit()
	} else if !errors.Is(err, sql.ErrNoRows) {
		return deliverycontrol.Operation{}, err
	}
	_, operationID, messageID, err := actionIDs("act")
	if err != nil {
		return deliverycontrol.Operation{}, err
	}
	operation := newOperation(operationID, instance.WorkspaceID, kind, instance.ID, key, now.UTC())
	switch value := payload.(type) {
	case *ConsolePayload:
		value.OperationID = operation.ID
		if err := signGrant(value, action, p.authorityKey, now.Add(30*time.Minute)); err != nil {
			return deliverycontrol.Operation{}, err
		}
	case *BackupPayload:
		value.OperationID = operation.ID
		if err := signGrant(value, action, p.authorityKey, now.Add(30*time.Minute)); err != nil {
			return deliverycontrol.Operation{}, err
		}
	}
	if err := insertOperation(ctx, tx, operation); err != nil {
		return deliverycontrol.Operation{}, err
	}
	if err := messaging.NewPostgres(p.database).InsertOutbox(ctx, tx, messaging.OutboxMessage{SchemaVersion: 1, ID: contract.EventID(messageID), MessageType: messageType, IdempotencyKey: contract.IdempotencyKey(key), Payload: payload, CreatedAt: now.UTC()}); err != nil {
		return deliverycontrol.Operation{}, err
	}
	return operation, tx.Commit()
}

func signGrant(payload any, action string, key []byte, expiresAt time.Time) error {
	var grant *Grant
	switch value := payload.(type) {
	case *ConsolePayload:
		value.AuthorityGrant = Grant{Issuer: "control-plane", Action: action, RegionID: value.RegionID, ExpiresAt: expiresAt.UTC()}
		grant = &value.AuthorityGrant
	case *BackupPayload:
		value.AuthorityGrant = Grant{Issuer: "control-plane", Action: action, RegionID: value.RegionID, ExpiresAt: expiresAt.UTC()}
		grant = &value.AuthorityGrant
	default:
		return ErrInvalidAction
	}
	unsigned := *grant
	unsigned.Signature = ""
	grant.Signature = ""
	encoded, err := json.Marshal(struct {
		Payload any
		Grant   Grant
	}{payload, unsigned})
	if err != nil {
		return err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(encoded)
	grant.Signature = hex.EncodeToString(mac.Sum(nil))
	return nil
}

func VerifyConsole(payload ConsolePayload, key []byte, now time.Time) bool {
	return verifyGrant(&payload, "console.execute", key, now)
}

func VerifyBackup(payload BackupPayload, key []byte, now time.Time) bool {
	return verifyGrant(&payload, "backup.execute", key, now)
}

func verifyGrant(payload any, action string, key []byte, now time.Time) bool {
	var grant Grant
	var regionID string
	switch value := payload.(type) {
	case *ConsolePayload:
		grant = value.AuthorityGrant
		regionID = value.RegionID
		value.AuthorityGrant.Signature = ""
	case *BackupPayload:
		grant = value.AuthorityGrant
		regionID = value.RegionID
		value.AuthorityGrant.Signature = ""
	default:
		return false
	}
	if grant.Issuer != "control-plane" || grant.Action != action || grant.RegionID != regionID || !now.Before(grant.ExpiresAt) {
		return false
	}
	unsigned := grant
	unsigned.Signature = ""
	encoded, err := json.Marshal(struct {
		Payload any
		Grant   Grant
	}{payload, unsigned})
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(encoded)
	return hmac.Equal([]byte(grant.Signature), []byte(hex.EncodeToString(mac.Sum(nil))))
}

func actionIDs(prefix string) (string, string, string, error) {
	resourceID, err := persistence.NewID(prefix)
	if err != nil {
		return "", "", "", err
	}
	operationID, err := persistence.NewID("op")
	if err != nil {
		return "", "", "", err
	}
	messageID, err := persistence.NewID("msg")
	return resourceID, operationID, messageID, err
}

func newOperation(id, workspaceID, kind, resourceID, key string, now time.Time) deliverycontrol.Operation {
	return deliverycontrol.Operation{ID: id, WorkspaceID: workspaceID, Kind: kind, ResourceType: "instance", ResourceID: resourceID, IdempotencyKey: key, Status: "queued", Steps: []deliverycontrol.Step{{Key: "accepted", Label: "Request accepted", Status: "succeeded"}, {Key: "executed", Label: "Executed by node", Status: "pending"}}, CreatedAt: now, UpdatedAt: now}
}

func insertOperation(ctx context.Context, query persistence.DBTX, operation deliverycontrol.Operation) error {
	_, err := query.ExecContext(ctx, `INSERT INTO operations (id,workspace_id,kind,resource_type,resource_id,idempotency_key,status,steps,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$9)`, operation.ID, operation.WorkspaceID, operation.Kind, operation.ResourceType, operation.ResourceID, operation.IdempotencyKey, operation.Status, jsonValue(operation.Steps), operation.CreatedAt)
	return err
}

func operationByKey(ctx context.Context, query persistence.DBTX, workspaceID, key string) (deliverycontrol.Operation, error) {
	return scanOperation(query.QueryRowContext(ctx, `SELECT id,workspace_id,kind,resource_type,resource_id,idempotency_key,status,steps,COALESCE(failure_code,''),created_at,updated_at FROM operations WHERE workspace_id=$1 AND idempotency_key=$2`, workspaceID, key))
}

func operationByID(ctx context.Context, query persistence.DBTX, id string) (deliverycontrol.Operation, error) {
	return scanOperation(query.QueryRowContext(ctx, `SELECT id,workspace_id,kind,resource_type,resource_id,idempotency_key,status,steps,COALESCE(failure_code,''),created_at,updated_at FROM operations WHERE id=$1`, id))
}

func scanOperation(row interface{ Scan(...any) error }) (deliverycontrol.Operation, error) {
	var operation deliverycontrol.Operation
	var steps []byte
	err := row.Scan(&operation.ID, &operation.WorkspaceID, &operation.Kind, &operation.ResourceType, &operation.ResourceID, &operation.IdempotencyKey, &operation.Status, &steps, &operation.FailureCode, &operation.CreatedAt, &operation.UpdatedAt)
	if err == nil {
		err = json.Unmarshal(steps, &operation.Steps)
	}
	return operation, err
}

const backupSelect = `SELECT id,workspace_id,logical_instance_id,region_id,operation_id,provider_release_id,game_version,configuration_revision_id,mod_lock,checksums,COALESCE(object_key,''),COALESCE(size_bytes,0),status,sequence,created_at,updated_at FROM managed_backups`

func backupByID(ctx context.Context, query persistence.DBTX, id string) (Backup, error) {
	return scanBackup(query.QueryRowContext(ctx, backupSelect+` WHERE id=$1`, id))
}

func backupByKey(ctx context.Context, query persistence.DBTX, workspaceID, key string) (Backup, error) {
	return scanBackup(query.QueryRowContext(ctx, backupSelect+` WHERE workspace_id=$1 AND idempotency_key=$2`, workspaceID, key))
}

func scanBackup(row interface{ Scan(...any) error }) (Backup, error) {
	var backup Backup
	var modLock, checksums []byte
	err := row.Scan(&backup.ID, &backup.WorkspaceID, &backup.LogicalInstanceID, &backup.RegionID, &backup.OperationID, &backup.ProviderReleaseID, &backup.GameVersion, &backup.ConfigurationRevisionID, &modLock, &checksums, &backup.ObjectKey, &backup.SizeBytes, &backup.Status, &backup.Sequence, &backup.CreatedAt, &backup.UpdatedAt)
	if err != nil {
		return Backup{}, err
	}
	if err = json.Unmarshal(modLock, &backup.ModLock); err != nil {
		return Backup{}, err
	}
	err = json.Unmarshal(checksums, &backup.Checksums)
	return backup, err
}

func jsonValue(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}

func sameModLock(left, right []contract.ModLockEntry) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
