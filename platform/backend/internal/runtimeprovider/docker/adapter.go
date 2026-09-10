package docker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instanceobservability"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/nodeworkload"
)

const defaultNetwork = "gamepanel-workloads"

var safeInstanceID = regexp.MustCompile(`^lin_[A-Za-z0-9]+$`)

type Adapter struct {
	client     *client.Client
	dataRoot   string
	backupRoot string
	network    string
}

func New(host string, roots ...string) (*Adapter, error) {
	dataRoot, backupRoot := "/var/lib/gamepanel/instances", "/var/lib/gamepanel/backups"
	if len(roots) > 0 && roots[0] != "" {
		dataRoot = roots[0]
	}
	if len(roots) > 1 && roots[1] != "" {
		backupRoot = roots[1]
	}
	for _, root := range []string{dataRoot, backupRoot} {
		absolute, err := filepath.Abs(root)
		if err != nil || absolute != filepath.Clean(root) {
			return nil, errors.New("Docker Runtime Provider roots must be absolute and clean")
		}
		if info, err := os.Lstat(root); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("Docker Runtime Provider root cannot be a symlink")
		}
	}
	options := []client.Opt{client.WithAPIVersionNegotiation()}
	if host == "" {
		options = append(options, client.FromEnv)
	} else {
		options = append(options, client.WithHost(host))
	}
	dockerClient, err := client.NewClientWithOpts(options...)
	if err != nil {
		return nil, err
	}
	return &Adapter{client: dockerClient, dataRoot: dataRoot, backupRoot: backupRoot, network: defaultNetwork}, nil
}

func (a *Adapter) Reconcile(ctx context.Context, logicalInstanceID string, spec nodeworkload.Specification, policy nodeworkload.NetworkPolicy) (nodeworkload.RuntimeResult, error) {
	if err := a.validateSpec(logicalInstanceID, spec, policy); err != nil {
		return nodeworkload.RuntimeResult{}, err
	}
	name := "gamepanel-" + logicalInstanceID
	inspected, inspectErr := a.client.ContainerInspect(ctx, name)
	if spec.DesiredState == "stopped" {
		if client.IsErrNotFound(inspectErr) {
			return a.observe(ctx, logicalInstanceID, "", "stopped", 0)
		}
		if inspectErr != nil {
			return nodeworkload.RuntimeResult{}, inspectErr
		}
		if inspected.State != nil && inspected.State.Running {
			timeout := 30
			if err := a.client.ContainerStop(ctx, inspected.ID, container.StopOptions{Timeout: &timeout}); err != nil {
				return nodeworkload.RuntimeResult{}, err
			}
		}
		return a.observe(ctx, logicalInstanceID, inspected.ID, "stopped", inspected.RestartCount)
	}
	hash, err := specHash(spec)
	if err != nil {
		return nodeworkload.RuntimeResult{}, err
	}
	if inspectErr == nil && inspected.Config != nil && inspected.Config.Labels["gamepanel.spec-hash"] == hash {
		if inspected.State == nil || !inspected.State.Running {
			if err := a.client.ContainerStart(ctx, inspected.ID, types.ContainerStartOptions{}); err != nil {
				return nodeworkload.RuntimeResult{}, err
			}
		}
		result, err := a.observe(ctx, logicalInstanceID, inspected.ID, "running", inspected.RestartCount)
		if err != nil {
			return nodeworkload.RuntimeResult{}, err
		}
		return a.withReadiness(ctx, inspected.ID, result, spec, networkAddress(inspected, a.network)), nil
	}
	if inspectErr != nil && !client.IsErrNotFound(inspectErr) {
		return nodeworkload.RuntimeResult{}, inspectErr
	}
	dataDir, err := a.prepareData(spec)
	if err != nil {
		return nodeworkload.RuntimeResult{}, err
	}
	if err := a.ensureImage(ctx, spec.Artifact); err != nil {
		return nodeworkload.RuntimeResult{}, err
	}
	if inspectErr == nil {
		if err := a.client.ContainerRemove(ctx, inspected.ID, types.ContainerRemoveOptions{Force: true}); err != nil {
			return nodeworkload.RuntimeResult{}, err
		}
	}
	ports, exposed, err := portConfiguration(spec.Listeners)
	if err != nil {
		return nodeworkload.RuntimeResult{}, err
	}
	binds, err := bindConfiguration(dataDir, spec.Mounts)
	if err != nil {
		return nodeworkload.RuntimeResult{}, err
	}
	response, err := a.client.ContainerCreate(ctx, &container.Config{Image: spec.Artifact, User: fmt.Sprintf("%d:%d", spec.RunAsUID, spec.RunAsGID), Env: environment(spec.Env), Cmd: append([]string(nil), spec.Args...), ExposedPorts: exposed, OpenStdin: true, AttachStdin: true, StdinOnce: false, Labels: map[string]string{"gamepanel.instance": logicalInstanceID, "gamepanel.spec-hash": hash, "gamepanel.fencing-token": strconv.FormatInt(spec.FencingToken, 10)}}, secureHostConfig(spec, binds, ports, a.network), nil, nil, name)
	if err != nil {
		return nodeworkload.RuntimeResult{}, err
	}
	if err := a.client.ContainerStart(ctx, response.ID, types.ContainerStartOptions{}); err != nil {
		return nodeworkload.RuntimeResult{}, err
	}
	created, inspectErr := a.client.ContainerInspect(ctx, response.ID)
	if inspectErr != nil {
		return nodeworkload.RuntimeResult{}, inspectErr
	}
	result, err := a.observe(ctx, logicalInstanceID, response.ID, "running", 0)
	if err != nil {
		return nodeworkload.RuntimeResult{}, err
	}
	return a.withReadiness(ctx, response.ID, result, spec, networkAddress(created, a.network)), nil
}

