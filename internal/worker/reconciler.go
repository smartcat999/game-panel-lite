// Package worker reconciles one assignment through an injected runtime.
package worker

import (
	"context"
	"errors"
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
	Start(context.Context, State) error
	Stop(context.Context, State) error
	Remove(context.Context, State) error
}

// PreparedRuntime owns verified temporary artifact bytes for one reconciliation.
// Release frees those bytes without closing the underlying runtime client.
type PreparedRuntime interface {
	Runtime
	Release() error
}
type ArtifactRuntime interface {
	ValidateArtifacts(workload.Assignment) error
	PrepareArtifacts(context.Context, workload.Assignment) (PreparedRuntime, error)
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
	if assignment.DesiredState == "running" {
		if _, err := workload.ResolvePortBindings(assignment.Spec.Network); err != nil {
			observation.LastError = err.Error()
			return observation
		}
	}
	if assignment.DesiredState == "running" && len(assignment.Spec.Options.Artifacts) > 0 {
		if err := workload.ValidateArtifacts(assignment.Spec.Options); err != nil {
			observation.LastError = err.Error()
			observation.Conditions = workload.SetCondition(observation.Conditions, workload.Condition{
				Type:               workload.ConditionArtifactsReady,
				Status:             workload.ConditionStatusFalse,
				Reason:             "ValidationFailed",
				Message:            err.Error(),
				ObservedGeneration: assignment.Generation,
				LastTransitionAt:   observation.ObservedAt,
			})
			observation.Artifacts = failedArtifactObservations(assignment.Spec.Options.Artifacts, err)
			return observation
		}
		capable, ok := runtime.(ArtifactRuntime)
		if !ok {
			observation.LastError = "runtime does not support workload artifacts"
			observation.Conditions = workload.SetCondition(observation.Conditions, workload.Condition{
				Type:               workload.ConditionArtifactsReady,
				Status:             workload.ConditionStatusFalse,
				Reason:             "RuntimeNotSupported",
				Message:            observation.LastError,
				ObservedGeneration: assignment.Generation,
				LastTransitionAt:   observation.ObservedAt,
			})
			observation.Artifacts = failedArtifactObservations(assignment.Spec.Options.Artifacts, errors.New(observation.LastError))
			return observation
		}
		if err := capable.ValidateArtifacts(assignment); err != nil {
			observation.LastError = err.Error()
			observation.Conditions = workload.SetCondition(observation.Conditions, workload.Condition{
				Type:               workload.ConditionArtifactsReady,
				Status:             workload.ConditionStatusFalse,
				Reason:             "ArtifactLimitsExceeded",
				Message:            err.Error(),
				ObservedGeneration: assignment.Generation,
				LastTransitionAt:   observation.ObservedAt,
			})
			observation.Artifacts = failedArtifactObservations(assignment.Spec.Options.Artifacts, err)
			return observation
		}
	}
	state, err := runtime.Inspect(ctx, assignment.ServerID)
	if err != nil {
		observation.LastError = err.Error()
		return observation
	}
	if err := validateObservedWorkload(state, assignment); err != nil {
		observation.LastError = err.Error()
		return observation
	}
	if assignment.DesiredState == "running" && len(assignment.Spec.Options.Artifacts) > 0 && (!state.Exists || state.Generation != assignment.Generation) {
		prepared, prepareErr := runtime.(ArtifactRuntime).PrepareArtifacts(ctx, assignment)
		if prepareErr != nil {
			observation.LastError = prepareErr.Error()
			observation.Conditions = workload.SetCondition(observation.Conditions, workload.Condition{
				Type:               workload.ConditionArtifactsReady,
				Status:             workload.ConditionStatusFalse,
				Reason:             "PreparationFailed",
				Message:            prepareErr.Error(),
				ObservedGeneration: assignment.Generation,
				LastTransitionAt:   observation.ObservedAt,
			})
			observation.Artifacts = failedArtifactObservations(assignment.Spec.Options.Artifacts, prepareErr)
			return observation
		}
		defer func() {
			if err := prepared.Release(); err != nil {
				if observation.LastError != "" {
					observation.LastError += "; "
				}
				observation.LastError += "artifact cleanup failed: " + err.Error()
			}
		}()
		runtime = prepared
		// Downloads can take time. Observe ownership again before replacing anything.
		state, err = runtime.Inspect(ctx, assignment.ServerID)
		if err != nil {
			observation.LastError = err.Error()
			return observation
		}
		if err := validateObservedWorkload(state, assignment); err != nil {
			observation.LastError = err.Error()
			return observation
		}
	}
	switch assignment.DesiredState {
	case "running":
		if state.Exists && (state.Generation != assignment.Generation) {
			if err = runtime.Remove(ctx, state); err != nil {
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
		if err = validateObservedWorkload(state, assignment); err != nil {
			break
		}
		if !state.Exists || state.Generation != assignment.Generation {
			err = fmt.Errorf("created workload is missing or has a different generation")
			break
		}
		if !state.Running {
			err = runtime.Start(ctx, state)
		}
	case "stopped":
		if state.Exists && state.Running {
			err = runtime.Stop(ctx, state)
		}
	case "deleted":
		if state.Exists {
			err = runtime.Remove(ctx, state)
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
	if err := validateObservedWorkload(state, assignment); err != nil {
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
	if assignment.DesiredState == "running" && len(assignment.Spec.Options.Artifacts) > 0 && observation.ActualState == "running" {
		observation.Conditions = workload.SetCondition(observation.Conditions, workload.Condition{
			Type:               workload.ConditionArtifactsReady,
			Status:             workload.ConditionStatusTrue,
			Reason:             "Ready",
			Message:            "Artifacts prepared and mounted successfully",
			ObservedGeneration: assignment.Generation,
			LastTransitionAt:   observation.ObservedAt,
		})
		observation.Artifacts = readyArtifactObservations(assignment.Spec.Options.Artifacts)
	}
	return observation
}

func failedArtifactObservations(artifacts []workload.Artifact, err error) []workload.ArtifactObservation {
	var target *workload.ArtifactError
	hasTarget := errors.As(err, &target)
	results := make([]workload.ArtifactObservation, 0, len(artifacts))
	failedSeen := false
	for _, item := range artifacts {
		if hasTarget {
			if !failedSeen && item.ID != target.Artifact.ID {
				results = append(results, workload.ArtifactObservation{
					ID:     item.ID,
					Path:   item.Path,
					Status: workload.ArtifactStatusReady,
				})
			} else if item.ID == target.Artifact.ID {
				failedSeen = true
				results = append(results, workload.ArtifactObservation{
					ID:     item.ID,
					Path:   item.Path,
					Status: workload.ArtifactStatusFailed,
					Error:  target.Err.Error(),
				})
			} else {
				results = append(results, workload.ArtifactObservation{
					ID:     item.ID,
					Path:   item.Path,
					Status: workload.ArtifactStatusFailed,
					Error:  "preparation aborted due to earlier failure",
				})
			}
		} else {
			results = append(results, workload.ArtifactObservation{
				ID:     item.ID,
				Path:   item.Path,
				Status: workload.ArtifactStatusFailed,
				Error:  err.Error(),
			})
		}
	}
	return results
}

func readyArtifactObservations(artifacts []workload.Artifact) []workload.ArtifactObservation {
	results := make([]workload.ArtifactObservation, 0, len(artifacts))
	for _, item := range artifacts {
		results = append(results, workload.ArtifactObservation{
			ID:     item.ID,
			Path:   item.Path,
			Status: workload.ArtifactStatusReady,
		})
	}
	return results
}

func validateObservedWorkload(state State, assignment workload.Assignment) error {
	if !state.Exists {
		return nil
	}
	if !state.Managed || state.ServerID != assignment.ServerID || state.NodeID != assignment.NodeID {
		return fmt.Errorf("existing workload is not owned by this server and node")
	}
	if state.UID != assignment.UID {
		return fmt.Errorf("existing workload belongs to another assignment")
	}
	if state.ID == "" || state.Generation <= 0 {
		return fmt.Errorf("existing workload identity is incomplete")
	}
	if state.Generation > assignment.Generation {
		return fmt.Errorf("assignment generation is older than the running workload")
	}
	return nil
}
