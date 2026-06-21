package server

import (
	"context"
	"errors"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestReconcilerNeedsReconcileForNewGeneration(t *testing.T) {
	reconciler := NewReconciler()
	server := domain.GameServer{
		Spec: domain.ServerSpec{Generation: 2, DesiredState: domain.DesiredRunning},
		Status: domain.ServerRuntimeStatus{
			ObservedGeneration: 1,
			ActualState:        domain.ActualStopped,
			Phase:              domain.PhaseStopped,
		},
	}
	if !reconciler.NeedsReconcile(server) {
		t.Fatal("expected new generation to need reconciliation")
	}
}

func TestReconcilerSkipsMatchedStoppedState(t *testing.T) {
	reconciler := NewReconciler()
	server := domain.GameServer{
		Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredStopped},
		Status: domain.ServerRuntimeStatus{
			ObservedGeneration: 1,
			ActualState:        domain.ActualStopped,
			Phase:              domain.PhaseStopped,
		},
	}
	if reconciler.NeedsReconcile(server) {
		t.Fatal("expected matched stopped state to skip reconciliation")
	}
}

type fakeBuilder struct {
	calls int
	err   error
}

func (b *fakeBuilder) BuildWorkloadSpec(_ context.Context, server domain.GameServer) (domain.WorkloadSpec, error) {
	b.calls++
	if b.err != nil {
		return domain.WorkloadSpec{}, b.err
	}
	return domain.WorkloadSpec{ServerID: server.ID, Name: server.Name, Image: "game:latest"}, nil
}

type fakeRuntime struct {
	created int
	started int
	stopped int
	removed int
	states  map[string]domain.ServerActualState
	errs    map[string]error
}

func newFakeRuntime() *fakeRuntime {
	return &fakeRuntime{states: map[string]domain.ServerActualState{}, errs: map[string]error{}}
}

func (r *fakeRuntime) Create(_ context.Context, spec domain.WorkloadSpec) (string, error) {
	if err := r.errs["create"]; err != nil {
		return "", err
	}
	r.created++
	id := "runtime-" + spec.ServerID
	r.states[id] = domain.ActualStopped
	return id, nil
}

func (r *fakeRuntime) Start(_ context.Context, runtimeID string) error {
	if err := r.errs["start"]; err != nil {
		return err
	}
	r.started++
	r.states[runtimeID] = domain.ActualRunning
	return nil
}

func (r *fakeRuntime) Stop(_ context.Context, runtimeID string) error {
	if err := r.errs["stop"]; err != nil {
		return err
	}
	r.stopped++
	r.states[runtimeID] = domain.ActualStopped
	return nil
}

func (r *fakeRuntime) Remove(_ context.Context, runtimeID string) error {
	if err := r.errs["remove"]; err != nil {
		return err
	}
	r.removed++
	if _, ok := r.states[runtimeID]; !ok {
		return ErrWorkloadNotFound
	}
	delete(r.states, runtimeID)
	return nil
}

func (r *fakeRuntime) Inspect(_ context.Context, runtimeID string) (domain.WorkloadStatus, error) {
	if err := r.errs["inspect"]; err != nil {
		return domain.WorkloadStatus{}, err
	}
	state, ok := r.states[runtimeID]
	if !ok {
		return domain.WorkloadStatus{}, ErrWorkloadNotFound
	}
	return domain.WorkloadStatus{RuntimeID: runtimeID, State: state}, nil
}

type fakeImageLoader struct {
	calls int
	err   error
}

func (l *fakeImageLoader) EnsureImage(context.Context, domain.GameServer, string) error {
	l.calls++
	return l.err
}

