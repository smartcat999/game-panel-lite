package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/smartcat999/game-panel-lite/internal/runtime/docker"
	"github.com/smartcat999/game-panel-lite/internal/worker"
)

type agentRuntime interface {
	worker.Runtime
	Console(context.Context, string, string) error
	Containers(context.Context) ([]worker.Container, error)
	Logs(context.Context, string) ([]string, error)
	Info(context.Context) (string, int, error)
}

type AgentConfig struct {
	MasterURL string
	Token     string
	NodeName  string
	PublicIP  string
	Interval  time.Duration
}

type RegisterPayload struct {
	Token         string `json:"token"`
	CPUCores      int    `json:"cpuCores"`
	MemoryTotalMB int    `json:"memoryTotalMb"`
	DiskTotalGB   int    `json:"diskTotalGb"`
	DockerVersion string `json:"dockerVersion"`
	AgentVersion  string `json:"agentVersion"`
	OSInfo        string `json:"osInfo"`
	PublicIP      string `json:"publicIp"`
}

type HeartbeatPayload struct {
	Token           string  `json:"token"`
	CPUUsagePercent float64 `json:"cpuUsagePercent"`
	MemoryUsedMB    int     `json:"memoryUsedMb"`
	DiskUsedGB      int     `json:"diskUsedGb"`
	RunningCount    int     `json:"runningCount"`
	PingLatencyMS   int     `json:"pingLatencyMs"`
}

const AgentVersion = "v0.4.48"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	dockerHost := os.Getenv("DOCKER_HOST")
	if dockerHost == "" {
		dockerHost = "unix:///var/run/docker.sock"
	}
	instanceRoot := os.Getenv("AGENT_INSTANCE_ROOT")
	if instanceRoot == "" {
		instanceRoot = "/var/lib/gamepanel/instances"
	}
	runtimeAdapter, err := docker.NewAdapter(dockerHost, instanceRoot)
	if err != nil {
		logger.Error("initialize runtime", "error", err)
		os.Exit(1)
	}
	defer runtimeAdapter.Close()

	masterURL := os.Getenv("MASTER_URL")
	if masterURL == "" {
		masterURL = os.Getenv("PANEL_URL")
	}
	token := os.Getenv("AGENT_TOKEN")
	if token == "" {
		token = os.Getenv("NODE_TOKEN")
	}
	if token == "" {
		token = os.Getenv("TOKEN")
	}
	publicIP := os.Getenv("PUBLIC_IP")

	if masterURL == "" || token == "" {
		logger.Error("missing required environment variables: MASTER_URL and AGENT_TOKEN must be provided")
		fmt.Println("Usage:")
		fmt.Println("  docker run -d --restart=always --name gamepanel-agent \\")
		fmt.Println("    -e MASTER_URL=\"https://your-panel.com\" \\")
		fmt.Println("    -e AGENT_TOKEN=\"gpl_agent_xxxx\" \\")
		fmt.Println("    -v /var/run/docker.sock:/var/run/docker.sock \\")
		fmt.Println("    -v /var/lib/gamepanel:/var/lib/gamepanel \\")
		fmt.Println("    smartcat99999/game-panel-lite-agent:v0.4.48")
		os.Exit(1)
	}

	masterURL = strings.TrimRight(masterURL, "/")

	cfg := AgentConfig{
		MasterURL: masterURL,
		Token:     token,
		PublicIP:  publicIP,
		Interval:  10 * time.Second,
	}

	logger.Info("starting gamepanel lite worker agent",
		"version", AgentVersion,
		"master_url", cfg.MasterURL,
		"os", runtime.GOOS,
		"arch", runtime.GOARCH,
	)

	// Step 1: Detect Hardware & Docker Environment
	cores := runtime.NumCPU()
	memTotalMB := getMemoryTotalMB()
	diskTotalGB := getDiskTotalGB("/")
	dockerVer, runningContainers := getDockerInfo(ctx, runtimeAdapter)
	osInfo := fmt.Sprintf("%s/%s (%s)", runtime.GOOS, runtime.GOARCH, getDistroName())

	logger.Info("detected system specifications",
		"cores", cores,
		"memory_total_mb", memTotalMB,
		"disk_total_gb", diskTotalGB,
		"docker_version", dockerVer,
		"running_containers", runningContainers,
		"os_info", osInfo,
	)

	client := &http.Client{Timeout: 10 * time.Second}

	// Step 2: Initial Registration
	regPayload := RegisterPayload{
		Token:         cfg.Token,
		CPUCores:      cores,
		MemoryTotalMB: memTotalMB,
		DiskTotalGB:   diskTotalGB,
		DockerVersion: dockerVer,
		AgentVersion:  AgentVersion,
		OSInfo:        osInfo,
		PublicIP:      cfg.PublicIP,
	}

	registered := false
	for i := 0; i < 5; i++ {
		start := time.Now()
		if err := sendRegister(ctx, client, cfg.MasterURL, regPayload); err != nil {
			logger.Warn("registration attempt failed, retrying in 3s...", "attempt", i+1, "error", err)
			if !retryDelay(ctx, 3*time.Second) {
				return
			}
			continue
		}
		latency := time.Since(start).Milliseconds()
		logger.Info("successfully registered node with master panel", "latency_ms", latency)
		registered = true
		break
	}

	if !registered {
		logger.Error("failed to register with master panel after 5 attempts, will continue attempting via heartbeat")
	}

	// Step 3: Start background Reverse Tunnel Loop & Log Streamer
	var loops sync.WaitGroup
	loops.Add(2)
	go func() { defer loops.Done(); startTunnelLoop(ctx, cfg, logger) }()
	go func() { defer loops.Done(); startLogStreamerLoop(ctx, client, cfg, logger, runtimeAdapter) }()
	defer func() { cancel(); loops.Wait() }()

	logger.Info("agent entered active heartbeat loop", "interval_sec", cfg.Interval.Seconds())
	runAgentControlLoops(ctx, cfg.Interval, func(ctx context.Context) {
		reportAgentHeartbeat(ctx, client, cfg, logger, runtimeAdapter)
	}, func(ctx context.Context) {
		reconcileAssignments(ctx, client, cfg, logger, runtimeAdapter)
		if ctx.Err() == nil {
			pollAndExecuteTasks(ctx, client, cfg, logger, runtimeAdapter)
		}
	})
	logger.Info("received shutdown signal, terminating worker agent")
}

