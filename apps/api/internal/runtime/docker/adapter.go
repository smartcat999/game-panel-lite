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

func (a *Adapter) ImageStatus(ctx context.Context, image string) domain.RuntimeImageStatus {
	image = strings.TrimSpace(image)
	if image == "" {
		return domain.RuntimeImageStatus{Status: runtime.ImageStatusFailed, Message: "image is empty", UpdatedAt: time.Now()}
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, _, err := a.client.ImageInspectWithRaw(ctx, image); err == nil {
		return domain.RuntimeImageStatus{Image: image, Status: runtime.ImageStatusReady, UpdatedAt: time.Now()}
	} else if client.IsErrNotFound(err) {
		return domain.RuntimeImageStatus{Image: image, Status: runtime.ImageStatusMissing, UpdatedAt: time.Now()}
	} else {
		return domain.RuntimeImageStatus{Image: image, Status: runtime.ImageStatusFailed, Message: err.Error(), UpdatedAt: time.Now()}
	}
}

func (a *Adapter) PrepareImage(ctx context.Context, image string) error {
	return a.ensureImage(ctx, image)
}

func (a *Adapter) PrepareImageWithProgress(ctx context.Context, image string, onProgress runtime.ImagePrepareProgressFunc) error {
	return a.ensureImageWithProgress(ctx, image, onProgress)
}

func (a *Adapter) SaveImageArchive(ctx context.Context, image string, path string) error {
	if strings.TrimSpace(image) == "" {
		return fmt.Errorf("image is empty")
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("image archive path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	reader, err := a.client.ImageSave(ctx, []string{image})
	if err != nil {
		return err
	}
	defer reader.Close()
	tempPath := path + ".tmp"
	file, err := os.Create(tempPath)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, reader)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tempPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tempPath)
		return closeErr
	}
	return os.Rename(tempPath, path)
}

func (a *Adapter) LoadImageArchive(ctx context.Context, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	response, err := a.client.ImageLoad(ctx, file, true)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	return nil
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
	return a.ensureImageWithProgress(ctx, image, nil)
}

func (a *Adapter) ensureImageWithProgress(ctx context.Context, image string, onProgress runtime.ImagePrepareProgressFunc) error {
	if _, _, err := a.client.ImageInspectWithRaw(ctx, image); err == nil {
		if onProgress != nil {
			onProgress(runtime.ImagePrepareProgress{Message: "image already installed", Progress: 100})
		}
		return nil
	}
	pull, err := a.client.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return imagePullError(image, err)
	}
	defer pull.Close()
	if err := consumeImagePullWithProgress(pull, onProgress); err != nil {
		return imagePullError(image, err)
	}
	return nil
}

func imagePullError(image string, err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	if isDSTImage(image) && isImageAccessDenied(message) {
		return fmt.Errorf("DST runtime image %s is not available from the registry. Build and load it on this Docker host with scripts/build-game-images.sh dst --platform linux/amd64 --load, or push it to a registry this host can pull from. Original error: %w", image, err)
	}
	return err
}

func isDSTImage(image string) bool {
	return strings.Contains(image, "dst-server:")
}

func isImageAccessDenied(message string) bool {
	message = strings.ToLower(message)
	return strings.Contains(message, "pull access denied") ||
		strings.Contains(message, "repository does not exist") ||
		strings.Contains(message, "requested access to the resource is denied")
}

type imagePullEvent struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Error       string `json:"error"`
	ErrorDetail struct {
		Message string `json:"message"`
	} `json:"errorDetail"`
	ProgressDetail struct {
		Current int64 `json:"current"`
		Total   int64 `json:"total"`
	} `json:"progressDetail"`
}

type imagePullLayerProgress struct {
	current int64
	total   int64
}

func consumeImagePull(reader io.Reader) error {
	return consumeImagePullWithProgress(reader, nil)
}

func consumeImagePullWithProgress(reader io.Reader, onProgress runtime.ImagePrepareProgressFunc) error {
	decoder := json.NewDecoder(reader)
	layers := map[string]imagePullLayerProgress{}
	maxProgress := 0
	for {
		var event imagePullEvent
		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				if onProgress != nil {
					onProgress(runtime.ImagePrepareProgress{Message: "image ready", Progress: 100})
				}
				return nil
			}
			return err
		}
		if message := strings.TrimSpace(event.ErrorDetail.Message); message != "" {
			return fmt.Errorf("image pull failed: %s", message)
		}
		if message := strings.TrimSpace(event.Error); message != "" {
			return fmt.Errorf("image pull failed: %s", message)
		}
		if onProgress == nil {
			continue
		}
		message := strings.TrimSpace(event.Status)
		if event.ID != "" {
			message = strings.TrimSpace(strings.Join([]string{event.ID, event.Status}, " "))
		}
		if event.ProgressDetail.Total > 0 && event.ID != "" {
			layers[event.ID] = imagePullLayerProgress{
				current: event.ProgressDetail.Current,
				total:   event.ProgressDetail.Total,
			}
			progress := aggregateImagePullProgress(layers)
			if progress < maxProgress {
				progress = maxProgress
			} else {
				maxProgress = progress
			}
			onProgress(runtime.ImagePrepareProgress{Message: message, Progress: progress})
			continue
		}
		if message != "" {
			onProgress(runtime.ImagePrepareProgress{Message: message, Progress: maxProgress})
		}
	}
}

func aggregateImagePullProgress(layers map[string]imagePullLayerProgress) int {
	var current int64
	var total int64
	for _, layer := range layers {
		if layer.total <= 0 {
			continue
		}
		layerCurrent := layer.current
		if layerCurrent < 0 {
			layerCurrent = 0
		}
		if layerCurrent > layer.total {
			layerCurrent = layer.total
		}
		current += layerCurrent
		total += layer.total
	}
	if total <= 0 {
		return 0
	}
	progress := int(float64(current) / float64(total) * 100)
	if progress <= 0 && current > 0 {
		return 1
	}
	if progress >= 100 {
		return 99
	}
	return progress
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
	if err := ensureRuntimeWritableDir(filepath.Dir(target)); err != nil {
		return err
	}
	if err := os.WriteFile(target, []byte(content), 0o666); err != nil {
		return err
	}
	return os.Chmod(target, 0o666)
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
			if err := ensureRuntimeWritablePath(hostPath); err != nil {
				return err
			}
			continue
		}
		if filepath.Ext(hostPath) != "" {
			if err := ensureRuntimeWritableDir(filepath.Dir(hostPath)); err != nil {
				return err
			}
			file, err := os.OpenFile(hostPath, os.O_CREATE, 0o666)
			if err != nil {
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
			if err := os.Chmod(hostPath, 0o666); err != nil {
				return err
			}
			continue
		}
		if err := ensureRuntimeWritableDir(hostPath); err != nil {
			return err
		}
	}
	return nil
}

func ensureRuntimeWritablePath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return os.Chmod(path, 0o777)
	}
	return os.Chmod(path, 0o666)
}

func ensureRuntimeWritableDir(path string) error {
	if err := os.MkdirAll(path, 0o777); err != nil {
		return err
	}
	return os.Chmod(path, 0o777)
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
