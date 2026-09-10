package regionexecution

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
)

var (
	ErrWrongRegion         = errors.New("deployment belongs to another region")
	ErrDeploymentNotFound  = errors.New("regional deployment not found")
	ErrNodeNotFound        = errors.New("node not found")
	ErrOverrideReason      = errors.New("placement override reason is required")
	ErrOverrideUnavailable = errors.New("requested node cannot host deployment")
)

type NodeState string

const (
	NodeReady    NodeState = "ready"
	NodeDraining NodeState = "draining"
	NodeStale    NodeState = "stale"
)

type ObservedState string

const (
	ObservedPending ObservedState = "pending"
	ObservedRunning ObservedState = "running"
	ObservedStopped ObservedState = "stopped"
	ObservedFailed  ObservedState = "failed"
	ObservedUnknown ObservedState = "unknown"
)

type UnschedulableReason string

const (
	UnschedulableNoReadyNodes         UnschedulableReason = "no_ready_nodes"
	UnschedulableIncompatibleGame     UnschedulableReason = "incompatible_game"
	UnschedulableInsufficientCapacity UnschedulableReason = "insufficient_capacity"
)

type Node struct {
	ID               contract.NodeID   `json:"id"`
	RegionID         contract.RegionID `json:"regionId"`
	Name             string            `json:"name"`
	State            NodeState         `json:"state"`
	Games            []string          `json:"games"`
	CPUCapacity      int               `json:"cpuCapacity"`
	MemoryCapacityMB int               `json:"memoryCapacityMb"`
	ReservedCPU      int               `json:"reservedCpu"`
	ReservedMemoryMB int               `json:"reservedMemoryMb"`
	LeaseUntil       time.Time         `json:"leaseUntil"`
	LastHeartbeatAt  time.Time         `json:"lastHeartbeatAt"`
}

type DesiredDeployment struct {
	MessageID          contract.EventID
	WorkspaceID        contract.WorkspaceID
	LogicalInstanceID  contract.LogicalInstanceID
	RegionID           contract.RegionID
	PlacementVersion   int
	InstanceRevisionID contract.InstanceRevisionID
	DesiredState       string
	GameKey            string
	CPUUnits           int
	MemoryMegabytes    int
}

type RegionalDeployment struct {
	ID                  contract.RegionalDeploymentID `json:"id"`
	WorkspaceID         contract.WorkspaceID          `json:"workspaceId"`
	LogicalInstanceID   contract.LogicalInstanceID    `json:"logicalInstanceId"`
	RegionID            contract.RegionID             `json:"regionId"`
	PlacementVersion    int                           `json:"placementVersion"`
	InstanceRevisionID  contract.InstanceRevisionID   `json:"instanceRevisionId"`
	DesiredState        string                        `json:"desiredState"`
	ObservedState       ObservedState                 `json:"observedState"`
	ObservationSequence int64                         `json:"observationSequence"`
	GameKey             string                        `json:"gameKey"`
	CPUUnits            int                           `json:"cpuUnits"`
	MemoryMegabytes     int                           `json:"memoryMegabytes"`
	NodeID              contract.NodeID               `json:"nodeId,omitempty"`
	UnschedulableReason UnschedulableReason           `json:"unschedulableReason,omitempty"`
	UpdatedAt           time.Time                     `json:"updatedAt"`
}

type Reservation struct {
	ID                   contract.ReservationID        `json:"id"`
	RegionalDeploymentID contract.RegionalDeploymentID `json:"regionalDeploymentId"`
	NodeID               contract.NodeID               `json:"nodeId"`
	CPUUnits             int                           `json:"cpuUnits"`
	MemoryMegabytes      int                           `json:"memoryMegabytes"`
	FencingToken         int64                         `json:"fencingToken"`
	Active               bool                          `json:"active"`
	CreatedAt            time.Time                     `json:"createdAt"`
}

type RegionalTask struct {
	ID                   contract.RegionalTaskID       `json:"id"`
	RegionalDeploymentID contract.RegionalDeploymentID `json:"regionalDeploymentId"`
	Kind                 string                        `json:"kind"`
	Status               string                        `json:"status"`
	Attempts             int                           `json:"attempts"`
	CreatedAt            time.Time                     `json:"createdAt"`
}

type Observation struct {
	MessageID            contract.EventID
	RegionalDeploymentID contract.RegionalDeploymentID
	Sequence             int64
	State                ObservedState
	ObservedAt           time.Time
}

