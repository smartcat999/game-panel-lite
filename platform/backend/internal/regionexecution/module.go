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

type AssignmentStatus string

const (
	AssignmentAvailable AssignmentStatus = "available"
	AssignmentClaimed   AssignmentStatus = "claimed"
	AssignmentSucceeded AssignmentStatus = "succeeded"
	AssignmentFailed    AssignmentStatus = "failed"
	AssignmentCancelled AssignmentStatus = "cancelled"
)

type WorkAssignment struct {
	ID                   contract.WorkAssignmentID     `json:"id"`
	RegionalTaskID       contract.RegionalTaskID       `json:"regionalTaskId"`
	RegionalDeploymentID contract.RegionalDeploymentID `json:"regionalDeploymentId"`
	NodeID               contract.NodeID               `json:"nodeId"`
	Action               string                        `json:"action"`
	Payload              map[string]string             `json:"payload"`
	FencingToken         int64                         `json:"fencingToken"`
	Status               AssignmentStatus              `json:"status"`
	ClaimedBy            contract.NodeID               `json:"claimedBy,omitempty"`
	ClaimLeaseUntil      time.Time                     `json:"claimLeaseUntil,omitempty"`
	Attempts             int                           `json:"attempts"`
	CreatedAt            time.Time                     `json:"createdAt"`
	CompletedAt          *time.Time                    `json:"completedAt,omitempty"`
}

type Observation struct {
	MessageID            contract.EventID
	RegionalDeploymentID contract.RegionalDeploymentID
	Sequence             int64
	State                ObservedState
	ObservedAt           time.Time
}

type BackupRequested struct {
	MessageID         contract.EventID
	BackupRequestID   contract.BackupRequestID
	LogicalInstanceID contract.LogicalInstanceID
	RegionID          contract.RegionID
	Kind              string
	ObjectKey         string
	TransferURL       string
	RelativePath      string
}

