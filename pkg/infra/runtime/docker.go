package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/network"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
)

func init() {
	Register(DefaultDockerDriverName, func(workDir string) Driver {
		return NewDockerDriver(workDir)
	})
}

const (
	DefaultDockerDriverName        = corev1.DefaultDockerDriverName
	dockerImagePullFastWaitTimeout = 250 * time.Millisecond
	dockerImagePullMaxTimeout      = 20 * time.Minute
	dockerReasonPullingImage       = corev1.ReasonPullingImage
	dockerReasonSyncingPackage     = corev1.ReasonSyncingPackage
	defaultDockerPidsLimit         = int64(512)
	defaultDockerNofileLimit       = int64(1048576)
)

type imagePullJob struct {
	done chan struct{}
	err  error
}

// DockerDriver implements the container runtime handler for RuntimeClass.spec.handler == "docker"
// using the official Docker Engine SDK (github.com/docker/docker/client).
type DockerDriver struct {
	workDir       string
	cli           *client.Client
	initErr       error
	lastRestartAt sync.Map
	pulling       sync.Map // map[string]*imagePullJob
	hostShellMu   sync.Mutex
	volumeQuota   volume.QuotaProvider
	// materializer gates container start and hardlinks immutable registry blobs into the instance workspace.
	materializer WorkspaceMaterializer
}

// DockerOption configures explicit execution capabilities on DockerDriver.
type DockerOption func(*DockerDriver)

func WithVolumeQuotaProvider(provider volume.QuotaProvider) DockerOption {
	return func(driver *DockerDriver) { driver.volumeQuota = provider }
}

func (d *DockerDriver) volumeQuotaFor(rc *corev1.RuntimeClass) (volume.QuotaProvider, error) {
	if d.volumeQuota != nil {
		return d.volumeQuota, nil
	}
	return volume.ForRuntimeClass(d.workDir, rc)
}

func volumeIdentity(in *corev1.Instance, hostDir string) (volume.Identity, error) {
	id, err := identityOf(in)
	if err != nil {
		return volume.Identity{}, err
	}
	return volume.Identity{InstanceUID: id.UID, NodeUID: id.NodeUID, AttemptID: id.AttemptID, Epoch: id.Epoch, ResourceRevision: id.Revision, Path: hostDir}, nil
}

func NewDockerDriver(workDir string, options ...DockerOption) *DockerDriver {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	d := &DockerDriver{workDir: workDir, cli: cli, initErr: err}
	for _, option := range options {
		option(d)
	}
	return d
}

func NewDockerDriverWithClient(workDir string, cli *client.Client, options ...DockerOption) *DockerDriver {
	d := &DockerDriver{workDir: workDir, cli: cli}
	for _, option := range options {
		option(d)
	}
	return d
}

func (d *DockerDriver) Handler() string { return DefaultDockerDriverName }
func (d *DockerDriver) Version() string { return "27.5" }

func (d *DockerDriver) ensureClient() error {
	if d.initErr != nil {
		return fmt.Errorf("initialize docker client: %w", d.initErr)
	}
	if d.cli == nil {
		return errors.New("docker client is not initialized")
	}
	return nil
}

func containerName(in *corev1.Instance) string {
	if in == nil {
		return "gp-unknown"
	}
	if uid := strings.TrimSpace(in.Metadata.UID); uid != "" {
		return "gp-" + uid
	}
	return "gp-unknown"
}

func verifyContainerOwnership(inspected types.ContainerJSON, in *corev1.Instance) error {
	request, err := identityOf(in)
	if err != nil {
		return err
	}
	if inspected.ContainerJSONBase == nil || inspected.ID == "" || inspected.Config == nil {
		return errors.New("container has no immutable ID or execution labels")
	}
	existing, err := identityFromLabels(inspected.Config.Labels)
	if err != nil {
		return err
	}
	return existing.accepts(request)
}

func verifyExactContainerBinding(inspected types.ContainerJSON, in *corev1.Instance) error {
	existing, err := identityFromLabels(inspected.Config.Labels)
	if err != nil {
		return err
	}
	request, err := identityOf(in)
	if err != nil {
		return err
	}
	if !existing.sameBinding(request) {
		return errors.New("runtime request binding differs from container binding")
	}
	return nil
}

func (d *DockerDriver) inspectInstanceContainer(ctx context.Context, in *corev1.Instance) (types.ContainerJSON, error) {
	inspected, err := d.cli.ContainerInspect(ctx, containerName(in))
	if err != nil {
		return types.ContainerJSON{}, err
	}
	if err := verifyContainerOwnership(inspected, in); err != nil {
		return types.ContainerJSON{}, err
	}
	return inspected, nil
}