func TestReconcileRunningCreatesAndStartsWorkload(t *testing.T) {
	builder := &fakeBuilder{}
	runtime := newFakeRuntime()
	reconciler := NewRuntimeReconciler(builder, runtime)
	server := domain.GameServer{
		ID:   "srv-1",
		Name: "Friends",
		Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning},
		Status: domain.ServerRuntimeStatus{
			Phase:       domain.PhasePending,
			ActualState: domain.ActualMissing,
		},
	}

	updated, err := reconciler.Reconcile(context.Background(), server)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if updated.Status.Phase != domain.PhaseRunning {
		t.Fatalf("expected running phase, got %q", updated.Status.Phase)
	}
	if updated.Status.RuntimeID == "" {
		t.Fatal("expected runtime ID")
	}
	if updated.Status.AppliedGeneration != 1 || updated.Status.ObservedGeneration != 1 {
		t.Fatalf("expected generation applied and observed, got %+v", updated.Status)
	}
	if builder.calls != 1 || runtime.created != 1 || runtime.started != 1 {
		t.Fatalf("expected one build/create/start, got builder=%d create=%d start=%d", builder.calls, runtime.created, runtime.started)
	}
}

func TestReconcileRunningRecordsLifecycleEvents(t *testing.T) {
	builder := &fakeBuilder{}
	runtime := newFakeRuntime()
	images := &fakeImageLoader{}
	reconciler := NewRuntimeReconciler(builder, runtime).WithImageLoader(images)
	server := domain.GameServer{
		ID:   "srv-events",
		Name: "Friends",
		Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning},
		Status: domain.ServerRuntimeStatus{
			Phase:       domain.PhasePending,
			ActualState: domain.ActualMissing,
		},
	}

	updated, events, err := reconciler.ReconcileWithEvents(context.Background(), server)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if updated.Status.Phase != domain.PhaseRunning {
		t.Fatalf("expected running phase, got %q", updated.Status.Phase)
	}
	want := []string{
		"server.image.load.started",
		"server.image.load.succeeded",
		"server.container.create.started",
		"server.container.create.succeeded",
		"server.container.start.started",
		"server.container.start.succeeded",
	}
	if len(events) != len(want) {
		t.Fatalf("expected lifecycle events %v, got %+v", want, events)
	}
	for index, eventType := range want {
		if events[index].Type != eventType {
			t.Fatalf("expected event %d to be %q, got %+v", index, eventType, events[index])
		}
	}
	if events[0].Payload["image"] != "game:latest" || events[3].Payload["runtimeId"] == "" || events[5].Payload["runtimeId"] == "" {
		t.Fatalf("expected lifecycle payload details, got %+v", events)
	}
}

func TestReconcileRunningFailsWhenImageLoadFails(t *testing.T) {
	builder := &fakeBuilder{}
	runtime := newFakeRuntime()
	images := &fakeImageLoader{err: errors.New("archive missing")}
	reconciler := NewRuntimeReconciler(builder, runtime).WithImageLoader(images)
	server := domain.GameServer{
		ID:   "srv-image",
		Name: "Image Missing",
		Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning},
		Status: domain.ServerRuntimeStatus{
			Phase:       domain.PhasePending,
			ActualState: domain.ActualMissing,
		},
	}

	updated, err := reconciler.Reconcile(context.Background(), server)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if updated.Status.Phase != domain.PhaseFailed {
		t.Fatalf("expected failed phase, got %q", updated.Status.Phase)
	}
	if updated.Status.LastError != "archive missing" {
		t.Fatalf("expected image load error, got %q", updated.Status.LastError)
	}
	if len(updated.Status.Conditions) != 1 || updated.Status.Conditions[0].Reason != "LoadImageFailed" {
		t.Fatalf("expected image load failure condition, got %+v", updated.Status.Conditions)
	}
	if images.calls != 1 || runtime.created != 0 || runtime.started != 0 {
		t.Fatalf("expected image load before create/start, got image=%d create=%d start=%d", images.calls, runtime.created, runtime.started)
	}
}

