package deliverycontrol

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/billing"
	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/persistence"
)

type Postgres struct {
	database     *sql.DB
	funding      *billing.PostgresStore
	fundingKey   []byte
	authorityKey []byte
}

func NewPostgres(database *sql.DB, fundingKey, authorityKey []byte) *Postgres {
	return &Postgres{database: database, funding: billing.NewPostgresStore(database), fundingKey: append([]byte(nil), fundingKey...), authorityKey: append([]byte(nil), authorityKey...)}
}

func (p *Postgres) Create(ctx context.Context, command CreateCommand, now time.Time) (Instance, Operation, error) {
	if err := validateCreate(command, now); err != nil {
		return Instance{}, Operation{}, err
	}
	tx, err := p.database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Instance{}, Operation{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, command.WorkspaceID+"|"+command.IdempotencyKey); err != nil {
		return Instance{}, Operation{}, err
	}
	if previous, err := operationByKey(ctx, tx, command.WorkspaceID, command.IdempotencyKey); err == nil {
		instance, loadErr := instanceByID(ctx, tx, previous.ResourceID)
		if loadErr != nil {
			return Instance{}, Operation{}, loadErr
		}
		revision, loadErr := revisionByID(ctx, tx, instance.InstanceRevisionID)
		if loadErr != nil {
			return Instance{}, Operation{}, loadErr
		}
		if !sameCreate(instance, revision, command) {
			return Instance{}, Operation{}, ErrImmutable
		}
		return instance, previous, tx.Commit()
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Instance{}, Operation{}, err
	}
	quote, err := p.funding.AuthorizedQuote(ctx, tx, command.WorkspaceID, command.QuoteID, now.UTC(), p.fundingKey)
	if err != nil {
		return Instance{}, Operation{}, err
	}
	instanceID, operationID, revisionID, messageID, err := createIDs()
	if err != nil {
		return Instance{}, Operation{}, err
	}
	now = now.UTC()
	operation := Operation{ID: operationID, WorkspaceID: command.WorkspaceID, Kind: "instance.create", ResourceType: "instance", ResourceID: instanceID, IdempotencyKey: command.IdempotencyKey, Status: "queued", Steps: initialSteps(), CreatedAt: now, UpdatedAt: now}
	instance := Instance{ID: instanceID, WorkspaceID: command.WorkspaceID, RegionID: quote.RegionID, Name: strings.TrimSpace(command.Name), ProviderReleaseID: command.ProviderReleaseID, GameVersion: command.GameVersion, QuoteID: quote.ID, InstanceRevisionID: revisionID, PlacementVersion: 1, ResourceSpec: quote.ResourceSpec, Configuration: cloneMap(command.Configuration), ModLock: cloneModLock(command.ModLock), ListenerRequirements: cloneListeners(command.ListenerRequirements), DesiredState: "running", ObservedState: "pending", LatestOperationID: operationID, CreatedAt: now, UpdatedAt: now}
	if err := insertInstance(ctx, tx, instance); err != nil {
		return Instance{}, Operation{}, err
	}
	revision := Revision{ID: revisionID, OperationID: operationID, WorkspaceID: instance.WorkspaceID, LogicalInstanceID: instance.ID, ProviderReleaseID: instance.ProviderReleaseID, GameVersion: instance.GameVersion, SchemaVersion: command.SchemaVersion, Configuration: instance.Configuration, ModLock: instance.ModLock, ApplyBehavior: "recreate-required", CreatedAt: now}
	if err := insertRevision(ctx, tx, revision); err != nil {
		return Instance{}, Operation{}, err
	}
	if err := insertOperation(ctx, tx, operation); err != nil {
		return Instance{}, Operation{}, err
	}
	payload := DesiredPayload{WorkspaceID: instance.WorkspaceID, LogicalInstanceID: instance.ID, RegionID: instance.RegionID, PlacementVersion: 1, InstanceRevisionID: revisionID, OperationID: operationID, DesiredState: "running", ProviderReleaseID: instance.ProviderReleaseID, GameVersion: instance.GameVersion, ApplyBehavior: "recreate-required", ResourceSpec: instance.ResourceSpec, Configuration: instance.Configuration, ModLock: instance.ModLock, ListenerRequirements: instance.ListenerRequirements}
	payload.AuthorityGrant, err = NewAuthorityGrant(payload, p.authorityKey, now.Add(30*time.Minute))
	if err != nil {
		return Instance{}, Operation{}, err
	}
	message := messaging.OutboxMessage{SchemaVersion: 1, ID: contract.EventID(messageID), MessageType: "deployment.desired.v1", IdempotencyKey: contract.IdempotencyKey("create:" + operationID), Payload: payload, CreatedAt: now}
	if err := messaging.NewPostgres(p.database).InsertOutbox(ctx, tx, message); err != nil {
		return Instance{}, Operation{}, err
	}
	if err := p.funding.ConsumeHold(ctx, tx, quote.ID); err != nil {
		return Instance{}, Operation{}, err
	}
	if err := tx.Commit(); err != nil {
		return Instance{}, Operation{}, err
	}
	return instance, operation, nil
}