func (d *DockerDriver) Validate(in *corev1.Instance, rc *corev1.RuntimeClass) error {
	if in == nil {
		return errors.New("instance is required")
	}
	if rc != nil && rc.Handler != HandlerDocker {
		return fmt.Errorf("expected handler docker, got %q", rc.Handler)
	}
	if strings.TrimSpace(in.Spec.Image) == "" {
		return errors.New("docker runtime handler requires spec.image")
	}
	if err := volume.ValidateInstanceFiles(in.Spec.Files); err != nil {
		return err
	}
	if dir := strings.TrimSpace(in.Spec.WorkingDir); dir != "" && (!path.IsAbs(dir) || strings.ContainsAny(dir, ":\\\x00") || path.Clean(dir) != dir) {
		return errors.New("docker workingDir must be a clean absolute container path")
	}
	if _, err := parseRuntimePositiveLimit(rc, "pidsLimit", defaultDockerPidsLimit); err != nil {
		return err
	}
	if _, err := parseRuntimePositiveLimit(rc, "nofileLimit", defaultDockerNofileLimit); err != nil {
		return err
	}
	return nil
}

func (d *DockerDriver) Reconcile(ctx context.Context, in *corev1.Instance, rc *corev1.RuntimeClass) (*RuntimeObservation, error) {
	if err := d.validateAndEnsurePreconditions(ctx, in, rc); err != nil {
		return nil, err
	}
	release, err := beginDockerExecution(ctx, d.workDir, in)
	if err != nil {
		return nil, err
	}
	defer release()

	inspected, inspectErr := d.inspectInstanceContainer(ctx, in)
	if inspectErr != nil && !errdefs.IsNotFound(inspectErr) {
		return nil, inspectErr
	}
	quota, hostDir, vol, err := d.prepareInstanceVolumeAndQuota(ctx, in, rc)
	if err != nil {
		return nil, err
	}
	needsCreate := errdefs.IsNotFound(inspectErr)
	needsRecreate := !needsCreate && d.shouldRecreateContainer(ctx, in, inspected)
	if needsCreate || needsRecreate {
		ready, err := d.ensureImage(ctx, in.Spec.Image)
		if err != nil {
			return nil, err
		}
		if !ready {
			return &RuntimeObservation{Phase: corev1.InstancePhaseStarting, Reason: dockerReasonPullingImage, Message: "正在拉取容器镜像 " + in.Spec.Image}, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	matCtx := volume.WithRequireReflink(ctx, volume.RequiresReflink(rc))
	if earlyObs, err := d.materializeWorkspaceIfNeeded(matCtx, hostDir, in, inspected, needsCreate, needsRecreate); err != nil || earlyObs != nil {
		return earlyObs, err
	}
	containerID, earlyObs, err := d.reconcileContainerLifecycle(ctx, in, rc, hostDir, inspected, needsCreate, needsRecreate)
	if err != nil || earlyObs != nil {
		return earlyObs, err
	}
	return d.observeContainer(ctx, in, containerID, vol, quota)
}

func (d *DockerDriver) materializeWorkspaceIfNeeded(ctx context.Context, hostDir string, in *corev1.Instance, inspected types.ContainerJSON, needsCreate, needsRecreate bool) (*RuntimeObservation, error) {
	if !d.shouldMaterializeWorkspace(in, inspected, needsCreate, needsRecreate) {
		return nil, nil
	}
	if in.Spec.State != corev1.InstanceStateStopped && d.materializer != nil {
		syncMsg, err := d.materializer.Prepare(ctx, in)
		if err != nil {
			if !needsCreate && inspected.State != nil && (inspected.State.Running || inspected.State.Restarting) &&
				(needsRecreate || d.shouldRestartContainer(containerName(in), inspected, in)) {
				_ = d.gracefulStopContainer(ctx, inspected, in, 15)
			}
			return nil, err
		}
		if syncMsg != "" {
			return &RuntimeObservation{Phase: corev1.InstancePhaseStarting, Reason: dockerReasonSyncingPackage, Message: syncMsg}, nil
		}
	}
	if err := volume.WriteInstanceFiles(hostDir, in.Spec.Files); err != nil {
		return nil, err
	}
	if in.Spec.State != corev1.InstanceStateStopped && d.materializer != nil {
		if err := d.materializer.Materialize(ctx, hostDir, in); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func (d *DockerDriver) shouldMaterializeWorkspace(in *corev1.Instance, inspected types.ContainerJSON, needsCreate, needsRecreate bool) bool {
	if needsCreate || needsRecreate || inspected.State == nil || !inspected.State.Running {
		return true
	}
	return d.shouldRestartContainer(containerName(in), inspected, in)
}

func (d *DockerDriver) validateAndEnsurePreconditions(ctx context.Context, in *corev1.Instance, rc *corev1.RuntimeClass) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateExecutionPorts(in); err != nil {
		return err
	}
	if _, err := executionResources(in); err != nil {
		return err
	}
	if err := d.Validate(in, rc); err != nil {
		return err
	}
	return d.ensureClient()
}

func (d *DockerDriver) prepareInstanceVolumeAndQuota(ctx context.Context, in *corev1.Instance, rc *corev1.RuntimeClass) (volume.QuotaProvider, string, volume.Identity, error) {
	quota, err := d.volumeQuotaFor(rc)
	if err != nil {
		return nil, "", volume.Identity{}, err
	}
	baseDir, err := filepath.Abs(d.workDir)
	if err != nil {
		return nil, "", volume.Identity{}, err
	}
	hostDir, err := volume.ResolveInstanceWorkDir(baseDir, in.Metadata.UID, in.Metadata.Tenant, in.Metadata.Name, true)
	if err != nil {
		return nil, "", volume.Identity{}, err
	}
	vol, err := volumeIdentity(in, hostDir)
	if err != nil {
		return nil, "", volume.Identity{}, err
	}
	resources, err := executionResources(in)
	if err != nil {
		return nil, "", volume.Identity{}, err
	}
	if err := volume.ApplyQuota(ctx, quota, vol, resources.StorageBytes); err != nil {
		return nil, "", volume.Identity{}, err
	}
	return quota, hostDir, vol, nil
}

func (d *DockerDriver) recordHandledRestartAnnotation(name string, in *corev1.Instance) {
	if restartAt := strings.TrimSpace(in.Metadata.Annotations[corev1.AnnotationRestartRequestedAt]); restartAt != "" {
		d.lastRestartAt.Store(name, restartAt)
	}
}

func (d *DockerDriver) reconcileContainerLifecycle(ctx context.Context, in *corev1.Instance, rc *corev1.RuntimeClass, hostDir string, inspected types.ContainerJSON, needsCreate, needsRecreate bool) (string, *RuntimeObservation, error) {
	name := containerName(in)
	if needsRecreate {
		_ = d.gracefulStopContainer(ctx, inspected, in, 15)
		if err := d.cli.ContainerRemove(ctx, inspected.ID, container.RemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
			return "", nil, fmt.Errorf("remove inspected container: %w", err)
		}
	}
	containerID := ""
	if !needsCreate {
		containerID = inspected.ID
	}
	if needsCreate || needsRecreate {
		return d.reconcileFreshContainer(ctx, name, hostDir, in, rc)
	}
	if in.Spec.State == corev1.InstanceStateStopped {
		if err := d.gracefulStopContainer(ctx, inspected, in, 15); err != nil {
			return "", nil, fmt.Errorf("stop container %s: %w", name, err)
		}
		return containerID, nil, nil
	}
	if d.shouldRestartContainer(name, inspected, in) {
		if err := d.gracefulStopContainer(ctx, inspected, in, 15); err != nil {
			return "", nil, fmt.Errorf("stop container %s for restart: %w", name, err)
		}
		if err := d.cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
			return "", nil, fmt.Errorf("restart container %s: %w", name, err)
		}
		d.recordHandledRestartAnnotation(name, in)
		return containerID, nil, nil
	}
	if inspected.State == nil || !inspected.State.Running {
		if obs := checkOOMRestartObservation(containerID, inspected, in); obs != nil {
			return "", obs, nil
		}
		if err := d.cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
			return "", nil, fmt.Errorf("start existing container %s: %w", name, err)
		}
	}
	return containerID, nil, nil
}

func (d *DockerDriver) reconcileFreshContainer(ctx context.Context, name, hostDir string, in *corev1.Instance, rc *corev1.RuntimeClass) (string, *RuntimeObservation, error) {
	containerID, err := d.createContainer(ctx, name, hostDir, in, rc)
	if err != nil {
		return "", nil, err
	}
	if in.Spec.State != corev1.InstanceStateStopped {
		if err := d.cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
			return "", nil, fmt.Errorf("start container %s: %w", name, err)
		}
	}
	d.recordHandledRestartAnnotation(name, in)
	return containerID, nil, nil
}

