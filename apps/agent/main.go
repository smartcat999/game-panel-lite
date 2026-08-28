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
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

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

const AgentVersion = "v0.4.45"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

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
		fmt.Println("    smartcat99999/game-panel-lite-agent:v0.4.35")
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
	dockerVer, runningContainers := getDockerInfo()
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
		if err := sendRegister(client, cfg.MasterURL, regPayload); err != nil {
			logger.Warn("registration attempt failed, retrying in 3s...", "attempt", i+1, "error", err)
			time.Sleep(3 * time.Second)
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
	go startTunnelLoop(cfg, logger)
	go startLogStreamerLoop(client, cfg, logger)

	// Step 4: Heartbeat Loop
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	logger.Info("agent entered active heartbeat loop", "interval_sec", cfg.Interval.Seconds())

	for {
		select {
		case <-ticker.C:
			memUsedMB := getMemoryUsedMB()
			diskUsedGB := getDiskUsedGB("/")
			_, activeCount := getDockerInfo()
			cpuPercent := getCPUUsagePercent()

			hbPayload := HeartbeatPayload{
				Token:           cfg.Token,
				CPUUsagePercent: cpuPercent,
				MemoryUsedMB:    memUsedMB,
				DiskUsedGB:      diskUsedGB,
				RunningCount:    activeCount,
			}

			start := time.Now()
			if err := sendHeartbeat(client, cfg.MasterURL, hbPayload); err != nil {
				logger.Warn("failed to send heartbeat to master", "error", err)
			} else {
				latency := int(time.Since(start).Milliseconds())
				logger.Debug("heartbeat reported successfully", "latency_ms", latency, "cpu_usage", cpuPercent, "mem_used_mb", memUsedMB)
			}

			// Poll and execute pending tasks from master
			reconcileAssignments(client, cfg, logger)
			pollAndExecuteTasks(client, cfg, logger)

		case sig := <-stopChan:
			logger.Info("received shutdown signal, terminating worker agent", "signal", sig.String())
			return
		}
	}
}

type TunnelRequest struct {
	StreamID   string `json:"streamId"`
	NodeID     string `json:"nodeId"`
	ServerID   string `json:"serverId"`
	TargetPort int    `json:"targetPort"`
}

func startTunnelLoop(cfg AgentConfig, logger *slog.Logger) {
	logger.Info("started reverse stream tunnel listener loop")
	client := &http.Client{Timeout: 30 * time.Second}

	for {
		url := fmt.Sprintf("%s/api/agent/tunnel/poll", cfg.MasterURL)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}
		req.Header.Set("X-Node-Token", cfg.Token)

		resp, err := client.Do(req)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		if resp.StatusCode == http.StatusOK {
			var tunnelReq TunnelRequest
			if err := json.NewDecoder(resp.Body).Decode(&tunnelReq); err == nil && tunnelReq.StreamID != "" {
				logger.Info("received incoming tunnel stream request", "stream_id", tunnelReq.StreamID, "target_port", tunnelReq.TargetPort)
				go bridgeReverseStream(cfg, tunnelReq, logger)
			}
		}
		resp.Body.Close()
	}
}

func bridgeReverseStream(cfg AgentConfig, req TunnelRequest, logger *slog.Logger) {
	targetPort := req.TargetPort
	if targetPort <= 0 {
		targetPort = 7777
	}

	logger.Info("bridge received tunnel request, dialing local container", "port", targetPort, "stream_id", req.StreamID)

	// 1. Connect to local game container port (try targetPort, fallback to 7777)
	localConn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", targetPort), 1500*time.Millisecond)
	if err != nil && targetPort != 7777 {
		localConn, err = net.DialTimeout("tcp", "127.0.0.1:7777", 2*time.Second)
	}
	if err != nil {
		logger.Warn("failed to connect to local game port for tunnel", "port", targetPort, "error", err)
		return
	}
	defer localConn.Close()
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
		rawConn, err = tlsDialer.Dial("tcp", hostPort)
	} else {
		rawConn, err = net.DialTimeout("tcp", hostPort, 8*time.Second)
	}
	if err != nil {
		logger.Warn("failed to dial master for tunnel connection", "error", err)
		return
	}
	defer rawConn.Close()
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

