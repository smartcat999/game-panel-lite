package regionexecution

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/persistence"
)

type Postgres struct {
	db       *sql.DB
	regionID contract.RegionID
}

func NewPostgres(db *sql.DB, regionID contract.RegionID) *Postgres {
	return &Postgres{db: db, regionID: regionID}
}

func (p *Postgres) RegisterNode(ctx context.Context, node Node) error {
	if node.RegionID != p.regionID {
		return ErrWrongRegion
	}
	games, err := json.Marshal(node.Games)
	if err != nil {
		return err
	}
	_, err = p.db.ExecContext(ctx, `INSERT INTO nodes (id, region_id, name, state, games, cpu_capacity, memory_capacity_mb, reserved_cpu, reserved_memory_mb, lease_until, last_heartbeat_at) VALUES ($1, $2, $3, $4, $5, $6, $7, 0, 0, $8, $9) ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, state = EXCLUDED.state, games = EXCLUDED.games, cpu_capacity = EXCLUDED.cpu_capacity, memory_capacity_mb = EXCLUDED.memory_capacity_mb, lease_until = EXCLUDED.lease_until, last_heartbeat_at = EXCLUDED.last_heartbeat_at`, node.ID, node.RegionID, node.Name, node.State, games, node.CPUCapacity, node.MemoryCapacityMB, node.LeaseUntil, node.LastHeartbeatAt)
	return err
}

func (p *Postgres) RenewLease(ctx context.Context, nodeID contract.NodeID, leaseUntil, heartbeatAt time.Time) error {
	result, err := p.db.ExecContext(ctx, `UPDATE nodes SET lease_until = $1, last_heartbeat_at = $2, state = CASE WHEN state = $3 THEN $4 ELSE state END WHERE id = $5 AND region_id = $6`, leaseUntil, heartbeatAt, NodeStale, NodeReady, nodeID, p.regionID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrNodeNotFound
	}
	return nil
}

