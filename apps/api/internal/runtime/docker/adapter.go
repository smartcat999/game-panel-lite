package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
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
	info, err := a.client.Info(ctx)
	if err != nil {
		return runtime.DockerStatus{Available: true, Message: "Docker daemon is available", Host: a.host}
	}
	return runtime.DockerStatus{Available: true, Message: "Docker daemon is available", Host: a.host, Architecture: info.Architecture}
}

func (a *Adapter) Create(ctx context.Context, spec runtime.ContainerSpec) (string, error) {
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
	hostConfig := &container.HostConfig{
		Binds:        dataBinds(dataDir, spec.Options.DataMounts),
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

func (a *Adapter) ensureImage(ctx context.Context, image string) error {
	if _, _, err := a.client.ImageInspectWithRaw(ctx, image); err == nil {
		return nil
	}
	pull, err := a.client.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer pull.Close()
	if _, err := io.Copy(io.Discard, pull); err != nil {
		return err
	}
	return nil
}

func (a *Adapter) Start(ctx context.Context, instance domain.GameServerInstance) error {
	containerID, err := a.resolveContainerID(ctx, instance)
	if err != nil {
		return err
	}
	return a.client.ContainerStart(ctx, containerID, types.ContainerStartOptions{})
}

func (a *Adapter) Stop(ctx context.Context, instance domain.GameServerInstance) error {
	containerID, err := a.resolveContainerID(ctx, instance)
	if err != nil {
		return err
	}
	timeout := 15
	return a.client.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
}

func (a *Adapter) Restart(ctx context.Context, instance domain.GameServerInstance) error {
	containerID, err := a.resolveContainerID(ctx, instance)
	if err != nil {
		return err
	}
	timeout := 15
	return a.client.ContainerRestart(ctx, containerID, container.StopOptions{Timeout: &timeout})
}

func (a *Adapter) Remove(ctx context.Context, instance domain.GameServerInstance) error {
	containerID, err := a.resolveContainerID(ctx, instance)
	if err != nil {
		return err
	}
	return a.client.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{Force: true})
}

func (a *Adapter) Inspect(ctx context.Context, instance domain.GameServerInstance) (domain.ServerStatus, error) {
	containerID, err := a.resolveContainerID(ctx, instance)
	if err != nil {
		return domain.StatusErrored, err
	}
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

func (a *Adapter) Stats(ctx context.Context, instance domain.GameServerInstance) (runtime.ContainerStats, error) {
	containerID, err := a.resolveContainerID(ctx, instance)
	if err != nil {
		return runtime.ContainerStats{}, err
	}
	resp, err := a.client.ContainerStats(ctx, containerID, false)
	if err != nil {
		return runtime.ContainerStats{}, err
	}
	defer resp.Body.Close()
	var data types.StatsJSON
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return runtime.ContainerStats{}, err
	}
	cpuPercent := 0.0
	cpuDelta := float64(data.CPUStats.CPUUsage.TotalUsage - data.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(data.CPUStats.SystemUsage - data.PreCPUStats.SystemUsage)
	onlineCPUs := float64(data.CPUStats.OnlineCPUs)
	if onlineCPUs == 0 {
		onlineCPUs = float64(len(data.CPUStats.CPUUsage.PercpuUsage))
	}
	if systemDelta > 0 && onlineCPUs > 0 {
		cpuPercent = (cpuDelta / systemDelta) * onlineCPUs * 100
	}
	return runtime.ContainerStats{
		CPUPercent:    cpuPercent,
		MemoryMB:      int64(data.MemoryStats.Usage) / 1024 / 1024,
		MemoryLimitMB: int64(data.MemoryStats.Limit) / 1024 / 1024,
	}, nil
}

func (a *Adapter) HostStats(ctx context.Context) (runtime.HostStats, error) {
	containers, err := a.client.ContainerList(ctx, types.ContainerListOptions{
		Filters: filters.NewArgs(filters.Arg("label", "gamepanel.instance")),
	})
	if err != nil {
		return runtime.HostStats{}, err
	}
	result := runtime.HostStats{RunningContainers: len(containers)}
	for _, c := range containers {
		resp, err := a.client.ContainerStats(ctx, c.ID, false)
		if err != nil {
			continue
		}
		var data types.StatsJSON
		decodeErr := json.NewDecoder(resp.Body).Decode(&data)
		resp.Body.Close()
		if decodeErr != nil {
			continue
		}
		cpuDelta := float64(data.CPUStats.CPUUsage.TotalUsage - data.PreCPUStats.CPUUsage.TotalUsage)
		systemDelta := float64(data.CPUStats.SystemUsage - data.PreCPUStats.SystemUsage)
		onlineCPUs := float64(data.CPUStats.OnlineCPUs)
		if onlineCPUs == 0 {
			onlineCPUs = float64(len(data.CPUStats.CPUUsage.PercpuUsage))
		}
		if systemDelta > 0 && onlineCPUs > 0 {
			result.TotalCPUPercent += (cpuDelta / systemDelta) * onlineCPUs * 100
		}
		result.TotalMemoryMB += int64(data.MemoryStats.Usage) / 1024 / 1024
		if int64(data.MemoryStats.Limit)/1024/1024 > result.MemoryLimitMB {
			result.MemoryLimitMB = int64(data.MemoryStats.Limit) / 1024 / 1024
		}
	}
	return result, nil
}

func (a *Adapter) Logs(ctx context.Context, instance domain.GameServerInstance) (io.ReadCloser, error) {
	containerID, err := a.resolveContainerID(ctx, instance)
	if err != nil {
		return nil, err
	}
	stream, err := a.client.ContainerLogs(ctx, containerID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Follow: instance.Status == domain.StatusRunning, Tail: "120"})
	if err != nil {
		return nil, err
	}
	return demuxLogStream(stream), nil
}

func (a *Adapter) LogSnapshot(ctx context.Context, instance domain.GameServerInstance) (io.ReadCloser, error) {
	containerID, err := a.resolveContainerID(ctx, instance)
	if err != nil {
		return nil, err
	}
	stream, err := a.client.ContainerLogs(ctx, containerID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Follow: false, Tail: "300"})
	if err != nil {
		return nil, err
	}
	return demuxLogStream(stream), nil
}