func checkOOMRestartObservation(containerID string, inspected types.ContainerJSON, in *corev1.Instance) *RuntimeObservation {
	if inspected.State != nil && (inspected.State.OOMKilled || inspected.State.ExitCode == 137) &&
		(in.Status.Phase == corev1.InstancePhaseRunning || in.Status.Phase == corev1.InstancePhaseFailed) {
		return &RuntimeObservation{
			Phase: corev1.InstancePhaseFailed, RuntimeID: "docker://" + containerID,
			Reason: corev1.ReasonOOMKilled, Message: "容器因内存不足 (OOMKilled) 异常退出，请调大内存配额或点击重启",
		}
	}
	return nil
}

func (d *DockerDriver) observeContainer(ctx context.Context, in *corev1.Instance, containerID string, vol volume.Identity, quota volume.QuotaProvider) (*RuntimeObservation, error) {
	inspected, err := d.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("inspect final container %s: %w", containerID, err)
	}
	if err := verifyContainerOwnership(inspected, in); err != nil {
		return nil, err
	}
	if inspected.ID != containerID {
		return nil, errors.New("final container inspection returned a different immutable ID")
	}
	if err := verifyExactContainerBinding(inspected, in); err != nil {
		return nil, err
	}
	if inspected.Config.Labels[corev1.LabelRuntimeSpecHash] != computeRuntimeSpecHash(in) {
		return nil, errors.New("final container configuration does not match the approved execution hash")
	}
	obs, settled, err := d.evaluateContainerStateObservation(in, inspected)
	if err != nil || !settled {
		return obs, err
	}
	return d.verifyAndPopulateAppliedObservation(ctx, in, inspected, obs, vol, quota)
}