func (a *Adapter) withReadiness(ctx context.Context, runtimeID string, result nodeworkload.RuntimeResult, spec nodeworkload.Specification, address string) nodeworkload.RuntimeResult {
	result = withReadinessCheck(result, spec.Listeners, func(port int) bool { return tcpListenerReady(address, port) })
	if result.State != "running" || spec.ReadyLogMarker == "" {
		return result
	}
	logs, err := a.client.ContainerLogs(ctx, runtimeID, container.LogsOptions{ShowStdout: true, ShowStderr: true, Tail: "5000"})
	if err != nil {
		result.State = "starting"
		return result
	}
	defer logs.Close()
	content, err := io.ReadAll(io.LimitReader(logs, 2<<20))
	if err != nil || !bytes.Contains(content, []byte(spec.ReadyLogMarker)) {
		result.State = "starting"
	}
	return result
}

func withReadinessCheck(result nodeworkload.RuntimeResult, listeners []nodeworkload.Listener, ready func(int) bool) nodeworkload.RuntimeResult {
	for _, listener := range listeners {
		if listener.Protocol != "tcp" {
			continue
		}
		if !ready(listener.InternalPort) {
			result.State = "starting"
			return result
		}
	}
	return result
}

func tcpListenerReady(address string, port int) bool {
	if address == "" {
		return false
	}
	connection, err := net.DialTimeout("tcp", net.JoinHostPort(address, strconv.Itoa(port)), 250*time.Millisecond)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}

func networkAddress(inspected types.ContainerJSON, network string) string {
	if inspected.NetworkSettings == nil || inspected.NetworkSettings.Networks[network] == nil {
		return ""
	}
	return inspected.NetworkSettings.Networks[network].IPAddress
}

