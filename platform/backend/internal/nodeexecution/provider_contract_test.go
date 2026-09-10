package nodeexecution

import (
	"context"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionexecution"
)

type fakeGame struct {
	image   string
	intents *[]WorkloadIntent
}

func (f fakeGame) Key() string { return "terraria" }
func (f fakeGame) Build(intent WorkloadIntent) (WorkloadSpec, error) {
	if f.intents != nil {
		*f.intents = append(*f.intents, intent)
	}
	return WorkloadSpec{LogicalInstanceID: intent.LogicalInstanceID, DesiredState: intent.DesiredState, Image: f.image, DataDir: intent.DataDir, CPUUnits: intent.CPUUnits, MemoryMegabytes: intent.MemoryMegabytes}, nil
}

type fakeRuntime struct{ specs []WorkloadSpec }

func (f *fakeRuntime) Reconcile(_ context.Context, spec WorkloadSpec) (WorkloadObservation, error) {
	f.specs = append(f.specs, spec)
	return WorkloadObservation{RuntimeID: "runtime-" + spec.LogicalInstanceID, State: spec.DesiredState}, nil
}

func TestProviderContractsAreReplaceableAtThePhase5CallSite(t *testing.T) {
	root, _ := NewScopedRoot(t.TempDir())
	assignment := regionexecution.WorkAssignment{Action: "reconcile_workload", Payload: map[string]string{"logicalInstanceId": "lin_test", "gameKey": "terraria", "gameVersion": "1.4.5.6", "configuration": `{"worldName":"Contract World"}`, "desiredState": "running", "dataRelative": "instances/lin_test", "cpuUnits": "1000", "memoryMegabytes": "2048"}}
	var intents []WorkloadIntent
	firstRuntime := &fakeRuntime{}
	first := Executor{Root: root, Games: map[string]GameProvider{"terraria": fakeGame{image: "image:first", intents: &intents}}, Runtime: firstRuntime}
	if err := first.Reconcile(context.Background(), assignment); err != nil {
		t.Fatal(err)
	}
	secondRuntime := &fakeRuntime{}
	second := Executor{Root: root, Games: map[string]GameProvider{"terraria": fakeGame{image: "image:second"}}, Runtime: secondRuntime, Clock: func() time.Time { return time.Time{} }}
	if err := second.Reconcile(context.Background(), assignment); err != nil {
		t.Fatal(err)
	}
	if firstRuntime.specs[0].Image != "image:first" || secondRuntime.specs[0].Image != "image:second" {
		t.Fatalf("providers were not replaceable: first=%#v second=%#v", firstRuntime.specs, secondRuntime.specs)
	}
	if len(intents) != 1 || intents[0].GameVersion != "1.4.5.6" || intents[0].Configuration["worldName"] != "Contract World" {
		t.Fatalf("version/configuration did not cross the provider contract: %#v", intents)
	}
}

func TestWorkloadResultIsSequencedAfterRuntimeReconciliation(t *testing.T) {
	region, now := assignedRegion(t)
	root, _ := NewScopedRoot(t.TempDir())
	runtime := &fakeRuntime{}
	var intents []WorkloadIntent
	executor := Executor{Root: root, Games: map[string]GameProvider{"terraria": fakeGame{image: "image:test", intents: &intents}}, Runtime: runtime, WorkloadResults: region, Clock: func() time.Time { return now.Add(time.Second) }}
	agent := Agent{NodeID: "nod_test", BatchSize: 1, ClaimTTL: time.Minute, Store: region, Reconcile: executor.Reconcile}
	if processed, err := agent.RunOnce(context.Background(), now); err != nil || processed != 1 {
		t.Fatalf("processed=%d error=%v", processed, err)
	}
	deployment := region.Deployments(context.Background())[0]
	if deployment.ObservedState != regionexecution.ObservedRunning || deployment.ObservationSequence != 1 || len(runtime.specs) != 1 {
		t.Fatalf("deployment=%#v specs=%#v", deployment, runtime.specs)
	}
	if len(intents) != 1 || intents[0].GameVersion != "1.4.5.6" || intents[0].Configuration["worldName"] != "Assigned World" {
		t.Fatalf("regional assignment lost provider input: %#v", intents)
	}
	if processed, err := agent.RunOnce(context.Background(), now.Add(2*time.Second)); err != nil || processed != 0 || region.Deployments(context.Background())[0].ObservationSequence != 1 {
		t.Fatalf("terminal effect duplicated processed=%d error=%v", processed, err)
	}
}