func (d *DockerDriver) evaluateContainerStateObservation(in *corev1.Instance, inspected types.ContainerJSON) (*RuntimeObservation, bool, error) {
	obs := &RuntimeObservation{RuntimeID: "docker://" + inspected.ID}
	state := inspected.State
	if state == nil {
		obs.Phase, obs.Reason, obs.Message = corev1.InstancePhaseUnknown, corev1.ReasonContainerStateMissing, "Docker did not report the final container state"
		return obs, false, nil
	}
	if in.Spec.State == corev1.InstanceStateStopped {
		if state.Running || state.Restarting || state.Paused {
			obs.Phase, obs.Reason, obs.Message = corev1.InstancePhaseStopping, corev1.ReasonContainerStillRunning, "Docker has not confirmed that the container stopped"
			return obs, false, nil
		}
		obs.Phase, obs.Reason, obs.Message = corev1.InstancePhaseStopped, corev1.ReasonContainerStopped, "Container stopped with the verified approved configuration"
		return obs, true, nil
	}
	if !state.Running || state.Paused || state.Restarting || state.Dead {
		obs.Phase, obs.Reason, obs.Message = corev1.InstancePhaseFailed, corev1.ReasonContainerNotRunning, fmt.Sprintf("Container is not running after execution (status=%s, exitCode=%d)", state.Status, state.ExitCode)
		if state.Restarting {
			obs.Phase, obs.Reason = corev1.InstancePhaseStarting, corev1.ReasonContainerRestarting
		}
		if state.OOMKilled {
			obs.Reason = corev1.ReasonOOMKilled
		}
		return obs, false, nil
	}
	started, err := time.Parse(time.RFC3339Nano, state.StartedAt)
	if err != nil || started.IsZero() || started.After(time.Now().Add(time.Minute)) {
		return nil, false, errors.New("running container lacks a valid observed start time")
	}
	started = started.UTC()
	obs.StartedAt = &started
	if restartAt := strings.TrimSpace(in.Metadata.Annotations[corev1.AnnotationRestartRequestedAt]); restartAt != "" {
		prev, ok := d.lastRestartAt.Load(containerName(in))
		if (!ok || prev != restartAt) && func() bool {
			requested, err := time.Parse(time.RFC3339Nano, restartAt)
			return err == nil && requested.After(started)
		}() {
			obs.Phase, obs.Reason, obs.Message = corev1.InstancePhaseStarting, corev1.ReasonRestartPending, "Docker has not confirmed the requested restart"
			return obs, false, nil
		}
		d.lastRestartAt.Store(containerName(in), restartAt)
	}
	if !isContainerGamePortReady(in, state.Pid, started) {
		obs.Phase, obs.Reason, obs.Message = corev1.InstancePhaseStarting, corev1.ReasonStarting, "容器已启动，正在加载游戏世界并等待服务端口就绪"
		return obs, false, nil
	}
	obs.Phase, obs.Reason, obs.Message = corev1.InstancePhaseRunning, corev1.ReasonContainerRunning, "Container is running with verified approved resources and ports"
	return obs, true, nil
}

