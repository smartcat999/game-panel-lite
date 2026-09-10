package instancecontrol

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
	ErrInvalidInstance = errors.New("invalid logical instance")
	ErrInstanceMissing = errors.New("logical instance not found")
)

type DesiredState string

const (
	DesiredRunning DesiredState = "running"
	DesiredStopped DesiredState = "stopped"
)

type BillingState string

const (
	BillingPending BillingState = "pending_payment"
	BillingActive  BillingState = "active"
)

type DeploymentState string

const (
	DeploymentPendingPayment DeploymentState = "pending_payment"
	DeploymentWaitingRegion  DeploymentState = "waiting_region"
	DeploymentRunning        DeploymentState = "running"
	DeploymentStopped        DeploymentState = "stopped"
	DeploymentFailed         DeploymentState = "failed"
)

type LogicalInstance struct {
	ID              contract.LogicalInstanceID `json:"id"`
	WorkspaceID     contract.WorkspaceID       `json:"workspaceId"`
	Name            string                     `json:"name"`
	GameKey         string                     `json:"gameKey"`
	DesiredState    DesiredState               `json:"desiredState"`
	BillingState    BillingState               `json:"billingState"`
	DeploymentState DeploymentState            `json:"deploymentState"`
	Stale           bool                       `json:"stale"`
	CreatedAt       time.Time                  `json:"createdAt"`
}

type InstanceRevision struct {
	ID                contract.InstanceRevisionID `json:"id"`
	LogicalInstanceID contract.LogicalInstanceID  `json:"logicalInstanceId"`
	Version           int                         `json:"version"`
	GameVersion       string                      `json:"gameVersion"`
	Configuration     map[string]any              `json:"configuration"`
	CreatedAt         time.Time                   `json:"createdAt"`
}

type Placement struct {
	ID                contract.PlacementID       `json:"id"`
	LogicalInstanceID contract.LogicalInstanceID `json:"logicalInstanceId"`
	RegionID          contract.RegionID          `json:"regionId"`
	Version           int                        `json:"version"`
	CreatedAt         time.Time                  `json:"createdAt"`
}

type CreateCommand struct {
	WorkspaceID   contract.WorkspaceID
	Name          string
	GameKey       string
	GameVersion   string
	Configuration map[string]any
	RegionID      contract.RegionID
}

type PreparedCreate struct {
	Instance  LogicalInstance
	Revision  InstanceRevision
	Placement Placement
}

type CreateIDs struct {
	InstanceID  contract.LogicalInstanceID
	RevisionID  contract.InstanceRevisionID
	PlacementID contract.PlacementID
}

type Detail struct {
	Instance  LogicalInstance  `json:"instance"`
	Revision  InstanceRevision `json:"revision"`
	Placement Placement        `json:"placement"`
}

type Module struct {
	mu         sync.RWMutex
	instances  map[contract.LogicalInstanceID]LogicalInstance
	revisions  map[contract.LogicalInstanceID]InstanceRevision
	placements map[contract.LogicalInstanceID]Placement
	nextID     int
}

func New() *Module {
	return &Module{
		instances:  make(map[contract.LogicalInstanceID]LogicalInstance),
		revisions:  make(map[contract.LogicalInstanceID]InstanceRevision),
		placements: make(map[contract.LogicalInstanceID]Placement),
	}
}

func (m *Module) PrepareCreate(command CreateCommand, now time.Time) (PreparedCreate, error) {
	if command.WorkspaceID == "" || command.RegionID == "" || strings.TrimSpace(command.Name) == "" || command.GameKey == "" || command.GameVersion == "" {
		return PreparedCreate{}, ErrInvalidInstance
	}
	m.mu.Lock()
	m.nextID++
	nextID := m.nextID
	m.mu.Unlock()
	instanceID := contract.LogicalInstanceID(fmt.Sprintf("lin_%06d", nextID))
	return PrepareCreateWithIDs(command, CreateIDs{InstanceID: instanceID, RevisionID: contract.InstanceRevisionID(fmt.Sprintf("rev_%06d", nextID)), PlacementID: contract.PlacementID(fmt.Sprintf("plc_%06d", nextID))}, now)
}

