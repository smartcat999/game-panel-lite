package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
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
	return runtime.DockerStatus{Available: true, Message: "Docker daemon is available", Host: a.host}
}

func (a *Adapter) Create(ctx context.Context, spec runtime.ContainerSpec) (string, error) {
	if err := os.MkdirAll(spec.DataDir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(spec.DataDir, "serverconfig.txt"), []byte(spec.ConfigText), 0o600); err != nil {
		return "", err
	}
	_, _ = a.client.ImagePull(ctx, spec.Image, types.ImagePullOptions{})
	resp, err := a.client.ContainerCreate(ctx, &container.Config{
		Image: spec.Image,
		Labels: map[string]string{
			"gamepanel.instance": spec.InstanceID,
		},
	}, &container.HostConfig{
		Binds:        []string{fmt.Sprintf("%s:/data", spec.DataDir)},
		PortBindings: natPortMap(spec.Port),
	}, nil, nil, "gamepanel-"+spec.InstanceID)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (a *Adapter) Start(ctx context.Context, instance domain.GameServerInstance) error {
	return a.client.ContainerStart(ctx, instance.ContainerID, types.ContainerStartOptions{})
}

func (a *Adapter) Stop(ctx context.Context, instance domain.GameServerInstance) error {
	timeout := 15
	return a.client.ContainerStop(ctx, instance.ContainerID, container.StopOptions{Timeout: &timeout})
}

func (a *Adapter) Restart(ctx context.Context, instance domain.GameServerInstance) error {
	timeout := 15
	return a.client.ContainerRestart(ctx, instance.ContainerID, container.StopOptions{Timeout: &timeout})
}

func (a *Adapter) Remove(ctx context.Context, instance domain.GameServerInstance) error {
	return a.client.ContainerRemove(ctx, instance.ContainerID, types.ContainerRemoveOptions{Force: true})
}

func (a *Adapter) Inspect(ctx context.Context, instance domain.GameServerInstance) (domain.ServerStatus, error) {
	got, err := a.client.ContainerInspect(ctx, instance.ContainerID)
	if err != nil {
		return domain.StatusErrored, err
	}
	if got.State != nil && got.State.Running {
		return domain.StatusRunning, nil
	}
	return domain.StatusStopped, nil
}

func (a *Adapter) Logs(ctx context.Context, instance domain.GameServerInstance) (io.ReadCloser, error) {
	return a.client.ContainerLogs(ctx, instance.ContainerID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Follow: true, Tail: "120"})
}

func natPortMap(port int) nat.PortMap {
	p := nat.Port(fmt.Sprintf("%d/tcp", port))
	return nat.PortMap{p: []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", port)}}}
}
