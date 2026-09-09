package regional

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/internal/workload"
)

var ErrExecutionUnavailable = errors.New("regional execution authority unavailable")
var ErrExecutionLeaseLost = errors.New("regional execution lease is no longer current")

// ExecutionCandidate binds one metadata-only Node task to the immutable global
// snapshot and the Region allocation it was scheduled from. It carries no
// executable configuration until ExecutionAuthorizer revalidates global state.
type ExecutionCandidate struct {
	Task       NodeTask
	Snapshot   RevisionSnapshot
	Allocation Allocation
}

func (c ExecutionCandidate) Validate(nodeID string, sessionEpoch int64) error {
	t := c.Task
	a := c.Allocation
	e := c.Snapshot.Event
	if _, err := executionGeneration(t.SpecGeneration); err != nil {
		return err
	}
	if !executionIdentifier(nodeID) || sessionEpoch < 1 ||
		!executionIdentifier(t.ID) || t.Kind != "run" || (t.Status != "awaiting_authority" && t.Status != "active") ||
		a.ID == "" || a.Status != "reserved" || t.AllocationID != a.ID || t.CapacityRequest != a.CapacityRequest ||
		a.NodeID != nodeID || a.SessionEpoch != sessionEpoch ||
		c.Snapshot.ValidateFor(e) != nil || c.Snapshot.DesiredState != "running" ||
		e.RegionID != a.RegionID || e.OrganizationID != a.OrganizationID || e.ServerID != a.ServerID ||
		e.PlacementEpoch != a.PlacementEpoch || e.RevisionID != a.RevisionID || e.SpecGeneration != a.SpecGeneration ||
		c.Snapshot.CurrentSpecGeneration != a.SpecGeneration || c.Snapshot.IntentVersion != a.IntentVersion {
		return ErrExecutionUnavailable
	}
	return nil
}

type ExecutionLease struct {
	TaskID, ServerID, NodeID, HolderID, ObservationToken string
	Generation                                           int
	Fence, GrantedAtMS, ExpiresAtMS                      int64
}

type ExecutionRepository interface {
	NextExecutionCandidate(context.Context, string, int64) (*ExecutionCandidate, error)
	ExecutionCandidate(context.Context, string, string, int64) (ExecutionCandidate, error)
	AcquireRegionalExecutionLease(context.Context, ExecutionCandidate, string, time.Duration, time.Duration) (ExecutionLease, error)
	RenewRegionalExecutionLease(context.Context, ExecutionCandidate, string, int64, time.Duration, time.Duration) (ExecutionLease, error)
	ReleaseRegionalExecutionLease(context.Context, ExecutionCandidate, string, int64) error
	SaveRegionalExecutionObservation(context.Context, ExecutionCandidate, string, int64, workload.Observation) error
}

type ExecutionPolicy interface {
	AuthorizeExecution(context.Context, RevisionSnapshot, Allocation) error
}

type WorkloadRenderer interface {
	RenderWorkload(context.Context, RevisionSnapshot, Allocation) (workload.Spec, error)
}

// ExecutionAuthorizer is the only module that turns a metadata-only regional
// task into executable Node input. Every acquire and renewal fetches current
// global state and policy. A global outage therefore cannot create or extend
// authority; an already running container is not treated as stopped.
type ExecutionAuthorizer struct {
	Tasks           ExecutionRepository
	Source          RevisionSource
	Policy          ExecutionPolicy
	Renderer        WorkloadRenderer
	LeaseTTL        time.Duration
	MaxHeartbeatAge time.Duration
}

func (a ExecutionAuthorizer) Acquire(ctx context.Context, nodeID string, sessionEpoch int64, holderID string) (*workload.AuthorizedAssignment, error) {
	if err := a.validate(holderID); err != nil || !executionIdentifier(nodeID) || sessionEpoch < 1 {
		return nil, ErrExecutionUnavailable
	}
	candidate, err := a.Tasks.NextExecutionCandidate(ctx, nodeID, sessionEpoch)
	if err != nil || candidate == nil {
		return nil, err
	}
	current, err := a.authorize(ctx, *candidate, nodeID, sessionEpoch)
	if err != nil {
		return nil, err
	}
	spec, err := a.Renderer.RenderWorkload(ctx, current, candidate.Allocation)
	if err != nil {
		return nil, err
	}
	lease, err := a.Tasks.AcquireRegionalExecutionLease(ctx, *candidate, holderID, a.LeaseTTL, a.MaxHeartbeatAge)
	if err != nil {
		return nil, err
	}
	if err := validateExecutionLease(lease, *candidate, holderID); err != nil {
		return nil, err
	}
	now := time.UnixMilli(lease.GrantedAtMS).UTC()
	assignment := workload.Assignment{ID: candidate.Task.ID, UID: candidate.Task.ID, ServerID: candidate.Allocation.ServerID, NodeID: nodeID, Generation: lease.Generation, DesiredState: "running", Spec: spec, ObservationToken: lease.ObservationToken, CreatedAt: now, UpdatedAt: now}
	return &workload.AuthorizedAssignment{Assignment: assignment, Lease: leaseGrant(lease)}, nil
}