func (a *Adapter) ExecuteConsole(ctx context.Context, handle nodeworkload.RuntimeHandle, command string) error {
	if handle.RuntimeID == "" || command == "" || strings.ContainsAny(command, "\r\n\x00") {
		return errors.New("invalid Docker console command")
	}
	attached, err := a.client.ContainerAttach(ctx, handle.RuntimeID, container.AttachOptions{Stream: true, Stdin: true})
	if err != nil {
		return err
	}
	defer attached.Close()
	_, err = io.WriteString(attached.Conn, command+"\n")
	return err
}

func (a *Adapter) Backup(_ context.Context, dataScope, objectKey string) (nodeworkload.BackupArtifact, error) {
	source, err := a.scopePath(a.dataRoot, dataScope)
	if err != nil {
		return nodeworkload.BackupArtifact{}, err
	}
	target, err := a.objectPath(objectKey)
	if err != nil {
		return nodeworkload.BackupArtifact{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return nodeworkload.BackupArtifact{}, err
	}
	temporary := target + ".partial"
	_ = os.Remove(temporary)
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return nodeworkload.BackupArtifact{}, err
	}
	digest := sha256.New()
	counter := &byteCounter{}
	archiveErr := writeArchive(source, io.MultiWriter(file, digest, counter))
	closeErr := file.Close()
	if archiveErr != nil || closeErr != nil {
		_ = os.Remove(temporary)
		return nodeworkload.BackupArtifact{}, errors.Join(archiveErr, closeErr)
	}
	if err := os.Rename(temporary, target); err != nil {
		_ = os.Remove(temporary)
		return nodeworkload.BackupArtifact{}, err
	}
	return nodeworkload.BackupArtifact{ObjectKey: objectKey, SizeBytes: counter.total, Checksums: map[string]string{"sha256": hex.EncodeToString(digest.Sum(nil))}}, nil
}

func (a *Adapter) Restore(_ context.Context, dataScope string, artifact nodeworkload.BackupArtifact) error {
	target, err := a.scopePath(a.dataRoot, dataScope)
	if err != nil {
		return err
	}
	source, err := a.objectPath(artifact.ObjectKey)
	if err != nil {
		return err
	}
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	return extractArchive(file, target)
}

func (a *Adapter) validateSpec(instanceID string, spec nodeworkload.Specification, policy nodeworkload.NetworkPolicy) error {
	if !safeInstanceID.MatchString(instanceID) || spec.LogicalInstanceID != instanceID || spec.DesiredState != "running" && spec.DesiredState != "stopped" || strings.TrimSpace(spec.Artifact) == "" || spec.DataScope != "instances/"+instanceID || spec.CPUMilli < 1 || spec.MemoryMiB < 1 || spec.RunAsUID < 1 || spec.RunAsGID < 1 || spec.FencingToken < 1 || !policy.InternetEgressAllowed || len(policy.DeniedManagementCIDRs) == 0 {
		return errors.New("invalid Docker workload spec")
	}
	if _, err := a.scopePath(a.dataRoot, spec.DataScope); err != nil {
		return err
	}
	for _, listener := range spec.Listeners {
		if listener.InternalPort < 1 || listener.InternalPort > 65535 || listener.HostPort < 1 || listener.HostPort > 65535 || listener.Protocol != "tcp" && listener.Protocol != "udp" {
			return errors.New("invalid Docker workload listener")
		}
	}
	return nil
}

func (a *Adapter) prepareData(spec nodeworkload.Specification) (string, error) {
	dataDir, err := a.scopePath(a.dataRoot, spec.DataScope)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return "", err
	}
	for name, content := range spec.Files {
		target, err := scopedPath(dataDir, name)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return "", err
		}
		if err := os.WriteFile(target, []byte(content), 0o640); err != nil {
			return "", err
		}
	}
	for host := range spec.Mounts {
		target, err := scopedPath(dataDir, host)
		if err != nil {
			return "", err
		}
		if _, isFile := spec.Files[host]; !isFile {
			if err := os.MkdirAll(target, 0o750); err != nil {
				return "", err
			}
		}
	}
	if err := filepath.WalkDir(dataDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("Docker data path cannot contain a symlink")
		}
		return os.Chown(path, spec.RunAsUID, spec.RunAsGID)
	}); err != nil {
		return "", err
	}
	return dataDir, nil
}

