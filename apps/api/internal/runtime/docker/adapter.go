package docker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

type Adapter struct {
	client *client.Client
	host   string
}

func NewAdapter(host string) (*Adapter, error) {
	opts := []client.Opt{client.WithAPIVersionNegotiation()}
	if host != "" {
		opts = append(opts, client.WithHost(host))
	} else {
		opts = append(opts, client.FromEnv)
	}
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, err
	}
	return &Adapter{client: cli, host: cli.DaemonHost()}, nil
}

func (a *Adapter) Check(ctx context.Context) runtime.DockerStatus {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, err := a.client.Ping(ctx); err != nil {
		return runtime.DockerStatus{Available: false, Message: err.Error(), Host: a.host}
	}
	info, err := a.client.Info(ctx)
	if err != nil {
		return runtime.DockerStatus{Available: true, Message: "Docker daemon is available", Host: a.host}
	}
	return runtime.DockerStatus{Available: true, Message: "Docker daemon is available", Host: a.host, Architecture: info.Architecture}
}

func (a *Adapter) createContainer(ctx context.Context, spec runtime.ContainerSpec) (string, error) {
	containerName := "gamepanel-" + spec.InstanceID
	if err := a.removeContainerByNameIfExists(ctx, containerName); err != nil {
		return "", err
	}
	dataDir, err := filepath.Abs(spec.DataDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dataDir, "serverconfig.txt"), []byte(spec.ConfigText), 0o644); err != nil {
		return "", err
	}
	for name, content := range spec.Options.Files {
		if err := writeDataFile(dataDir, name, content); err != nil {
			return "", err
		}
	}
	if err := prepareDataMounts(dataDir, spec.Options.DataMounts); err != nil {
		return "", err
	}
	if err := a.ensureImage(ctx, spec.Image); err != nil {
		return "", err
	}
	binds, err := dataBinds(dataDir, spec.Options.DataMounts)
	if err != nil {
		return "", err
	}
	hostConfig := &container.HostConfig{
		Binds:        binds,
		PortBindings: natPortMap(spec.Port, spec.HostPort, spec.Options.PortProtocol),
	}
	if spec.Resources.CPULimitCores > 0 {
		hostConfig.Resources.NanoCPUs = int64(spec.Resources.CPULimitCores * 1_000_000_000)
	}
	if spec.Resources.MemoryLimitMB > 0 {
		hostConfig.Resources.Memory = int64(spec.Resources.MemoryLimitMB) * 1024 * 1024
	}
	resp, err := a.client.ContainerCreate(ctx, &container.Config{
		Image:        spec.Image,
		Env:          spec.Options.Env,
		Cmd:          spec.Options.Cmd,
		ExposedPorts: natPortSet(spec.Port, spec.Options.PortProtocol),
		OpenStdin:    true,
		AttachStdin:  true,
		Labels: map[string]string{
			"gamepanel.instance": spec.InstanceID,
		},
	}, hostConfig, nil, nil, containerName)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (a *Adapter) CreateWorkload(ctx context.Context, spec domain.WorkloadSpec) (string, error) {
	return a.createContainer(ctx, runtime.ContainerSpecFromWorkload(spec))
}

func (a *Adapter) removeContainerByNameIfExists(ctx context.Context, name string) error {
	inspected, err := a.client.ContainerInspect(ctx, name)
	if err == nil {
		return a.client.ContainerRemove(ctx, inspected.ID, types.ContainerRemoveOptions{Force: true})
	}
	if client.IsErrNotFound(err) {
		return nil
	}
	return err
}

func (a *Adapter) StartWorkload(ctx context.Context, runtimeID string) error {
	return a.client.ContainerStart(ctx, runtimeID, types.ContainerStartOptions{})
}

func (a *Adapter) StopWorkload(ctx context.Context, runtimeID string) error {
	timeout := 15
	return a.client.ContainerStop(ctx, runtimeID, container.StopOptions{Timeout: &timeout})
}

func (a *Adapter) RemoveWorkload(ctx context.Context, runtimeID string) error {
	return a.client.ContainerRemove(ctx, runtimeID, types.ContainerRemoveOptions{})
}

func (a *Adapter) InspectWorkload(ctx context.Context, runtimeID string) (domain.WorkloadStatus, error) {
	status, err := a.inspectContainerState(ctx, runtimeID)
	if err != nil {
		return domain.WorkloadStatus{}, err
	}
	return runtime.WorkloadStatusFromServerStatus(runtimeID, status), nil
}

func (a *Adapter) inspectContainerState(ctx context.Context, containerID string) (domain.ServerStatus, error) {
	got, err := a.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return domain.StatusErrored, err
	}
	if got.State != nil && got.State.Running {
		return domain.StatusRunning, nil
	}
	if got.State != nil && got.State.ExitCode != 0 {
		detail := strings.TrimSpace(got.State.Error)
		if detail == "" {
			detail = strings.TrimSpace(got.State.Status)
		}
		if detail == "" {
			detail = "container exited"
		}
		return domain.StatusErrored, fmt.Errorf("%s (exit code %d)", detail, got.State.ExitCode)
	}
	return domain.StatusStopped, nil
}
