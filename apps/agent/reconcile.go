package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

type workloadAssignment struct {
	UID          string            `json:"uid"`
	ServerID     string            `json:"serverId"`
	NodeID       string            `json:"nodeId"`
	Generation   int               `json:"generation"`
	DesiredState string            `json:"desiredState"`
	Spec         agentWorkloadSpec `json:"spec"`
}

type agentWorkloadSpec struct {
	ServerID string `json:"serverId"`
	Name     string `json:"name"`
	Image    string `json:"image"`
	Options  struct {
		Env        []string          `json:"env"`
		Cmd        []string          `json:"cmd"`
		Files      map[string]string `json:"files"`
		DataMounts []string          `json:"dataMounts"`
	} `json:"options"`
}

type workloadObservation struct {
	ObservedGeneration int       `json:"observedGeneration"`
	RuntimeID          string    `json:"runtimeId,omitempty"`
	ActualState        string    `json:"actualState"`
	LastError          string    `json:"lastError,omitempty"`
	ObservedAt         time.Time `json:"observedAt"`
}

type dockerContainerState struct {
	Exists     bool
	ID         string
	Running    bool
	UID        string
	Generation string
}

func reconcileAssignments(client *http.Client, cfg AgentConfig, logger *slog.Logger) {
	req, err := http.NewRequest(http.MethodGet, cfg.MasterURL+"/api/agent/assignments", nil)
	if err != nil {
		return
	}
	req.Header.Set("X-Node-Token", cfg.Token)
	resp, err := client.Do(req)
	if err != nil {
		logger.Warn("failed to fetch workload assignments", "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logger.Warn("master rejected workload assignment poll", "status", resp.StatusCode)
		return
	}
	var assignments []workloadAssignment
	if err := json.NewDecoder(resp.Body).Decode(&assignments); err != nil {
		logger.Warn("failed to decode workload assignments", "error", err)
		return
	}
	for _, assignment := range assignments {
		observation := reconcileWorkload(assignment, logger)
		if err := reportWorkloadObservation(client, cfg, assignment, observation); err != nil {
			logger.Warn("failed to report workload observation", "server_id", assignment.ServerID, "error", err)
		}
	}
}

func reconcileWorkload(assignment workloadAssignment, logger *slog.Logger) workloadObservation {
	return reconcileWorkloadWithClient(assignment, logger, newDockerSocketClient(90*time.Second))
}

func reconcileWorkloadWithClient(assignment workloadAssignment, logger *slog.Logger, dockerClient *http.Client) workloadObservation {
	observation := workloadObservation{
		ObservedGeneration: assignment.Generation,
		ActualState:        "unknown",
		ObservedAt:         time.Now().UTC(),
	}
	containerName := "gamepanel-" + assignment.ServerID
	state, err := inspectAgentContainer(dockerClient, containerName)
	if err != nil {
		observation.LastError = err.Error()
		return observation
	}

	switch assignment.DesiredState {
	case "running":
		generation := fmt.Sprintf("%d", assignment.Generation)
		if state.Exists && (state.UID != assignment.UID || state.Generation != generation) {
			if err := removeAgentContainer(dockerClient, containerName); err != nil {
				observation.LastError = err.Error()
				return observation
			}
			state = dockerContainerState{}
		}
		if !state.Exists {
			if err := createAgentContainer(dockerClient, assignment, logger); err != nil {
				observation.LastError = err.Error()
				return observation
			}
		}
		state, err = inspectAgentContainer(dockerClient, containerName)
		if err == nil && !state.Running {
			err = startAgentContainer(dockerClient, containerName)
		}
	case "stopped":
		if state.Exists && state.Running {
			err = stopAgentContainer(dockerClient, containerName)
		}
	case "deleted":
		if state.Exists {
			err = removeAgentContainer(dockerClient, containerName)
		}
	default:
		err = fmt.Errorf("unsupported desired state %q", assignment.DesiredState)
	}
	if err != nil {
		observation.LastError = err.Error()
		return observation
	}

	state, err = inspectAgentContainer(dockerClient, containerName)
	if err != nil {
		observation.LastError = err.Error()
		return observation
	}
	observation.RuntimeID = state.ID
	switch {
	case !state.Exists:
		observation.ActualState = "missing"
	case state.Running:
		observation.ActualState = "running"
	default:
		observation.ActualState = "stopped"
	}
	return observation
}

func reportWorkloadObservation(client *http.Client, cfg AgentConfig, assignment workloadAssignment, observation workloadObservation) error {
	payload, err := json.Marshal(observation)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/api/agent/assignments/%s/status", cfg.MasterURL, url.PathEscape(assignment.UID))
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Node-Token", cfg.Token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("master returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func newDockerSocketClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", "/var/run/docker.sock")
		}},
		Timeout: timeout,
	}
}

