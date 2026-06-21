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

type Reconciler struct {
	builder WorkloadBuilder
	runtime RuntimeClient
	now     Clock
}

func NewReconciler() *Reconciler {
	return &Reconciler{}
}

func NewRuntimeReconciler(builder WorkloadBuilder, runtime RuntimeClient) *Reconciler {
	return &Reconciler{builder: builder, runtime: runtime, now: time.Now}
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
	if err := ctx.Err(); err != nil {
		return server, err
	}
	now := r.clock()
	server.Status.LastReconcileAt = now
	if r.builder == nil || r.runtime == nil {
		return server, nil
	}
	setPhase(&server.Status, domain.PhaseReconciling, now)

	switch server.Spec.DesiredState {
	case domain.DesiredRunning:
		return r.reconcileRunning(ctx, server, now)
	case domain.DesiredStopped:
		return r.reconcileStopped(ctx, server, now)
	case domain.DesiredDeleted:
		return r.reconcileDeleted(ctx, server, now)
	default:
		return r.markFailed(server, now, "InvalidDesiredState", "unsupported desired state"), nil
	}
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

func (r *Reconciler) reconcileRunning(ctx context.Context, server domain.GameServer, now time.Time) (domain.GameServer, error) {
	needsRecreate := server.Status.RuntimeID == "" || server.Status.AppliedGeneration < server.Spec.Generation
	if !needsRecreate {
		status, err := r.runtime.Inspect(ctx, server.Status.RuntimeID)
		if err != nil {
			if !errors.Is(err, ErrWorkloadNotFound) {
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
		if err := r.removeWorkload(ctx, server.Status.RuntimeID); err != nil {
			return r.markFailed(server, now, "RemoveFailed", err.Error()), nil
		}
		server.Status.RuntimeID = ""
	}
	if server.Status.RuntimeID == "" {
		spec, err := r.builder.BuildWorkloadSpec(ctx, server)
		if err != nil {
			return r.markFailed(server, now, "BuildSpecFailed", err.Error()), nil
		}
		runtimeID, err := r.runtime.Create(ctx, spec)
		if err != nil {
			return r.markFailed(server, now, "CreateFailed", err.Error()), nil
		}
		server.Status.RuntimeID = runtimeID
	}
	if err := r.runtime.Start(ctx, server.Status.RuntimeID); err != nil {
		return r.markFailed(server, now, "StartFailed", err.Error()), nil
	}
	return r.markRunning(server, now), nil
}

func (r *Reconciler) reconcileStopped(ctx context.Context, server domain.GameServer, now time.Time) (domain.GameServer, error) {
	if server.Status.RuntimeID != "" {
		if server.Status.ActualState == domain.ActualRunning {
			if err := r.runtime.Stop(ctx, server.Status.RuntimeID); err != nil {
				return r.markFailed(server, now, "StopFailed", err.Error()), nil
			}
			server.Status.ActualState = domain.ActualStopped
			server.Status.ObservedGeneration = server.Spec.Generation
			setPhase(&server.Status, domain.PhaseStopped, now)
			return server, nil
		}
		status, err := r.runtime.Inspect(ctx, server.Status.RuntimeID)
		if err != nil {
			if !errors.Is(err, ErrWorkloadNotFound) {
				return r.markFailed(server, now, "InspectFailed", err.Error()), nil
			}
			server.Status.RuntimeID = ""
			server.Status.ActualState = domain.ActualMissing
		} else if status.State == domain.ActualRunning {
			if err := r.runtime.Stop(ctx, server.Status.RuntimeID); err != nil {
				return r.markFailed(server, now, "StopFailed", err.Error()), nil
			}
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

func (r *Reconciler) reconcileDeleted(ctx context.Context, server domain.GameServer, now time.Time) (domain.GameServer, error) {
	if server.Status.RuntimeID != "" {
		if err := r.removeWorkload(ctx, server.Status.RuntimeID); err != nil {
			return r.markFailed(server, now, "DeleteFailed", err.Error()), nil
		}
	}
	server.Status.RuntimeID = ""
	server.Status.ActualState = domain.ActualMissing
	server.Status.ObservedGeneration = server.Spec.Generation
	setPhase(&server.Status, domain.PhaseDeleted, now)
	return server, nil
}

func (r *Reconciler) removeWorkload(ctx context.Context, runtimeID string) error {
	status, err := r.runtime.Inspect(ctx, runtimeID)
	if errors.Is(err, ErrWorkloadNotFound) {
		return nil
	}
	if err != nil {
		if stopErr := r.runtime.Stop(ctx, runtimeID); stopErr != nil && !errors.Is(stopErr, ErrWorkloadNotFound) {
			return stopErr
		}
	} else if status.State != domain.ActualMissing {
		if stopErr := r.runtime.Stop(ctx, runtimeID); stopErr != nil && !errors.Is(stopErr, ErrWorkloadNotFound) {
			return stopErr
		}
	}
	if err := r.runtime.Remove(ctx, runtimeID); err != nil && !errors.Is(err, ErrWorkloadNotFound) {
		return err
	}
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
