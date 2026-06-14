package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
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
	for name, content := range spec.Options.Files {
		if err := writeDataFile(spec.DataDir, name, content); err != nil {
			return "", err
		}
	}
	pull, err := a.client.ImagePull(ctx, spec.Image, types.ImagePullOptions{})
	if err != nil {
		return "", err
	}
	defer pull.Close()
	if _, err := io.Copy(io.Discard, pull); err != nil {
		return "", err
	}
	resp, err := a.client.ContainerCreate(ctx, &container.Config{
		Image: spec.Image,
		Env:   spec.Options.Env,
		Cmd:   spec.Options.Cmd,
		Labels: map[string]string{
			"gamepanel.instance": spec.InstanceID,
		},
	}, &container.HostConfig{
		Binds:        dataBinds(spec.DataDir, spec.Options.DataMounts),
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
	stream, err := a.client.ContainerLogs(ctx, instance.ContainerID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Follow: true, Tail: "120"})
	if err != nil {
		return nil, err
	}
	reader, writer := io.Pipe()
	go func() {
		defer stream.Close()
		_, copyErr := stdcopy.StdCopy(writer, writer, stream)
		_ = writer.CloseWithError(copyErr)
	}()
	return reader, nil
}

func writeDataFile(dataDir string, name string, content string) error {
	clean := filepath.Clean(name)
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return fmt.Errorf("invalid container data file path: %s", name)
	}
	target := filepath.Join(dataDir, clean)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(content), 0o600)
}

func dataBinds(dataDir string, mounts []string) []string {
	if len(mounts) == 0 {
		mounts = []string{"/data"}
	}
	binds := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		if mount == "" {
			continue
		}
		binds = append(binds, fmt.Sprintf("%s:%s", dataDir, mount))
	}
	return binds
}

func natPortMap(port int) nat.PortMap {
	p := nat.Port(fmt.Sprintf("%d/tcp", port))
	return nat.PortMap{p: []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", port)}}}
}
