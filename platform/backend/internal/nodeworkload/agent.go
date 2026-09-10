package nodeworkload

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instanceaction"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionaldelivery"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionaltask"
)

type AssignmentStore interface {
	ClaimAssignment(context.Context, string, string, time.Time) (regionaldelivery.Assignment, bool, error)
	ClaimObservation(context.Context, string, time.Time) (regionaldelivery.Assignment, bool, error)
	CompleteAssignment(context.Context, string, string, int64, bool, string, time.Time) (bool, error)
	State(context.Context, string) (regionaldelivery.State, error)
}

type TaskStore interface {
	Claim(context.Context, string, string, time.Time) (regionaltask.Task, bool, error)
	Complete(context.Context, string, string, int64, regionaltask.BackupResult, time.Time) (bool, error)
}

type ManifestRegistry interface {
	Verified(context.Context, string) (providercontract.Manifest, error)
}

type Agent struct {
	NodeID      string
	WorkerID    string
	Assignments AssignmentStore
	Tasks       TaskStore
	Registry    ManifestRegistry
	Providers   map[string]GameProvider
	Runtime     RuntimeProvider
	Telemetry   TelemetrySink
	Management  []string
	Clock       func() time.Time
}

func (a *Agent) Tick(ctx context.Context) error {
	if a.NodeID == "" || a.WorkerID == "" || a.Assignments == nil || a.Registry == nil || a.Runtime == nil || a.Telemetry == nil {
		return ErrInvalidWorkload
	}
	now := time.Now().UTC()
	if a.Clock != nil {
		now = a.Clock().UTC()
	}
	assignment, ok, err := a.Assignments.ClaimAssignment(ctx, a.NodeID, a.WorkerID, now)
	if err != nil {
		return err
	}
	if ok {
		return a.executeAssignment(ctx, assignment, now)
	}
	if a.Tasks != nil {
		task, taskOK, taskErr := a.Tasks.Claim(ctx, a.NodeID, a.WorkerID, now)
		if taskErr != nil {
			return taskErr
		}
		if taskOK {
			return a.executeTask(ctx, task, now)
		}
	}
	observation, ok, err := a.Assignments.ClaimObservation(ctx, a.NodeID, now)
	if err != nil || !ok {
		return err
	}
	return a.observeAssignment(ctx, observation, now)
}