type BackupResult struct {
	MessageID         contract.EventID           `json:"messageId"`
	BackupRequestID   contract.BackupRequestID   `json:"backupRequestId"`
	LogicalInstanceID contract.LogicalInstanceID `json:"logicalInstanceId"`
	RegionID          contract.RegionID          `json:"regionId"`
	Sequence          int64                      `json:"sequence"`
	Status            string                     `json:"status"`
	ObjectKey         string                     `json:"objectKey,omitempty"`
	SizeBytes         int64                      `json:"sizeBytes,omitempty"`
	Checksum          string                     `json:"checksum,omitempty"`
	ObservedAt        time.Time                  `json:"observedAt"`
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

type Monitoring struct {
	InboxLag                int   `json:"inboxLag"`
	OutboxLag               int   `json:"outboxLag"`
	StaleNodes              int   `json:"staleNodes"`
	TaskLatencyMilliseconds int64 `json:"taskLatencyMilliseconds"`
	ReconciliationFailures  int   `json:"reconciliationFailures"`
}

type Module struct {
	mu            sync.RWMutex
	regionID      contract.RegionID
	nodes         map[contract.NodeID]Node
	deployments   map[contract.RegionalDeploymentID]RegionalDeployment
	byInstance    map[contract.LogicalInstanceID]contract.RegionalDeploymentID
	reservations  map[contract.RegionalDeploymentID]Reservation
	tasks         map[contract.RegionalTaskID]RegionalTask
	assignments   map[contract.WorkAssignmentID]WorkAssignment
	inbox         map[contract.EventID]bool
	outbox        map[contract.EventID]Observation
	backupResults map[contract.BackupRequestID]BackupResult
	audit         []AuditRecord
	nextID        int64
}

func New(regionID contract.RegionID, nodes []Node) *Module {
	m := &Module{regionID: regionID, nodes: make(map[contract.NodeID]Node), deployments: make(map[contract.RegionalDeploymentID]RegionalDeployment), byInstance: make(map[contract.LogicalInstanceID]contract.RegionalDeploymentID), reservations: make(map[contract.RegionalDeploymentID]Reservation), tasks: make(map[contract.RegionalTaskID]RegionalTask), assignments: make(map[contract.WorkAssignmentID]WorkAssignment), inbox: make(map[contract.EventID]bool), outbox: make(map[contract.EventID]Observation), backupResults: make(map[contract.BackupRequestID]BackupResult)}
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
	m.createAssignmentLocked(task, reservation, "reconcile_workload", nil, now)
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

func (m *Module) ReceiveBackup(_ context.Context, requested BackupRequested, now time.Time) (WorkAssignment, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if requested.RegionID != m.regionID {
		return WorkAssignment{}, false, ErrWrongRegion
	}
	if m.inbox[requested.MessageID] {
		for _, assignment := range m.assignments {
			if assignment.Payload["backupRequestId"] == string(requested.BackupRequestID) {
				return assignment, false, nil
			}
		}
		return WorkAssignment{}, false, nil
	}
	deploymentID, ok := m.byInstance[requested.LogicalInstanceID]
	if !ok {
		return WorkAssignment{}, false, ErrDeploymentNotFound
	}
	reservation, ok := m.reservations[deploymentID]
	if !ok || !reservation.Active {
		return WorkAssignment{}, false, ErrOverrideUnavailable
	}
	if requested.Kind != "backup" && requested.Kind != "restore" {
		return WorkAssignment{}, false, errors.New("invalid backup task kind")
	}
	task := RegionalTask{ID: contract.RegionalTaskID(m.next("rtk")), RegionalDeploymentID: deploymentID, Kind: requested.Kind, Status: "pending", CreatedAt: now}
	assignment := WorkAssignment{ID: contract.WorkAssignmentID(m.next("was")), RegionalTaskID: task.ID, RegionalDeploymentID: deploymentID, NodeID: reservation.NodeID, Action: requested.Kind, Payload: map[string]string{"backupRequestId": string(requested.BackupRequestID), "objectKey": requested.ObjectKey, "transferUrl": requested.TransferURL, "relativePath": requested.RelativePath}, FencingToken: reservation.FencingToken, Status: AssignmentAvailable, CreatedAt: now}
	m.tasks[task.ID], m.assignments[assignment.ID], m.inbox[requested.MessageID] = task, assignment, true
	return assignment, true, nil
}

func (m *Module) RecordBackupResult(_ context.Context, assignment WorkAssignment, result BackupResult) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.assignments[assignment.ID]
	reservation, active := m.reservations[assignment.RegionalDeploymentID]
	node := m.nodes[assignment.NodeID]
	if !ok || current.Status != AssignmentClaimed || current.ClaimedBy != assignment.NodeID || !active || !reservation.Active || reservation.FencingToken != assignment.FencingToken || node.State != NodeReady || !node.LeaseUntil.After(result.ObservedAt) {
		return false, errors.New("stale backup result")
	}
	requestID := contract.BackupRequestID(assignment.Payload["backupRequestId"])
	if requestID == "" || result.BackupRequestID != requestID {
		return false, errors.New("backup result does not match assignment")
	}
	if previous, exists := m.backupResults[requestID]; exists && previous.Sequence >= result.Sequence {
		return false, nil
	}
	result.LogicalInstanceID = m.deployments[assignment.RegionalDeploymentID].LogicalInstanceID
	result.RegionID = m.regionID
	m.backupResults[requestID] = result
	return true, nil
}

func (m *Module) BackupResults(_ context.Context) []BackupResult {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]BackupResult, 0, len(m.backupResults))
	for _, item := range m.backupResults {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].BackupRequestID < result[j].BackupRequestID })
	return result
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
	task := RegionalTask{ID: contract.RegionalTaskID(m.next("rtk")), RegionalDeploymentID: deployment.ID, Kind: "reconcile_workload", Status: "pending", CreatedAt: now}
	m.tasks[task.ID] = task
	m.createAssignmentLocked(task, reservation, "reconcile_workload", nil, now)
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
	for id, assignment := range m.assignments {
		if assignment.RegionalDeploymentID == deploymentID && (assignment.Status == AssignmentAvailable || assignment.Status == AssignmentClaimed) {
			assignment.Status = AssignmentCancelled
			m.assignments[id] = assignment
		}
	}
}