func (a ExecutionAuthorizer) Renew(ctx context.Context, nodeID, taskID string, sessionEpoch int64, holderID string, fence int64) (workload.LeaseGrant, error) {
	if err := a.validate(holderID); err != nil || !executionIdentifier(nodeID) || !executionIdentifier(taskID) || sessionEpoch < 1 || fence < 1 {
		return workload.LeaseGrant{}, ErrExecutionUnavailable
	}
	candidate, err := a.Tasks.ExecutionCandidate(ctx, taskID, nodeID, sessionEpoch)
	if err != nil {
		return workload.LeaseGrant{}, err
	}
	if _, err := a.authorize(ctx, candidate, nodeID, sessionEpoch); err != nil {
		return workload.LeaseGrant{}, err
	}
	lease, err := a.Tasks.RenewRegionalExecutionLease(ctx, candidate, holderID, fence, a.LeaseTTL, a.MaxHeartbeatAge)
	if err != nil {
		return workload.LeaseGrant{}, err
	}
	if err := validateExecutionLease(lease, candidate, holderID); err != nil || lease.Fence != fence {
		return workload.LeaseGrant{}, ErrExecutionLeaseLost
	}
	return leaseGrant(lease), nil
}

func (a ExecutionAuthorizer) Release(ctx context.Context, nodeID, taskID string, sessionEpoch int64, holderID string, fence int64) error {
	if a.Tasks == nil || !executionIdentifier(nodeID) || !executionIdentifier(taskID) || !executionIdentifier(holderID) || sessionEpoch < 1 || fence < 1 {
		return ErrExecutionLeaseLost
	}
	candidate, err := a.Tasks.ExecutionCandidate(ctx, taskID, nodeID, sessionEpoch)
	if err != nil {
		return err
	}
	return a.Tasks.ReleaseRegionalExecutionLease(ctx, candidate, holderID, fence)
}

func (a ExecutionAuthorizer) Observe(ctx context.Context, nodeID, taskID string, sessionEpoch int64, observation workload.Observation) error {
	if a.Tasks == nil || !executionIdentifier(nodeID) || !executionIdentifier(taskID) || !executionIdentifier(observation.LeaseHolderID) || sessionEpoch < 1 || observation.LeaseFence < 1 {
		return ErrExecutionLeaseLost
	}
	candidate, err := a.Tasks.ExecutionCandidate(ctx, taskID, nodeID, sessionEpoch)
	if err != nil {
		return err
	}
	return a.Tasks.SaveRegionalExecutionObservation(ctx, candidate, observation.LeaseHolderID, observation.LeaseFence, observation)
}

func (a ExecutionAuthorizer) validate(holderID string) error {
	if a.Tasks == nil || a.Source == nil || a.Policy == nil || a.Renderer == nil || a.LeaseTTL < time.Second || a.LeaseTTL > 5*time.Minute || a.MaxHeartbeatAge < time.Second || a.MaxHeartbeatAge > time.Hour || !executionIdentifier(holderID) {
		return ErrExecutionUnavailable
	}
	return nil
}

func (a ExecutionAuthorizer) authorize(ctx context.Context, candidate ExecutionCandidate, nodeID string, sessionEpoch int64) (RevisionSnapshot, error) {
	if candidate.Validate(nodeID, sessionEpoch) != nil {
		return RevisionSnapshot{}, ErrExecutionUnavailable
	}
	current, err := a.Source.GetRevision(ctx, candidate.Snapshot.Event)
	if err != nil {
		return RevisionSnapshot{}, err
	}
	if current.ValidateFor(candidate.Snapshot.Event) != nil || current.CurrentSpecGeneration != candidate.Task.SpecGeneration ||
		current.IntentVersion != candidate.Task.IntentVersion || current.DesiredState != "running" ||
		!reflect.DeepEqual(current.Revision, candidate.Snapshot.Revision) || !reflect.DeepEqual(current.Assets, candidate.Snapshot.Assets) {
		return RevisionSnapshot{}, ErrExecutionUnavailable
	}
	if err := a.Policy.AuthorizeExecution(ctx, current, candidate.Allocation); err != nil {
		return RevisionSnapshot{}, err
	}
	return current, nil
}

func validateExecutionLease(lease ExecutionLease, candidate ExecutionCandidate, holderID string) error {
	generation, err := executionGeneration(candidate.Task.SpecGeneration)
	if err != nil {
		return err
	}
	if lease.TaskID != candidate.Task.ID || lease.ServerID != candidate.Task.ServerID || lease.NodeID != candidate.Task.NodeID || lease.HolderID != holderID ||
		lease.Generation != generation || lease.Fence < 1 || lease.GrantedAtMS < 0 || lease.ExpiresAtMS <= lease.GrantedAtMS {
		return ErrExecutionLeaseLost
	}
	return nil
}

func leaseGrant(lease ExecutionLease) workload.LeaseGrant {
	observation := lease.ObservationToken
	return workload.LeaseGrant{ObservationToken: &observation, AssignmentUID: lease.TaskID, ServerID: lease.ServerID, NodeID: lease.NodeID, Generation: lease.Generation, HolderID: lease.HolderID, Fence: lease.Fence, ValidForMS: lease.ExpiresAtMS - lease.GrantedAtMS}
}

func executionIdentifier(value string) bool {
	if value == "" || len(value) > 128 || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\x00\r\n") {
		return false
	}
	for _, ch := range value {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_' || ch == '.' || ch == ':') {
			return false
		}
	}
	return true
}

func executionGeneration(value int64) (int, error) {
	if value < 1 || value > int64(math.MaxInt) {
		return 0, ErrExecutionUnavailable
	}
	return int(value), nil
}