func (d *DockerDriver) verifyAndPopulateAppliedObservation(ctx context.Context, in *corev1.Instance, inspected types.ContainerJSON, obs *RuntimeObservation, vol volume.Identity, quota volume.QuotaProvider) (*RuntimeObservation, error) {
	resources, err := executionResources(in)
	if err != nil {
		return nil, err
	}
	if inspected.HostConfig == nil || inspected.HostConfig.NanoCPUs != resources.CPUMillicores*1_000_000 || inspected.HostConfig.Memory != resources.MemoryBytes {
		return nil, errors.New("inspected Docker CPU or memory limit differs from the approved resources")
	}
	if err := verifyDockerVolumeMount(inspected, in, vol); err != nil {
		return nil, err
	}
	ports, err := verifiedDockerPorts(inspected, in.Spec.Ports, obs.Phase == corev1.InstancePhaseRunning)
	if err != nil {
		return nil, err
	}
	storage, err := quota.Observe(ctx, vol)
	if err != nil {
		return nil, fmt.Errorf("observe final storage quota: %w", err)
	}
	if err := volume.VerifyObservation(storage, vol, resources.StorageBytes); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	obs.AppliedResources = appliedResourceList(inspected.HostConfig.NanoCPUs/1_000_000, inspected.HostConfig.Memory, storage.HardLimitBytes)
	obs.AppliedPorts = ports
	obs.ObservedAt = &now
	if obs.Phase == corev1.InstancePhaseRunning {
		obs.Endpoint = corev1.InstanceEndpoint{HostIP: "127.0.0.1", Ports: append([]corev1.InstancePort(nil), ports...)}
	}
	return obs, nil
}

func verifyDockerVolumeMount(inspected types.ContainerJSON, in *corev1.Instance, vol volume.Identity) error {
	target := in.Spec.WorkingDir
	if strings.TrimSpace(target) == "" {
		target = "/data"
	}
	expectedBind := vol.Path + ":" + target
	if inspected.HostConfig == nil || len(inspected.HostConfig.Binds) != 1 || inspected.HostConfig.Binds[0] != expectedBind || len(inspected.HostConfig.Mounts) != 0 {
		return errors.New("docker bind configuration does not match the quota-enforced instance volume")
	}
	found := false
	for _, mounted := range inspected.Mounts {
		if mounted.Type == "bind" && mounted.Source == vol.Path && mounted.Destination == target && mounted.RW {
			if found {
				return errors.New("docker reported duplicate instance volume mounts")
			}
			found = true
			continue
		}
		if mounted.RW {
			return errors.New("docker reported a writable volume outside the quota-enforced instance volume")
		}
	}
	if !found {
		return errors.New("docker did not report the quota-enforced instance volume mount")
	}
	return nil
}

func verifiedDockerPorts(inspected types.ContainerJSON, expected []corev1.InstancePort, running bool) ([]corev1.InstancePort, error) {
	if inspected.HostConfig == nil {
		return nil, errors.New("docker did not report the configured host port bindings")
	}
	if err := verifyDockerPortMap(inspected.HostConfig.PortBindings, expected); err != nil {
		return nil, fmt.Errorf("configured Docker ports: %w", err)
	}
	if running {
		if inspected.NetworkSettings == nil {
			return nil, errors.New("docker did not report active host port bindings")
		}
		if err := verifyDockerPortMap(inspected.NetworkSettings.Ports, expected); err != nil {
			return nil, fmt.Errorf("active Docker ports: %w", err)
		}
	}
	ports := make([]corev1.InstancePort, len(expected))
	for i, port := range expected {
		port.Protocol = network.NormalizedProtocol(port.Protocol)
		ports[i] = port
	}
	return ports, nil
}

func verifyDockerPortMap(actual nat.PortMap, expected []corev1.InstancePort) error {
	wanted := make(map[nat.Port]map[int]bool)
	for _, port := range expected {
		natPort, err := nat.NewPort(strings.ToLower(network.NormalizedProtocol(port.Protocol)), strconv.Itoa(port.TargetPort))
		if err != nil {
			return err
		}
		if wanted[natPort] == nil {
			wanted[natPort] = make(map[int]bool)
		}
		wanted[natPort][port.PublishedPort] = true
	}
	seen := make(map[nat.Port]map[int]bool)
	for target, bindings := range actual {
		for _, binding := range bindings {
			published, err := strconv.Atoi(binding.HostPort)
			if err != nil || !wanted[target][published] {
				return fmt.Errorf("unexpected or unallocated binding %s:%s for %s", binding.HostIP, binding.HostPort, target)
			}
			// An admitted port is an externally published port. A loopback-only
			// binding does not prove the requested host port is available.
			if binding.HostIP != "" && binding.HostIP != "0.0.0.0" && binding.HostIP != "::" {
				return fmt.Errorf("binding for %s/%d is not published on the host", target, published)
			}
			if seen[target] == nil {
				seen[target] = make(map[int]bool)
			}
			seen[target][published] = true
		}
	}
	for target, publishedPorts := range wanted {
		for published := range publishedPorts {
			if !seen[target][published] {
				return fmt.Errorf("missing admitted binding %s/%d", target, published)
			}
		}
	}
	return nil
}