func (a *Adapter) observe(ctx context.Context, instanceID, runtimeID, state string, restarts int) (nodeworkload.RuntimeResult, error) {
	now := time.Now().UTC()
	attemptMaterial := runtimeID
	if attemptMaterial == "" {
		attemptMaterial = instanceID + ":absent"
	}
	attemptDigest := sha256.Sum256([]byte(attemptMaterial))
	attemptID := "rta_" + hex.EncodeToString(attemptDigest[:8])
	result := nodeworkload.RuntimeResult{Handle: nodeworkload.RuntimeHandle{RuntimeAttemptID: attemptID, RuntimeID: runtimeID}, State: state}
	result.Metrics = []instanceobservability.MetricSample{
		{ID: "met_state_" + attemptID, LogicalInstanceID: instanceID, RuntimeAttemptID: attemptID, Metric: "process.state", Value: boolValue(state == "running"), Unit: "boolean", Source: "platform", SampledAt: now},
		{ID: "met_restart_" + attemptID, LogicalInstanceID: instanceID, RuntimeAttemptID: attemptID, Metric: "restart.count", Value: float64(restarts), Unit: "count", Source: "platform", SampledAt: now},
	}
	if runtimeID == "" {
		return result, nil
	}
	logs, err := a.client.ContainerLogs(ctx, runtimeID, container.LogsOptions{ShowStdout: true, ShowStderr: true, Since: "6s", Tail: "200", Timestamps: true})
	if err != nil {
		return nodeworkload.RuntimeResult{}, err
	}
	defer logs.Close()
	result.Logs, err = decodeDockerLogs(io.LimitReader(logs, 1<<20), instanceID, attemptID, now)
	if err != nil {
		return nodeworkload.RuntimeResult{}, err
	}
	return result, nil
}

func decodeDockerLogs(reader io.Reader, instanceID, attemptID string, observedAt time.Time) ([]instanceobservability.LogEntry, error) {
	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, reader); err != nil {
		return nil, err
	}
	entries := make([]instanceobservability.LogEntry, 0, 2)
	for _, stream := range []struct {
		name    string
		content string
	}{{"stdout", stdout.String()}, {"stderr", stderr.String()}} {
		message := strings.TrimSpace(strings.ReplaceAll(stream.content, "\x00", ""))
		if message == "" {
			continue
		}
		entries = append(entries, instanceobservability.LogEntry{ID: "log_" + attemptID + "_" + stream.name + "_" + strconv.FormatInt(observedAt.UnixNano(), 36), LogicalInstanceID: instanceID, RuntimeAttemptID: attemptID, Stream: stream.name, Message: message, ObservedAt: observedAt})
	}
	return entries, nil
}

func secureHostConfig(spec nodeworkload.Specification, binds []string, ports nat.PortMap, network string) *container.HostConfig {
	temporary := "rw,noexec,nosuid,nodev,size=256m"
	if spec.ExecutableTemp {
		temporary = "rw,exec,nosuid,nodev,size=256m"
	}
	return &container.HostConfig{Binds: binds, PortBindings: ports, NetworkMode: container.NetworkMode(network), RestartPolicy: container.RestartPolicy{Name: "unless-stopped"}, SecurityOpt: []string{"no-new-privileges:true"}, CapDrop: []string{"ALL"}, ReadonlyRootfs: true, Tmpfs: map[string]string{"/tmp": temporary}, Resources: container.Resources{NanoCPUs: spec.CPUMilli * 1_000_000, Memory: spec.MemoryMiB * 1024 * 1024, PidsLimit: int64Pointer(512)}}
}

