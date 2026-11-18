package runtime

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/errdefs"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
)

// WithQuiescedWorkload acquires the per-instance execution lock, stops any running container,
// runs fn against hostDir, re-writes managed files/addons, and restarts if desired state is running.
func (d *DockerDriver) WithQuiescedWorkload(ctx context.Context, in *corev1.Instance, rc *corev1.RuntimeClass, onStage func(stage string), fn func(hostDir string) error) error {
	if err := d.validateQuiescePreconditions(ctx, in, rc, fn); err != nil {
		return err
	}
	release, err := beginDockerExecution(ctx, d.workDir, in)
	if err != nil {
		return err
	}
	defer release()

	hostDir, err := d.resolveQuiesceHostDir(in)
	if err != nil {
		return err
	}
	inspected, hasContainer, err := d.stopRunningContainerForMaintenance(ctx, in, onStage)
	if err != nil {
		return err
	}
	if err := fn(hostDir); err != nil {
		d.restoreContainerAfterFailedMaintenance(ctx, in, inspected, hasContainer)
		return err
	}
	matCtx := volume.WithRequireReflink(ctx, volume.RequiresReflink(rc))
	if err := d.reapplyWorkspaceAfterMaintenance(matCtx, hostDir, in); err != nil {
		return &PostQuiesceError{Stage: QuiesceStageReapply, Err: err}
	}
	if err := d.restartContainerAfterMaintenance(ctx, in, inspected, hasContainer, onStage); err != nil {
		return &PostQuiesceError{Stage: QuiesceStageRestarting, Err: err}
	}
	return nil
}

func (d *DockerDriver) validateQuiescePreconditions(ctx context.Context, in *corev1.Instance, rc *corev1.RuntimeClass, fn func(hostDir string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if in == nil {
		return errors.New("instance is required")
	}
	if fn == nil {
		return errors.New("maintenance callback is required")
	}
	if rc != nil && rc.Handler != HandlerDocker {
		return fmt.Errorf("runtime driver docker cannot handle %q", rc.Handler)
	}
	return d.ensureClient()
}

func (d *DockerDriver) resolveQuiesceHostDir(in *corev1.Instance) (string, error) {
	baseDir, err := filepath.Abs(d.workDir)
	if err != nil {
		return "", err
	}
	return volume.ResolveInstanceWorkDir(baseDir, in.Metadata.UID, in.Metadata.Tenant, in.Metadata.Name, true)
}

func (d *DockerDriver) stopRunningContainerForMaintenance(ctx context.Context, in *corev1.Instance, onStage func(stage string)) (types.ContainerJSON, bool, error) {
	inspected, err := d.inspectInstanceContainer(ctx, in)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return types.ContainerJSON{}, false, nil
		}
		return types.ContainerJSON{}, false, err
	}
	if err := verifyExactContainerBinding(inspected, in); err != nil {
		return types.ContainerJSON{}, false, err
	}
	if inspected.State != nil && (inspected.State.Running || inspected.State.Restarting) {
		emitQuiesceStage(onStage, QuiesceStageStopping)
		if err := d.gracefulStopContainer(ctx, inspected, in, 20); err != nil {
			return types.ContainerJSON{}, false, fmt.Errorf("stop container before maintenance: %w", err)
		}
	}
	return inspected, true, nil
}

func (d *DockerDriver) restoreContainerAfterFailedMaintenance(ctx context.Context, in *corev1.Instance, inspected types.ContainerJSON, hasContainer bool) {
	if !hasContainer || in.Spec.State == corev1.InstanceStateStopped {
		return
	}
	if inspected.State != nil && (inspected.State.Running || inspected.State.Restarting) {
		_ = d.cli.ContainerStart(ctx, inspected.ID, container.StartOptions{})
	}
}

func (d *DockerDriver) reapplyWorkspaceAfterMaintenance(ctx context.Context, hostDir string, in *corev1.Instance) error {
	if err := volume.WriteInstanceFiles(hostDir, in.Spec.Files); err != nil {
		return err
	}
	if in.Spec.State != corev1.InstanceStateStopped && d.materializer != nil {
		return d.materializer.Materialize(ctx, hostDir, in)
	}
	return nil
}

func (d *DockerDriver) restartContainerAfterMaintenance(ctx context.Context, in *corev1.Instance, inspected types.ContainerJSON, hasContainer bool, onStage func(stage string)) error {
	if !hasContainer || in.Spec.State == corev1.InstanceStateStopped {
		return nil
	}
	emitQuiesceStage(onStage, QuiesceStageRestarting)
	if err := d.cli.ContainerStart(ctx, inspected.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("restart container after maintenance: %w", err)
	}
	return nil
}

func emitQuiesceStage(onStage func(stage string), stage string) {
	if onStage != nil {
		onStage(stage)
	}
}