func (d *DockerDriver) shouldRestartContainer(name string, inspected types.ContainerJSON, in *corev1.Instance) bool {
	if in == nil || len(in.Metadata.Annotations) == 0 {
		return false
	}
	restartAt := strings.TrimSpace(in.Metadata.Annotations[corev1.AnnotationRestartRequestedAt])
	if restartAt == "" {
		return false
	}
	if prev, ok := d.lastRestartAt.Load(name); ok && prev == restartAt {
		return false
	}
	reqTime, err := time.Parse(time.RFC3339Nano, restartAt)
	if err != nil {
		return false
	}
	if inspected.State != nil && inspected.State.StartedAt != "" {
		if startedAt, parseErr := time.Parse(time.RFC3339Nano, inspected.State.StartedAt); parseErr == nil {
			if !reqTime.After(startedAt) {
				d.lastRestartAt.Store(name, restartAt)
				return false
			}
		}
	}
	return true
}

func (d *DockerDriver) shouldRecreateContainer(ctx context.Context, in *corev1.Instance, inspected types.ContainerJSON) bool {
	if inspected.Config == nil || inspected.Config.Labels[corev1.LabelRuntimeSpecHash] != computeRuntimeSpecHash(in) {
		return true
	}
	if in == nil || in.Spec.State == corev1.InstanceStateStopped || inspected.ContainerJSONBase == nil {
		return false
	}
	notRunning := inspected.State == nil || !inspected.State.Running
	if !notRunning && !d.shouldRestartContainer(containerName(in), inspected, in) {
		return false
	}
	return d.isContainerImageStale(ctx, in.Spec.Image, inspected.Image)
}

func (d *DockerDriver) isContainerImageStale(ctx context.Context, imageRef, containerImageID string) bool {
	imageRef = strings.TrimSpace(imageRef)
	containerImageID = strings.TrimSpace(containerImageID)
	if d.cli == nil || imageRef == "" || containerImageID == "" {
		return false
	}
	img, _, err := d.cli.ImageInspectWithRaw(ctx, imageRef)
	if err != nil {
		return false
	}
	resolvedID := strings.TrimSpace(img.ID)
	return resolvedID != "" && resolvedID != containerImageID
}

func (d *DockerDriver) createContainer(ctx context.Context, name, hostDir string, in *corev1.Instance, rc *corev1.RuntimeClass) (string, error) {
	var envList []string
	for k, v := range in.Spec.Env {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}
	exposedPorts, portBindings, err := buildDockerPortBindings(in.Spec.Ports)
	if err != nil {
		return "", err
	}
	workDir := strings.TrimSpace(in.Spec.WorkingDir)
	if workDir == "" {
		workDir = "/data"
	}
	cfg := &container.Config{
		Image:        in.Spec.Image,
		Entrypoint:   in.Spec.Command,
		Cmd:          in.Spec.Args,
		Env:          envList,
		WorkingDir:   workDir,
		ExposedPorts: exposedPorts,
		OpenStdin:    true,
		AttachStdin:  true,
		StdinOnce:    false,
		Labels:       runtimeLabels(in),
	}
	hostCfg, err := buildDockerHostConfig(hostDir, workDir, portBindings, in, rc)
	if err != nil {
		return "", err
	}
	created, err := d.cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, name)
	if err != nil {
		return "", fmt.Errorf("create container %s: %w", name, err)
	}
	return created.ID, nil
}

func buildDockerPortBindings(ports []corev1.InstancePort) (nat.PortSet, nat.PortMap, error) {
	exposedPorts := nat.PortSet{}
	portBindings := nat.PortMap{}
	for _, p := range ports {
		proto := strings.ToLower(strings.TrimSpace(p.Protocol))
		if proto == "" {
			proto = strings.ToLower(corev1.ProtocolTCP)
		}
		natPort, err := nat.NewPort(proto, strconv.Itoa(p.TargetPort))
		if err != nil {
			return nil, nil, fmt.Errorf("invalid port %d/%s: %w", p.TargetPort, proto, err)
		}
		exposedPorts[natPort] = struct{}{}
		hostPort := ""
		if p.PublishedPort > 0 {
			hostPort = strconv.Itoa(p.PublishedPort)
		}
		portBindings[natPort] = append(portBindings[natPort], nat.PortBinding{HostIP: "0.0.0.0", HostPort: hostPort})
	}
	return exposedPorts, portBindings, nil
}

func parseRuntimePositiveLimit(rc *corev1.RuntimeClass, key string, defaultVal int64) (int64, error) {
	if rc == nil || rc.Parameters == nil {
		return defaultVal, nil
	}
	raw := strings.TrimSpace(rc.Parameters[key])
	if raw == "" {
		return defaultVal, nil
	}
	val, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || val <= 0 {
		return 0, fmt.Errorf("docker %s must be a finite positive integer", key)
	}
	return val, nil
}

