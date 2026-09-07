// Package docker implements the worker runtime against the Docker SDK.
package docker

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/go-connections/nat"
	"github.com/smartcat999/game-panel-lite/internal/worker"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

const labelServer = "io.gamepanel.server-id"
const labelNode = "io.gamepanel.node-id"
const labelUID = "io.gamepanel.assignment-uid"
const labelGeneration = "io.gamepanel.generation"
const labelManaged = "io.gamepanel.managed"

type Adapter struct {
	client  *client.Client
	dataDir string
}

var _ worker.Runtime = (*Adapter)(nil)

func NewAdapter(host, dataDir string) (*Adapter, error) {
	cli, err := client.NewClientWithOpts(client.WithHost(host), client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Adapter{client: cli, dataDir: dataDir}, nil
}
func (a *Adapter) Close() error { return a.client.Close() }

func containerName(serverID string) (string, error) {
	if serverID == "" || serverID == "." || strings.Contains(serverID, "..") || strings.ContainsAny(serverID, "/\\") || strings.TrimSpace(serverID) != serverID {
		return "", fmt.Errorf("invalid server ID")
	}
	return "gamepanel-" + serverID, nil
}
func (a *Adapter) Inspect(ctx context.Context, serverID string) (worker.State, error) {
	name, err := containerName(serverID)
	if err != nil {
		return worker.State{}, err
	}
	result, err := a.client.ContainerInspect(ctx, name)
	if client.IsErrNotFound(err) {
		return worker.State{}, nil
	}
	if err != nil {
		return worker.State{}, err
	}
	state := worker.State{Exists: true, ID: result.ID}
	if result.State != nil {
		state.Running = result.State.Running
	}
	if result.Config != nil {
		labels := result.Config.Labels
		state.Managed = labels[labelManaged] == "true"
		state.UID = labels[labelUID]
		state.ServerID = labels[labelServer]
		state.NodeID = labels[labelNode]
		state.Generation, _ = strconv.Atoi(labels[labelGeneration])
	}
	return state, nil
}
func (a *Adapter) Create(ctx context.Context, assignment workload.Assignment) error {
	name, err := containerName(assignment.ServerID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(assignment.Spec.Image) == "" {
		return fmt.Errorf("workload image is required")
	}
	if assignment.Spec.Resources.CPULimitCores < 0 || assignment.Spec.Resources.MemoryLimitMB < 0 {
		return fmt.Errorf("negative workload resource limit")
	}
	ports, bindings, err := networkBindings(assignment.Spec.Network)
	if err != nil {
		return err
	}
	unlock, err := lockInstanceCreation(ctx, a.dataDir, assignment.ServerID)
	if err != nil {
		return err
	}
	defer unlock()
	existing, err := a.Inspect(ctx, assignment.ServerID)
	if err != nil {
		return err
	}
	if existing.Exists {
		return fmt.Errorf("instance container already exists; observe before creating")
	}
	stream, err := a.client.ImagePull(ctx, assignment.Spec.Image, types.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer stream.Close()
	if err := consumePull(stream); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	instanceDir := filepath.Join(a.dataDir, assignment.ServerID)
	_, err = prepareFilesAndCommit(instanceDir, assignment.Spec.Options, func(binds []string) error {
		host := &container.HostConfig{
			Binds: binds, PortBindings: bindings,
			RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
			Resources:     container.Resources{NanoCPUs: int64(assignment.Spec.Resources.CPULimitCores * 1e9), Memory: int64(assignment.Spec.Resources.MemoryLimitMB) * 1024 * 1024},
		}
		_, createErr := a.client.ContainerCreate(ctx, &container.Config{
			Image: assignment.Spec.Image, Env: assignment.Spec.Options.Env, Cmd: assignment.Spec.Options.Cmd,
			OpenStdin: true, AttachStdin: true, ExposedPorts: ports,
			Labels: map[string]string{labelManaged: "true", labelServer: assignment.ServerID, labelNode: assignment.NodeID, labelUID: assignment.UID, labelGeneration: strconv.Itoa(assignment.Generation)},
		}, host, nil, nil, name)
		if createErr == nil {
			return nil
		}
		verifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		observed, inspectErr := a.Inspect(verifyCtx, assignment.ServerID)
		if inspectErr != nil {
			return errors.Join(errCreationUncertain, createErr, inspectErr)
		}
		if !observed.Exists {
			return createErr
		}
		if observed.Managed && observed.ServerID == assignment.ServerID && observed.NodeID == assignment.NodeID && observed.UID == assignment.UID && observed.Generation == assignment.Generation {
			return nil
		}
		return errors.Join(errCreationUncertain, createErr)
	})
	return err
}
func (a *Adapter) Start(ctx context.Context, observed worker.State) error {
	id, err := mutationContainerID(observed)
	if err != nil {
		return err
	}
	unlock, err := lockInstanceCreation(ctx, a.dataDir, observed.ServerID)
	if err != nil {
		return err
	}
	defer unlock()
	if err := checkInstanceRecovery(a.dataDir, observed.ServerID); err != nil {
		return err
	}
	err = a.client.ContainerStart(ctx, id, types.ContainerStartOptions{})
	if errdefs.IsNotModified(err) {
		return nil
	}
	return err
}
func (a *Adapter) Stop(ctx context.Context, observed worker.State) error {
	id, err := mutationContainerID(observed)
	if err != nil {
		return err
	}
	unlock, err := lockInstanceCreation(ctx, a.dataDir, observed.ServerID)
	if err != nil {
		return err
	}
	defer unlock()
	timeout := 20
	err = a.client.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout})
	if errdefs.IsNotModified(err) || client.IsErrNotFound(err) {
		return nil
	}
	return err
}
func (a *Adapter) Remove(ctx context.Context, observed worker.State) error {
	id, err := mutationContainerID(observed)
	if err != nil {
		return err
	}
	unlock, err := lockInstanceCreation(ctx, a.dataDir, observed.ServerID)
	if err != nil {
		return err
	}
	defer unlock()
	err = a.client.ContainerRemove(ctx, id, types.ContainerRemoveOptions{Force: true})
	if client.IsErrNotFound(err) {
		return nil
	}
	return err
}
func (a *Adapter) Console(ctx context.Context, serverID, input string) error {
	name, err := containerName(serverID)
	if err != nil {
		return err
	}
	connection, err := a.client.ContainerAttach(ctx, name, types.ContainerAttachOptions{Stream: true, Stdin: true})
	if err != nil {
		return err
	}
	defer connection.Close()
	deadline := time.Now().Add(10 * time.Second)
	if value, ok := ctx.Deadline(); ok && value.Before(deadline) {
		deadline = value
	}
	if err := connection.Conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	_, err = connection.Conn.Write([]byte(strings.TrimSpace(input) + "\n"))
	return err
}

func (a *Adapter) Containers(ctx context.Context) ([]worker.Container, error) {
	items, err := a.client.ContainerList(ctx, types.ContainerListOptions{All: true, Filters: filters.NewArgs(filters.Arg("label", labelManaged+"=true"))})
	if err != nil {
		return nil, err
	}
	result := make([]worker.Container, 0, len(items))
	for _, item := range items {
		if id := item.Labels[labelServer]; id != "" {
			result = append(result, worker.Container{ID: item.ID, ServerID: id})
		}
	}
	return result, nil
}
func (a *Adapter) Logs(ctx context.Context, id string) ([]string, error) {
	stream, err := a.client.ContainerLogs(ctx, id, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Tail: "200"})
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	data, err := io.ReadAll(io.LimitReader(stream, 4<<20))
	if err != nil {
		return nil, err
	}
	return cleanLogLines(data), nil
}
func (a *Adapter) Info(ctx context.Context) (string, int, error) {
	info, err := a.client.Info(ctx)
	if err != nil {
		return "", 0, err
	}
	return info.ServerVersion, info.ContainersRunning, nil
}
func consumePull(reader io.Reader) error {
	decoder := json.NewDecoder(reader)
	for {
		var event struct {
			Error       string `json:"error"`
			ErrorDetail struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}
		err := decoder.Decode(&event)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if event.Error != "" {
			return fmt.Errorf("pull image: %s", event.Error)
		}
		if event.ErrorDetail.Message != "" {
			return fmt.Errorf("pull image: %s", event.ErrorDetail.Message)
		}
	}
}
func networkBindings(network workload.Network) (nat.PortSet, nat.PortMap, error) {
	ports := nat.PortSet{}
	bindings := nat.PortMap{}
	items := append([]workload.Port(nil), network.AdditionalPorts...)
	if network.Port != 0 {
		items = append(items, workload.Port{Port: network.Port, HostPort: network.HostPort, Protocol: network.Protocol})
	}
	for _, item := range items {
		if item.HostPort == 0 {
			item.HostPort = item.Port
		}
		if item.Protocol == "" {
			item.Protocol = "tcp"
		}
		if item.Port < 1 || item.Port > 65535 || item.HostPort < 1 || item.HostPort > 65535 || (item.Protocol != "tcp" && item.Protocol != "udp") {
			return nil, nil, fmt.Errorf("invalid workload port")
		}
		port, err := nat.NewPort(item.Protocol, strconv.Itoa(item.Port))
		if err != nil {
			return nil, nil, err
		}
		ports[port] = struct{}{}
		bindings[port] = append(bindings[port], nat.PortBinding{HostPort: strconv.Itoa(item.HostPort)})
	}
	return ports, bindings, nil
}

// Require the immutable full ID returned by Inspect, never a reusable name or
// short ID prefix. A removed target must not resolve to a replacement container.
func mutationContainerID(observed worker.State) (string, error) {
	if !observed.Exists || !observed.Managed || observed.ServerID == "" || observed.NodeID == "" || observed.UID == "" || observed.Generation <= 0 || len(observed.ID) != 64 {
		return "", fmt.Errorf("complete observed container identity is required")
	}
	if _, err := hex.DecodeString(observed.ID); err != nil {
		return "", fmt.Errorf("invalid observed container ID")
	}
	return observed.ID, nil
}