func reportAgentHeartbeat(ctx context.Context, client *http.Client, cfg AgentConfig, logger *slog.Logger, runtime agentRuntime) {
	memUsedMB := getMemoryUsedMB()
	diskUsedGB := getDiskUsedGB("/")
	_, activeCount := getDockerInfo(ctx, runtime)
	cpuPercent := getCPUUsagePercent()
	payload := HeartbeatPayload{
		Token: cfg.Token, CPUUsagePercent: cpuPercent,
		MemoryUsedMB: memUsedMB, DiskUsedGB: diskUsedGB, RunningCount: activeCount,
	}
	start := time.Now()
	if err := sendHeartbeat(ctx, client, cfg.MasterURL, payload); err != nil {
		logger.Warn("failed to send heartbeat to master", "error", err)
	} else {
		logger.Debug("heartbeat reported successfully", "latency_ms", time.Since(start).Milliseconds(), "cpu_usage", cpuPercent, "mem_used_mb", memUsedMB)
	}
}

type TunnelRequest struct {
	StreamID   string `json:"streamId"`
	NodeID     string `json:"nodeId"`
	ServerID   string `json:"serverId"`
	TargetPort int    `json:"targetPort"`
}

func startTunnelLoop(ctx context.Context, cfg AgentConfig, logger *slog.Logger) {
	var bridges sync.WaitGroup
	defer bridges.Wait()
	logger.Info("started reverse stream tunnel listener loop")
	client := &http.Client{Timeout: 30 * time.Second}

	for ctx.Err() == nil {
		url := fmt.Sprintf("%s/api/agent/tunnel/poll", cfg.MasterURL)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			if !retryDelay(ctx, 3*time.Second) {
				return
			}
			continue
		}
		req.Header.Set("X-Node-Token", cfg.Token)

		resp, err := client.Do(req)
		if err != nil {
			if !retryDelay(ctx, 2*time.Second) {
				return
			}
			continue
		}

		if resp.StatusCode == http.StatusOK {
			var tunnelReq TunnelRequest
			if err := json.NewDecoder(resp.Body).Decode(&tunnelReq); err == nil && tunnelReq.StreamID != "" {
				logger.Info("received incoming tunnel stream request", "stream_id", tunnelReq.StreamID, "target_port", tunnelReq.TargetPort)
				bridges.Add(1)
				go func() { defer bridges.Done(); bridgeReverseStream(ctx, cfg, tunnelReq, logger) }()
			}
		}
		resp.Body.Close()
	}
}

