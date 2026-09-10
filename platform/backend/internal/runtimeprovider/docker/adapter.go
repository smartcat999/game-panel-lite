package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/nodeexecution"
)

var safeInstanceID = regexp.MustCompile(`^lin_[A-Za-z0-9]+$`)

type Adapter struct {
	client *client.Client
}

func New(host string) (*Adapter, error) {
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
	return &Adapter{client: dockerClient}, nil
}

func (a *Adapter) Reconcile(ctx context.Context, spec nodeexecution.WorkloadSpec) (nodeexecution.WorkloadObservation, error) {
	if err := validateSpec(spec); err != nil {
		return nodeexecution.WorkloadObservation{}, err
	}
	name := "gamepanel-" + spec.LogicalInstanceID
	inspected, inspectErr := a.client.ContainerInspect(ctx, name)
	if spec.DesiredState == "stopped" {
		if client.IsErrNotFound(inspectErr) {
			return nodeexecution.WorkloadObservation{State: "stopped"}, nil
		}
		if inspectErr != nil {
			return nodeexecution.WorkloadObservation{}, inspectErr
		}
		if inspected.State != nil && inspected.State.Running {
			timeout := 15
			if err := a.client.ContainerStop(ctx, inspected.ID, container.StopOptions{Timeout: &timeout}); err != nil {
				return nodeexecution.WorkloadObservation{}, err
			}
		}
		return nodeexecution.WorkloadObservation{RuntimeID: inspected.ID, State: "stopped"}, nil
	}
	hash, err := specHash(spec)
	if err != nil {
		return nodeexecution.WorkloadObservation{}, err
	}
	if inspectErr == nil && inspected.Config != nil && inspected.Config.Labels["gamepanel.spec-hash"] == hash {
		if inspected.State == nil || !inspected.State.Running {
			if err := a.client.ContainerStart(ctx, inspected.ID, types.ContainerStartOptions{}); err != nil {
				return nodeexecution.WorkloadObservation{}, err
			}
		}
		return nodeexecution.WorkloadObservation{RuntimeID: inspected.ID, State: "running"}, nil
	}
	if inspectErr != nil && !client.IsErrNotFound(inspectErr) {
		return nodeexecution.WorkloadObservation{}, inspectErr
	}
	if err := prepareData(spec); err != nil {
		return nodeexecution.WorkloadObservation{}, err
	}
	if err := a.ensureImage(ctx, spec.Image); err != nil {
		return nodeexecution.WorkloadObservation{}, err
	}
	if inspectErr == nil {
		if err := a.client.ContainerRemove(ctx, inspected.ID, types.ContainerRemoveOptions{Force: true}); err != nil {
			return nodeexecution.WorkloadObservation{}, err
		}
	}
	ports, exposed, err := portConfiguration(spec)
	if err != nil {
		return nodeexecution.WorkloadObservation{}, err
	}
	binds, err := bindConfiguration(spec)
	if err != nil {
		return nodeexecution.WorkloadObservation{}, err
	}
	response, err := a.client.ContainerCreate(ctx, &container.Config{Image: spec.Image, Env: append([]string(nil), spec.Environment...), Cmd: append([]string(nil), spec.Command...), ExposedPorts: exposed, OpenStdin: true, AttachStdin: true, Labels: map[string]string{"gamepanel.instance": spec.LogicalInstanceID, "gamepanel.spec-hash": hash, "gamepanel.fencing-token": strconv.FormatInt(spec.FencingToken, 10)}}, secureHostConfig(spec, binds, ports), nil, nil, name)
	if err != nil {
		return nodeexecution.WorkloadObservation{}, err
	}
	if err := a.client.ContainerStart(ctx, response.ID, types.ContainerStartOptions{}); err != nil {
		return nodeexecution.WorkloadObservation{}, err
	}
	return nodeexecution.WorkloadObservation{RuntimeID: response.ID, State: "running"}, nil
}

func validateSpec(spec nodeexecution.WorkloadSpec) error {
	if !safeInstanceID.MatchString(spec.LogicalInstanceID) || (spec.DesiredState != "running" && spec.DesiredState != "stopped") || strings.TrimSpace(spec.Image) == "" || !filepath.IsAbs(spec.DataDir) {
		return errors.New("invalid Docker workload spec")
	}
	if spec.Port < 1 || spec.Port > 65535 || spec.HostPort < 0 || spec.HostPort > 65535 || (spec.Protocol != "tcp" && spec.Protocol != "udp") || spec.CPUUnits < 1 || spec.MemoryMegabytes < 1 {
		return errors.New("invalid Docker workload resources or network")
	}
	if info, err := os.Lstat(spec.DataDir); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Docker workload data root cannot be a symlink")
	}
	return nil
}

func prepareData(spec nodeexecution.WorkloadSpec) error {
	if err := os.MkdirAll(spec.DataDir, 0o750); err != nil {
		return err
	}
	for name, content := range spec.Files {
		target, err := scopedPath(spec.DataDir, name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(content), 0o640); err != nil {
			return err
		}
	}
	for host := range spec.DataMounts {
		target, err := scopedPath(spec.DataDir, host)
		if err != nil {
			return err
		}
		if filepath.Ext(target) == "" {
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
		}
	}
	return nil
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

func bindConfiguration(spec nodeexecution.WorkloadSpec) ([]string, error) {
	hosts := make([]string, 0, len(spec.DataMounts))
	for host := range spec.DataMounts {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	binds := make([]string, 0, len(hosts))
	for _, host := range hosts {
		containerPath := strings.TrimSpace(spec.DataMounts[host])
		if !strings.HasPrefix(containerPath, "/") || strings.Contains(containerPath, ":") {
			return nil, errors.New("invalid container mount path")
		}
		hostPath, err := scopedPath(spec.DataDir, host)
		if err != nil {
			return nil, err
		}
		binds = append(binds, hostPath+":"+containerPath)
	}
	return binds, nil
}

func portConfiguration(spec nodeexecution.WorkloadSpec) (nat.PortMap, nat.PortSet, error) {
	port, err := nat.NewPort(spec.Protocol, strconv.Itoa(spec.Port))
	if err != nil {
		return nil, nil, err
	}
	hostPort := ""
	if spec.HostPort > 0 {
		hostPort = strconv.Itoa(spec.HostPort)
	}
	return nat.PortMap{port: {{HostIP: "0.0.0.0", HostPort: hostPort}}}, nat.PortSet{port: struct{}{}}, nil
}

func secureHostConfig(spec nodeexecution.WorkloadSpec, binds []string, ports nat.PortMap) *container.HostConfig {
	return &container.HostConfig{Binds: binds, PortBindings: ports, RestartPolicy: container.RestartPolicy{Name: "unless-stopped"}, SecurityOpt: []string{"no-new-privileges:true"}, CapDrop: []string{"ALL"}, Resources: container.Resources{NanoCPUs: int64(spec.CPUUnits) * 1_000_000, Memory: int64(spec.MemoryMegabytes) * 1024 * 1024, PidsLimit: int64Pointer(512)}}
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

func specHash(spec nodeexecution.WorkloadSpec) (string, error) {
	raw, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func int64Pointer(value int64) *int64 { return &value }

var _ nodeexecution.RuntimeProvider = (*Adapter)(nil)
