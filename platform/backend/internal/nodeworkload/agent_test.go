package nodeworkload

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/billing"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionaldelivery"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionaltask"
)

type agentAssignmentStore struct {
	assignment  regionaldelivery.Assignment
	state       regionaldelivery.State
	claimed     bool
	completed   bool
	ready       bool
	failure     string
	observation regionaldelivery.Assignment
	observed    bool
}

func (s *agentAssignmentStore) ClaimObservation(context.Context, string, time.Time) (regionaldelivery.Assignment, bool, error) {
	if s.observed || s.observation.RegionalDeliveryID == "" {
		return regionaldelivery.Assignment{}, false, nil
	}
	s.observed = true
	return s.observation, true, nil
}

func (s *agentAssignmentStore) ClaimAssignment(context.Context, string, string, time.Time) (regionaldelivery.Assignment, bool, error) {
	if s.claimed || s.assignment.RegionalDeliveryID == "" {
		return regionaldelivery.Assignment{}, false, nil
	}
	s.claimed = true
	return s.assignment, true, nil
}

func (s *agentAssignmentStore) CompleteAssignment(_ context.Context, _ string, _ string, _ int64, ready bool, failure string, _ time.Time) (bool, error) {
	s.completed, s.ready, s.failure = true, ready, failure
	return true, nil
}

func (s *agentAssignmentStore) State(context.Context, string) (regionaldelivery.State, error) {
	return s.state, nil
}

type agentTaskStore struct {
	task      regionaltask.Task
	claimed   bool
	completed bool
	result    regionaltask.BackupResult
}

func (s *agentTaskStore) Claim(context.Context, string, string, time.Time) (regionaltask.Task, bool, error) {
	if s.claimed || s.task.ID == "" {
		return regionaltask.Task{}, false, nil
	}
	s.claimed = true
	return s.task, true, nil
}

func (s *agentTaskStore) Complete(_ context.Context, _ string, _ string, _ int64, result regionaltask.BackupResult, _ time.Time) (bool, error) {
	s.completed, s.result = true, result
	return true, nil
}

type agentRegistry struct {
	manifests map[string]providercontract.Manifest
}

func (r agentRegistry) Verified(_ context.Context, releaseID string) (providercontract.Manifest, error) {
	manifest, ok := r.manifests[releaseID]
	if !ok {
		return providercontract.Manifest{}, errors.New("release not verified")
	}
	return manifest, nil
}

func validAgentAssignment() regionaldelivery.Assignment {
	return regionaldelivery.Assignment{RegionalDeliveryID: "rdl_one", WorkspaceID: "ws_one", LogicalInstanceID: "lin_one", RegionID: "reg_one", NodeID: "node_one", FencingToken: 3, DesiredState: "running", ProviderReleaseID: "gpr_one", GameVersion: "1.0", ApplyBehavior: "restart-required", ResourceSpec: billing.ResourceSpec{CPUMilli: 1000, MemoryMiB: 2048, DiskGiB: 10}, Attempt: 1, TelemetrySequence: 8}
}

func TestAgentReconcilesOnlyVerifiedInstalledRelease(t *testing.T) {
	assignments := &agentAssignmentStore{assignment: validAgentAssignment()}
	runtime := &FakeRuntimeProvider{Name: "docker"}
	telemetry := &MemoryTelemetry{}
	agent := &Agent{NodeID: "node_one", WorkerID: "worker_one", Assignments: assignments, Registry: agentRegistry{manifests: map[string]providercontract.Manifest{"gpr_one": {ProviderReleaseID: "gpr_one"}}}, Providers: map[string]GameProvider{"gpr_one": &FakeGameProvider{Artifact: "image@sha256:test"}}, Runtime: runtime, Telemetry: telemetry, Management: []string{"10.0.0.0/8"}}
	if err := agent.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !assignments.completed || !assignments.ready || assignments.failure != "" {
		t.Fatalf("completion=%#v", assignments)
	}
	observations := telemetry.ByInstance["lin_one"]
	if len(observations) != 1 || observations[0].FencingToken != 3 || observations[0].Sequence != 9 || len(runtime.Policies) != 1 || !runtime.Policies[0].InternetEgressAllowed {
		t.Fatalf("observations=%#v policies=%#v", observations, runtime.Policies)
	}
}

func TestAgentKeepsAssignmentPendingUntilRuntimeIsReady(t *testing.T) {
	assignments := &agentAssignmentStore{assignment: validAgentAssignment()}
	agent := &Agent{NodeID: "node_one", WorkerID: "worker_one", Assignments: assignments, Registry: agentRegistry{manifests: map[string]providercontract.Manifest{"gpr_one": {ProviderReleaseID: "gpr_one"}}}, Providers: map[string]GameProvider{"gpr_one": &FakeGameProvider{Artifact: "image@sha256:test"}}, Runtime: &FakeRuntimeProvider{State: "starting"}, Telemetry: &MemoryTelemetry{}, Management: []string{"10.0.0.0/8"}}
	if err := agent.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if assignments.completed {
		t.Fatal("starting runtime completed the deployment assignment")
	}
}