func bridgeReverseStream(ctx context.Context, cfg AgentConfig, req TunnelRequest, logger *slog.Logger) {
	targetPort := req.TargetPort
	if targetPort < 1 || targetPort > 65535 {
		logger.Warn("invalid tunnel target port", "port", targetPort)
		return
	}
	dialer := net.Dialer{Timeout: 1500 * time.Millisecond}
	localConn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", targetPort))
	if err != nil {
		logger.Warn("failed to connect to local game port for tunnel", "port", targetPort, "error", err)
		return
	}
	defer localConn.Close()
	stopLocal := context.AfterFunc(ctx, func() { _ = localConn.Close() })
	defer stopLocal()
	logger.Info("bridge connected to local game container port successfully", "stream_id", req.StreamID)

	// 2. Connect to Master tunnel endpoint
	// Determine if HTTPS or HTTP
	isHTTPS := strings.HasPrefix(cfg.MasterURL, "https://")
	hostPort := strings.TrimPrefix(strings.TrimPrefix(cfg.MasterURL, "https://"), "http://")
	if !strings.Contains(hostPort, ":") {
		if isHTTPS {
			hostPort += ":443"
		} else {
			hostPort += ":80"
		}
	}

	var rawConn net.Conn
	if isHTTPS {
		serverHost := strings.Split(hostPort, ":")[0]
		tlsDialer := &tls.Dialer{
			Config: &tls.Config{
				ServerName: serverHost,
			},
			NetDialer: &net.Dialer{
				Timeout: 8 * time.Second,
			},
		}
		rawConn, err = tlsDialer.DialContext(ctx, "tcp", hostPort)
	} else {
		rawConn, err = (&net.Dialer{Timeout: 8 * time.Second}).DialContext(ctx, "tcp", hostPort)
	}
	if err != nil {
		logger.Warn("failed to dial master for tunnel connection", "error", err)
		return
	}
	defer rawConn.Close()
	stopRemote := context.AfterFunc(ctx, func() { _ = rawConn.Close() })
	defer stopRemote()
	logger.Info("bridge connected to master tunnel endpoint successfully", "stream_id", req.StreamID)

	reqPath := fmt.Sprintf("/api/agent/tunnel/connect?streamId=%s", req.StreamID)
	hostHeader := strings.Split(hostPort, ":")[0]
	handshake := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: stream-tunnel\r\nConnection: Upgrade\r\nX-Node-Token: %s\r\n\r\n", reqPath, hostHeader, cfg.Token)
	if _, err := rawConn.Write([]byte(handshake)); err != nil {
		logger.Warn("failed to send tunnel handshake", "error", err)
		return
	}

	// Read response headers until \r\n\r\n
	buf := make([]byte, 1024)
	n, err := rawConn.Read(buf)
	if err != nil || !strings.Contains(string(buf[:n]), "101") {
		logger.Warn("invalid tunnel handshake response from master", "response", string(buf[:n]), "error", err)
		return
	}

	logger.Info("tunnel bridge fully active, bidirectional streaming game traffic", "stream_id", req.StreamID, "target_port", targetPort)

	// Bi-directional pipe
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(localConn, rawConn)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(rawConn, localConn)
		done <- struct{}{}
	}()

	<-done
	logger.Info("tunnel bridge closed", "stream_id", req.StreamID)
}

func startLogStreamerLoop(ctx context.Context, client *http.Client, cfg AgentConfig, logger *slog.Logger, runtime agentRuntime) {
	logger.Info("started background container log streamer")
	lastSentDigest := make(map[string][sha256.Size]byte)
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		pollCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		containers, err := runtime.Containers(pollCtx)
		cancel()
		if err != nil {
			continue
		}
		active := make(map[string]bool, len(containers))
		for _, c := range containers {
			active[c.ServerID] = true
		}
		for id := range lastSentDigest {
			if !active[id] {
				delete(lastSentDigest, id)
			}
		}
		for _, c := range containers {
			logCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			lines, err := runtime.Logs(logCtx, c.ID)
			cancel()
			if err != nil || len(lines) == 0 {
				continue
			}
			payload, _ := json.Marshal(map[string][]string{"lines": lines})
			digest := sha256.Sum256(payload)
			if lastSentDigest[c.ServerID] == digest {
				continue
			}
			endpoint := fmt.Sprintf("%s/api/agent/servers/%s/logs", cfg.MasterURL, url.PathEscape(c.ServerID))
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
			if err != nil {
				continue
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Node-Token", cfg.Token)
			response, err := client.Do(req)
			if err != nil {
				logger.Warn("failed to upload log snapshot", "server_id", c.ServerID, "error", err)
				continue
			}
			if response.StatusCode >= 200 && response.StatusCode < 300 {
				lastSentDigest[c.ServerID] = digest
			}
			response.Body.Close()
		}
	}
}

