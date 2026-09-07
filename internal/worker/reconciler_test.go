package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/smartcat999/game-panel-lite/internal/workload"
)

type memoryRuntime struct {
	state State
	calls []string
	err   error
}

func (r *memoryRuntime) Inspect(ctx context.Context, _ string) (State, error) {
	r.calls = append(r.calls, "inspect")
	if ctx.Err() != nil {
		return State{}, ctx.Err()
	}
	return r.state, r.err
}
func (r *memoryRuntime) Create(_ context.Context, a workload.Assignment) error {
	r.calls = append(r.calls, "create")
	r.state = State{Exists: true, ID: "runtime", Managed: true, ServerID: a.ServerID, NodeID: a.NodeID, UID: a.UID, Generation: a.Generation}
	return r.err
}
func (r *memoryRuntime) Start(context.Context, State) error {
	r.calls = append(r.calls, "start")
	r.state.Running = true
	return r.err
}
func (r *memoryRuntime) Stop(context.Context, State) error {
	r.calls = append(r.calls, "stop")
	r.state.Running = false
	return r.err
}
func (r *memoryRuntime) Remove(context.Context, State) error {
	r.calls = append(r.calls, "remove")
	r.state = State{}
	return r.err
}
func assignment() workload.Assignment {
	return workload.Assignment{UID: "uid", ServerID: "server", NodeID: "node", Generation: 2, DesiredState: "running"}
}
func TestReconcileConvergesAcrossRestarts(t *testing.T) {
	r := &memoryRuntime{}
	a := assignment()
	for i := 0; i < 2; i++ {
		observation := Reconcile(context.Background(), a, r)
		if observation.LastError != "" || observation.ActualState != "running" {
			t.Fatalf("unexpected observation %+v", observation)
		}
	}
	created := 0
	for _, call := range r.calls {
		if call == "create" {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("created %d times", created)
	}
	a.DesiredState = "stopped"
	if got := Reconcile(context.Background(), a, r); got.LastError != "" || got.ActualState != "stopped" {
		t.Fatalf("stop: %+v", got)
	}
	a.DesiredState = "deleted"
	for i := 0; i < 2; i++ {
		if got := Reconcile(context.Background(), a, r); got.LastError != "" || got.ActualState != "missing" {
			t.Fatalf("delete: %+v", got)
		}
	}
}
func TestReconcileFencesForeignAndNewerWorkloads(t *testing.T) {
	for _, mode := range []string{"unmanaged", "other-server", "other-node", "other-assignment", "newer"} {
		t.Run(mode, func(t *testing.T) {
			a := assignment()
			r := &memoryRuntime{state: State{ID: "runtime", Exists: true, Managed: true, ServerID: a.ServerID, NodeID: a.NodeID, UID: a.UID, Generation: a.Generation}}
			switch mode {
			case "unmanaged":
				r.state.Managed = false
			case "other-server":
				r.state.ServerID = "other"
			case "other-node":
				r.state.NodeID = "other"
			case "other-assignment":
				r.state.UID = "other-uid"
			case "newer":
				r.state.Generation++
			}
			if got := Reconcile(context.Background(), a, r); got.LastError == "" {
				t.Fatal("unsafe workload accepted")
			}
			if len(r.calls) != 1 || r.calls[0] != "inspect" {
				t.Fatalf("unexpected mutation: %v", r.calls)
			}
		})
	}
}
func TestReconcileReplacesOldGeneration(t *testing.T) {
	a := assignment()
	r := &memoryRuntime{state: State{ID: "runtime", Exists: true, Running: true, Managed: true, ServerID: a.ServerID, NodeID: a.NodeID, UID: a.UID, Generation: 1}}
	if got := Reconcile(context.Background(), a, r); got.LastError != "" || got.ActualState != "running" {
		t.Fatalf("replace: %+v", got)
	}
	if r.state.Generation != 2 {
		t.Fatal("old generation retained")
	}
}
func TestReconcileReportsErrorsAndCancellation(t *testing.T) {
	r := &memoryRuntime{err: errors.New("daemon unavailable")}
	if got := Reconcile(context.Background(), assignment(), r); got.LastError != "daemon unavailable" || got.ActualState != "unknown" {
		t.Fatalf("failure: %+v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := Reconcile(ctx, assignment(), r); got.LastError != context.Canceled.Error() {
		t.Fatalf("cancellation: %+v", got)
	}
}

func TestObservationEchoesAssignmentTokenIncludingFailures(t *testing.T) {
	for _, runtimeErr := range []error{nil, errors.New("runtime unavailable")} {
		work := assignment()
		work.ObservationToken = "opaque-control-plane-token"
		got := Reconcile(context.Background(), work, &memoryRuntime{err: runtimeErr})
		if got.ObservationToken != work.ObservationToken {
			t.Fatalf("token not preserved: %+v", got)
		}
	}
}

type replacementRuntime struct {
	memoryRuntime
	replacement State
	replaced    bool
	target      State
}

func (r *replacementRuntime) Inspect(ctx context.Context, id string) (State, error) {
	observed, err := r.memoryRuntime.Inspect(ctx, id)
	if !r.replaced {
		r.state = r.replacement
		r.replaced = true
	}
	return observed, err
}
func (r *replacementRuntime) Start(_ context.Context, target State) error {
	r.target = target
	return errors.New("old container no longer exists")
}
func (r *replacementRuntime) Stop(_ context.Context, target State) error {
	r.target = target
	return nil
}
func (r *replacementRuntime) Remove(_ context.Context, target State) error {
	r.target = target
	return nil
}

func TestMutationRetainsObservedIdentityAcrossReplacement(t *testing.T) {
	for _, desired := range []string{"running", "stopped", "deleted"} {
		t.Run(desired, func(t *testing.T) {
			a := assignment()
			a.DesiredState = desired
			old := State{ID: "old-container", Exists: true, Managed: true, ServerID: a.ServerID, NodeID: a.NodeID, UID: a.UID, Generation: a.Generation, Running: desired != "running"}
			replacement := old
			replacement.ID = "replacement-container"
			replacement.UID = "replacement-assignment"
			runtime := &replacementRuntime{memoryRuntime: memoryRuntime{state: old}, replacement: replacement}
			got := Reconcile(context.Background(), a, runtime)
			if runtime.target.ID != old.ID {
				t.Fatalf("mutation targeted replacement or name: %+v", runtime.target)
			}
			if got.LastError == "" || got.ActualState != "unknown" {
				t.Fatalf("reported replacement as own state: %+v", got)
			}
		})
	}
}

func TestAllLifecycleIntentsRejectAnotherAssignment(t *testing.T) {
	for _, desired := range []string{"running", "stopped", "deleted"} {
		a := assignment()
		a.DesiredState = desired
		runtime := &memoryRuntime{state: State{ID: "other-container", Exists: true, Running: true, Managed: true, ServerID: a.ServerID, NodeID: a.NodeID, UID: "another-uid", Generation: 1}}
		if got := Reconcile(context.Background(), a, runtime); got.LastError == "" {
			t.Fatalf("accepted %s for another assignment", desired)
		}
		if len(runtime.calls) != 1 || runtime.calls[0] != "inspect" {
			t.Fatalf("mutated another assignment: %v", runtime.calls)
		}
	}
}

type competingCreateRuntime struct{ memoryRuntime }

func (r *competingCreateRuntime) Create(ctx context.Context, a workload.Assignment) error {
	if err := r.memoryRuntime.Create(ctx, a); err != nil {
		return err
	}
	r.state.UID = "competing-assignment"
	return nil
}
func TestCreatedContainerIsValidatedBeforeStart(t *testing.T) {
	runtime := &competingCreateRuntime{}
	got := Reconcile(context.Background(), assignment(), runtime)
	if got.LastError == "" {
		t.Fatal("accepted competing container after create")
	}
	for _, call := range runtime.calls {
		if call == "start" {
			t.Fatal("started competing container")
		}
	}
}