func inspectAgentContainer(client *http.Client, name string) (dockerContainerState, error) {
	resp, err := client.Get("http://localhost/containers/" + url.PathEscape(name) + "/json")
	if err != nil {
		return dockerContainerState{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return dockerContainerState{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return dockerContainerState{}, dockerStatusError("inspect container", resp)
	}
	var payload struct {
		ID    string `json:"Id"`
		State struct {
			Running bool `json:"Running"`
		} `json:"State"`
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return dockerContainerState{}, err
	}
	return dockerContainerState{
		Exists:     true,
		ID:         payload.ID,
		Running:    payload.State.Running,
		UID:        payload.Config.Labels["io.gamepanel.assignment-uid"],
		Generation: payload.Config.Labels["io.gamepanel.generation"],
	}, nil
}

func createAgentContainer(client *http.Client, assignment workloadAssignment, logger *slog.Logger) error {
	if strings.TrimSpace(assignment.Spec.Image) == "" {
		return fmt.Errorf("workload image is required")
	}
	instanceDir := filepath.Join("/var/lib/gamepanel/instances", assignment.ServerID)
	if err := os.MkdirAll(instanceDir, 0o777); err != nil {
		return err
	}
	if err := os.Chmod(instanceDir, 0o777); err != nil {
		return err
	}
	for filename, content := range assignment.Spec.Options.Files {
		path, err := safeAgentInstancePath(instanceDir, filename)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o666); err != nil {
			return err
		}
		if err := os.Chmod(path, 0o666); err != nil {
			return err
		}
	}
	pullURL := "http://localhost/images/create?fromImage=" + url.QueryEscape(assignment.Spec.Image)
	pullReq, _ := http.NewRequest(http.MethodPost, pullURL, nil)
	pullResp, err := client.Do(pullReq)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, pullResp.Body)
	pullResp.Body.Close()
	if pullResp.StatusCode < 200 || pullResp.StatusCode >= 300 {
		return fmt.Errorf("pull image returned status %d", pullResp.StatusCode)
	}

	mounts := assignment.Spec.Options.DataMounts
	if len(mounts) == 0 {
		mounts = []string{"/data"}
	}
	binds := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		hostPath, containerPath, err := agentDataBindPaths(instanceDir, mount)
		if err != nil {
			return err
		}
		if _, err := os.Stat(hostPath); os.IsNotExist(err) {
			if filepath.Ext(hostPath) != "" {
				if err := os.MkdirAll(filepath.Dir(hostPath), 0o777); err != nil {
					return err
				}
				if err := os.WriteFile(hostPath, nil, 0o666); err != nil {
					return err
				}
			} else if err := os.MkdirAll(hostPath, 0o777); err != nil {
				return err
			}
		}
		binds = append(binds, hostPath+":"+containerPath)
	}
	payload := map[string]any{
		"Image":       assignment.Spec.Image,
		"Env":         assignment.Spec.Options.Env,
		"Cmd":         assignment.Spec.Options.Cmd,
		"OpenStdin":   true,
		"AttachStdin": true,
		"Labels": map[string]string{
			"io.gamepanel.managed":        "true",
			"io.gamepanel.server-id":      assignment.ServerID,
			"io.gamepanel.assignment-uid": assignment.UID,
			"io.gamepanel.generation":     fmt.Sprintf("%d", assignment.Generation),
			"io.gamepanel.node-id":        assignment.NodeID,
		},
		"HostConfig": map[string]any{
			"RestartPolicy": map[string]string{"Name": "unless-stopped"},
			"NetworkMode":   "host",
			"Binds":         binds,
		},
	}
	data, _ := json.Marshal(payload)
	endpoint := "http://localhost/containers/create?name=" + url.QueryEscape("gamepanel-"+assignment.ServerID)
	req, _ := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return dockerStatusError("create container", resp)
	}
	logger.Info("created reconciled workload container", "server_id", assignment.ServerID, "generation", assignment.Generation)
	return nil
}

func startAgentContainer(client *http.Client, name string) error {
	return agentDockerPost(client, "http://localhost/containers/"+url.PathEscape(name)+"/start", http.StatusNoContent, http.StatusNotModified)
}

func stopAgentContainer(client *http.Client, name string) error {
	return agentDockerPost(client, "http://localhost/containers/"+url.PathEscape(name)+"/stop?t=20", http.StatusNoContent, http.StatusNotModified)
}

func removeAgentContainer(client *http.Client, name string) error {
	req, _ := http.NewRequest(http.MethodDelete, "http://localhost/containers/"+url.PathEscape(name)+"?force=true", nil)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return dockerStatusError("remove container", resp)
	}
	return nil
}

func agentDockerPost(client *http.Client, endpoint string, expected ...int) error {
	req, _ := http.NewRequest(http.MethodPost, endpoint, nil)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	for _, status := range expected {
		if resp.StatusCode == status {
			return nil
		}
	}
	return dockerStatusError("docker operation", resp)
}

func dockerStatusError(operation string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return fmt.Errorf("%s returned status %d: %s", operation, resp.StatusCode, strings.TrimSpace(string(body)))
}

func safeAgentInstancePath(root, relative string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(relative))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid instance path %q", relative)
	}
	path := filepath.Join(root, clean)
	if path != root && !strings.HasPrefix(path, root+string(filepath.Separator)) {
		return "", fmt.Errorf("instance path escapes root")
	}
	return path, nil
}

func agentDataBindPaths(root, mount string) (string, string, error) {
	hostPath := root
	containerPath := strings.TrimSpace(mount)
	if hostSubpath, target, ok := strings.Cut(mount, ":"); ok {
		var err error
		hostPath, err = safeAgentInstancePath(root, strings.TrimSpace(hostSubpath))
		if err != nil {
			return "", "", err
		}
		containerPath = strings.TrimSpace(target)
	}
	if !strings.HasPrefix(containerPath, "/") {
		return "", "", fmt.Errorf("invalid data mount target %q", containerPath)
	}
	return hostPath, containerPath, nil
}

func sendAgentConsoleInput(ctx context.Context, containerName, input string) error {
	dockerClient, err := client.NewClientWithOpts(client.WithHost("unix:///var/run/docker.sock"), client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer dockerClient.Close()
	connection, err := dockerClient.ContainerAttach(ctx, containerName, types.ContainerAttachOptions{
		Stream: true,
		Stdin:  true,
	})
	if err != nil {
		return err
	}
	defer connection.Close()
	_, err = connection.Conn.Write([]byte(strings.TrimSpace(input) + "\n"))
	return err
}