type AuditRecord struct {
	ID                   contract.AuditRecordID        `json:"id"`
	ActorUserID          contract.UserID               `json:"actorUserId"`
	RegionID             contract.RegionID             `json:"regionId"`
	RegionalDeploymentID contract.RegionalDeploymentID `json:"regionalDeploymentId"`
	PreviousNodeID       contract.NodeID               `json:"previousNodeId,omitempty"`
	RequestedNodeID      contract.NodeID               `json:"requestedNodeId"`
	Reason               string                        `json:"reason"`
	CreatedAt            time.Time                     `json:"createdAt"`
}

type Overview struct {
	RegionID           contract.RegionID `json:"regionId"`
	ReadyNodes         int               `json:"readyNodes"`
	DeploymentCount    int               `json:"deploymentCount"`
	PendingTasks       int               `json:"pendingTasks"`
	UnschedulableCount int               `json:"unschedulableCount"`
}

type Capacity struct {
	CPUCapacity      int `json:"cpuCapacity"`
	CPUReserved      int `json:"cpuReserved"`
	MemoryCapacityMB int `json:"memoryCapacityMb"`
	MemoryReservedMB int `json:"memoryReservedMb"`
}

type Module struct {
	mu           sync.RWMutex
	regionID     contract.RegionID
	nodes        map[contract.NodeID]Node
	deployments  map[contract.RegionalDeploymentID]RegionalDeployment
	byInstance   map[contract.LogicalInstanceID]contract.RegionalDeploymentID
	reservations map[contract.RegionalDeploymentID]Reservation
	tasks        map[contract.RegionalTaskID]RegionalTask
	inbox        map[contract.EventID]bool
	outbox       map[contract.EventID]Observation
	audit        []AuditRecord
	nextID       int64
}

func New(regionID contract.RegionID, nodes []Node) *Module {
	m := &Module{regionID: regionID, nodes: make(map[contract.NodeID]Node), deployments: make(map[contract.RegionalDeploymentID]RegionalDeployment), byInstance: make(map[contract.LogicalInstanceID]contract.RegionalDeploymentID), reservations: make(map[contract.RegionalDeploymentID]Reservation), tasks: make(map[contract.RegionalTaskID]RegionalTask), inbox: make(map[contract.EventID]bool), outbox: make(map[contract.EventID]Observation)}
	for _, node := range nodes {
		node.Games = append([]string(nil), node.Games...)
		m.nodes[node.ID] = node
	}
	return m
}

