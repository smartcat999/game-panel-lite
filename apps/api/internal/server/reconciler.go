package server

import (
	"context"
	"errors"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

type WorkloadBuilder interface {
	BuildWorkloadSpec(context.Context, domain.GameServer) (domain.WorkloadSpec, error)
}

type RuntimeClient interface {
	Create(context.Context, domain.WorkloadSpec) (string, error)
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Remove(context.Context, string) error
	Inspect(context.Context, string) (domain.WorkloadStatus, error)
}

type ImageLoader interface {
	EnsureImage(context.Context, domain.GameServer, string) error
}

type LifecycleEvent struct {
	Type       string
	Message    string
	Payload    map[string]any
	OccurredAt time.Time
}

type Reconciler struct {
	builder WorkloadBuilder
	runtime RuntimeClient
	images  ImageLoader
	now     Clock
}

func NewReconciler() *Reconciler {
	return &Reconciler{}
}

func NewRuntimeReconciler(builder WorkloadBuilder, runtime RuntimeClient) *Reconciler {
	return &Reconciler{builder: builder, runtime: runtime, now: time.Now}
}

func (r *Reconciler) WithImageLoader(loader ImageLoader) *Reconciler {
	r.images = loader
	return r
}

func (r *Reconciler) NeedsReconcile(server domain.GameServer) bool {
	if server.Status.ObservedGeneration < server.Spec.Generation {
		return true
	}
	switch server.Status.Phase {
	case domain.PhasePending, domain.PhaseReconciling, domain.PhaseDeleting:
		return true
	case domain.PhaseFailed:
		return server.Status.ObservedGeneration < server.Spec.Generation || server.Spec.DesiredState == domain.DesiredDeleted
	default:
		return desiredMatchesActual(server.Spec.DesiredState, server.Status.ActualState) == false
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, server domain.GameServer) (domain.GameServer, error) {
	updated, _, err := r.ReconcileWithEvents(ctx, server)
	return updated, err
}

func (r *Reconciler) ReconcileWithEvents(ctx context.Context, server domain.GameServer) (domain.GameServer, []LifecycleEvent, error) {
	if err := ctx.Err(); err != nil {
		return server, nil, err
	}
	recorder := &lifecycleRecorder{server: server}
	now := r.clock()
	server.Status.LastReconcileAt = now
	if r.builder == nil || r.runtime == nil {
		return server, nil, nil
	}
	setPhase(&server.Status, domain.PhaseReconciling, now)

	var updated domain.GameServer
	var err error
	switch server.Spec.DesiredState {
	case domain.DesiredRunning:
		updated, err = r.reconcileRunning(ctx, server, now, recorder)
	case domain.DesiredStopped:
		updated, err = r.reconcileStopped(ctx, server, now, recorder)
	case domain.DesiredDeleted:
		updated, err = r.reconcileDeleted(ctx, server, now, recorder)
	default:
		updated, err = r.markFailed(server, now, "InvalidDesiredState", "unsupported desired state"), nil
	}
	return updated, recorder.events, err
}

func desiredMatchesActual(desired domain.ServerDesiredState, actual domain.ServerActualState) bool {
	switch desired {
	case domain.DesiredRunning:
		return actual == domain.ActualRunning
	case domain.DesiredStopped:
		return actual == domain.ActualStopped || actual == domain.ActualMissing
	case domain.DesiredDeleted:
		return false
	default:
		return false
	}
}

func (r *Reconciler) reconcileRunning(ctx context.Context, server domain.GameServer, now time.Time, recorder *lifecycleRecorder) (domain.GameServer, error) {
	needsRecreate := server.Status.RuntimeID == "" || server.Status.AppliedGeneration < server.Spec.Generation
	if !needsRecreate {
		status, err := r.runtime.Inspect(ctx, server.Status.RuntimeID)
		if err != nil {
			if !errors.Is(err, ErrWorkloadNotFound) {
				recorder.failed("server.container.inspect.failed", "Inspect runtime container", server.Status.RuntimeID, "", err)
				return r.markFailed(server, now, "InspectFailed", err.Error()), nil
			}
			needsRecreate = true
			server.Status.RuntimeID = ""
			server.Status.ActualState = domain.ActualMissing
		} else if status.State == domain.ActualRunning {
			return r.markRunning(server, now), nil
		}
	}
	if needsRecreate && server.Status.RuntimeID != "" {
		if err := r.removeWorkload(ctx, server.Status.RuntimeID, recorder); err != nil {
			return r.markFailed(server, now, "RemoveFailed", err.Error()), nil
		}
		server.Status.RuntimeID = ""
	}
	if server.Status.RuntimeID == "" {
		spec, err := r.builder.BuildWorkloadSpec(ctx, server)
		if err != nil {
			recorder.failed("server.container.prepare.failed", "Build runtime container spec", "", "", err)
			return r.markFailed(server, now, "BuildSpecFailed", err.Error()), nil
		}
		if r.images != nil {
			recorder.event("server.image.load.started", "Load runtime image for server "+server.Name, map[string]any{"image": spec.Image})
			if err := r.images.EnsureImage(ctx, server, spec.Image); err != nil {
				recorder.failed("server.image.load.failed", "Load runtime image", "", spec.Image, err)
				return r.markFailed(server, now, "LoadImageFailed", err.Error()), nil
			}
			recorder.event("server.image.load.succeeded", "Loaded runtime image for server "+server.Name, map[string]any{"image": spec.Image})
		}
		recorder.event("server.container.create.started", "Create runtime container for server "+server.Name, map[string]any{"image": spec.Image})
		runtimeID, err := r.runtime.Create(ctx, spec)
		if err != nil {
			recorder.failed("server.container.create.failed", "Create runtime container", "", spec.Image, err)
			return r.markFailed(server, now, "CreateFailed", err.Error()), nil
		}
		server.Status.RuntimeID = runtimeID
		recorder.event("server.container.create.succeeded", "Created runtime container for server "+server.Name, map[string]any{"image": spec.Image, "runtimeId": runtimeID})
	}
	recorder.event("server.container.start.started", "Start runtime container for server "+server.Name, map[string]any{"runtimeId": server.Status.RuntimeID})
	if err := r.runtime.Start(ctx, server.Status.RuntimeID); err != nil {
		recorder.failed("server.container.start.failed", "Start runtime container", server.Status.RuntimeID, "", err)
		return r.markFailed(server, now, "StartFailed", err.Error()), nil
	}
	recorder.event("server.container.start.succeeded", "Started runtime container for server "+server.Name, map[string]any{"runtimeId": server.Status.RuntimeID})
	return r.markRunning(server, now), nil
}

func (r *Reconciler) reconcileStopped(ctx context.Context, server domain.GameServer, now time.Time, recorder *lifecycleRecorder) (domain.GameServer, error) {
	if server.Status.RuntimeID != "" {
		if server.Status.ActualState == domain.ActualRunning {
			recorder.event("server.container.stop.started", "Stop runtime container for server "+server.Name, map[string]any{"runtimeId": server.Status.RuntimeID})
			if err := r.runtime.Stop(ctx, server.Status.RuntimeID); err != nil {
				recorder.failed("server.container.stop.failed", "Stop runtime container", server.Status.RuntimeID, "", err)
				return r.markFailed(server, now, "StopFailed", err.Error()), nil
			}
			recorder.event("server.container.stop.succeeded", "Stopped runtime container for server "+server.Name, map[string]any{"runtimeId": server.Status.RuntimeID})
			server.Status.ActualState = domain.ActualStopped
			server.Status.ObservedGeneration = server.Spec.Generation
			setPhase(&server.Status, domain.PhaseStopped, now)
			return server, nil
		}
		status, err := r.runtime.Inspect(ctx, server.Status.RuntimeID)
		if err != nil {
			if !errors.Is(err, ErrWorkloadNotFound) {
				recorder.failed("server.container.inspect.failed", "Inspect runtime container", server.Status.RuntimeID, "", err)
				return r.markFailed(server, now, "InspectFailed", err.Error()), nil
			}
			server.Status.RuntimeID = ""
			server.Status.ActualState = domain.ActualMissing
		} else if status.State == domain.ActualRunning {
			recorder.event("server.container.stop.started", "Stop runtime container for server "+server.Name, map[string]any{"runtimeId": server.Status.RuntimeID})
			if err := r.runtime.Stop(ctx, server.Status.RuntimeID); err != nil {
				recorder.failed("server.container.stop.failed", "Stop runtime container", server.Status.RuntimeID, "", err)
				return r.markFailed(server, now, "StopFailed", err.Error()), nil
			}
			recorder.event("server.container.stop.succeeded", "Stopped runtime container for server "+server.Name, map[string]any{"runtimeId": server.Status.RuntimeID})
			server.Status.ActualState = domain.ActualStopped
		} else {
			server.Status.ActualState = status.State
		}
	} else {
		server.Status.ActualState = domain.ActualMissing
	}
	server.Status.ObservedGeneration = server.Spec.Generation
	setPhase(&server.Status, domain.PhaseStopped, now)
	return server, nil
}

func (r *Reconciler) reconcileDeleted(ctx context.Context, server domain.GameServer, now time.Time, recorder *lifecycleRecorder) (domain.GameServer, error) {
	if server.Status.RuntimeID != "" {
		if err := r.removeWorkload(ctx, server.Status.RuntimeID, recorder); err != nil {
			return r.markFailed(server, now, "DeleteFailed", err.Error()), nil
		}
	}
	server.Status.RuntimeID = ""
	server.Status.ActualState = domain.ActualMissing
	server.Status.ObservedGeneration = server.Spec.Generation
	setPhase(&server.Status, domain.PhaseDeleted, now)
	return server, nil
}

func (r *Reconciler) removeWorkload(ctx context.Context, runtimeID string, recorder *lifecycleRecorder) error {
	status, err := r.runtime.Inspect(ctx, runtimeID)
	if errors.Is(err, ErrWorkloadNotFound) {
		return nil
	}
	if err != nil {
		recorder.failed("server.container.inspect.failed", "Inspect runtime container", runtimeID, "", err)
		recorder.event("server.container.stop.started", "Stop runtime container for server "+recorder.server.Name, map[string]any{"runtimeId": runtimeID})
		if stopErr := r.runtime.Stop(ctx, runtimeID); stopErr != nil && !errors.Is(stopErr, ErrWorkloadNotFound) {
			recorder.failed("server.container.stop.failed", "Stop runtime container", runtimeID, "", stopErr)
			return stopErr
		}
	} else if status.State != domain.ActualMissing {
		recorder.event("server.container.stop.started", "Stop runtime container for server "+recorder.server.Name, map[string]any{"runtimeId": runtimeID})
		if stopErr := r.runtime.Stop(ctx, runtimeID); stopErr != nil && !errors.Is(stopErr, ErrWorkloadNotFound) {
			recorder.failed("server.container.stop.failed", "Stop runtime container", runtimeID, "", stopErr)
			return stopErr
		}
		recorder.event("server.container.stop.succeeded", "Stopped runtime container for server "+recorder.server.Name, map[string]any{"runtimeId": runtimeID})
	}
	recorder.event("server.container.remove.started", "Remove runtime container for server "+recorder.server.Name, map[string]any{"runtimeId": runtimeID})
	if err := r.runtime.Remove(ctx, runtimeID); err != nil && !errors.Is(err, ErrWorkloadNotFound) {
		recorder.failed("server.container.remove.failed", "Remove runtime container", runtimeID, "", err)
		return err
	}
	recorder.event("server.container.remove.succeeded", "Removed runtime container for server "+recorder.server.Name, map[string]any{"runtimeId": runtimeID})
	return nil
}

func (r *Reconciler) markRunning(server domain.GameServer, now time.Time) domain.GameServer {
	server.Status.ActualState = domain.ActualRunning
	server.Status.ObservedGeneration = server.Spec.Generation
	server.Status.AppliedGeneration = server.Spec.Generation
	server.Status.LastError = ""
	setPhase(&server.Status, domain.PhaseRunning, now)
	return server
}

type lifecycleRecorder struct {
	server domain.GameServer
	events []LifecycleEvent
}

func (r *lifecycleRecorder) event(eventType string, message string, payload map[string]any) {
	if r == nil {
		return
	}
	next := map[string]any{}
	for key, value := range payload {
		next[key] = value
	}
	r.events = append(r.events, LifecycleEvent{Type: eventType, Message: message, Payload: next, OccurredAt: time.Now().UTC()})
}

func (r *lifecycleRecorder) failed(eventType string, action string, runtimeID string, image string, err error) {
	if r == nil || err == nil {
		return
	}
	payload := map[string]any{
		"action": action,
		"error":  err.Error(),
	}
	if runtimeID != "" {
		payload["runtimeId"] = runtimeID
	}
	if image != "" {
		payload["image"] = image
	}
	r.event(eventType, action+" failed for server "+r.server.Name+": "+err.Error(), payload)
}

func (r *Reconciler) markFailed(server domain.GameServer, now time.Time, reason string, message string) domain.GameServer {
	server.Status.LastError = message
	server.Status.Conditions = setCondition(server.Status.Conditions, domain.ServerCondition{
		Type:               "ReconcileReady",
		Status:             "False",
		Reason:             reason,
		Message:            message,
		ObservedGeneration: server.Spec.Generation,
		LastTransitionAt:   now,
	})
	setPhase(&server.Status, domain.PhaseFailed, now)
	return server
}

func (r *Reconciler) clock() time.Time {
	if r.now == nil {
		return time.Now()
	}
	return r.now()
}

func setCondition(conditions []domain.ServerCondition, condition domain.ServerCondition) []domain.ServerCondition {
	for index := range conditions {
		if conditions[index].Type == condition.Type {
			conditions[index] = condition
			return conditions
		}
	}
	return append(conditions, condition)
}
