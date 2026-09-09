package regional

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

type executionRepositoryProbe struct {
	candidate                  ExecutionCandidate
	acquires, renews, releases int
	observations               []workload.Observation
}

func (p *executionRepositoryProbe) NextExecutionCandidate(context.Context, string, int64) (*ExecutionCandidate, error) {
	c := p.candidate
	return &c, nil
}
func (p *executionRepositoryProbe) ExecutionCandidate(context.Context, string, string, int64) (ExecutionCandidate, error) {
	return p.candidate, nil
}
func (p *executionRepositoryProbe) AcquireRegionalExecutionLease(_ context.Context, c ExecutionCandidate, holder string, ttl, _ time.Duration) (ExecutionLease, error) {
	p.acquires++
	return executionLeaseFixture(c, holder, 1, ttl), nil
}
func (p *executionRepositoryProbe) RenewRegionalExecutionLease(_ context.Context, c ExecutionCandidate, holder string, fence int64, ttl, _ time.Duration) (ExecutionLease, error) {
	p.renews++
	return executionLeaseFixture(c, holder, fence, ttl), nil
}
func (p *executionRepositoryProbe) ReleaseRegionalExecutionLease(context.Context, ExecutionCandidate, string, int64) error {
	p.releases++
	return nil
}
func (p *executionRepositoryProbe) SaveRegionalExecutionObservation(_ context.Context, _ ExecutionCandidate, _ string, _ int64, observation workload.Observation) error {
	p.observations = append(p.observations, observation)
	return nil
}

type executionPolicyProbe struct {
	calls int
	err   error
}

func (p *executionPolicyProbe) AuthorizeExecution(context.Context, RevisionSnapshot, Allocation) error {
	p.calls++
	return p.err
}

type executionRendererProbe struct{ calls int }

func (p *executionRendererProbe) RenderWorkload(_ context.Context, snapshot RevisionSnapshot, allocation Allocation) (workload.Spec, error) {
	p.calls++
	return workload.Spec{ServerID: snapshot.Event.ServerID, Name: snapshot.Event.ServerID, Image: "registry.example/game@sha256:fixture", Network: workload.Network{Port: 7777, HostPort: allocation.Ports[0].HostPort}}, nil
}

func TestExecutionAuthorizerAcquiresOnlyAfterCurrentGlobalChecks(t *testing.T) {
	candidate := executionCandidateFixture()
	repo := &executionRepositoryProbe{candidate: candidate}
	policy := &executionPolicyProbe{}
	renderer := &executionRendererProbe{}
	sourceCalls := 0
	authorizer := ExecutionAuthorizer{Tasks: repo, Source: schedulingSourceFunc(func(_ context.Context, event instances.RevisionAvailable) (RevisionSnapshot, error) {
		sourceCalls++
		if event != candidate.Snapshot.Event {
			t.Fatal("wrong global event")
		}
		return candidate.Snapshot, nil
	}), Policy: policy, Renderer: renderer, LeaseTTL: 2 * time.Minute, MaxHeartbeatAge: time.Minute}

	granted, err := authorizer.Acquire(context.Background(), "node-a", 3, "process-a")
	if err != nil {
		t.Fatal(err)
	}
	if sourceCalls != 1 || policy.calls != 1 || renderer.calls != 1 || repo.acquires != 1 {
		t.Fatalf("wrong execution sequence: %d %d %d %d", sourceCalls, policy.calls, renderer.calls, repo.acquires)
	}
	if granted.Assignment.UID != candidate.Task.ID || granted.Assignment.NodeID != "node-a" || granted.Assignment.Spec.ServerID != "server-a" || granted.Lease.Fence != 1 || granted.Lease.ValidForMS != (2*time.Minute).Milliseconds() {
		t.Fatalf("unexpected grant: %+v", granted)
	}
}