func (m *Module) RegisterNode(_ context.Context, node Node) error {
	if node.RegionID != m.regionID {
		return ErrWrongRegion
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	node.Games = append([]string(nil), node.Games...)
	if current, ok := m.nodes[node.ID]; ok {
		node.ReservedCPU = current.ReservedCPU
		node.ReservedMemoryMB = current.ReservedMemoryMB
	}
	m.nodes[node.ID] = node
	return nil
}

func (m *Module) RenewLease(_ context.Context, nodeID contract.NodeID, leaseUntil, heartbeatAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	node, ok := m.nodes[nodeID]
	if !ok {
		return ErrNodeNotFound
	}
	node.LeaseUntil, node.LastHeartbeatAt = leaseUntil, heartbeatAt
	if node.State == NodeStale {
		node.State = NodeReady
	}
	m.nodes[nodeID] = node
	return nil
}

func (m *Module) ReceiveDesired(_ context.Context, desired DesiredDeployment, now time.Time) (RegionalDeployment, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if desired.RegionID != m.regionID {
		return RegionalDeployment{}, false, ErrWrongRegion
	}
	if m.inbox[desired.MessageID] {
		deployment := m.deployments[m.byInstance[desired.LogicalInstanceID]]
		return deployment, false, nil
	}
	m.inbox[desired.MessageID] = true
	if deploymentID, ok := m.byInstance[desired.LogicalInstanceID]; ok {
		deployment := m.deployments[deploymentID]
		if desired.PlacementVersion <= deployment.PlacementVersion {
			return deployment, false, nil
		}
		m.releaseReservationLocked(deployment.ID)
		deployment.WorkspaceID = desired.WorkspaceID
		deployment.PlacementVersion = desired.PlacementVersion
		deployment.InstanceRevisionID = desired.InstanceRevisionID
		deployment.DesiredState = desired.DesiredState
		deployment.GameKey = desired.GameKey
		deployment.CPUUnits = desired.CPUUnits
		deployment.MemoryMegabytes = desired.MemoryMegabytes
		deployment.NodeID = ""
		deployment.UnschedulableReason = ""
		deployment.UpdatedAt = now
		m.deployments[deployment.ID] = deployment
		return deployment, true, nil
	}
	deployment := RegionalDeployment{ID: contract.RegionalDeploymentID(m.next("rdp")), WorkspaceID: desired.WorkspaceID, LogicalInstanceID: desired.LogicalInstanceID, RegionID: desired.RegionID, PlacementVersion: desired.PlacementVersion, InstanceRevisionID: desired.InstanceRevisionID, DesiredState: desired.DesiredState, ObservedState: ObservedPending, GameKey: desired.GameKey, CPUUnits: desired.CPUUnits, MemoryMegabytes: desired.MemoryMegabytes, UpdatedAt: now}
	m.deployments[deployment.ID] = deployment
	m.byInstance[deployment.LogicalInstanceID] = deployment.ID
	return deployment, true, nil
}

func (m *Module) Reconcile(_ context.Context, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(m.deployments))
	for id := range m.deployments {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	for _, rawID := range ids {
		deployment := m.deployments[contract.RegionalDeploymentID(rawID)]
		if deployment.DesiredState != "running" || m.reservations[deployment.ID].Active {
			continue
		}
		m.scheduleLocked(deployment.ID, now)
	}
}

func (m *Module) Schedule(_ context.Context, deploymentID contract.RegionalDeploymentID, now time.Time) (Reservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.scheduleLocked(deploymentID, now)
}

func (m *Module) scheduleLocked(deploymentID contract.RegionalDeploymentID, now time.Time) (Reservation, error) {
	deployment, ok := m.deployments[deploymentID]
	if !ok {
		return Reservation{}, ErrDeploymentNotFound
	}
	if current := m.reservations[deploymentID]; current.Active {
		return current, nil
	}
	ready, compatible, capable := 0, 0, 0
	var selected Node
	bestScore := int(^uint(0) >> 1)
	for _, node := range m.nodes {
		if node.State != NodeReady || !node.LeaseUntil.After(now) {
			continue
		}
		ready++
		if !supports(node.Games, deployment.GameKey) {
			continue
		}
		compatible++
		freeCPU := node.CPUCapacity - node.ReservedCPU
		freeMemory := node.MemoryCapacityMB - node.ReservedMemoryMB
		if freeCPU < deployment.CPUUnits || freeMemory < deployment.MemoryMegabytes {
			continue
		}
		capable++
		score := freeCPU - deployment.CPUUnits + freeMemory - deployment.MemoryMegabytes
		if selected.ID == "" || score < bestScore || score == bestScore && node.ID < selected.ID {
			selected, bestScore = node, score
		}
	}
	if capable == 0 {
		switch {
		case ready == 0:
			deployment.UnschedulableReason = UnschedulableNoReadyNodes
		case compatible == 0:
			deployment.UnschedulableReason = UnschedulableIncompatibleGame
		default:
			deployment.UnschedulableReason = UnschedulableInsufficientCapacity
		}
		deployment.UpdatedAt = now
		m.deployments[deployment.ID] = deployment
		return Reservation{}, fmt.Errorf("unschedulable: %s", deployment.UnschedulableReason)
	}
	selected.ReservedCPU += deployment.CPUUnits
	selected.ReservedMemoryMB += deployment.MemoryMegabytes
	m.nodes[selected.ID] = selected
	reservation := Reservation{ID: contract.ReservationID(m.next("rsv")), RegionalDeploymentID: deployment.ID, NodeID: selected.ID, CPUUnits: deployment.CPUUnits, MemoryMegabytes: deployment.MemoryMegabytes, FencingToken: m.nextID, Active: true, CreatedAt: now}
	m.reservations[deployment.ID] = reservation
	deployment.NodeID = selected.ID
	deployment.UnschedulableReason = ""
	deployment.UpdatedAt = now
	m.deployments[deployment.ID] = deployment
	task := RegionalTask{ID: contract.RegionalTaskID(m.next("rtk")), RegionalDeploymentID: deployment.ID, Kind: "reconcile_workload", Status: "pending", CreatedAt: now}
	m.tasks[task.ID] = task
	return reservation, nil
}

func (m *Module) ApplyObservation(_ context.Context, observation Observation) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inbox[observation.MessageID] {
		return false, nil
	}
	m.inbox[observation.MessageID] = true
	deployment, ok := m.deployments[observation.RegionalDeploymentID]
	if !ok {
		return false, ErrDeploymentNotFound
	}
	if observation.Sequence <= deployment.ObservationSequence {
		return false, nil
	}
	deployment.ObservationSequence = observation.Sequence
	deployment.ObservedState = observation.State
	deployment.UpdatedAt = observation.ObservedAt
	m.deployments[deployment.ID] = deployment
	m.outbox[observation.MessageID] = observation
	return true, nil
}

