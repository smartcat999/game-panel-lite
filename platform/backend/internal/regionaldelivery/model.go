package regionaldelivery

import (
	"errors"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/billing"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliverycontrol"
)

var (
	ErrInvalidDesired   = errors.New("invalid desired deployment")
	ErrWrongRegion      = errors.New("deployment belongs to another region")
	ErrNoCapacity       = errors.New("no regional capacity")
	ErrNoEndpoint       = errors.New("no compatible endpoint capacity")
	ErrAssignmentLease  = errors.New("assignment lease is not current")
	ErrFencingToken     = errors.New("stale fencing token")
	ErrDeliveryNotFound = errors.New("regional delivery not found")
)

const (
	PhasePending   = "pending"
	PhaseScheduled = "scheduled"
	PhaseEndpoints = "endpoints-allocated"
	PhaseAssigned  = "assigned"
	PhaseReady     = "ready"
	PhaseFailed    = "failed"
	PhasePublished = "published"
	PhaseCleaned   = "cleaned"
)

type Node struct {
	ID                string
	RegionID          string
	State             string
	CPUCapacityMilli  int64
	MemoryCapacityMiB int64
	DiskCapacityGiB   int64
	ReservedCPUMilli  int64
	ReservedMemoryMiB int64
	ReservedDiskGiB   int64
	LeaseUntil        time.Time
	UpdatedAt         time.Time
}

type EndpointPool struct {
	ID           string
	RegionID     string
	DeliveryMode string
	Address      string
	PortStart    *int
	PortEnd      *int
	Stability    string
	Active       bool
}

type State struct {
	ID                      string
	WorkspaceID             string
	LogicalInstanceID       string
	RegionID                string
	PlacementVersion        int64
	InstanceRevisionID      string
	OperationID             string
	DesiredState            string
	ProviderReleaseID       string
	GameVersion             string
	ApplyBehavior           string
	ResourceSpec            billing.ResourceSpec
	Configuration           map[string]any
	ModLock                 []deliverycontrol.ModLockEntry
	ListenerRequirements    []deliverycontrol.ListenerRequirement
	Phase                   string
	NodeID                  string
	FencingToken            int64
	ReconcileOwner          string
	ReconcileLeaseUntil     time.Time
	AssignmentOwner         string
	AssignmentLeaseUntil    time.Time
	AssignmentAttempts      int
	ObservationSequence     int64
	TelemetrySequence       int64
	FailureCode             string
	ResidualCleanupRequired bool
	UpdatedAt               time.Time
}

type Assignment struct {
	RegionalDeliveryID string
	WorkspaceID        string
	LogicalInstanceID  string
	RegionID           string
	NodeID             string
	FencingToken       int64
	DesiredState       string
	ProviderReleaseID  string
	GameVersion        string
	ApplyBehavior      string
	ResourceSpec       billing.ResourceSpec
	Configuration      map[string]any
	ModLock            []deliverycontrol.ModLockEntry
	Endpoints          []deliverycontrol.EndpointBinding
	LeaseUntil         time.Time
	Attempt            int
	TelemetrySequence  int64
}