func TestReconcileRunningRecreatesWhenGenerationChanged(t *testing.T) {
	builder := &fakeBuilder{}
	runtime := newFakeRuntime()
	runtime.states["old-runtime"] = domain.ActualRunning
	reconciler := NewRuntimeReconciler(builder, runtime)
	server := domain.GameServer{
		ID:   "srv-1",
		Name: "Friends",
		Spec: domain.ServerSpec{Generation: 2, DesiredState: domain.DesiredRunning},
		Status: domain.ServerRuntimeStatus{
			Phase:              domain.PhaseRunning,
			ActualState:        domain.ActualRunning,
			RuntimeID:          "old-runtime",
			AppliedGeneration:  1,
			ObservedGeneration: 1,
		},
	}

	updated, err := reconciler.Reconcile(context.Background(), server)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if updated.Status.RuntimeID == "old-runtime" {
		t.Fatal("expected runtime to be recreated")
	}
	if runtime.stopped != 1 || runtime.removed != 1 || runtime.created != 1 || runtime.started != 1 {
		t.Fatalf("expected stop/remove/create/start, got stop=%d remove=%d create=%d start=%d", runtime.stopped, runtime.removed, runtime.created, runtime.started)
	}
	if updated.Status.AppliedGeneration != 2 {
		t.Fatalf("expected applied generation 2, got %d", updated.Status.AppliedGeneration)
	}
}

func TestReconcileStoppedStopsRunningWorkload(t *testing.T) {
	runtime := newFakeRuntime()
	runtime.states["runtime-1"] = domain.ActualRunning
	reconciler := NewRuntimeReconciler(&fakeBuilder{}, runtime)
	server := domain.GameServer{
		Spec: domain.ServerSpec{Generation: 3, DesiredState: domain.DesiredStopped},
		Status: domain.ServerRuntimeStatus{
			Phase:     domain.PhaseRunning,
			RuntimeID: "runtime-1",
		},
	}

	updated, err := reconciler.Reconcile(context.Background(), server)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if updated.Status.Phase != domain.PhaseStopped {
		t.Fatalf("expected stopped phase, got %q", updated.Status.Phase)
	}
	if runtime.stopped != 1 {
		t.Fatalf("expected one stop, got %d", runtime.stopped)
	}
}

func TestReconcileDeletedRemovesWorkload(t *testing.T) {
	runtime := newFakeRuntime()
	runtime.states["runtime-1"] = domain.ActualRunning
	reconciler := NewRuntimeReconciler(&fakeBuilder{}, runtime)
	server := domain.GameServer{
		Spec: domain.ServerSpec{Generation: 4, DesiredState: domain.DesiredDeleted},
		Status: domain.ServerRuntimeStatus{
			Phase:     domain.PhaseDeleting,
			RuntimeID: "runtime-1",
		},
	}

	updated, err := reconciler.Reconcile(context.Background(), server)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if updated.Status.Phase != domain.PhaseDeleted {
		t.Fatalf("expected deleted phase, got %q", updated.Status.Phase)
	}
	if updated.Status.RuntimeID != "" {
		t.Fatalf("expected runtime id cleared, got %q", updated.Status.RuntimeID)
	}
	if runtime.stopped != 1 || runtime.removed != 1 {
		t.Fatalf("expected stop/remove, got stop=%d remove=%d", runtime.stopped, runtime.removed)
	}
}

func TestReconcileFailureSetsFailedCondition(t *testing.T) {
	builder := &fakeBuilder{err: errors.New("bad config")}
	reconciler := NewRuntimeReconciler(builder, newFakeRuntime())
	server := domain.GameServer{
		Spec:   domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning},
		Status: domain.ServerRuntimeStatus{Phase: domain.PhasePending},
	}

	updated, err := reconciler.Reconcile(context.Background(), server)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if updated.Status.Phase != domain.PhaseFailed {
		t.Fatalf("expected failed phase, got %q", updated.Status.Phase)
	}
	if updated.Status.LastError != "bad config" {
		t.Fatalf("expected last error, got %q", updated.Status.LastError)
	}
	if len(updated.Status.Conditions) != 1 || updated.Status.Conditions[0].Reason != "BuildSpecFailed" {
		t.Fatalf("expected build failure condition, got %+v", updated.Status.Conditions)
	}
}