func PrepareCreateWithIDs(command CreateCommand, ids CreateIDs, now time.Time) (PreparedCreate, error) {
	if command.WorkspaceID == "" || command.RegionID == "" || strings.TrimSpace(command.Name) == "" || command.GameKey == "" || command.GameVersion == "" || ids.InstanceID == "" || ids.RevisionID == "" || ids.PlacementID == "" {
		return PreparedCreate{}, ErrInvalidInstance
	}
	configuration := make(map[string]any, len(command.Configuration))
	for key, value := range command.Configuration {
		configuration[key] = value
	}
	return PreparedCreate{
		Instance: LogicalInstance{
			ID: ids.InstanceID, WorkspaceID: command.WorkspaceID, Name: strings.TrimSpace(command.Name), GameKey: command.GameKey,
			DesiredState: DesiredRunning, BillingState: BillingPending, DeploymentState: DeploymentPendingPayment, CreatedAt: now,
		},
		Revision: InstanceRevision{
			ID: ids.RevisionID, LogicalInstanceID: ids.InstanceID, Version: 1,
			GameVersion: command.GameVersion, Configuration: configuration, CreatedAt: now,
		},
		Placement: Placement{
			ID: ids.PlacementID, LogicalInstanceID: ids.InstanceID, RegionID: command.RegionID,
			Version: 1, CreatedAt: now,
		},
	}, nil
}

func (m *Module) CommitCreate(prepared PreparedCreate) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.instances[prepared.Instance.ID] = prepared.Instance
	m.revisions[prepared.Instance.ID] = prepared.Revision
	m.placements[prepared.Instance.ID] = prepared.Placement
}

func (m *Module) Activate(instanceID contract.LogicalInstanceID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	instance, ok := m.instances[instanceID]
	if !ok {
		return ErrInstanceMissing
	}
	instance.BillingState = BillingActive
	instance.DeploymentState = DeploymentWaitingRegion
	m.instances[instanceID] = instance
	return nil
}

func (m *Module) SetPreviewState(instanceID contract.LogicalInstanceID, state DeploymentState, stale bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	instance, ok := m.instances[instanceID]
	if !ok {
		return ErrInstanceMissing
	}
	instance.DeploymentState = state
	instance.Stale = stale
	if state != DeploymentPendingPayment {
		instance.BillingState = BillingActive
	}
	m.instances[instanceID] = instance
	return nil
}

func (m *Module) List(_ context.Context, workspaceID contract.WorkspaceID) []LogicalInstance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []LogicalInstance
	for _, instance := range m.instances {
		if instance.WorkspaceID == workspaceID {
			result = append(result, instance)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (m *Module) All(_ context.Context) []LogicalInstance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]LogicalInstance, 0, len(m.instances))
	for _, instance := range m.instances {
		result = append(result, instance)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (m *Module) Get(_ context.Context, workspaceID contract.WorkspaceID, instanceID contract.LogicalInstanceID) (Detail, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	instance, ok := m.instances[instanceID]
	if !ok || instance.WorkspaceID != workspaceID {
		return Detail{}, ErrInstanceMissing
	}
	return Detail{Instance: instance, Revision: m.revisions[instanceID], Placement: m.placements[instanceID]}, nil
}

func (m *Module) GetByID(_ context.Context, instanceID contract.LogicalInstanceID) (Detail, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	instance, ok := m.instances[instanceID]
	if !ok {
		return Detail{}, ErrInstanceMissing
	}
	return Detail{Instance: instance, Revision: m.revisions[instanceID], Placement: m.placements[instanceID]}, nil
}

func (m *Module) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.instances)
}