func buildDockerHostConfig(hostDir, workDir string, portBindings nat.PortMap, in *corev1.Instance, rc *corev1.RuntimeClass) (*container.HostConfig, error) {
	resources, err := executionResources(in)
	if err != nil {
		return nil, err
	}
	pidsLimit, err := parseRuntimePositiveLimit(rc, "pidsLimit", defaultDockerPidsLimit)
	if err != nil {
		return nil, err
	}
	nofileLimit, err := parseRuntimePositiveLimit(rc, "nofileLimit", defaultDockerNofileLimit)
	if err != nil {
		return nil, err
	}
	hostCfg := &container.HostConfig{
		Binds:         []string{fmt.Sprintf("%s:%s", hostDir, workDir)},
		PortBindings:  portBindings,
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyOnFailure, MaximumRetryCount: 5},
		Resources: container.Resources{
			NanoCPUs:  resources.CPUMillicores * 1_000_000,
			Memory:    resources.MemoryBytes,
			PidsLimit: &pidsLimit,
			Ulimits: []*container.Ulimit{
				{Name: "nofile", Soft: nofileLimit, Hard: nofileLimit},
			},
		},
	}
	if rc != nil && rc.Parameters != nil {
		if rp := strings.TrimSpace(rc.Parameters["restartPolicy"]); rp != "" {
			hostCfg.RestartPolicy = container.RestartPolicy{Name: container.RestartPolicyMode(rp)}
		}
	}
	return hostCfg, nil
}

func (d *DockerDriver) PrePullImage(ctx context.Context, imageRef string) (bool, error) {
	imageRef = strings.TrimSpace(imageRef)
	if imageRef == "" {
		return false, nil
	}
	if err := d.ensureClient(); err != nil {
		return false, err
	}
	return d.ensureImageWait(ctx, imageRef, dockerImagePullMaxTimeout)
}

func (d *DockerDriver) ensureImage(ctx context.Context, imageRef string) (bool, error) {
	return d.ensureImageWait(ctx, imageRef, dockerImagePullFastWaitTimeout)
}

func (d *DockerDriver) ensureImageWait(ctx context.Context, imageRef string, waitTimeout time.Duration) (bool, error) {
	if _, _, err := d.cli.ImageInspectWithRaw(ctx, imageRef); err == nil {
		return true, nil
	} else if !errdefs.IsNotFound(err) {
		return false, fmt.Errorf("inspect image %s: %w", imageRef, err)
	}

	newJob := &imagePullJob{done: make(chan struct{})}
	actual, loaded := d.pulling.LoadOrStore(imageRef, newJob)
	job := actual.(*imagePullJob)
	if !loaded {
		go func() {
			defer func() {
				d.pulling.Delete(imageRef)
				close(job.done)
			}()
			// Pulling is a separate bounded background job so returning Starting
			// does not cancel a slow download. This call's wait still respects ctx.
			pullCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), dockerImagePullMaxTimeout)
			defer cancel()
			pullResp, pullErr := d.cli.ImagePull(pullCtx, imageRef, image.PullOptions{})
			if pullErr != nil {
				job.err = pullErr
				return
			}
			_, job.err = io.Copy(io.Discard, pullResp)
			if closeErr := pullResp.Close(); job.err == nil {
				job.err = closeErr
			}
		}()
	}

	timer := time.NewTimer(waitTimeout)
	defer timer.Stop()
	select {
	case <-job.done:
		if job.err != nil {
			return false, fmt.Errorf("pull image %s: %w", imageRef, job.err)
		}
		return true, nil
	case <-ctx.Done():
		return false, ctx.Err()
	case <-timer.C:
		return false, nil
	}
}

func (d *DockerDriver) Destroy(ctx context.Context, in *corev1.Instance, _ *corev1.RuntimeClass) error {
	if err := d.ensureClient(); err != nil {
		return err
	}
	release, err := beginDockerCleanup(ctx, d.workDir, in)
	if err != nil {
		return err
	}
	defer release()
	name := containerName(in)
	inspected, err := d.inspectInstanceContainer(ctx, in)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("inspect container %s before remove: %w", name, err)
	}
	if err := verifyExactContainerBinding(inspected, in); err != nil {
		return err
	}
	target := inspected.ID
	err = d.cli.ContainerRemove(ctx, target, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	})
	if err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("remove container %s: %w", name, err)
	}
	return nil
}