func TestAgentFailsClosedWhenVerifiedReleaseIsNotInstalled(t *testing.T) {
	assignment := validAgentAssignment()
	assignment.Attempt = 5
	assignments := &agentAssignmentStore{assignment: assignment}
	agent := &Agent{NodeID: "node_one", WorkerID: "worker_one", Assignments: assignments, Registry: agentRegistry{manifests: map[string]providercontract.Manifest{"gpr_one": {ProviderReleaseID: "gpr_one"}}}, Runtime: &FakeRuntimeProvider{}, Telemetry: &MemoryTelemetry{}}
	if err := agent.Tick(t.Context()); !errors.Is(err, ErrInvalidWorkload) {
		t.Fatalf("expected fail-closed workload error, got %v", err)
	}
	if !assignments.completed || assignments.ready || assignments.failure != "provider_not_installed" {
		t.Fatalf("completion=%#v", assignments)
	}
}

func TestAgentCollectsTelemetryForPublishedAssignment(t *testing.T) {
	assignment := validAgentAssignment()
	assignments := &agentAssignmentStore{observation: assignment}
	telemetry := &MemoryTelemetry{}
	agent := &Agent{NodeID: "node_one", WorkerID: "worker_one", Assignments: assignments, Registry: agentRegistry{manifests: map[string]providercontract.Manifest{"gpr_one": {ProviderReleaseID: "gpr_one"}}}, Providers: map[string]GameProvider{"gpr_one": &FakeGameProvider{}}, Runtime: &FakeRuntimeProvider{}, Telemetry: telemetry, Management: []string{"10.0.0.0/8"}}
	if err := agent.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	observations := telemetry.ByInstance[assignment.LogicalInstanceID]
	if !assignments.observed || len(observations) != 1 || observations[0].Sequence != assignment.TelemetrySequence+1 {
		t.Fatalf("observed=%t observations=%#v", assignments.observed, observations)
	}
}

func TestAgentCompletesBackupAgainstCurrentNodeAndFence(t *testing.T) {
	assignments := &agentAssignmentStore{state: regionaldelivery.State{LogicalInstanceID: "lin_one", NodeID: "node_one", FencingToken: 4, ProviderReleaseID: "gpr_one"}}
	tasks := &agentTaskStore{task: regionaltask.Task{ID: "tsk_one", LogicalInstanceID: "lin_one", NodeID: "node_one", Kind: "backup", Payload: []byte(`{"objectKey":"object://regions/reg_one/backup.tar.gz"}`), FencingToken: 4}}
	runtime := &FakeRuntimeProvider{}
	agent := &Agent{NodeID: "node_one", WorkerID: "worker_one", Assignments: assignments, Tasks: tasks, Registry: agentRegistry{manifests: map[string]providercontract.Manifest{"gpr_one": {ProviderReleaseID: "gpr_one", Capabilities: []string{"backup"}}}}, Providers: map[string]GameProvider{"gpr_one": &FakeGameProvider{}}, Runtime: runtime, Telemetry: &MemoryTelemetry{}}
	if err := agent.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !tasks.completed || tasks.result.Status != "completed" || tasks.result.ObjectKey == "" || runtime.BackupData["instances/lin_one"].ObjectKey == "" {
		t.Fatalf("task=%#v backups=%#v", tasks, runtime.BackupData)
	}
}

func TestAgentRejectsTaskAfterNodeOrFenceChanges(t *testing.T) {
	assignments := &agentAssignmentStore{state: regionaldelivery.State{LogicalInstanceID: "lin_one", NodeID: "node_two", FencingToken: 5, ProviderReleaseID: "gpr_one"}}
	tasks := &agentTaskStore{task: regionaltask.Task{ID: "tsk_one", LogicalInstanceID: "lin_one", Kind: "backup", Payload: []byte(`{"objectKey":"backup.tar.gz"}`), FencingToken: 4}}
	agent := &Agent{NodeID: "node_one", WorkerID: "worker_one", Assignments: assignments, Tasks: tasks, Registry: agentRegistry{}, Runtime: &FakeRuntimeProvider{}, Telemetry: &MemoryTelemetry{}}
	if err := agent.Tick(t.Context()); !errors.Is(err, ErrInvalidWorkload) {
		t.Fatalf("expected stale task rejection, got %v", err)
	}
	if tasks.completed {
		t.Fatal("stale task was completed by the wrong node")
	}
}