func TestExecutionAuthorizerRejectsChangedGlobalStateBeforeRendering(t *testing.T) {
	base := executionCandidateFixture()
	for name, mutate := range map[string]func(*RevisionSnapshot){
		"stopped":        func(s *RevisionSnapshot) { s.DesiredState = "stopped" },
		"new intent":     func(s *RevisionSnapshot) { s.IntentVersion++ },
		"new generation": func(s *RevisionSnapshot) { s.CurrentSpecGeneration++ },
		"changed config": func(s *RevisionSnapshot) { s.Revision.Specification.Configuration.Ciphertext = []byte("changed") },
	} {
		t.Run(name, func(t *testing.T) {
			current := base.Snapshot
			mutate(&current)
			repo := &executionRepositoryProbe{candidate: base}
			policy := &executionPolicyProbe{}
			renderer := &executionRendererProbe{}
			authorizer := ExecutionAuthorizer{Tasks: repo, Source: schedulingSourceFunc(func(context.Context, instances.RevisionAvailable) (RevisionSnapshot, error) { return current, nil }), Policy: policy, Renderer: renderer, LeaseTTL: time.Minute, MaxHeartbeatAge: time.Minute}
			if _, err := authorizer.Acquire(context.Background(), "node-a", 3, "process-a"); !errors.Is(err, ErrExecutionUnavailable) {
				t.Fatalf("unexpected error: %v", err)
			}
			if policy.calls != 0 || renderer.calls != 0 || repo.acquires != 0 {
				t.Fatal("invalid global state reached execution adapters")
			}
		})
	}
}

func TestExecutionRenewalRechecksGlobalPolicy(t *testing.T) {
	candidate := executionCandidateFixture()
	repo := &executionRepositoryProbe{candidate: candidate}
	policy := &executionPolicyProbe{err: errors.New("global unavailable")}
	authorizer := ExecutionAuthorizer{Tasks: repo, Source: schedulingSourceFunc(func(context.Context, instances.RevisionAvailable) (RevisionSnapshot, error) {
		return candidate.Snapshot, nil
	}), Policy: policy, Renderer: &executionRendererProbe{}, LeaseTTL: time.Minute, MaxHeartbeatAge: time.Minute}
	if _, err := authorizer.Renew(context.Background(), "node-a", candidate.Task.ID, 3, "process-a", 1); err == nil {
		t.Fatal("renewal ignored current policy failure")
	}
	if repo.renews != 0 {
		t.Fatal("failed policy extended the regional lease")
	}
}

func executionCandidateFixture() ExecutionCandidate {
	event := instances.RevisionAvailable{SchemaVersion: 1, EventID: "event-a", OperationID: "operation-a", OrganizationID: "tenant-a", ServerID: "server-a", RegionID: "region-a", RevisionID: "revision-a", SpecGeneration: 2, PlacementEpoch: 4}
	snapshot := RevisionSnapshot{Event: event, CurrentSpecGeneration: 2, IntentVersion: 5, DesiredState: "running", Revision: instances.Revision{ID: event.RevisionID, ServerID: event.ServerID, SpecGeneration: 2, Specification: instances.Specification{ProviderKey: "fixture", GameVersion: "1", ConfigSchemaVersion: 1, Configuration: instances.ProtectedConfiguration{KeyID: "key-a", Ciphertext: []byte("protected")}, Resources: instances.Resources{CPU: 2, MemoryMB: 1024}}}}
	request := CapacityRequest{RegionID: event.RegionID, OrganizationID: event.OrganizationID, DeploymentID: "deployment-a", ServerID: event.ServerID, PlacementEpoch: event.PlacementEpoch, RevisionID: event.RevisionID, SpecGeneration: event.SpecGeneration, IntentVersion: 5, NodeID: "node-a", NodeVersion: 2, SessionEpoch: 3}
	allocation := Allocation{ID: "allocation-a", CapacityRequest: request, CPU: 2, MemoryMB: 1024, Status: "reserved", Ports: []workload.Port{{Port: 7777, HostPort: 30001, Protocol: "tcp"}}}
	return ExecutionCandidate{Task: NodeTask{ID: "task-a", AllocationID: allocation.ID, CapacityRequest: request, Kind: "run", Status: "awaiting_authority"}, Snapshot: snapshot, Allocation: allocation}
}

func executionLeaseFixture(candidate ExecutionCandidate, holder string, fence int64, ttl time.Duration) ExecutionLease {
	return ExecutionLease{TaskID: candidate.Task.ID, ServerID: candidate.Task.ServerID, NodeID: candidate.Task.NodeID, HolderID: holder, Generation: int(candidate.Task.SpecGeneration), Fence: fence, GrantedAtMS: 1000, ExpiresAtMS: 1000 + ttl.Milliseconds()}
}