func (d *DockerDriver) Logs(ctx context.Context, in *corev1.Instance, opts LogOptions) (io.ReadCloser, error) {
	if err := d.ensureClient(); err != nil {
		return nil, err
	}
	if err := verifyDockerReadAccess(d.workDir, in); err != nil {
		return nil, err
	}
	inspected, err := d.inspectInstanceContainer(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("inspect container for logs: %w", err)
	}
	if err := verifyExactContainerBinding(inspected, in); err != nil {
		return nil, err
	}
	target := inspected.ID
	tail := "all"
	if opts.TailLines > 0 {
		tail = strconv.Itoa(opts.TailLines)
	}
	rc, err := d.cli.ContainerLogs(ctx, target, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     opts.Follow,
		Tail:       tail,
	})
	if err != nil {
		return nil, fmt.Errorf("container logs: %w", err)
	}

	pr, pw := io.Pipe()
	go func() {
		defer rc.Close()
		_, copyErr := stdcopy.StdCopy(pw, pw, rc)
		_ = pw.CloseWithError(copyErr)
	}()
	return pr, nil
}

func (d *DockerDriver) Exec(ctx context.Context, in *corev1.Instance, req ExecRequest) (string, error) {
	if len(req.Command) == 0 {
		return "", errors.New("exec command is required")
	}
	if err := d.ensureClient(); err != nil {
		return "", err
	}
	if err := verifyDockerReadAccess(d.workDir, in); err != nil {
		return "", err
	}
	inspected, err := d.inspectInstanceContainer(ctx, in)
	if err != nil {
		return "", fmt.Errorf("inspect container for exec: %w", err)
	}
	if err := verifyExactContainerBinding(inspected, in); err != nil {
		return "", err
	}
	target := inspected.ID
	if req.Command[0] == ExecStdinMarker {
		input := strings.TrimSpace(strings.Join(req.Command[1:], " "))
		if input == "" {
			return "", errors.New("stdin payload cannot be empty")
		}
		return d.writeContainerStdin(ctx, target, input)
	}
	return d.runContainerExec(ctx, target, req.Command)
}

func (d *DockerDriver) writeContainerStdin(ctx context.Context, cName, line string) (string, error) {
	attachResp, err := d.cli.ContainerAttach(ctx, cName, container.AttachOptions{
		Stream: true,
		Stdin:  true,
	})
	if err != nil {
		return "", fmt.Errorf("attach container stdin: %w", err)
	}
	defer attachResp.Close()
	if _, err := io.WriteString(attachResp.Conn, line+"\n"); err != nil {
		return "", fmt.Errorf("write container stdin: %w", err)
	}
	return "", nil
}

func (d *DockerDriver) runContainerExec(ctx context.Context, cName string, cmd []string) (string, error) {
	execCreated, err := d.cli.ContainerExecCreate(ctx, cName, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return "", fmt.Errorf("create exec: %w", err)
	}
	hijacked, err := d.cli.ContainerExecAttach(ctx, execCreated.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", fmt.Errorf("attach exec: %w", err)
	}
	defer hijacked.Close()

	var out bytes.Buffer
	if _, err := stdcopy.StdCopy(&out, &out, io.LimitReader(hijacked.Reader, 1<<20)); err != nil {
		return "", fmt.Errorf("read exec output: %w", err)
	}
	return strings.TrimSpace(out.String()), nil
}

func computeRuntimeSpecHash(in *corev1.Instance) string {
	if in == nil {
		return ""
	}
	// Lifecycle intent and API-only settings do not change container config.
	// Generation still fences operations through the durable execution journal.
	specCopy := corev1.InstanceSpec{
		Image: in.Spec.Image, Command: in.Spec.Command, Args: in.Spec.Args,
		WorkingDir: in.Spec.WorkingDir, Resources: in.Spec.Resources,
		Ports: in.Spec.Ports, Env: in.Spec.Env, Files: in.Spec.Files,
		ServerArtifact: in.Spec.ServerArtifact, Addons: in.Spec.Addons,
	}
	id, _ := identityOf(in)
	id.Generation = 0
	var binding any
	if in.Status.ResourceControl != nil {
		binding = in.Status.ResourceControl.RuntimeClass
	}
	raw, _ := json.Marshal(struct {
		Spec         corev1.InstanceSpec
		Identity     executionIdentity
		RuntimeClass any
		AppVersion   string
	}{specCopy, id, binding, strings.TrimSpace(in.Metadata.Annotations[corev1.AnnotationInstanceAppVersion])})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:16])
}

func runtimeLabels(in *corev1.Instance) map[string]string {
	id, _ := identityOf(in)
	labels := id.labels()
	labels[corev1.LabelRuntimeTenant] = in.Metadata.Tenant
	labels[corev1.LabelRuntimeInstanceName] = in.Metadata.Name
	labels[corev1.LabelRuntimeSpecHash] = computeRuntimeSpecHash(in)
	return labels
}