func (a *Agent) Run(ctx context.Context) error {
	backoff := 250 * time.Millisecond
	for {
		if err := a.Tick(ctx); err != nil {
			slog.Warn("node workload tick failed", "nodeId", a.NodeID, "error", err)
			if !wait(ctx, backoff) {
				return ctx.Err()
			}
			if backoff < 5*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = 250 * time.Millisecond
		if !wait(ctx, backoff) {
			return ctx.Err()
		}
	}
}

func (a *Agent) executeAssignment(ctx context.Context, assignment regionaldelivery.Assignment, now time.Time) error {
	manifest, err := a.Registry.Verified(ctx, assignment.ProviderReleaseID)
	if err != nil {
		return a.failAssignment(ctx, assignment, "provider_release_invalid", now, err)
	}
	provider := a.Providers[assignment.ProviderReleaseID]
	if provider == nil {
		return a.failAssignment(ctx, assignment, "provider_not_installed", now, ErrInvalidWorkload)
	}
	module := New(provider, a.Runtime, a.Telemetry, a.Management)
	result, err := module.Reconcile(ctx, manifest, Intent{WorkspaceID: assignment.WorkspaceID, LogicalInstanceID: assignment.LogicalInstanceID, RegionID: assignment.RegionID, RegionalDeploymentID: assignment.RegionalDeliveryID, ProviderReleaseID: assignment.ProviderReleaseID, GameVersion: assignment.GameVersion, DesiredState: assignment.DesiredState, ApplyBehavior: assignment.ApplyBehavior, CPUMilli: assignment.ResourceSpec.CPUMilli, MemoryMiB: assignment.ResourceSpec.MemoryMiB, DiskGiB: assignment.ResourceSpec.DiskGiB, FencingToken: assignment.FencingToken, Configuration: assignment.Configuration, ModLock: assignment.ModLock, Endpoints: assignment.Endpoints, DataScope: "instances/" + assignment.LogicalInstanceID}, assignment.TelemetrySequence+1, now)
	if err != nil {
		return a.failAssignment(ctx, assignment, "runtime_reconcile_failed", now, err)
	}
	if result.State != assignment.DesiredState {
		return nil
	}
	_, completeErr := a.Assignments.CompleteAssignment(ctx, assignment.RegionalDeliveryID, a.WorkerID, assignment.FencingToken, true, "", now)
	return completeErr
}

func (a *Agent) failAssignment(ctx context.Context, assignment regionaldelivery.Assignment, code string, now time.Time, cause error) error {
	if assignment.Attempt < 5 {
		return cause
	}
	_, err := a.Assignments.CompleteAssignment(ctx, assignment.RegionalDeliveryID, a.WorkerID, assignment.FencingToken, false, code, now)
	return errors.Join(cause, err)
}

func (a *Agent) observeAssignment(ctx context.Context, assignment regionaldelivery.Assignment, now time.Time) error {
	manifest, err := a.Registry.Verified(ctx, assignment.ProviderReleaseID)
	if err != nil {
		return err
	}
	provider := a.Providers[assignment.ProviderReleaseID]
	if provider == nil {
		return ErrInvalidWorkload
	}
	module := New(provider, a.Runtime, a.Telemetry, a.Management)
	_, err = module.Reconcile(ctx, manifest, Intent{WorkspaceID: assignment.WorkspaceID, LogicalInstanceID: assignment.LogicalInstanceID, RegionID: assignment.RegionID, RegionalDeploymentID: assignment.RegionalDeliveryID, ProviderReleaseID: assignment.ProviderReleaseID, GameVersion: assignment.GameVersion, DesiredState: assignment.DesiredState, ApplyBehavior: assignment.ApplyBehavior, CPUMilli: assignment.ResourceSpec.CPUMilli, MemoryMiB: assignment.ResourceSpec.MemoryMiB, DiskGiB: assignment.ResourceSpec.DiskGiB, FencingToken: assignment.FencingToken, Configuration: assignment.Configuration, ModLock: assignment.ModLock, Endpoints: assignment.Endpoints, DataScope: "instances/" + assignment.LogicalInstanceID}, assignment.TelemetrySequence+1, now)
	return err
}

func (a *Agent) executeTask(ctx context.Context, task regionaltask.Task, now time.Time) error {
	state, err := a.Assignments.State(ctx, task.LogicalInstanceID)
	if err != nil || state.FencingToken != task.FencingToken || state.NodeID != a.NodeID {
		return errors.Join(ErrInvalidWorkload, err)
	}
	manifest, err := a.Registry.Verified(ctx, state.ProviderReleaseID)
	if err != nil {
		return err
	}
	provider := a.Providers[state.ProviderReleaseID]
	if provider == nil {
		return ErrInvalidWorkload
	}
	module := New(provider, a.Runtime, a.Telemetry, a.Management)
	result := regionaltask.BackupResult{Status: "completed"}
	switch task.Kind {
	case "console":
		var payload instanceaction.ConsolePayload
		if err = json.Unmarshal(task.Payload, &payload); err == nil {
			err = module.Console(ctx, manifest, RuntimeHandle{RuntimeID: "gamepanel-" + task.LogicalInstanceID}, payload.Command)
		}
	case "backup":
		var payload instanceaction.BackupPayload
		var artifact BackupArtifact
		if err = json.Unmarshal(task.Payload, &payload); err == nil {
			artifact, err = module.Backup(ctx, manifest, task.LogicalInstanceID, payload.ObjectKey)
			result.ObjectKey, result.SizeBytes, result.Checksums = artifact.ObjectKey, artifact.SizeBytes, artifact.Checksums
		}
	case "restore":
		var payload instanceaction.BackupPayload
		if err = json.Unmarshal(task.Payload, &payload); err == nil {
			err = module.Restore(ctx, manifest, task.LogicalInstanceID, BackupArtifact{ObjectKey: payload.ObjectKey})
		}
	default:
		err = ErrInvalidWorkload
	}
	if err != nil {
		if task.AttemptCount < 5 {
			return err
		}
		result.Status, result.FailureCode = "failed", "runtime_task_failed"
	}
	_, completeErr := a.Tasks.Complete(ctx, task.ID, a.WorkerID, task.FencingToken, result, now)
	return errors.Join(err, completeErr)
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