func bindConfiguration(dataDir string, mounts map[string]string) ([]string, error) {
	hosts := make([]string, 0, len(mounts))
	for host := range mounts {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	binds := make([]string, 0, len(hosts))
	for _, host := range hosts {
		containerPath := strings.TrimSpace(mounts[host])
		if !strings.HasPrefix(containerPath, "/") || strings.Contains(containerPath, ":") {
			return nil, errors.New("invalid container mount path")
		}
		hostPath, err := scopedPath(dataDir, host)
		if err != nil {
			return nil, err
		}
		binds = append(binds, hostPath+":"+containerPath)
	}
	return binds, nil
}

func portConfiguration(listeners []nodeworkload.Listener) (nat.PortMap, nat.PortSet, error) {
	ports, exposed := nat.PortMap{}, nat.PortSet{}
	for _, listener := range listeners {
		port, err := nat.NewPort(listener.Protocol, strconv.Itoa(listener.InternalPort))
		if err != nil {
			return nil, nil, err
		}
		ports[port] = append(ports[port], nat.PortBinding{HostIP: "0.0.0.0", HostPort: strconv.Itoa(listener.HostPort)})
		exposed[port] = struct{}{}
	}
	return ports, exposed, nil
}

func environment(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}

func (a *Adapter) ensureImage(ctx context.Context, image string) error {
	if _, _, err := a.client.ImageInspectWithRaw(ctx, image); err == nil {
		return nil
	} else if !client.IsErrNotFound(err) {
		return err
	}
	stream, err := a.client.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer stream.Close()
	decoder := json.NewDecoder(stream)
	for {
		var message struct {
			Error string `json:"error"`
		}
		if err := decoder.Decode(&message); errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return err
		}
		if message.Error != "" {
			return errors.New(message.Error)
		}
	}
}

func (a *Adapter) scopePath(root, scope string) (string, error) { return scopedPath(root, scope) }

func (a *Adapter) objectPath(objectKey string) (string, error) {
	key := strings.TrimPrefix(objectKey, "object://")
	return scopedPath(a.backupRoot, key)
}

func scopedPath(root, relative string) (string, error) {
	clean := filepath.Clean(relative)
	if clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("Docker data path escapes workload root")
	}
	target := filepath.Join(root, clean)
	current := root
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("Docker data path cannot traverse a symlink")
		}
	}
	return target, nil
}

func specHash(spec nodeworkload.Specification) (string, error) {
	raw, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func writeArchive(source string, destination io.Writer) error {
	gzipWriter := gzip.NewWriter(destination)
	tarWriter := tar.NewWriter(gzipWriter)
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("backup cannot include symlinks")
		}
		relative, err := filepath.Rel(source, path)
		if err != nil || relative == "." {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		if err := tarWriter.WriteHeader(header); err != nil || !info.Mode().IsRegular() {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tarWriter, file)
		return errors.Join(copyErr, file.Close())
	})
	return errors.Join(err, tarWriter.Close(), gzipWriter.Close())
}

func extractArchive(source io.Reader, target string) error {
	reader, err := gzip.NewReader(source)
	if err != nil {
		return err
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		destination, err := scopedPath(target, filepath.FromSlash(header.Name))
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(destination, 0o750)
		case tar.TypeReg:
			if err = os.MkdirAll(filepath.Dir(destination), 0o750); err == nil {
				var file *os.File
				file, err = os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
				if err == nil {
					_, copyErr := io.Copy(file, tarReader)
					err = errors.Join(copyErr, file.Close())
				}
			}
		default:
			err = errors.New("unsupported backup entry")
		}
		if err != nil {
			return err
		}
	}
}

type byteCounter struct{ total int64 }

func (c *byteCounter) Write(data []byte) (int, error) {
	c.total += int64(len(data))
	return len(data), nil
}
func boolValue(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
func int64Pointer(value int64) *int64 { return &value }

var _ nodeworkload.RuntimeProvider = (*Adapter)(nil)