func pollAndExecuteTasks(ctx context.Context, client *http.Client, cfg AgentConfig, logger *slog.Logger, runtime agentRuntime) {
	url := fmt.Sprintf("%s/api/agent/tasks", cfg.MasterURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	req.Header.Set("X-Node-Token", cfg.Token)
	req.Header.Set("X-Node-ID", cfg.Token)
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return
	}
	defer resp.Body.Close()

	var tasks []NodeTask
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil || len(tasks) == 0 {
		return
	}

	for _, task := range tasks {
		if ctx.Err() != nil {
			return
		}
		logger.Info("executing remote node task", "task_id", task.ID, "action", task.Action, "server_id", task.ServerID)
		var taskErr error
		if task.Action != "exec_command" {
			taskErr = fmt.Errorf("lifecycle task %q is obsolete; workload state is reconciled from assignments", task.Action)
		} else {
			taskErr = executeRuntimeTask(ctx, task, runtime)
		}
		ackStatus := "completed"
		errMsg := ""
		if taskErr != nil {
			ackStatus = "failed"
			errMsg = taskErr.Error()
			logger.Error("failed to execute node task", "task_id", task.ID, "error", taskErr)
		} else {
			logger.Info("task completed successfully", "task_id", task.ID)
		}
		_ = ackTask(ctx, client, cfg.MasterURL, task.ID, ackStatus, errMsg, cfg.Token)
	}
}

type NodeTask struct {
	ID       string `json:"id"`
	NodeID   string `json:"nodeId"`
	ServerID string `json:"serverId"`
	Action   string `json:"action"`
	Payload  string `json:"payload"`
	Image    string `json:"image"`
	Env      string `json:"env"`
	Ports    string `json:"ports"`
	Status   string `json:"status"`
}

func ackTask(ctx context.Context, client *http.Client, masterURL, taskID, status, errMsg, token string) error {
	url := fmt.Sprintf("%s/api/agent/tasks/%s/ack", masterURL, taskID)
	payload := map[string]string{
		"status": status,
		"error":  errMsg,
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Node-Token", token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func executeRuntimeTask(ctx context.Context, task NodeTask, runtime agentRuntime) error {
	if task.Action != "exec_command" {
		return fmt.Errorf("lifecycle task %q is obsolete; use assignments", task.Action)
	}
	if strings.TrimSpace(task.Payload) == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	state, err := runtime.Inspect(ctx, task.ServerID)
	if err != nil {
		return err
	}
	if !state.Exists || !state.Managed || state.ServerID != task.ServerID || state.NodeID != task.NodeID {
		return fmt.Errorf("console target is not owned by the assigned server and node")
	}
	return runtime.Console(ctx, task.ServerID, task.Payload)
}

func sendRegister(ctx context.Context, client *http.Client, masterURL string, payload RegisterPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/api/agent/register", masterURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("master returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func sendHeartbeat(ctx context.Context, client *http.Client, masterURL string, payload HeartbeatPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/api/agent/heartbeat", masterURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("master returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// System info helpers
func getMemoryTotalMB() int {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 8192 // fallback
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				if kb, err := strconv.Atoi(parts[1]); err == nil {
					return kb / 1024
				}
			}
		}
	}
	return 8192
}

func getMemoryUsedMB() int {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 1024
	}
	var total, avail int
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				total, _ = strconv.Atoi(parts[1])
			}
		} else if strings.HasPrefix(line, "MemAvailable:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				avail, _ = strconv.Atoi(parts[1])
			}
		}
	}
	if total > 0 && avail > 0 {
		return (total - avail) / 1024
	}
	return total / 2 / 1024
}

func getDiskTotalGB(path string) int {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 100
	}
	return int((stat.Blocks * uint64(stat.Bsize)) / (1024 * 1024 * 1024))
}

func getDiskUsedGB(path string) int {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 20
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	return int((total - free) / (1024 * 1024 * 1024))
}

func getCPUUsagePercent() float64 {
	// Simple estimate from /proc/stat
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 5.0
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return 5.0
	}
	fields := strings.Fields(lines[0])
	if len(fields) < 5 || fields[0] != "cpu" {
		return 5.0
	}
	var total, idle uint64
	for i := 1; i < len(fields); i++ {
		val, _ := strconv.ParseUint(fields[i], 10, 64)
		total += val
		if i == 4 {
			idle = val
		}
	}
	if total == 0 {
		return 5.0
	}
	usage := float64(total-idle) / float64(total) * 100.0
	if usage < 0.1 {
		usage = 1.0
	}
	return usage
}

func getDistroName() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "Linux"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			val := strings.TrimPrefix(line, "PRETTY_NAME=")
			return strings.Trim(val, "\"")
		}
	}
	return "Linux"
}

func getDockerInfo(ctx context.Context, runtime agentRuntime) (string, int) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	version, count, err := runtime.Info(ctx)
	if err != nil {
		return "N/A", 0
	}
	return version, count
}

func retryDelay(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
