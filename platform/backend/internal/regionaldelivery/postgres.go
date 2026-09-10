package regionaldelivery

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliverycontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/persistence"
)

type Postgres struct {
	database     *sql.DB
	regionID     string
	authorityKey []byte
	lease        time.Duration
}

func NewPostgres(database *sql.DB, regionID string, authorityKey []byte) *Postgres {
	return &Postgres{database: database, regionID: regionID, authorityKey: append([]byte(nil), authorityKey...), lease: 30 * time.Second}
}

func (p *Postgres) RegisterNode(ctx context.Context, node Node) error {
	if node.ID == "" || node.RegionID != p.regionID || node.CPUCapacityMilli <= 0 || node.MemoryCapacityMiB <= 0 || node.DiskCapacityGiB <= 0 || node.LeaseUntil.IsZero() {
		return ErrInvalidDesired
	}
	_, err := p.database.ExecContext(ctx, `INSERT INTO regional_delivery_nodes (id,region_id,state,cpu_capacity_milli,memory_capacity_mib,disk_capacity_gib,reserved_cpu_milli,reserved_memory_mib,reserved_disk_gib,lease_until,updated_at) VALUES ($1,$2,$3,$4,$5,$6,0,0,0,$7,$8) ON CONFLICT (id) DO UPDATE SET state=EXCLUDED.state,cpu_capacity_milli=EXCLUDED.cpu_capacity_milli,memory_capacity_mib=EXCLUDED.memory_capacity_mib,disk_capacity_gib=EXCLUDED.disk_capacity_gib,lease_until=EXCLUDED.lease_until,updated_at=EXCLUDED.updated_at`, node.ID, node.RegionID, node.State, node.CPUCapacityMilli, node.MemoryCapacityMiB, node.DiskCapacityGiB, node.LeaseUntil, node.UpdatedAt)
	return err
}

func (p *Postgres) AddEndpointPool(ctx context.Context, pool EndpointPool) error {
	validMode := pool.DeliveryMode == "gateway" || pool.DeliveryMode == "node-direct" || pool.DeliveryMode == "dedicated-ip"
	portsValid := pool.DeliveryMode == "dedicated-ip" && pool.PortStart == nil && pool.PortEnd == nil || pool.DeliveryMode != "dedicated-ip" && pool.PortStart != nil && pool.PortEnd != nil && *pool.PortStart >= 1 && *pool.PortEnd >= *pool.PortStart && *pool.PortEnd <= 65535
	if pool.ID == "" || pool.RegionID != p.regionID || pool.Address == "" || !validMode || !portsValid || pool.Stability != "stable" && pool.Stability != "may-change" {
		return ErrInvalidDesired
	}
	_, err := p.database.ExecContext(ctx, `INSERT INTO endpoint_pools (id,region_id,delivery_mode,address,port_start,port_end,stability,active) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (id) DO NOTHING`, pool.ID, pool.RegionID, pool.DeliveryMode, pool.Address, pool.PortStart, pool.PortEnd, pool.Stability, pool.Active)
	return err
}