func startLogStreamerLoop(client *http.Client, cfg AgentConfig, logger *slog.Logger) {
	logger.Info("started background container log streamer")
	socketPath := "/var/run/docker.sock"
	httpc := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 5 * time.Second,
	}

	lastSentDigest := make(map[string][sha256.Size]byte)

	for {
		time.Sleep(1500 * time.Millisecond)

		// List all containers
		resp, err := httpc.Get("http://localhost/containers/json?all=1")
		if err != nil {
			continue
		}
		var containers []struct {
			ID    string   `json:"Id"`
			Names []string `json:"Names"`
			State string   `json:"State"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&containers)
		resp.Body.Close()

		for _, c := range containers {
			var serverID string
			for _, name := range c.Names {
				clean := strings.TrimPrefix(name, "/")
				if strings.HasPrefix(clean, "gamepanel-") {
					serverID = strings.TrimPrefix(clean, "gamepanel-")
					break
				}
				if len(clean) == 36 && strings.Count(clean, "-") == 4 {
					serverID = clean
					break
				}
			}
			if serverID == "" {
				continue
			}

			// Read recent 200 lines
			logsURL := fmt.Sprintf("http://localhost/containers/%s/logs?stdout=1&stderr=1&timestamps=0&tail=200", c.ID)
			lResp, lErr := httpc.Get(logsURL)
			if lErr != nil {
				continue
			}

			logData, _ := io.ReadAll(lResp.Body)
			lResp.Body.Close()

			if len(logData) == 0 {
				continue
			}

			lines := cleanDockerLogLines(logData)
			if len(lines) == 0 {
				continue
			}

			digest := sha256.Sum256(logData)
			if previous, ok := lastSentDigest[serverID]; !ok || previous != digest {
				uploadURL := fmt.Sprintf("%s/api/agent/servers/%s/logs", cfg.MasterURL, serverID)
				payload := map[string][]string{"lines": lines}
				pBytes, _ := json.Marshal(payload)
				uReq, _ := http.NewRequest(http.MethodPost, uploadURL, bytes.NewReader(pBytes))
				uReq.Header.Set("Content-Type", "application/json")
				uReq.Header.Set("X-Node-Token", cfg.Token)
				if uResp, uErr := client.Do(uReq); uErr == nil {
					if uResp.StatusCode >= 200 && uResp.StatusCode < 300 {
						lastSentDigest[serverID] = digest
					} else {
						logger.Warn("master rejected container log snapshot", "server_id", serverID, "status", uResp.StatusCode)
					}
					uResp.Body.Close()
				} else {
					logger.Warn("failed to upload container log snapshot", "server_id", serverID, "error", uErr)
				}
			}
		}
	}
}

func cleanDockerLogLines(data []byte) []string {
	var lines []string
	idx := 0
	for idx < len(data) {
		if idx+8 <= len(data) && (data[idx] == 1 || data[idx] == 2 || data[idx] == 0) && data[idx+1] == 0 && data[idx+2] == 0 && data[idx+3] == 0 {
			frameLen := int(data[idx+4])<<24 | int(data[idx+5])<<16 | int(data[idx+6])<<8 | int(data[idx+7])
			idx += 8
			if frameLen > 0 && idx+frameLen <= len(data) {
				chunk := string(data[idx : idx+frameLen])
				for _, line := range strings.Split(chunk, "\n") {
					line = strings.TrimRight(line, "\r")
					if strings.TrimSpace(line) != "" {
						lines = append(lines, line)
					}
				}
				idx += frameLen
				continue
			}
		}
		// Fallback line by line
		chunk := string(data[idx:])
		for _, line := range strings.Split(chunk, "\n") {
			line = strings.TrimRight(line, "\r")
			if strings.TrimSpace(line) != "" {
				lines = append(lines, line)
			}
		}
		break
	}
	return lines
}

func pollAndExecuteTasks(client *http.Client, cfg AgentConfig, logger *slog.Logger) {
	url := fmt.Sprintf("%s/api/agent/tasks", cfg.MasterURL)
	req, err := http.NewRequest(http.MethodGet, url, nil)
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
		logger.Info("executing remote node task", "task_id", task.ID, "action", task.Action, "server_id", task.ServerID)
		var taskErr error
		if task.Action != "exec_command" {
			taskErr = fmt.Errorf("lifecycle task %q is obsolete; workload state is reconciled from assignments", task.Action)
		} else {
			taskErr = executeDockerTask(task, logger)
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
		_ = ackTask(client, cfg.MasterURL, task.ID, ackStatus, errMsg, cfg.Token)
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

func ackTask(client *http.Client, masterURL, taskID, status, errMsg, token string) error {
	url := fmt.Sprintf("%s/api/agent/tasks/%s/ack", masterURL, taskID)
	payload := map[string]string{
		"status": status,
		"error":  errMsg,
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
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

func executeDockerTask(task NodeTask, logger *slog.Logger) error {
	socketPath := "/var/run/docker.sock"
	httpc := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 60 * time.Second,
	}

	containerName := fmt.Sprintf("gamepanel-%s", task.ServerID)

	switch task.Action {
	case "exec_command":
		cmd := task.Payload
		if cmd == "" {
			return nil
		}
		return sendAgentConsoleInput(context.Background(), containerName, cmd)

	case "stop":
		url := fmt.Sprintf("http://localhost/containers/%s/stop?t=10", containerName)
		req, _ := http.NewRequest(http.MethodPost, url, nil)
		resp, err := httpc.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		return nil

	case "delete":
		url := fmt.Sprintf("http://localhost/containers/%s?force=true", containerName)
		req, _ := http.NewRequest(http.MethodDelete, url, nil)
		resp, err := httpc.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		return nil

	case "start", "create", "restart":
		// 1. Parse Workload Spec Payload if available
		type WorkloadPayload struct {
			Image   string `json:"image"`
			DataDir string `json:"dataDir"`
			Options struct {
				Env        []string          `json:"env"`
				Cmd        []string          `json:"cmd"`
				Files      map[string]string `json:"files"`
				DataMounts []string          `json:"dataMounts"`
			} `json:"options"`
		}

		var spec WorkloadPayload
		if task.Payload != "" {
			_ = json.Unmarshal([]byte(task.Payload), &spec)
		}

		image := task.Image
		if image == "" && spec.Image != "" {
			image = spec.Image
		}
		if image == "" {
			image = "smartcat99999/terraria-vanilla:1.4.5.6"
		}

		// 2. Prepare Local Host Instance Directory & Config Files
		localInstanceDir := fmt.Sprintf("/var/lib/gamepanel/instances/%s", task.ServerID)
		_ = os.MkdirAll(localInstanceDir, 0o777)
		_ = os.Chmod(localInstanceDir, 0o777)
		_ = os.MkdirAll(fmt.Sprintf("%s/Worlds", localInstanceDir), 0o777)
		_ = os.Chmod(fmt.Sprintf("%s/Worlds", localInstanceDir), 0o777)
		_ = os.MkdirAll(fmt.Sprintf("%s/logs", localInstanceDir), 0o777)
		_ = os.Chmod(fmt.Sprintf("%s/logs", localInstanceDir), 0o777)
		for filename, content := range spec.Options.Files {
			filePath := fmt.Sprintf("%s/%s", localInstanceDir, filename)
			_ = os.WriteFile(filePath, []byte(content), 0o666)
			_ = os.Chmod(filePath, 0o666)
			logger.Info("wrote container config file on worker node", "file", filePath, "bytes", len(content))
		}

		// 3. Check if container already exists
		checkURL := fmt.Sprintf("http://localhost/containers/%s/json", containerName)
		checkReq, _ := http.NewRequest(http.MethodGet, checkURL, nil)
		checkResp, err := httpc.Do(checkReq)
		if err == nil && checkResp.StatusCode == http.StatusOK {
			checkResp.Body.Close()
			// Remove old container to apply new volume mounts/envs cleanly
			rmURL := fmt.Sprintf("http://localhost/containers/%s?force=true", containerName)
			rmReq, _ := http.NewRequest(http.MethodDelete, rmURL, nil)
			if rmResp, rmErr := httpc.Do(rmReq); rmErr == nil {
				rmResp.Body.Close()
			}
		}
		if checkResp != nil {
			checkResp.Body.Close()
		}

		// 4. Pull Image if necessary
		pullURL := fmt.Sprintf("http://localhost/images/create?fromImage=%s", image)
		pullReq, _ := http.NewRequest(http.MethodPost, pullURL, nil)
		if pResp, pErr := httpc.Do(pullReq); pErr == nil {
			_, _ = io.Copy(io.Discard, pResp.Body)
			pResp.Body.Close()
		}

		// 5. Build exact Binds list from spec.Options.DataMounts
		var binds []string
		for _, mount := range spec.Options.DataMounts {
			if mount == "" {
				continue
			}
			if hostSub, contSub, ok := strings.Cut(mount, ":"); ok {
				hostPath := fmt.Sprintf("%s/%s", localInstanceDir, strings.TrimSpace(hostSub))
				if strings.HasSuffix(hostSub, ".txt") || strings.HasSuffix(hostSub, ".json") {
					if _, err := os.Stat(hostPath); os.IsNotExist(err) {
						_ = os.WriteFile(hostPath, []byte(""), 0o666)
					}
				} else {
					_ = os.MkdirAll(hostPath, 0o777)
				}
				binds = append(binds, fmt.Sprintf("%s:%s", hostPath, strings.TrimSpace(contSub)))
			}
		}
		if len(binds) == 0 {
			binds = []string{
				fmt.Sprintf("%s/serverconfig.txt:/home/container/serverconfig.txt", localInstanceDir),
				fmt.Sprintf("%s/Worlds:/home/container/Worlds", localInstanceDir),
				fmt.Sprintf("%s/logs:/home/container/logs", localInstanceDir),
			}
		}

		// 6. Ensure full recursive permissions across all files and dirs in instance path
		_ = filepath.Walk(localInstanceDir, func(path string, info os.FileInfo, err error) error {
			if err == nil {
				if info.IsDir() {
					_ = os.Chmod(path, 0o777)
				} else {
					_ = os.Chmod(path, 0o666)
				}
			}
			return nil
		})

		// 7. Create Container with host network, volume binds, envs, cmd, OpenStdin, User root
		createURL := fmt.Sprintf("http://localhost/containers/create?name=%s", containerName)
		createPayload := map[string]interface{}{
			"Image":       image,
			"User":        "0:0",
			"Env":         spec.Options.Env,
			"Cmd":         spec.Options.Cmd,
			"OpenStdin":   true,
			"AttachStdin": true,
			"HostConfig": map[string]interface{}{
				"RestartPolicy": map[string]interface{}{
					"Name": "unless-stopped",
				},
				"NetworkMode": "host",
				"Binds":       binds,
			},
		}
		createData, _ := json.Marshal(createPayload)
		cReq, _ := http.NewRequest(http.MethodPost, createURL, bytes.NewReader(createData))
		cReq.Header.Set("Content-Type", "application/json")
		cResp, err := httpc.Do(cReq)
		if err != nil {
			return err
		}
		defer cResp.Body.Close()

		// 6. Start Container
		startURL := fmt.Sprintf("http://localhost/containers/%s/start", containerName)
		sReq, _ := http.NewRequest(http.MethodPost, startURL, nil)
		sResp, err := httpc.Do(sReq)
		if err != nil {
			return err
		}
		defer sResp.Body.Close()
		return nil
	}
	return nil
}

func sendRegister(client *http.Client, masterURL string, payload RegisterPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/api/agent/register", masterURL)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
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

func sendHeartbeat(client *http.Client, masterURL string, payload HeartbeatPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/api/agent/heartbeat", masterURL)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
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

func getDockerInfo() (string, int) {
	// Query local Docker socket via Unix domain socket
	socketPath := "/var/run/docker.sock"
	if _, err := os.Stat(socketPath); err != nil {
		return "N/A", 0
	}

	httpc := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 3 * time.Second,
	}

	// 1. Get Docker version
	dockerVersion := "v24.0+"
	resp, err := httpc.Get("http://localhost/version")
	if err == nil {
		defer resp.Body.Close()
		var verResp struct {
			Version string `json:"Version"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&verResp); err == nil && verResp.Version != "" {
			dockerVersion = verResp.Version
		}
	}

	// 2. Get running containers count
	runningCount := 0
	resp2, err := httpc.Get("http://localhost/containers/json")
	if err == nil {
		defer resp2.Body.Close()
		var containers []map[string]interface{}
		if err := json.NewDecoder(resp2.Body).Decode(&containers); err == nil {
			runningCount = len(containers)
		}
	}

	return dockerVersion, runningCount
}