func (p *Postgres) ReceiveDesired(ctx context.Context, desired DesiredDeployment, now time.Time) (RegionalDeployment, bool, error) {
	if desired.RegionID != p.regionID {
		return RegionalDeployment{}, false, ErrWrongRegion
	}
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return RegionalDeployment{}, false, err
	}
	defer tx.Rollback()
	configuration, err := json.Marshal(desired.Configuration)
	if err != nil {
		return RegionalDeployment{}, false, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO regional_inbox (message_id, message_type, received_at) VALUES ($1, $2, $3) ON CONFLICT (message_id) DO NOTHING`, desired.MessageID, "deployment.desired.v1", now)
	if err != nil {
		return RegionalDeployment{}, false, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return RegionalDeployment{}, false, err
	}
	if inserted == 0 {
		deployment, err := deploymentByInstance(ctx, tx, desired.LogicalInstanceID, false)
		return deployment, false, err
	}
	deployment, err := deploymentByInstance(ctx, tx, desired.LogicalInstanceID, true)
	if errors.Is(err, sql.ErrNoRows) {
		id, idErr := persistence.NewID("rdp")
		if idErr != nil {
			return RegionalDeployment{}, false, idErr
		}
		deployment = RegionalDeployment{ID: contract.RegionalDeploymentID(id), WorkspaceID: desired.WorkspaceID, LogicalInstanceID: desired.LogicalInstanceID, RegionID: desired.RegionID, PlacementVersion: desired.PlacementVersion, InstanceRevisionID: desired.InstanceRevisionID, DesiredState: desired.DesiredState, ObservedState: ObservedPending, GameKey: desired.GameKey, GameVersion: desired.GameVersion, Configuration: desired.Configuration, CPUUnits: desired.CPUUnits, MemoryMegabytes: desired.MemoryMegabytes, UpdatedAt: now}
		if _, err := tx.ExecContext(ctx, `INSERT INTO regional_deployments (id, workspace_id, logical_instance_id, region_id, placement_version, instance_revision_id, desired_state, observed_state, observation_sequence, game_key, game_version, configuration, cpu_units, memory_megabytes, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 0, $9, $10, $11, $12, $13, $14)`, deployment.ID, deployment.WorkspaceID, deployment.LogicalInstanceID, deployment.RegionID, deployment.PlacementVersion, deployment.InstanceRevisionID, deployment.DesiredState, deployment.ObservedState, deployment.GameKey, deployment.GameVersion, configuration, deployment.CPUUnits, deployment.MemoryMegabytes, deployment.UpdatedAt); err != nil {
			return RegionalDeployment{}, false, err
		}
	} else if err != nil {
		return RegionalDeployment{}, false, err
	} else if desired.PlacementVersion <= deployment.PlacementVersion {
		if _, err := tx.ExecContext(ctx, `UPDATE regional_inbox SET handled_at = $1 WHERE message_id = $2`, now, desired.MessageID); err != nil {
			return RegionalDeployment{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return RegionalDeployment{}, false, err
		}
		return deployment, false, nil
	} else {
		if err := releaseReservation(ctx, tx, deployment.ID); err != nil {
			return RegionalDeployment{}, false, err
		}
		deployment.WorkspaceID, deployment.PlacementVersion, deployment.InstanceRevisionID = desired.WorkspaceID, desired.PlacementVersion, desired.InstanceRevisionID
		deployment.DesiredState, deployment.GameKey, deployment.GameVersion, deployment.Configuration = desired.DesiredState, desired.GameKey, desired.GameVersion, desired.Configuration
		deployment.CPUUnits, deployment.MemoryMegabytes = desired.CPUUnits, desired.MemoryMegabytes
		deployment.NodeID, deployment.UnschedulableReason, deployment.UpdatedAt = "", "", now
		if _, err := tx.ExecContext(ctx, `UPDATE regional_deployments SET workspace_id = $1, placement_version = $2, instance_revision_id = $3, desired_state = $4, game_key = $5, game_version = $6, configuration = $7, cpu_units = $8, memory_megabytes = $9, node_id = NULL, unschedulable_reason = NULL, updated_at = $10 WHERE id = $11`, deployment.WorkspaceID, deployment.PlacementVersion, deployment.InstanceRevisionID, deployment.DesiredState, deployment.GameKey, deployment.GameVersion, configuration, deployment.CPUUnits, deployment.MemoryMegabytes, now, deployment.ID); err != nil {
			return RegionalDeployment{}, false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE regional_inbox SET handled_at = $1 WHERE message_id = $2`, now, desired.MessageID); err != nil {
		return RegionalDeployment{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return RegionalDeployment{}, false, err
	}
	return deployment, true, nil
}

func (p *Postgres) Schedule(ctx context.Context, deploymentID contract.RegionalDeploymentID, now time.Time) (Reservation, error) {
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Reservation{}, err
	}
	defer tx.Rollback()
	deployment, err := deploymentByID(ctx, tx, deploymentID, true)
	if errors.Is(err, sql.ErrNoRows) {
		return Reservation{}, ErrDeploymentNotFound
	}
	if err != nil {
		return Reservation{}, err
	}
	if current, found, err := activeReservation(ctx, tx, deploymentID); err != nil {
		return Reservation{}, err
	} else if found {
		return current, nil
	}
	nodes, err := availableNodes(ctx, tx, p.regionID, now)
	if err != nil {
		return Reservation{}, err
	}
	selected, reason := chooseNode(nodes, deployment)
	if selected.ID == "" {
		if _, err := tx.ExecContext(ctx, `UPDATE regional_deployments SET unschedulable_reason = $1, updated_at = $2 WHERE id = $3`, reason, now, deploymentID); err != nil {
			return Reservation{}, err
		}
		if err := tx.Commit(); err != nil {
			return Reservation{}, err
		}
		return Reservation{}, fmt.Errorf("unschedulable: %s", reason)
	}
	reservation, err := reserve(ctx, tx, deployment, selected, now)
	if err != nil {
		return Reservation{}, err
	}
	if err := tx.Commit(); err != nil {
		return Reservation{}, err
	}
	return reservation, nil
}