func (p *Postgres) ReceiveDesired(ctx context.Context, messageID string, desired deliverycontrol.DesiredPayload, receivedAt time.Time) (bool, error) {
	if err := p.validateDesired(desired, receivedAt); err != nil {
		return false, err
	}
	tx, err := p.database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO regional_inbox (message_id,message_type,received_at) VALUES ($1,'deployment.desired.v1',$2) ON CONFLICT (message_id) DO NOTHING`, messageID, receivedAt)
	if err != nil {
		return false, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if inserted == 0 {
		return false, nil
	}
	current, err := stateByInstance(ctx, tx, desired.LogicalInstanceID, true)
	if err == nil && desired.PlacementVersion <= current.PlacementVersion {
		_, err = tx.ExecContext(ctx, `UPDATE regional_inbox SET handled_at=$2 WHERE message_id=$1`, messageID, receivedAt)
		if err != nil {
			return false, err
		}
		return false, tx.Commit()
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if err == nil && current.Phase != PhaseCleaned {
		if err := cleanup(ctx, tx, current, receivedAt); err != nil {
			return false, err
		}
	}
	stateID, err := persistence.NewID("rdp")
	if err != nil {
		return false, err
	}
	configuration, _ := json.Marshal(desired.Configuration)
	listeners, _ := json.Marshal(desired.ListenerRequirements)
	_, err = tx.ExecContext(ctx, `INSERT INTO regional_delivery_states (id,workspace_id,logical_instance_id,region_id,placement_version,instance_revision_id,operation_id,desired_state,provider_release_id,cpu_milli,memory_mib,disk_gib,configuration,listener_requirements,phase,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) ON CONFLICT (logical_instance_id) DO UPDATE SET id=EXCLUDED.id,workspace_id=EXCLUDED.workspace_id,region_id=EXCLUDED.region_id,placement_version=EXCLUDED.placement_version,instance_revision_id=EXCLUDED.instance_revision_id,operation_id=EXCLUDED.operation_id,desired_state=EXCLUDED.desired_state,provider_release_id=EXCLUDED.provider_release_id,cpu_milli=EXCLUDED.cpu_milli,memory_mib=EXCLUDED.memory_mib,disk_gib=EXCLUDED.disk_gib,configuration=EXCLUDED.configuration,listener_requirements=EXCLUDED.listener_requirements,phase=EXCLUDED.phase,node_id=NULL,fencing_token=0,reconcile_owner=NULL,reconcile_lease_until=NULL,assignment_owner=NULL,assignment_lease_until=NULL,assignment_attempts=0,observation_sequence=0,failure_code=NULL,residual_cleanup_required=false,updated_at=EXCLUDED.updated_at`, stateID, desired.WorkspaceID, desired.LogicalInstanceID, desired.RegionID, desired.PlacementVersion, desired.InstanceRevisionID, desired.OperationID, desired.DesiredState, desired.ProviderReleaseID, desired.ResourceSpec.CPUMilli, desired.ResourceSpec.MemoryMiB, desired.ResourceSpec.DiskGiB, configuration, listeners, PhasePending, receivedAt)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE regional_inbox SET handled_at=$2 WHERE message_id=$1`, messageID, receivedAt); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (p *Postgres) ReconcileOne(ctx context.Context, worker string, now time.Time) (bool, error) {
	state, ok, err := p.claimReconcile(ctx, worker, now)
	if err != nil || !ok {
		return ok, err
	}
	switch state.Phase {
	case PhasePending:
		err = p.schedule(ctx, state, worker, now)
	case PhaseScheduled:
		err = p.allocateEndpoints(ctx, state, worker, now)
	case PhaseEndpoints:
		err = p.makeAssignment(ctx, state, worker, now)
	case PhaseReady:
		err = p.publishObservation(ctx, state, worker, now)
	case PhaseFailed:
		err = p.cleanupFailed(ctx, state, worker, now)
	default:
		err = p.releaseReconcile(ctx, state.ID, worker)
	}
	return true, err
}