func (p *Postgres) ApplyRevision(ctx context.Context, command ApplyRevisionCommand, now time.Time) (Revision, Operation, error) {
	if command.WorkspaceID == "" || command.LogicalInstanceID == "" || command.BaseRevisionID == "" || command.ProviderReleaseID == "" || command.GameVersion == "" || command.SchemaVersion < 1 || len(command.IdempotencyKey) < 8 || now.IsZero() || command.ApplyBehavior != "hot-reload" && command.ApplyBehavior != "restart-required" && command.ApplyBehavior != "recreate-required" {
		return Revision{}, Operation{}, ErrInvalidCommand
	}
	tx, err := p.database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Revision{}, Operation{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, command.WorkspaceID+"|"+command.IdempotencyKey); err != nil {
		return Revision{}, Operation{}, err
	}
	if previous, err := operationByKey(ctx, tx, command.WorkspaceID, command.IdempotencyKey); err == nil {
		revision, revisionErr := revisionByOperation(ctx, tx, previous.ID)
		if revisionErr != nil {
			return Revision{}, Operation{}, revisionErr
		}
		if !sameApply(revision, command) {
			return Revision{}, Operation{}, ErrImmutable
		}
		return revision, previous, tx.Commit()
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Revision{}, Operation{}, err
	}
	instance, err := instanceByID(ctx, tx, command.LogicalInstanceID)
	if err != nil || instance.WorkspaceID != command.WorkspaceID || instance.InstanceRevisionID != command.BaseRevisionID || instance.ProviderReleaseID != command.ProviderReleaseID || instance.GameVersion != command.GameVersion {
		return Revision{}, Operation{}, ErrImmutable
	}
	revisionID, operationID, messageID, err := createRevisionIDs()
	if err != nil {
		return Revision{}, Operation{}, err
	}
	now = now.UTC()
	operation := Operation{ID: operationID, WorkspaceID: command.WorkspaceID, Kind: "instance.configuration.apply", ResourceType: "instance", ResourceID: instance.ID, IdempotencyKey: command.IdempotencyKey, Status: "queued", Steps: initialSteps(), CreatedAt: now, UpdatedAt: now}
	revision := Revision{ID: revisionID, OperationID: operationID, WorkspaceID: command.WorkspaceID, LogicalInstanceID: instance.ID, ProviderReleaseID: command.ProviderReleaseID, GameVersion: command.GameVersion, SchemaVersion: command.SchemaVersion, Configuration: cloneMap(command.Configuration), ModLock: cloneModLock(command.ModLock), ApplyBehavior: command.ApplyBehavior, CreatedAt: now}
	if err := insertRevision(ctx, tx, revision); err != nil {
		return Revision{}, Operation{}, err
	}
	if err := insertOperation(ctx, tx, operation); err != nil {
		return Revision{}, Operation{}, err
	}
	configuration, err := json.Marshal(revision.Configuration)
	if err != nil {
		return Revision{}, Operation{}, err
	}
	modLock, err := json.Marshal(revision.ModLock)
	if err != nil {
		return Revision{}, Operation{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE managed_instances SET instance_revision_id=$2,configuration=$3,mod_lock=$4,observed_state='pending',observation_sequence=0,latest_operation_id=$5,updated_at=$6 WHERE id=$1 AND instance_revision_id=$7`, instance.ID, revision.ID, configuration, modLock, operation.ID, now, command.BaseRevisionID)
	if err != nil {
		return Revision{}, Operation{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return Revision{}, Operation{}, ErrImmutable
	}
	payload := DesiredPayload{WorkspaceID: instance.WorkspaceID, LogicalInstanceID: instance.ID, RegionID: instance.RegionID, PlacementVersion: instance.PlacementVersion, InstanceRevisionID: revision.ID, OperationID: operation.ID, DesiredState: instance.DesiredState, ProviderReleaseID: instance.ProviderReleaseID, GameVersion: instance.GameVersion, ApplyBehavior: command.ApplyBehavior, ResourceSpec: instance.ResourceSpec, Configuration: revision.Configuration, ModLock: revision.ModLock, ListenerRequirements: instance.ListenerRequirements}
	payload.AuthorityGrant, err = NewAuthorityGrant(payload, p.authorityKey, now.Add(30*time.Minute))
	if err != nil {
		return Revision{}, Operation{}, err
	}
	if err := messaging.NewPostgres(p.database).InsertOutbox(ctx, tx, messaging.OutboxMessage{SchemaVersion: 1, ID: contract.EventID(messageID), MessageType: "deployment.desired.v1", IdempotencyKey: contract.IdempotencyKey("apply:" + operation.ID), Payload: payload, CreatedAt: now}); err != nil {
		return Revision{}, Operation{}, err
	}
	if err := tx.Commit(); err != nil {
		return Revision{}, Operation{}, err
	}
	return revision, operation, nil
}

func (p *Postgres) ChangeState(ctx context.Context, command ChangeStateCommand, now time.Time) (Operation, error) {
	if command.WorkspaceID == "" || command.LogicalInstanceID == "" || command.Action != "start" && command.Action != "stop" && command.Action != "restart" || len(command.IdempotencyKey) < 8 || now.IsZero() {
		return Operation{}, ErrInvalidCommand
	}
	tx, err := p.database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Operation{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, command.WorkspaceID+"|"+command.IdempotencyKey); err != nil {
		return Operation{}, err
	}
	expectedKind := "instance." + command.Action
	if previous, err := operationByKey(ctx, tx, command.WorkspaceID, command.IdempotencyKey); err == nil {
		if previous.Kind != expectedKind || previous.ResourceID != command.LogicalInstanceID {
			return Operation{}, ErrImmutable
		}
		return previous, tx.Commit()
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Operation{}, err
	}
	instance, err := instanceByID(ctx, tx, command.LogicalInstanceID)
	if err != nil || instance.WorkspaceID != command.WorkspaceID {
		return Operation{}, ErrNotFound
	}
	if command.Action == "start" && instance.DesiredState != "stopped" || command.Action == "stop" && instance.DesiredState == "stopped" {
		return Operation{}, ErrInvalidCommand
	}
	if command.Action == "restart" && instance.DesiredState != "running" {
		return Operation{}, ErrInvalidCommand
	}
	operationID, err := persistence.NewID("op")
	if err != nil {
		return Operation{}, err
	}
	messageID, err := persistence.NewID("msg")
	if err != nil {
		return Operation{}, err
	}
	now = now.UTC()
	operation := Operation{ID: operationID, WorkspaceID: command.WorkspaceID, Kind: expectedKind, ResourceType: "instance", ResourceID: instance.ID, IdempotencyKey: command.IdempotencyKey, Status: "queued", Steps: initialSteps(), CreatedAt: now, UpdatedAt: now}
	if err := insertOperation(ctx, tx, operation); err != nil {
		return Operation{}, err
	}
	desiredState := "running"
	if command.Action == "stop" {
		desiredState = "stopped"
	}
	instance.PlacementVersion++
	result, err := tx.ExecContext(ctx, `UPDATE managed_instances SET desired_state=$2,observed_state='pending',placement_version=$3,observation_sequence=0,latest_operation_id=$4,updated_at=$5 WHERE id=$1 AND placement_version=$6`, instance.ID, desiredState, instance.PlacementVersion, operation.ID, now, instance.PlacementVersion-1)
	if err != nil {
		return Operation{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return Operation{}, ErrImmutable
	}
	payload := DesiredPayload{WorkspaceID: instance.WorkspaceID, LogicalInstanceID: instance.ID, RegionID: instance.RegionID, PlacementVersion: instance.PlacementVersion, InstanceRevisionID: instance.InstanceRevisionID, OperationID: operation.ID, DesiredState: desiredState, ProviderReleaseID: instance.ProviderReleaseID, GameVersion: instance.GameVersion, ApplyBehavior: "restart-required", ResourceSpec: instance.ResourceSpec, Configuration: instance.Configuration, ModLock: instance.ModLock, ListenerRequirements: instance.ListenerRequirements}
	payload.AuthorityGrant, err = NewAuthorityGrant(payload, p.authorityKey, now.Add(30*time.Minute))
	if err != nil {
		return Operation{}, err
	}
	if err := messaging.NewPostgres(p.database).InsertOutbox(ctx, tx, messaging.OutboxMessage{SchemaVersion: 1, ID: contract.EventID(messageID), MessageType: "deployment.desired.v1", IdempotencyKey: contract.IdempotencyKey(command.Action + ":" + operation.ID), Payload: payload, CreatedAt: now}); err != nil {
		return Operation{}, err
	}
	return operation, tx.Commit()
}

func (p *Postgres) ApplyObservation(ctx context.Context, observation Observation, now time.Time) (bool, error) {
	if observation.MessageID == "" || observation.WorkspaceID == "" || observation.LogicalInstanceID == "" || observation.RegionalDeploymentID == "" || observation.RuntimeAttemptID == "" || observation.RegionID == "" || observation.Sequence < 1 || observation.ObservedAt.IsZero() {
		return false, ErrInvalidCommand
	}
	messages := messaging.NewPostgres(p.database)
	return messages.HandleOnce(ctx, contract.EventID(observation.MessageID), "deployment.observed.v1", now.UTC(), func(query persistence.DBTX) error {
		instance, err := instanceByID(ctx, query, observation.LogicalInstanceID)
		if err != nil {
			return err
		}
		if instance.WorkspaceID != observation.WorkspaceID || instance.RegionID != observation.RegionID || observation.PlacementVersion != instance.PlacementVersion {
			return nil
		}
		if observation.Sequence <= instance.ObservationSequence {
			return nil
		}
		endpoints, err := json.Marshal(observation.EndpointBindings)
		if err != nil {
			return err
		}
		result, err := query.ExecContext(ctx, `UPDATE managed_instances SET observed_state = $2, observation_sequence = $3, endpoint_bindings = $4, updated_at = $5 WHERE id = $1 AND observation_sequence < $3`, instance.ID, observation.ObservedState, observation.Sequence, endpoints, observation.ObservedAt)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil || changed == 0 {
			return err
		}
		operation, err := operationByID(ctx, query, instance.LatestOperationID)
		if err != nil {
			return err
		}
		operation = applyOperationObservation(operation, observation, now.UTC())
		return updateOperation(ctx, query, operation)
	})
}

func (p *Postgres) Instance(ctx context.Context, workspaceID, instanceID string) (Instance, error) {
	instance, err := instanceByID(ctx, p.database, instanceID)
	if err != nil || instance.WorkspaceID != workspaceID {
		return Instance{}, ErrNotFound
	}
	return instance, nil
}

func (p *Postgres) ListInstances(ctx context.Context, workspaceID string, limit int) ([]Instance, error) {
	if limit < 1 || limit > 100 {
		limit = 100
	}
	rows, err := p.database.QueryContext(ctx, `SELECT id, workspace_id, region_id, name, provider_release_id, game_version, quote_id, instance_revision_id, placement_version, cpu_milli, memory_mib, disk_gib, configuration, mod_lock, listener_requirements, desired_state, observed_state, observation_sequence, endpoint_bindings, latest_operation_id, created_at, updated_at FROM managed_instances WHERE workspace_id=$1 ORDER BY created_at DESC,id LIMIT $2`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Instance, 0)
	for rows.Next() {
		instance, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, instance)
	}
	return result, rows.Err()
}

func (p *Postgres) Operation(ctx context.Context, workspaceID, operationID string) (Operation, error) {
	operation, err := operationByID(ctx, p.database, operationID)
	if err != nil || operation.WorkspaceID != workspaceID {
		return Operation{}, ErrNotFound
	}
	return operation, nil
}

func validateCreate(command CreateCommand, now time.Time) error {
	if command.WorkspaceID == "" || strings.TrimSpace(command.Name) == "" || command.ProviderReleaseID == "" || command.GameVersion == "" || command.SchemaVersion < 1 || command.QuoteID == "" || len(command.ListenerRequirements) == 0 || len(command.IdempotencyKey) < 8 || now.IsZero() {
		return ErrInvalidCommand
	}
	primary := 0
	names := make(map[string]bool, len(command.ListenerRequirements))
	for _, listener := range command.ListenerRequirements {
		if listener.Name == "" || names[listener.Name] || listener.Purpose == "" || listener.InternalPort < 1 || listener.InternalPort > 65535 || len(listener.Transports) == 0 || listener.ExternalPortPolicy != "allocated" && listener.ExternalPortPolicy != "default-required" || listener.AddressMode != "ip-port" && listener.AddressMode != "ip-only" {
			return ErrInvalidCommand
		}
		names[listener.Name] = true
		transports := map[string]bool{}
		for _, transport := range listener.Transports {
			if transports[transport] || transport != "tcp" && transport != "udp" {
				return ErrInvalidCommand
			}
			transports[transport] = true
		}
		if listener.Primary {
			primary++
		}
	}
	if primary != 1 {
		return ErrInvalidCommand
	}
	return nil
}

func initialSteps() []Step {
	return []Step{{Key: "accepted", Label: "Request accepted", Status: "succeeded"}, {Key: "scheduled", Label: "Capacity reserved", Status: "pending"}, {Key: "endpoint", Label: "Endpoint allocated", Status: "pending"}, {Key: "workload", Label: "Workload ready", Status: "pending"}}
}

func applyOperationObservation(operation Operation, observation Observation, now time.Time) Operation {
	operation.UpdatedAt = now
	failureStep := ""
	if observation.ObservedState == "failed" {
		switch observation.ReasonCode {
		case "insufficient_capacity":
			failureStep = "scheduled"
		case "endpoint_unavailable":
			failureStep = "endpoint"
		default:
			failureStep = "workload"
		}
	}
	for index := range operation.Steps {
		if failureStep == operation.Steps[index].Key {
			operation.Steps[index].Status = "failed"
			operation.Steps[index].Detail = observation.ReasonCode
			continue
		}
		switch operation.Steps[index].Key {
		case "scheduled":
			if failureStep != "scheduled" && observation.ObservedState != "pending" {
				operation.Steps[index].Status = "succeeded"
			}
		case "endpoint":
			if failureStep == "workload" || len(observation.EndpointBindings) > 0 {
				operation.Steps[index].Status = "succeeded"
			}
		case "workload":
			if observation.ObservedState == "running" || observation.ObservedState == "stopped" {
				operation.Steps[index].Status = "succeeded"
			}
		}
	}
	switch observation.ObservedState {
	case "running", "stopped":
		operation.Status = "succeeded"
	case "failed":
		operation.Status = "failed"
		operation.FailureCode = observation.ReasonCode
	default:
		operation.Status = "running"
	}
	return operation
}

func createIDs() (string, string, string, string, error) {
	instanceID, err := persistence.NewID("lin")
	if err != nil {
		return "", "", "", "", err
	}
	operationID, err := persistence.NewID("op")
	if err != nil {
		return "", "", "", "", err
	}
	revisionID, err := persistence.NewID("rev")
	if err != nil {
		return "", "", "", "", err
	}
	messageID, err := persistence.NewID("msg")
	return instanceID, operationID, revisionID, messageID, err
}

func createRevisionIDs() (string, string, string, error) {
	revisionID, err := persistence.NewID("rev")
	if err != nil {
		return "", "", "", err
	}
	operationID, err := persistence.NewID("op")
	if err != nil {
		return "", "", "", err
	}
	messageID, err := persistence.NewID("msg")
	return revisionID, operationID, messageID, err
}

func insertInstance(ctx context.Context, query persistence.DBTX, instance Instance) error {
	configuration, err := json.Marshal(instance.Configuration)
	if err != nil {
		return err
	}
	listeners, err := json.Marshal(instance.ListenerRequirements)
	if err != nil {
		return err
	}
	endpoints, err := json.Marshal(instance.EndpointBindings)
	if err != nil {
		return err
	}
	modLock, err := json.Marshal(instance.ModLock)
	if err != nil {
		return err
	}
	_, err = query.ExecContext(ctx, `INSERT INTO managed_instances (id, workspace_id, region_id, name, provider_release_id, game_version, quote_id, instance_revision_id, placement_version, cpu_milli, memory_mib, disk_gib, configuration, mod_lock, listener_requirements, desired_state, observed_state, observation_sequence, endpoint_bindings, latest_operation_id, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`, instance.ID, instance.WorkspaceID, instance.RegionID, instance.Name, instance.ProviderReleaseID, instance.GameVersion, instance.QuoteID, instance.InstanceRevisionID, instance.PlacementVersion, instance.ResourceSpec.CPUMilli, instance.ResourceSpec.MemoryMiB, instance.ResourceSpec.DiskGiB, configuration, modLock, listeners, instance.DesiredState, instance.ObservedState, instance.ObservationSequence, endpoints, instance.LatestOperationID, instance.CreatedAt, instance.UpdatedAt)
	return err
}

func insertOperation(ctx context.Context, query persistence.DBTX, operation Operation) error {
	steps, err := json.Marshal(operation.Steps)
	if err != nil {
		return err
	}
	_, err = query.ExecContext(ctx, `INSERT INTO operations (id, workspace_id, kind, resource_type, resource_id, idempotency_key, status, steps, failure_code, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),$10,$11)`, operation.ID, operation.WorkspaceID, operation.Kind, operation.ResourceType, operation.ResourceID, operation.IdempotencyKey, operation.Status, steps, operation.FailureCode, operation.CreatedAt, operation.UpdatedAt)
	return err
}

func updateOperation(ctx context.Context, query persistence.DBTX, operation Operation) error {
	steps, err := json.Marshal(operation.Steps)
	if err != nil {
		return err
	}
	_, err = query.ExecContext(ctx, `UPDATE operations SET status=$2, steps=$3, failure_code=NULLIF($4,''), updated_at=$5 WHERE id=$1`, operation.ID, operation.Status, steps, operation.FailureCode, operation.UpdatedAt)
	return err
}

func operationByKey(ctx context.Context, query persistence.DBTX, workspaceID, key string) (Operation, error) {
	return scanOperation(query.QueryRowContext(ctx, `SELECT id, workspace_id, kind, resource_type, resource_id, idempotency_key, status, steps, COALESCE(failure_code,''), created_at, updated_at FROM operations WHERE workspace_id=$1 AND idempotency_key=$2`, workspaceID, key))
}

func operationByID(ctx context.Context, query persistence.DBTX, id string) (Operation, error) {
	return scanOperation(query.QueryRowContext(ctx, `SELECT id, workspace_id, kind, resource_type, resource_id, idempotency_key, status, steps, COALESCE(failure_code,''), created_at, updated_at FROM operations WHERE id=$1`, id))
}

func scanOperation(row rowScanner) (Operation, error) {
	var operation Operation
	var steps []byte
	err := row.Scan(&operation.ID, &operation.WorkspaceID, &operation.Kind, &operation.ResourceType, &operation.ResourceID, &operation.IdempotencyKey, &operation.Status, &steps, &operation.FailureCode, &operation.CreatedAt, &operation.UpdatedAt)
	if err == nil {
		err = json.Unmarshal(steps, &operation.Steps)
	}
	return operation, err
}

func instanceByID(ctx context.Context, query persistence.DBTX, id string) (Instance, error) {
	return scanInstance(query.QueryRowContext(ctx, `SELECT id, workspace_id, region_id, name, provider_release_id, game_version, quote_id, instance_revision_id, placement_version, cpu_milli, memory_mib, disk_gib, configuration, mod_lock, listener_requirements, desired_state, observed_state, observation_sequence, endpoint_bindings, latest_operation_id, created_at, updated_at FROM managed_instances WHERE id=$1`, id))
}

func scanInstance(row rowScanner) (Instance, error) {
	var instance Instance
	var configuration, modLock, listeners, endpoints []byte
	err := row.Scan(&instance.ID, &instance.WorkspaceID, &instance.RegionID, &instance.Name, &instance.ProviderReleaseID, &instance.GameVersion, &instance.QuoteID, &instance.InstanceRevisionID, &instance.PlacementVersion, &instance.ResourceSpec.CPUMilli, &instance.ResourceSpec.MemoryMiB, &instance.ResourceSpec.DiskGiB, &configuration, &modLock, &listeners, &instance.DesiredState, &instance.ObservedState, &instance.ObservationSequence, &endpoints, &instance.LatestOperationID, &instance.CreatedAt, &instance.UpdatedAt)
	if err != nil {
		return Instance{}, err
	}
	if err = json.Unmarshal(configuration, &instance.Configuration); err != nil {
		return Instance{}, err
	}
	if err = json.Unmarshal(modLock, &instance.ModLock); err != nil {
		return Instance{}, err
	}
	if err = json.Unmarshal(listeners, &instance.ListenerRequirements); err != nil {
		return Instance{}, err
	}
	err = json.Unmarshal(endpoints, &instance.EndpointBindings)
	return instance, err
}

func sameCreate(instance Instance, revision Revision, command CreateCommand) bool {
	return instance.WorkspaceID == command.WorkspaceID && instance.Name == strings.TrimSpace(command.Name) && instance.ProviderReleaseID == command.ProviderReleaseID && instance.GameVersion == command.GameVersion && instance.QuoteID == command.QuoteID && revision.SchemaVersion == command.SchemaVersion && reflect.DeepEqual(instance.Configuration, command.Configuration) && equalModLock(instance.ModLock, command.ModLock) && reflect.DeepEqual(instance.ListenerRequirements, command.ListenerRequirements)
}

func equalModLock(left, right []ModLockEntry) bool {
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

func sameApply(revision Revision, command ApplyRevisionCommand) bool {
	return revision.WorkspaceID == command.WorkspaceID && revision.LogicalInstanceID == command.LogicalInstanceID && revision.ProviderReleaseID == command.ProviderReleaseID && revision.GameVersion == command.GameVersion && revision.SchemaVersion == command.SchemaVersion && revision.ApplyBehavior == command.ApplyBehavior && reflect.DeepEqual(revision.Configuration, command.Configuration) && equalModLock(revision.ModLock, command.ModLock)
}

func cloneMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneListeners(source []ListenerRequirement) []ListenerRequirement {
	result := append([]ListenerRequirement(nil), source...)
	for index := range result {
		result[index].Transports = append([]string(nil), result[index].Transports...)
	}
	return result
}

func cloneModLock(source []ModLockEntry) []ModLockEntry {
	return append([]ModLockEntry{}, source...)
}

type rowScanner interface{ Scan(...any) error }