func (m *Module) OverridePlacement(_ context.Context, actor contract.UserID, deploymentID contract.RegionalDeploymentID, nodeID contract.NodeID, reason string, now time.Time) (Reservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if strings.TrimSpace(reason) == "" {
		return Reservation{}, ErrOverrideReason
	}
	deployment, ok := m.deployments[deploymentID]
	if !ok {
		return Reservation{}, ErrDeploymentNotFound
	}
	target, ok := m.nodes[nodeID]
	if !ok {
		return Reservation{}, ErrNodeNotFound
	}
	current := m.reservations[deploymentID]
	freeCPU := target.CPUCapacity - target.ReservedCPU
	freeMemory := target.MemoryCapacityMB - target.ReservedMemoryMB
	if current.Active && current.NodeID == target.ID {
		freeCPU += current.CPUUnits
		freeMemory += current.MemoryMegabytes
	}
	if target.State != NodeReady || !target.LeaseUntil.After(now) || !supports(target.Games, deployment.GameKey) || freeCPU < deployment.CPUUnits || freeMemory < deployment.MemoryMegabytes {
		return Reservation{}, ErrOverrideUnavailable
	}
	previousNodeID := current.NodeID
	m.releaseReservationLocked(deploymentID)
	target = m.nodes[nodeID]
	target.ReservedCPU += deployment.CPUUnits
	target.ReservedMemoryMB += deployment.MemoryMegabytes
	m.nodes[nodeID] = target
	reservation := Reservation{ID: contract.ReservationID(m.next("rsv")), RegionalDeploymentID: deploymentID, NodeID: nodeID, CPUUnits: deployment.CPUUnits, MemoryMegabytes: deployment.MemoryMegabytes, FencingToken: m.nextID, Active: true, CreatedAt: now}
	m.reservations[deploymentID] = reservation
	deployment.NodeID = nodeID
	deployment.UnschedulableReason = ""
	deployment.UpdatedAt = now
	m.deployments[deploymentID] = deployment
	m.audit = append(m.audit, AuditRecord{ID: contract.AuditRecordID(m.next("aud")), ActorUserID: actor, RegionID: m.regionID, RegionalDeploymentID: deploymentID, PreviousNodeID: previousNodeID, RequestedNodeID: nodeID, Reason: strings.TrimSpace(reason), CreatedAt: now})
	return reservation, nil
}

func (m *Module) releaseReservationLocked(deploymentID contract.RegionalDeploymentID) {
	reservation := m.reservations[deploymentID]
	if !reservation.Active {
		return
	}
	node := m.nodes[reservation.NodeID]
	node.ReservedCPU -= reservation.CPUUnits
	node.ReservedMemoryMB -= reservation.MemoryMegabytes
	m.nodes[node.ID] = node
	reservation.Active = false
	m.reservations[deploymentID] = reservation
}

func (m *Module) Overview(_ context.Context) Overview {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := Overview{RegionID: m.regionID, DeploymentCount: len(m.deployments)}
	for _, node := range m.nodes {
		if node.State == NodeReady {
			result.ReadyNodes++
		}
	}
	for _, task := range m.tasks {
		if task.Status == "pending" {
			result.PendingTasks++
		}
	}
	for _, deployment := range m.deployments {
		if deployment.UnschedulableReason != "" {
			result.UnschedulableCount++
		}
	}
	return result
}

func (m *Module) Nodes(_ context.Context) []Node {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Node, 0, len(m.nodes))
	for _, item := range m.nodes {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
func (m *Module) Deployments(_ context.Context) []RegionalDeployment {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]RegionalDeployment, 0, len(m.deployments))
	for _, item := range m.deployments {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
func (m *Module) Tasks(_ context.Context) []RegionalTask {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]RegionalTask, 0, len(m.tasks))
	for _, item := range m.tasks {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
func (m *Module) Audits(_ context.Context) []AuditRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]AuditRecord(nil), m.audit...)
}
func (m *Module) Capacity(_ context.Context) Capacity {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result Capacity
	for _, node := range m.nodes {
		result.CPUCapacity += node.CPUCapacity
		result.CPUReserved += node.ReservedCPU
		result.MemoryCapacityMB += node.MemoryCapacityMB
		result.MemoryReservedMB += node.ReservedMemoryMB
	}
	return result
}
func (m *Module) OutboxCount() int { m.mu.RLock(); defer m.mu.RUnlock(); return len(m.outbox) }

func (m *Module) next(prefix string) string {
	m.nextID++
	return fmt.Sprintf("%s_%06d", prefix, m.nextID)
}
func supports(games []string, game string) bool {
	for _, candidate := range games {
		if candidate == game {
			return true
		}
	}
	return false
}
