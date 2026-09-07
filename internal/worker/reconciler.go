// Package worker reconciles one assignment through an injected runtime.
package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/smartcat999/game-panel-lite/internal/workload"
)

type Container struct {
	ID       string
	ServerID string
}

type State struct {
	Exists     bool
	ID         string
	Running    bool
	Managed    bool
	ServerID   string
	NodeID     string
	UID        string
	Generation int
}

type Runtime interface {
	Inspect(context.Context, string) (State, error)
	Create(context.Context, workload.Assignment) error
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Remove(context.Context, string) error
}

// Reconcile observes after every mutation and returns an observation even when
// an operation fails. The caller reports it and retries from current state.
func Reconcile(ctx context.Context, assignment workload.Assignment, runtime Runtime) (observation workload.Observation) {
	started := time.Now()
	defer func() { observation.ReconcileDurationSeconds = time.Since(started).Seconds() }()
	observation = workload.Observation{ObservationToken: assignment.ObservationToken, ObservedGeneration: assignment.Generation, ActualState: "unknown", ObservedAt: time.Now().UTC()}
	if assignment.UID == "" || assignment.NodeID == "" || assignment.ServerID == "" || assignment.Generation <= 0 {
		observation.LastError = "assignment identity and positive generation are required"
		return observation
	}
	if assignment.Spec.ServerID != "" && assignment.Spec.ServerID != assignment.ServerID {
		observation.LastError = "workload server ID does not match assignment"
		return observation
	}
	state, err := runtime.Inspect(ctx, assignment.ServerID)
	if err != nil {
		observation.LastError = err.Error()
		return observation
	}
	if state.Exists && (!state.Managed || state.ServerID != assignment.ServerID || state.NodeID != assignment.NodeID) {
		observation.LastError = "existing workload is not owned by this server and node"
		return observation
	}
	if state.Exists && state.UID == assignment.UID && state.Generation > assignment.Generation {
		observation.LastError = "assignment generation is older than the running workload"
		return observation
	}
	switch assignment.DesiredState {
	case "running":
		if state.Exists && (state.UID != assignment.UID || state.Generation != assignment.Generation) {
			if err = runtime.Remove(ctx, assignment.ServerID); err != nil {
				break
			}
			state = State{}
		}
		if !state.Exists {
			if err = runtime.Create(ctx, assignment); err != nil {
				break
			}
			state, err = runtime.Inspect(ctx, assignment.ServerID)
			if err != nil {
				break
			}
		}
		if !state.Running {
			err = runtime.Start(ctx, assignment.ServerID)
		}
	case "stopped":
		if state.Exists && state.Running {
			err = runtime.Stop(ctx, assignment.ServerID)
		}
	case "deleted":
		if state.Exists {
			err = runtime.Remove(ctx, assignment.ServerID)
		}
	default:
		err = fmt.Errorf("unsupported desired state %q", assignment.DesiredState)
	}
	if err != nil {
		observation.LastError = err.Error()
		return observation
	}
	state, err = runtime.Inspect(ctx, assignment.ServerID)
	if err != nil {
		observation.LastError = err.Error()
		return observation
	}
	observation.RuntimeID = state.ID
	switch {
	case !state.Exists:
		observation.ActualState = "missing"
	case state.Running:
		observation.ActualState = "running"
	default:
		observation.ActualState = "stopped"
	}
	return observation
}