func (p *Postgres) Reconcile(ctx context.Context, now time.Time) {
	rows, err := p.db.QueryContext(ctx, `SELECT id FROM regional_deployments WHERE region_id = $1 AND desired_state = 'running' AND node_id IS NULL ORDER BY id LIMIT 100`, p.regionID)
	if err != nil {
		return
	}
	var ids []contract.RegionalDeploymentID
	for rows.Next() {
		var id contract.RegionalDeploymentID
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		_, _ = p.Schedule(ctx, id, now)
	}
}

func (p *Postgres) ApplyObservation(ctx context.Context, observation Observation) (bool, error) {
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO regional_inbox (message_id, message_type, received_at) VALUES ($1, $2, $3) ON CONFLICT (message_id) DO NOTHING`, observation.MessageID, "workload.observed.v1", observation.ObservedAt)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows == 0 {
		return false, nil
	}
	deployment, err := deploymentByID(ctx, tx, observation.RegionalDeploymentID, true)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrDeploymentNotFound
	}
	if err != nil {
		return false, err
	}
	if observation.Sequence > deployment.ObservationSequence {
		if _, err := tx.ExecContext(ctx, `INSERT INTO regional_observations (message_id, regional_deployment_id, sequence, observed_state, observed_at) VALUES ($1, $2, $3, $4, $5)`, observation.MessageID, observation.RegionalDeploymentID, observation.Sequence, observation.State, observation.ObservedAt); err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE regional_deployments SET observed_state = $1, observation_sequence = $2, updated_at = $3 WHERE id = $4`, observation.State, observation.Sequence, observation.ObservedAt, observation.RegionalDeploymentID); err != nil {
			return false, err
		}
		payload, _ := json.Marshal(map[string]any{"logicalInstanceId": deployment.LogicalInstanceID, "regionalDeploymentId": deployment.ID, "regionId": deployment.RegionID, "sequence": observation.Sequence, "observedState": observation.State, "observedAt": observation.ObservedAt})
		if _, err := tx.ExecContext(ctx, `INSERT INTO regional_outbox (id, message_type, schema_version, idempotency_key, payload, created_at) VALUES ($1, $2, 1, $3, $4, $5) ON CONFLICT (message_type, idempotency_key) DO NOTHING`, observation.MessageID, "deployment.observed.v1", observation.MessageID, payload, observation.ObservedAt); err != nil {
			return false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE regional_inbox SET handled_at = $1 WHERE message_id = $2`, observation.ObservedAt, observation.MessageID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return observation.Sequence > deployment.ObservationSequence, nil
}

func (p *Postgres) OverridePlacement(ctx context.Context, actor contract.UserID, deploymentID contract.RegionalDeploymentID, nodeID contract.NodeID, reason string, now time.Time) (Reservation, error) {
	if strings.TrimSpace(reason) == "" {
		return Reservation{}, ErrOverrideReason
	}
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Reservation{}, err
	}
	defer tx.Rollback()
	deployment, err := deploymentByID(ctx, tx, deploymentID, true)
	if errors.Is(err, sql.ErrNoRows) {
		return Reservation{}, ErrDeploymentNotFound
	}
	if err != nil {
		return Reservation{}, err
	}
	current, _, err := activeReservation(ctx, tx, deploymentID)
	if err != nil {
		return Reservation{}, err
	}
	target, err := nodeByID(ctx, tx, nodeID, true)
	if errors.Is(err, sql.ErrNoRows) {
		return Reservation{}, ErrNodeNotFound
	}
	if err != nil {
		return Reservation{}, err
	}
	freeCPU, freeMemory := target.CPUCapacity-target.ReservedCPU, target.MemoryCapacityMB-target.ReservedMemoryMB
	if current.Active && current.NodeID == target.ID {
		freeCPU += current.CPUUnits
		freeMemory += current.MemoryMegabytes
	}
	if target.RegionID != p.regionID || target.State != NodeReady || !target.LeaseUntil.After(now) || !supports(target.Games, deployment.GameKey) || freeCPU < deployment.CPUUnits || freeMemory < deployment.MemoryMegabytes {
		return Reservation{}, ErrOverrideUnavailable
	}
	if err := releaseReservation(ctx, tx, deploymentID); err != nil {
		return Reservation{}, err
	}
	target, err = nodeByID(ctx, tx, nodeID, true)
	if err != nil {
		return Reservation{}, err
	}
	reservation, err := reserve(ctx, tx, deployment, target, now)
	if err != nil {
		return Reservation{}, err
	}
	auditID, err := persistence.NewID("aud")
	if err != nil {
		return Reservation{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO region_audit_records (id, actor_user_id, region_id, regional_deployment_id, previous_node_id, requested_node_id, reason, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, auditID, actor, p.regionID, deploymentID, nullableNode(current.NodeID), nodeID, strings.TrimSpace(reason), now); err != nil {
		return Reservation{}, err
	}
	if err := tx.Commit(); err != nil {
		return Reservation{}, err
	}
	return reservation, nil
}

func (p *Postgres) Nodes(ctx context.Context) []Node {
	rows, err := p.db.QueryContext(ctx, nodeSelect+` WHERE region_id = $1 ORDER BY id LIMIT 100`, p.regionID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []Node
	for rows.Next() {
		node, err := scanNode(rows)
		if err == nil {
			result = append(result, node)
		}
	}
	return result
}
func (p *Postgres) Deployments(ctx context.Context) []RegionalDeployment {
	rows, err := p.db.QueryContext(ctx, deploymentSelect+` WHERE region_id = $1 ORDER BY id LIMIT 100`, p.regionID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []RegionalDeployment
	for rows.Next() {
		deployment, err := scanDeployment(rows)
		if err == nil {
			result = append(result, deployment)
		}
	}
	return result
}
func (p *Postgres) Tasks(ctx context.Context) []RegionalTask {
	rows, err := p.db.QueryContext(ctx, `SELECT id, regional_deployment_id, kind, status, attempts, created_at FROM regional_tasks ORDER BY id LIMIT 100`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []RegionalTask
	for rows.Next() {
		var task RegionalTask
		if rows.Scan(&task.ID, &task.RegionalDeploymentID, &task.Kind, &task.Status, &task.Attempts, &task.CreatedAt) == nil {
			result = append(result, task)
		}
	}
	return result
}
func (p *Postgres) Audits(ctx context.Context) []AuditRecord {
	rows, err := p.db.QueryContext(ctx, `SELECT id, actor_user_id, region_id, regional_deployment_id, COALESCE(previous_node_id, ''), requested_node_id, reason, created_at FROM region_audit_records WHERE region_id = $1 ORDER BY created_at DESC LIMIT 100`, p.regionID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []AuditRecord
	for rows.Next() {
		var item AuditRecord
		if rows.Scan(&item.ID, &item.ActorUserID, &item.RegionID, &item.RegionalDeploymentID, &item.PreviousNodeID, &item.RequestedNodeID, &item.Reason, &item.CreatedAt) == nil {
			result = append(result, item)
		}
	}
	return result
}
func (p *Postgres) Capacity(ctx context.Context) Capacity {
	var result Capacity
	for _, node := range p.Nodes(ctx) {
		result.CPUCapacity += node.CPUCapacity
		result.CPUReserved += node.ReservedCPU
		result.MemoryCapacityMB += node.MemoryCapacityMB
		result.MemoryReservedMB += node.ReservedMemoryMB
	}
	return result
}
func (p *Postgres) Overview(ctx context.Context) Overview {
	result := Overview{RegionID: p.regionID}
	nodes := p.Nodes(ctx)
	deployments := p.Deployments(ctx)
	tasks := p.Tasks(ctx)
	result.DeploymentCount = len(deployments)
	for _, node := range nodes {
		if node.State == NodeReady {
			result.ReadyNodes++
		}
	}
	for _, deployment := range deployments {
		if deployment.UnschedulableReason != "" {
			result.UnschedulableCount++
		}
	}
	for _, task := range tasks {
		if task.Status == "pending" {
			result.PendingTasks++
		}
	}
	return result
}

const deploymentSelect = `SELECT id, workspace_id, logical_instance_id, region_id, placement_version, instance_revision_id, desired_state, observed_state, observation_sequence, game_key, game_version, configuration, cpu_units, memory_megabytes, COALESCE(node_id, ''), COALESCE(unschedulable_reason, ''), updated_at FROM regional_deployments`
const nodeSelect = `SELECT id, region_id, name, state, games, cpu_capacity, memory_capacity_mb, reserved_cpu, reserved_memory_mb, lease_until, last_heartbeat_at FROM nodes`

func deploymentByID(ctx context.Context, query persistence.DBTX, id contract.RegionalDeploymentID, lock bool) (RegionalDeployment, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	return scanDeployment(query.QueryRowContext(ctx, deploymentSelect+` WHERE id = $1`+suffix, id))
}
func deploymentByInstance(ctx context.Context, query persistence.DBTX, id contract.LogicalInstanceID, lock bool) (RegionalDeployment, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	return scanDeployment(query.QueryRowContext(ctx, deploymentSelect+` WHERE logical_instance_id = $1`+suffix, id))
}

type scanner interface{ Scan(...any) error }

func scanDeployment(row scanner) (RegionalDeployment, error) {
	var item RegionalDeployment
	var configuration []byte
	err := row.Scan(&item.ID, &item.WorkspaceID, &item.LogicalInstanceID, &item.RegionID, &item.PlacementVersion, &item.InstanceRevisionID, &item.DesiredState, &item.ObservedState, &item.ObservationSequence, &item.GameKey, &item.GameVersion, &configuration, &item.CPUUnits, &item.MemoryMegabytes, &item.NodeID, &item.UnschedulableReason, &item.UpdatedAt)
	if err == nil {
		err = json.Unmarshal(configuration, &item.Configuration)
	}
	return item, err
}
func scanNode(row scanner) (Node, error) {
	var item Node
	var games []byte
	err := row.Scan(&item.ID, &item.RegionID, &item.Name, &item.State, &games, &item.CPUCapacity, &item.MemoryCapacityMB, &item.ReservedCPU, &item.ReservedMemoryMB, &item.LeaseUntil, &item.LastHeartbeatAt)
	if err == nil {
		err = json.Unmarshal(games, &item.Games)
	}
	return item, err
}
func nodeByID(ctx context.Context, query persistence.DBTX, id contract.NodeID, lock bool) (Node, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	return scanNode(query.QueryRowContext(ctx, nodeSelect+` WHERE id = $1`+suffix, id))
}
func availableNodes(ctx context.Context, query persistence.DBTX, regionID contract.RegionID, now time.Time) ([]Node, error) {
	rows, err := query.QueryContext(ctx, nodeSelect+` WHERE region_id = $1 AND state = $2 AND lease_until > $3 ORDER BY id LIMIT 100 FOR UPDATE`, regionID, NodeReady, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Node
	for rows.Next() {
		item, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
func chooseNode(nodes []Node, deployment RegionalDeployment) (Node, UnschedulableReason) {
	ready, compatible := len(nodes), 0
	var selected Node
	best := int(^uint(0) >> 1)
	for _, node := range nodes {
		if !supports(node.Games, deployment.GameKey) {
			continue
		}
		compatible++
		freeCPU, freeMemory := node.CPUCapacity-node.ReservedCPU, node.MemoryCapacityMB-node.ReservedMemoryMB
		if freeCPU < deployment.CPUUnits || freeMemory < deployment.MemoryMegabytes {
			continue
		}
		score := freeCPU - deployment.CPUUnits + freeMemory - deployment.MemoryMegabytes
		if selected.ID == "" || score < best || score == best && node.ID < selected.ID {
			selected, best = node, score
		}
	}
	if selected.ID != "" {
		return selected, ""
	}
	if ready == 0 {
		return Node{}, UnschedulableNoReadyNodes
	}
	if compatible == 0 {
		return Node{}, UnschedulableIncompatibleGame
	}
	return Node{}, UnschedulableInsufficientCapacity
}
func activeReservation(ctx context.Context, query persistence.DBTX, deploymentID contract.RegionalDeploymentID) (Reservation, bool, error) {
	var item Reservation
	err := query.QueryRowContext(ctx, `SELECT id, regional_deployment_id, node_id, cpu_units, memory_megabytes, fencing_token, active, created_at FROM reservations WHERE regional_deployment_id = $1 AND active = true LIMIT 1 FOR UPDATE`, deploymentID).Scan(&item.ID, &item.RegionalDeploymentID, &item.NodeID, &item.CPUUnits, &item.MemoryMegabytes, &item.FencingToken, &item.Active, &item.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Reservation{}, false, nil
	}
	return item, err == nil, err
}
func reserve(ctx context.Context, query persistence.DBTX, deployment RegionalDeployment, node Node, now time.Time) (Reservation, error) {
	id, err := persistence.NewID("rsv")
	if err != nil {
		return Reservation{}, err
	}
	taskID, err := persistence.NewID("rtk")
	if err != nil {
		return Reservation{}, err
	}
	assignmentID, err := persistence.NewID("was")
	if err != nil {
		return Reservation{}, err
	}
	var token int64
	if err := query.QueryRowContext(ctx, `SELECT nextval('reservation_fencing_token_seq')`).Scan(&token); err != nil {
		return Reservation{}, err
	}
	reservation := Reservation{ID: contract.ReservationID(id), RegionalDeploymentID: deployment.ID, NodeID: node.ID, CPUUnits: deployment.CPUUnits, MemoryMegabytes: deployment.MemoryMegabytes, FencingToken: token, Active: true, CreatedAt: now}
	if _, err := query.ExecContext(ctx, `UPDATE nodes SET reserved_cpu = reserved_cpu + $1, reserved_memory_mb = reserved_memory_mb + $2 WHERE id = $3`, deployment.CPUUnits, deployment.MemoryMegabytes, node.ID); err != nil {
		return Reservation{}, err
	}
	if _, err := query.ExecContext(ctx, `INSERT INTO reservations (id, regional_deployment_id, node_id, cpu_units, memory_megabytes, fencing_token, active, created_at) VALUES ($1, $2, $3, $4, $5, $6, true, $7)`, reservation.ID, reservation.RegionalDeploymentID, reservation.NodeID, reservation.CPUUnits, reservation.MemoryMegabytes, reservation.FencingToken, reservation.CreatedAt); err != nil {
		return Reservation{}, err
	}
	if _, err := query.ExecContext(ctx, `UPDATE regional_deployments SET node_id = $1, unschedulable_reason = NULL, updated_at = $2 WHERE id = $3`, node.ID, now, deployment.ID); err != nil {
		return Reservation{}, err
	}
	if _, err := query.ExecContext(ctx, `INSERT INTO regional_tasks (id, regional_deployment_id, kind, status, attempts, created_at) VALUES ($1, $2, $3, $4, 0, $5)`, taskID, deployment.ID, "reconcile_workload", "pending", now); err != nil {
		return Reservation{}, err
	}
	payload, err := json.Marshal(workloadPayload(deployment))
	if err != nil {
		return Reservation{}, err
	}
	if _, err := query.ExecContext(ctx, `INSERT INTO work_assignments (id, regional_task_id, regional_deployment_id, node_id, action, payload, fencing_token, status, attempts, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 0, $9)`, assignmentID, taskID, deployment.ID, node.ID, "reconcile_workload", payload, token, AssignmentAvailable, now); err != nil {
		return Reservation{}, err
	}
	return reservation, nil
}
func releaseReservation(ctx context.Context, query persistence.DBTX, deploymentID contract.RegionalDeploymentID) error {
	reservation, found, err := activeReservation(ctx, query, deploymentID)
	if err != nil || !found {
		return err
	}
	if _, err := query.ExecContext(ctx, `UPDATE nodes SET reserved_cpu = reserved_cpu - $1, reserved_memory_mb = reserved_memory_mb - $2 WHERE id = $3`, reservation.CPUUnits, reservation.MemoryMegabytes, reservation.NodeID); err != nil {
		return err
	}
	_, err = query.ExecContext(ctx, `UPDATE reservations SET active = false WHERE id = $1`, reservation.ID)
	if err == nil {
		_, err = query.ExecContext(ctx, `UPDATE work_assignments SET status = $1 WHERE regional_deployment_id = $2 AND status IN ($3, $4)`, AssignmentCancelled, deploymentID, AssignmentAvailable, AssignmentClaimed)
	}
	return err
}
func nullableNode(id contract.NodeID) any {
	if id == "" {
		return nil
	}
	return id
}