func (m *Module) createAssignmentLocked(task RegionalTask, reservation Reservation, action string, payload map[string]string, now time.Time) WorkAssignment {
	assignment := WorkAssignment{ID: contract.WorkAssignmentID(m.next("was")), RegionalTaskID: task.ID, RegionalDeploymentID: task.RegionalDeploymentID, NodeID: reservation.NodeID, Action: action, Payload: payload, FencingToken: reservation.FencingToken, Status: AssignmentAvailable, CreatedAt: now}
	m.assignments[assignment.ID] = assignment
	return assignment
}

func (m *Module) PollAssignments(_ context.Context, nodeID contract.NodeID, limit int, now time.Time) []WorkAssignment {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit < 1 {
		return nil
	}
	if limit > 100 {
		limit = 100
	}
	node, ok := m.nodes[nodeID]
	if !ok || node.State != NodeReady || !node.LeaseUntil.After(now) {
		return nil
	}
	var result []WorkAssignment
	for _, assignment := range m.assignments {
		if assignment.NodeID == nodeID && (assignment.Status == AssignmentAvailable || assignment.Status == AssignmentClaimed && !assignment.ClaimLeaseUntil.After(now)) {
			result = append(result, assignment)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func (m *Module) ClaimAssignment(_ context.Context, assignmentID contract.WorkAssignmentID, nodeID contract.NodeID, leaseUntil, now time.Time) (WorkAssignment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	assignment, ok := m.assignments[assignmentID]
	if !ok {
		return WorkAssignment{}, errors.New("work assignment not found")
	}
	node := m.nodes[nodeID]
	reservation := m.reservations[assignment.RegionalDeploymentID]
	if assignment.NodeID != nodeID || node.State != NodeReady || !node.LeaseUntil.After(now) || !reservation.Active || reservation.FencingToken != assignment.FencingToken {
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
	m.assignments[assignment.ID] = assignment
	return assignment, nil
}

func (m *Module) CompleteAssignment(_ context.Context, assignmentID contract.WorkAssignmentID, nodeID contract.NodeID, fencingToken int64, succeeded bool, now time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	assignment, ok := m.assignments[assignmentID]
	if !ok {
		return false, errors.New("work assignment not found")
	}
	if assignment.Status == AssignmentSucceeded || assignment.Status == AssignmentFailed {
		return false, nil
	}
	node := m.nodes[nodeID]
	reservation := m.reservations[assignment.RegionalDeploymentID]
	if assignment.Status != AssignmentClaimed || assignment.ClaimedBy != nodeID || !assignment.ClaimLeaseUntil.After(now) || node.State != NodeReady || !node.LeaseUntil.After(now) || !reservation.Active || reservation.FencingToken != fencingToken || assignment.FencingToken != fencingToken {
		return false, errors.New("stale work assignment")
	}
	if succeeded {
		assignment.Status = AssignmentSucceeded
	} else {
		assignment.Status = AssignmentFailed
	}
	completedAt := now
	assignment.CompletedAt = &completedAt
	m.assignments[assignment.ID] = assignment
	task := m.tasks[assignment.RegionalTaskID]
	if succeeded {
		task.Status = "succeeded"
	} else {
		task.Status = "failed"
	}
	task.Attempts = assignment.Attempts
	m.tasks[task.ID] = task
	return true, nil
}

func (m *Module) Assignments(_ context.Context) []WorkAssignment {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]WorkAssignment, 0, len(m.assignments))
	for _, item := range m.assignments {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
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

func (m *Module) Monitoring(_ context.Context) Monitoring {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := Monitoring{OutboxLag: len(m.outbox)}
	for _, node := range m.nodes {
		if node.State == NodeStale {
			result.StaleNodes++
		}
	}
	for _, assignment := range m.assignments {
		if assignment.Status == AssignmentFailed {
			result.ReconciliationFailures++
		}
		if assignment.CompletedAt != nil {
			latency := assignment.CompletedAt.Sub(assignment.CreatedAt).Milliseconds()
			if latency > result.TaskLatencyMilliseconds {
				result.TaskLatencyMilliseconds = latency
			}
		}
	}
	return result
}

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