func (p *Postgres) ClaimAssignment(ctx context.Context, nodeID, worker string, now time.Time) (Assignment, bool, error) {
	tx, err := p.database.BeginTx(ctx, nil)
	if err != nil {
		return Assignment{}, false, err
	}
	defer tx.Rollback()
	state, err := scanState(tx.QueryRowContext(ctx, stateSelect+` WHERE node_id=$1 AND phase=$2 AND (assignment_lease_until IS NULL OR assignment_lease_until <= $3) ORDER BY updated_at,id FOR UPDATE SKIP LOCKED LIMIT 1`, nodeID, PhaseAssigned, now))
	if errors.Is(err, sql.ErrNoRows) {
		return Assignment{}, false, nil
	}
	if err != nil {
		return Assignment{}, false, err
	}
	leaseUntil := now.Add(p.lease)
	state.AssignmentAttempts++
	if _, err := tx.ExecContext(ctx, `UPDATE regional_delivery_states SET assignment_owner=$2,assignment_lease_until=$3,assignment_attempts=$4,updated_at=$5 WHERE id=$1`, state.ID, worker, leaseUntil, state.AssignmentAttempts, now); err != nil {
		return Assignment{}, false, err
	}
	endpoints, err := endpointsFor(ctx, tx, state.ID)
	if err != nil {
		return Assignment{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Assignment{}, false, err
	}
	return Assignment{RegionalDeliveryID: state.ID, LogicalInstanceID: state.LogicalInstanceID, NodeID: state.NodeID, FencingToken: state.FencingToken, DesiredState: state.DesiredState, ProviderReleaseID: state.ProviderReleaseID, ResourceSpec: state.ResourceSpec, Configuration: state.Configuration, Endpoints: endpoints, LeaseUntil: leaseUntil, Attempt: state.AssignmentAttempts}, true, nil
}

func (p *Postgres) CompleteAssignment(ctx context.Context, deliveryID, worker string, fencingToken int64, ready bool, failureCode string, now time.Time) (bool, error) {
	tx, err := p.database.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	state, err := stateByID(ctx, tx, deliveryID, true)
	if err != nil {
		return false, err
	}
	if state.Phase == PhaseReady || state.Phase == PhaseFailed || state.Phase == PhasePublished || state.Phase == PhaseCleaned {
		return false, nil
	}
	if state.FencingToken != fencingToken {
		return false, ErrFencingToken
	}
	if state.Phase != PhaseAssigned || state.AssignmentOwner != worker || !state.AssignmentLeaseUntil.After(now) {
		return false, ErrAssignmentLease
	}
	phase := PhaseReady
	cleanupRequired := false
	if !ready {
		phase, cleanupRequired = PhaseFailed, true
	}
	_, err = tx.ExecContext(ctx, `UPDATE regional_delivery_states SET phase=$2,failure_code=NULLIF($3,''),residual_cleanup_required=$4,assignment_owner=NULL,assignment_lease_until=NULL,updated_at=$5 WHERE id=$1`, deliveryID, phase, failureCode, cleanupRequired, now)
	if err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (p *Postgres) State(ctx context.Context, instanceID string) (State, error) {
	return stateByInstance(ctx, p.database, instanceID, false)
}

func (p *Postgres) claimReconcile(ctx context.Context, worker string, now time.Time) (State, bool, error) {
	tx, err := p.database.BeginTx(ctx, nil)
	if err != nil {
		return State{}, false, err
	}
	defer tx.Rollback()
	state, err := scanState(tx.QueryRowContext(ctx, stateSelect+` WHERE phase = ANY($1) AND (reconcile_lease_until IS NULL OR reconcile_lease_until <= $2) ORDER BY updated_at,id FOR UPDATE SKIP LOCKED LIMIT 1`, []string{PhasePending, PhaseScheduled, PhaseEndpoints, PhaseReady, PhaseFailed}, now))
	if errors.Is(err, sql.ErrNoRows) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, err
	}
	state.ReconcileOwner, state.ReconcileLeaseUntil = worker, now.Add(p.lease)
	if _, err := tx.ExecContext(ctx, `UPDATE regional_delivery_states SET reconcile_owner=$2,reconcile_lease_until=$3 WHERE id=$1`, state.ID, worker, state.ReconcileLeaseUntil); err != nil {
		return State{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return State{}, false, err
	}
	return state, true, nil
}

func (p *Postgres) schedule(ctx context.Context, state State, worker string, now time.Time) error {
	tx, err := p.database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := stateByID(ctx, tx, state.ID, true)
	if err != nil {
		return err
	}
	if current.Phase != PhasePending || current.ReconcileOwner != worker {
		return tx.Commit()
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,region_id,state,cpu_capacity_milli,memory_capacity_mib,disk_capacity_gib,reserved_cpu_milli,reserved_memory_mib,reserved_disk_gib,lease_until,updated_at FROM regional_delivery_nodes WHERE region_id=$1 AND state='ready' AND lease_until>$2 ORDER BY id FOR UPDATE`, p.regionID, now)
	if err != nil {
		return err
	}
	var nodes []Node
	for rows.Next() {
		var node Node
		if err := rows.Scan(&node.ID, &node.RegionID, &node.State, &node.CPUCapacityMilli, &node.MemoryCapacityMiB, &node.DiskCapacityGiB, &node.ReservedCPUMilli, &node.ReservedMemoryMiB, &node.ReservedDiskGiB, &node.LeaseUntil, &node.UpdatedAt); err != nil {
			rows.Close()
			return err
		}
		nodes = append(nodes, node)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	var selected *Node
	var best int64
	for index := range nodes {
		node := &nodes[index]
		freeCPU, freeMemory, freeDisk := node.CPUCapacityMilli-node.ReservedCPUMilli, node.MemoryCapacityMiB-node.ReservedMemoryMiB, node.DiskCapacityGiB-node.ReservedDiskGiB
		if freeCPU < current.ResourceSpec.CPUMilli || freeMemory < current.ResourceSpec.MemoryMiB || freeDisk < current.ResourceSpec.DiskGiB {
			continue
		}
		score := freeCPU - current.ResourceSpec.CPUMilli + freeMemory - current.ResourceSpec.MemoryMiB + (freeDisk-current.ResourceSpec.DiskGiB)*1024
		if selected == nil || score < best {
			selected, best = node, score
		}
	}
	if selected == nil {
		return p.failInTx(ctx, tx, current, "insufficient_capacity", false, now)
	}
	result, err := tx.ExecContext(ctx, `UPDATE regional_delivery_nodes SET reserved_cpu_milli=reserved_cpu_milli+$2,reserved_memory_mib=reserved_memory_mib+$3,reserved_disk_gib=reserved_disk_gib+$4,updated_at=$5 WHERE id=$1 AND reserved_cpu_milli+$2<=cpu_capacity_milli AND reserved_memory_mib+$3<=memory_capacity_mib AND reserved_disk_gib+$4<=disk_capacity_gib`, selected.ID, current.ResourceSpec.CPUMilli, current.ResourceSpec.MemoryMiB, current.ResourceSpec.DiskGiB, now)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return ErrNoCapacity
	}
	if err := tx.QueryRowContext(ctx, `SELECT nextval('regional_delivery_fencing_token_seq')`).Scan(&current.FencingToken); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE regional_delivery_states SET phase=$2,node_id=$3,fencing_token=$4,residual_cleanup_required=true,reconcile_owner=NULL,reconcile_lease_until=NULL,updated_at=$5 WHERE id=$1`, current.ID, PhaseScheduled, selected.ID, current.FencingToken, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (p *Postgres) allocateEndpoints(ctx context.Context, state State, worker string, now time.Time) error {
	tx, err := p.database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := stateByID(ctx, tx, state.ID, true)
	if err != nil {
		return err
	}
	if current.Phase != PhaseScheduled || current.ReconcileOwner != worker {
		return tx.Commit()
	}
	pools, err := loadPools(ctx, tx, p.regionID)
	if err != nil {
		return err
	}
	poolIDs := make([]string, len(pools))
	for index := range pools {
		poolIDs[index] = pools[index].ID
	}
	used, err := loadActiveAllocationKeys(ctx, tx, poolIDs)
	if err != nil {
		return err
	}
	allocations, err := chooseEndpoints(current, pools, used, now)
	if err != nil {
		return p.failInTx(ctx, tx, current, "endpoint_unavailable", true, now)
	}
	payload, _ := json.Marshal(allocations)
	_, err = tx.ExecContext(ctx, `INSERT INTO endpoint_allocations (id,regional_delivery_id,logical_instance_id,pool_id,listener_name,purpose,address,port,transports,stability,display_address,is_primary,allocation_key,active,created_at) SELECT x.id,x."regionalDeliveryId",x."logicalInstanceId",x."poolId",x."listenerName",x.purpose,x.address,x.port,x.transports,x.stability,x."displayAddress",x."isPrimary",x."allocationKey",true,$2 FROM jsonb_to_recordset($1::jsonb) AS x(id text,"regionalDeliveryId" text,"logicalInstanceId" text,"poolId" text,"listenerName" text,purpose text,address text,port integer,transports jsonb,stability text,"displayAddress" text,"isPrimary" boolean,"allocationKey" text)`, payload, now)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE regional_delivery_states SET phase=$2,reconcile_owner=NULL,reconcile_lease_until=NULL,updated_at=$3 WHERE id=$1`, current.ID, PhaseEndpoints, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (p *Postgres) makeAssignment(ctx context.Context, state State, worker string, now time.Time) error {
	result, err := p.database.ExecContext(ctx, `UPDATE regional_delivery_states SET phase=$2,reconcile_owner=NULL,reconcile_lease_until=NULL,updated_at=$3 WHERE id=$1 AND phase=$4 AND reconcile_owner=$5`, state.ID, PhaseAssigned, now, PhaseEndpoints, worker)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return p.releaseReconcile(ctx, state.ID, worker)
	}
	return nil
}

func (p *Postgres) publishObservation(ctx context.Context, state State, worker string, now time.Time) error {
	tx, err := p.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := stateByID(ctx, tx, state.ID, true)
	if err != nil {
		return err
	}
	if current.Phase != PhaseReady || current.ReconcileOwner != worker {
		return tx.Commit()
	}
	endpoints, err := endpointsFor(ctx, tx, current.ID)
	if err != nil {
		return err
	}
	current.ObservationSequence++
	messageID, err := persistence.NewID("msg")
	if err != nil {
		return err
	}
	payload := deliverycontrol.Observation{MessageID: messageID, WorkspaceID: current.WorkspaceID, LogicalInstanceID: current.LogicalInstanceID, RegionalDeploymentID: current.ID, RuntimeAttemptID: fmt.Sprintf("rta_%s_%d", current.ID, current.AssignmentAttempts), RegionID: current.RegionID, PlacementVersion: current.PlacementVersion, Sequence: current.ObservationSequence, ObservedState: "running", EndpointBindings: endpoints, ObservedAt: now}
	message := messaging.OutboxMessage{SchemaVersion: 1, ID: contract.EventID(messageID), MessageType: "deployment.observed.v1", IdempotencyKey: contract.IdempotencyKey(fmt.Sprintf("observe:%s:%d", current.ID, current.ObservationSequence)), Payload: payload, CreatedAt: now}
	if err := messaging.NewPostgres(p.database).InsertOutboxTo(ctx, tx, messaging.RegionOutbox, message); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE regional_delivery_states SET phase=$2,observation_sequence=$3,residual_cleanup_required=false,reconcile_owner=NULL,reconcile_lease_until=NULL,updated_at=$4 WHERE id=$1`, current.ID, PhasePublished, current.ObservationSequence, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (p *Postgres) cleanupFailed(ctx context.Context, state State, worker string, now time.Time) error {
	tx, err := p.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := stateByID(ctx, tx, state.ID, true)
	if err != nil {
		return err
	}
	if current.Phase != PhaseFailed || current.ReconcileOwner != worker {
		return tx.Commit()
	}
	if err := cleanup(ctx, tx, current, now); err != nil {
		return err
	}
	current.ObservationSequence++
	messageID, err := persistence.NewID("msg")
	if err != nil {
		return err
	}
	payload := deliverycontrol.Observation{MessageID: messageID, WorkspaceID: current.WorkspaceID, LogicalInstanceID: current.LogicalInstanceID, RegionalDeploymentID: current.ID, RuntimeAttemptID: fmt.Sprintf("rta_%s_%d", current.ID, current.AssignmentAttempts), RegionID: current.RegionID, PlacementVersion: current.PlacementVersion, Sequence: current.ObservationSequence, ObservedState: "failed", EndpointBindings: []deliverycontrol.EndpointBinding{}, ReasonCode: current.FailureCode, ObservedAt: now}
	message := messaging.OutboxMessage{SchemaVersion: 1, ID: contract.EventID(messageID), MessageType: "deployment.observed.v1", IdempotencyKey: contract.IdempotencyKey(fmt.Sprintf("observe:%s:%d", current.ID, current.ObservationSequence)), Payload: payload, CreatedAt: now}
	if err := messaging.NewPostgres(p.database).InsertOutboxTo(ctx, tx, messaging.RegionOutbox, message); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE regional_delivery_states SET phase=$2,observation_sequence=$3,residual_cleanup_required=false,reconcile_owner=NULL,reconcile_lease_until=NULL,updated_at=$4 WHERE id=$1`, current.ID, PhaseCleaned, current.ObservationSequence, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (p *Postgres) failInTx(ctx context.Context, tx *sql.Tx, state State, code string, cleanupRequired bool, now time.Time) error {
	_, err := tx.ExecContext(ctx, `UPDATE regional_delivery_states SET phase=$2,failure_code=$3,residual_cleanup_required=$4,reconcile_owner=NULL,reconcile_lease_until=NULL,updated_at=$5 WHERE id=$1`, state.ID, PhaseFailed, code, cleanupRequired, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func cleanup(ctx context.Context, tx *sql.Tx, state State, now time.Time) error {
	if state.NodeID != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE regional_delivery_nodes SET reserved_cpu_milli=GREATEST(0,reserved_cpu_milli-$2),reserved_memory_mib=GREATEST(0,reserved_memory_mib-$3),reserved_disk_gib=GREATEST(0,reserved_disk_gib-$4),updated_at=$5 WHERE id=$1`, state.NodeID, state.ResourceSpec.CPUMilli, state.ResourceSpec.MemoryMiB, state.ResourceSpec.DiskGiB, now); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `UPDATE endpoint_allocations SET active=false WHERE regional_delivery_id=$1 AND active=true`, state.ID)
	return err
}

func (p *Postgres) releaseReconcile(ctx context.Context, id, worker string) error {
	_, err := p.database.ExecContext(ctx, `UPDATE regional_delivery_states SET reconcile_owner=NULL,reconcile_lease_until=NULL WHERE id=$1 AND reconcile_owner=$2`, id, worker)
	return err
}

func (p *Postgres) validateDesired(desired deliverycontrol.DesiredPayload, now time.Time) error {
	if desired.RegionID != p.regionID {
		return ErrWrongRegion
	}
	if desired.WorkspaceID == "" || desired.LogicalInstanceID == "" || desired.OperationID == "" || desired.ProviderReleaseID == "" || desired.PlacementVersion < 1 || len(desired.ListenerRequirements) == 0 || !deliverycontrol.VerifyAuthority(desired, p.authorityKey, now) {
		return ErrInvalidDesired
	}
	return nil
}