func demuxLogStream(stream io.ReadCloser) io.ReadCloser {
	reader, writer := io.Pipe()
	go func() {
		defer stream.Close()
		_, copyErr := stdcopy.StdCopy(writer, writer, stream)
		_ = writer.CloseWithError(copyErr)
	}()
	return reader
}

func (a *Adapter) SendCommand(ctx context.Context, instance domain.GameServerInstance, command string) error {
	containerID, err := a.resolveContainerID(ctx, instance)
	if err != nil {
		return err
	}
	conn, err := a.client.ContainerAttach(ctx, containerID, types.ContainerAttachOptions{
		Stream: true,
		Stdin:  true,
	})
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Conn.Write([]byte(command + "\n"))
	return err
}

func (a *Adapter) resolveContainerID(ctx context.Context, instance domain.GameServerInstance) (string, error) {
	if id := strings.TrimSpace(instance.ContainerID); id != "" {
		if _, err := a.client.ContainerInspect(ctx, id); err == nil {
			return id, nil
		}
	}
	matches, err := a.client.ContainerList(ctx, types.ContainerListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", "gamepanel.instance="+instance.ID)),
	})
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no Docker container found for server %s", instance.ID)
	}
	return matches[0].ID, nil
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
	return os.WriteFile(target, []byte(content), 0o644)
}

func dataBinds(dataDir string, mounts []string) []string {
	if abs, err := filepath.Abs(dataDir); err == nil {
		dataDir = abs
	}
	if len(mounts) == 0 {
		mounts = []string{"/data"}
	}
	binds := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		if mount == "" {
			continue
		}
		hostPath, containerPath := dataBindPaths(dataDir, mount)
		binds = append(binds, fmt.Sprintf("%s:%s", hostPath, containerPath))
	}
	return binds
}

func prepareDataMounts(dataDir string, mounts []string) error {
	if len(mounts) == 0 {
		return nil
	}
	for _, mount := range mounts {
		if mount == "" {
			continue
		}
		hostPath, _ := dataBindPaths(dataDir, mount)
		if _, err := os.Stat(hostPath); err == nil {
			continue
		}
		if filepath.Ext(hostPath) != "" {
			if err := os.MkdirAll(filepath.Dir(hostPath), 0o777); err != nil {
				return err
			}
			file, err := os.OpenFile(hostPath, os.O_CREATE, 0o666)
			if err != nil {
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(hostPath, 0o777); err != nil {
			return err
		}
		if err := os.Chmod(hostPath, 0o777); err != nil {
			return err
		}
	}
	return nil
}

func dataBindPaths(dataDir string, mount string) (string, string) {
	hostPath := dataDir
	containerPath := mount
	if host, container, ok := strings.Cut(mount, ":"); ok {
		hostPath = filepath.Join(dataDir, filepath.Clean(host))
		containerPath = container
	}
	return hostPath, containerPath
}

func natPortMap(containerPort int, hostPort int, protocol string) nat.PortMap {
	p := nat.Port(fmt.Sprintf("%d/%s", containerPort, normalizePortProtocol(protocol)))
	return nat.PortMap{p: []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", hostPort)}}}
}

func natPortSet(containerPort int, protocol string) nat.PortSet {
	return nat.PortSet{nat.Port(fmt.Sprintf("%d/%s", containerPort, normalizePortProtocol(protocol))): struct{}{}}
}

func normalizePortProtocol(protocol string) string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "udp":
		return "udp"
	default:
		return "tcp"
	}
}
