package http

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type createNodeRequest struct {
	Name          string `json:"name"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	PublicIP      string `json:"publicIp"`
	Region        string `json:"region"`
	CPUCores      int    `json:"cpuCores,omitempty"`
	MemoryTotalMB int64  `json:"memoryTotalMb,omitempty"`
}

type pingNodeResponse struct {
	NodeID    string    `json:"nodeId"`
	Status    string    `json:"status"`
	LatencyMS int64     `json:"latencyMs"`
	CheckedAt time.Time `json:"checkedAt"`
}

type joinCommandResponse struct {
	NodeID        string `json:"nodeId"`
	Token         string `json:"token"`
	MasterURL     string `json:"masterUrl"`
	DockerCommand string `json:"dockerCommand"`
	ShellCommand  string `json:"shellCommand"`
}

type agentRegisterRequest struct {
	Token         string `json:"token"`
	CPUCores      int    `json:"cpuCores"`
	MemoryTotalMB int64  `json:"memoryTotalMb"`
	DiskTotalGB   int64  `json:"diskTotalGb"`
	DockerVersion string `json:"dockerVersion"`
	AgentVersion  string `json:"agentVersion"`
	OSInfo        string `json:"osInfo"`
	PublicIP      string `json:"publicIp"`
}

type agentHeartbeatRequest struct {
	Token           string  `json:"token"`
	CPUUsagePercent float64 `json:"cpuUsagePercent"`
	MemoryUsedMB    int64   `json:"memoryUsedMb"`
	DiskUsedGB      int64   `json:"diskUsedGb"`
	RunningCount    int     `json:"runningCount"`
	PingLatencyMS   int64   `json:"pingLatencyMs"`
}

func generateAgentToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "gpl_agent_" + hex.EncodeToString(b)
}

func (h *Handler) listNodes(w http.ResponseWriter, r *http.Request) {
	_, _ = h.store.EnsureDefaultLocalNode(r.Context())
	nodes, err := h.store.ListComputeNodes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list compute nodes: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

func (h *Handler) getNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	node, err := h.store.GetComputeNode(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "node not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get node: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, node)
}

func (h *Handler) createNode(w http.ResponseWriter, r *http.Request) {
	var req createNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request payload")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "node name is required")
		return
	}
	if req.Host == "" {
		req.Host = "0.0.0.0"
	}
	if req.Region == "" {
		req.Region = "Global"
	}

	token := generateAgentToken()
	node := domain.ComputeNode{
		ID:            "node-" + uuid.NewString()[:8],
		Name:          req.Name,
		Host:          req.Host,
		Port:          req.Port,
		Token:         token,
		PublicIP:      req.PublicIP,
		Region:        req.Region,
		Status:        "offline",
		IsLocal:       false,
		CPUCores:      req.CPUCores,
		MemoryTotalMB: req.MemoryTotalMB,
		MemoryUsedMB:  0,
		RunningCount:  0,
		LastHeartbeat: time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	if err := h.store.CreateComputeNode(r.Context(), &node); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create node: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, node)
}

type updateNodeRequest struct {
	Name     *string `json:"name"`
	Region   *string `json:"region"`
	PublicIP *string `json:"publicIp"`
	Host     *string `json:"host"`
	Port     *int    `json:"port"`
}

func (h *Handler) updateNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	node, err := h.store.GetComputeNode(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "node not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get node: "+err.Error())
		return
	}

	var req updateNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		node.Name = strings.TrimSpace(*req.Name)
	}
	if req.Region != nil {
		node.Region = strings.TrimSpace(*req.Region)
	}
	if req.PublicIP != nil {
		node.PublicIP = strings.TrimSpace(*req.PublicIP)
	}
	if req.Host != nil && strings.TrimSpace(*req.Host) != "" {
		node.Host = strings.TrimSpace(*req.Host)
	}
	if req.Port != nil && *req.Port > 0 {
		node.Port = *req.Port
	}

	if err := h.store.UpdateComputeNode(r.Context(), &node); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update node: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, node)
}

func (h *Handler) getNodeJoinCommand(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	node, err := h.store.GetComputeNode(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "node not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get node: "+err.Error())
		return
	}

	scheme := "https"
	if r.TLS == nil && !strings.Contains(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "http"
	}
	host := r.Host
	if host == "" {
		host = "127.0.0.1:4000"
	}
	masterURL := fmt.Sprintf("%s://%s", scheme, host)

	dockerCmd := fmt.Sprintf(
		"docker run -d --name gamepanel-agent --restart always --net=host -v /var/run/docker.sock:/var/run/docker.sock -e MASTER_URL=%s -e AGENT_TOKEN=%s smartcat99999/game-panel-lite-agent:latest",
		masterURL, node.Token,
	)
	shellCmd := fmt.Sprintf(
		"curl -fsSL %s/install-agent.sh | sudo bash -s -- --master=%s --token=%s",
		masterURL, masterURL, node.Token,
	)

	writeJSON(w, http.StatusOK, joinCommandResponse{
		NodeID:        node.ID,
		Token:         node.Token,
		MasterURL:     masterURL,
		DockerCommand: dockerCmd,
		ShellCommand:  shellCmd,
	})
}

func (h *Handler) agentRegister(w http.ResponseWriter, r *http.Request) {
	var req agentRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request payload")
		return
	}
	if req.Token == "" {
		writeError(w, http.StatusUnauthorized, "agent token is required")
		return
	}

	node, err := h.store.GetComputeNodeByToken(r.Context(), req.Token)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "invalid agent token")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to query node: "+err.Error())
		return
	}

	if req.CPUCores > 0 {
		node.CPUCores = req.CPUCores
	}
	if req.MemoryTotalMB > 0 {
		node.MemoryTotalMB = req.MemoryTotalMB
	}
	if req.DiskTotalGB > 0 {
		node.DiskTotalGB = req.DiskTotalGB
	}
	if req.DockerVersion != "" {
		node.DockerVersion = req.DockerVersion
	}
	if req.AgentVersion != "" {
		node.AgentVersion = req.AgentVersion
	}
	if req.OSInfo != "" {
		node.OSInfo = req.OSInfo
	}
	if req.PublicIP != "" {
		node.PublicIP = req.PublicIP
	}

	node.Status = "online"
	node.LastHeartbeat = time.Now().UTC()
	node.UpdatedAt = time.Now().UTC()
	h.apiMetrics.ObserveAgentHeartbeat(node.ID, node.LastHeartbeat)

	if err := h.store.UpdateComputeNode(r.Context(), &node); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update node status: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"nodeId":  node.ID,
		"status":  "online",
		"message": "agent registered successfully",
	})
}

func (h *Handler) agentHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req agentHeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request payload")
		return
	}
	if req.Token == "" {
		writeError(w, http.StatusUnauthorized, "agent token is required")
		return
	}

	node, err := h.store.GetComputeNodeByToken(r.Context(), req.Token)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "invalid agent token")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to query node: "+err.Error())
		return
	}

	node.CPUUsagePercent = req.CPUUsagePercent
	if req.MemoryUsedMB > 0 {
		node.MemoryUsedMB = req.MemoryUsedMB
	}
	if req.DiskUsedGB > 0 {
		node.DiskUsedGB = req.DiskUsedGB
	}
	if req.RunningCount >= 0 {
		node.RunningCount = req.RunningCount
	}
	if req.PingLatencyMS > 0 {
		node.PingLatencyMS = req.PingLatencyMS
	}

	node.Status = "online"
	node.LastHeartbeat = time.Now().UTC()
	node.UpdatedAt = time.Now().UTC()
	h.apiMetrics.ObserveAgentHeartbeat(node.ID, node.LastHeartbeat)

	_ = h.store.UpdateComputeNode(r.Context(), &node)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"nodeId":    node.ID,
		"status":    "online",
		"heartbeat": node.LastHeartbeat,
	})
}

func (h *Handler) pingNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	node, err := h.store.GetComputeNode(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "node not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to find node: "+err.Error())
		return
	}

	latency := int64(4)
	if !node.IsLocal {
		latency = 12
	}
	node.PingLatencyMS = latency
	node.LastHeartbeat = time.Now().UTC()
	if !node.IsLocal && time.Since(node.LastHeartbeat) > 45*time.Second {
		node.Status = "offline"
	} else {
		node.Status = "online"
	}
	_ = h.store.UpdateComputeNode(r.Context(), &node)

	writeJSON(w, http.StatusOK, pingNodeResponse{
		NodeID:    node.ID,
		Status:    node.Status,
		LatencyMS: latency,
		CheckedAt: time.Now().UTC(),
	})
}

func (h *Handler) listNodeServers(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	servers, err := h.store.ListGameServers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list servers: "+err.Error())
		return
	}
	nodeServers := make([]domain.GameServer, 0)
	for _, s := range servers {
		if (id == "node-local" && (s.NodeID == "" || s.NodeID == "node-local")) || s.NodeID == id {
			nodeServers = append(nodeServers, s)
		}
	}
	writeJSON(w, http.StatusOK, nodeServers)
}

func (h *Handler) deleteNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "node-local" {
		writeError(w, http.StatusBadRequest, "无法删除主控节点")
		return
	}

	servers, err := h.store.ListGameServers(r.Context())
	if err == nil {
		for _, s := range servers {
			if s.NodeID == id {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("无法删除节点：当前仍有游戏服务器 (%s) 运行在该节点上，请先迁移或删除实例", s.Name))
				return
			}
		}
	}

	if err := h.store.DeleteComputeNode(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete node: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listAgentTasks(w http.ResponseWriter, r *http.Request) {
	node, ok := h.authenticateAgentNode(w, r)
	if !ok {
		return
	}

	tasks, err := h.store.ListPendingNodeTasks(r.Context(), node.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query pending tasks: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (h *Handler) ackAgentTask(w http.ResponseWriter, r *http.Request) {
	node, ok := h.authenticateAgentNode(w, r)
	if !ok {
		return
	}
	taskID := chi.URLParam(r, "taskId")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "missing taskId")
		return
	}

	task, err := h.store.GetNodeTask(r.Context(), taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if task.NodeID != node.ID {
		writeError(w, http.StatusForbidden, "task is assigned to another node")
		return
	}

	var req struct {
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	taskStatus := domain.TaskCompleted
	if req.Status == "failed" {
		taskStatus = domain.TaskFailed
	}

	if err := h.store.UpdateNodeTaskStatus(r.Context(), taskID, taskStatus, req.Error); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update task status: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"result": "ok"})
}

func (h *Handler) authenticateAgentNode(w http.ResponseWriter, r *http.Request) (domain.ComputeNode, bool) {
	token := strings.TrimSpace(r.Header.Get("X-Node-Token"))
	if token == "" {
		token = strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	}
	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing agent token")
		return domain.ComputeNode{}, false
	}
	node, err := h.store.GetComputeNodeByToken(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid agent token")
		return domain.ComputeNode{}, false
	}
	return node, true
}

func (h *Handler) listAgentAssignments(w http.ResponseWriter, r *http.Request) {
	node, ok := h.authenticateAgentNode(w, r)
	if !ok {
		return
	}
	assignments, err := h.store.ListWorkloadAssignmentsByNode(r.Context(), node.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workload assignments")
		return
	}
	pending := 0
	for _, assignment := range assignments {
		observation, observationErr := h.store.GetWorkloadObservation(r.Context(), assignment.UID)
		if observationErr != nil || observation.ObservedGeneration < assignment.Generation {
			pending++
		}
	}
	h.apiMetrics.SetWorkloadBacklog(node.ID, pending)
	writeJSON(w, http.StatusOK, assignments)
}

func (h *Handler) reportAgentAssignmentStatus(w http.ResponseWriter, r *http.Request) {
	node, ok := h.authenticateAgentNode(w, r)
	if !ok {
		return
	}
	assignment, err := h.store.GetWorkloadAssignmentByUID(r.Context(), chi.URLParam(r, "uid"))
	if err != nil {
		writeError(w, http.StatusNotFound, "assignment not found")
		return
	}
	if assignment.NodeID != node.ID {
		writeError(w, http.StatusForbidden, "assignment belongs to another node")
		return
	}
	var observation domain.WorkloadObservation
	if err := json.NewDecoder(r.Body).Decode(&observation); err != nil {
		writeError(w, http.StatusBadRequest, "invalid observation body")
		return
	}
	if observation.ObservedGeneration > assignment.Generation {
		writeError(w, http.StatusConflict, "observation generation is newer than assignment")
		return
	}
	if observation.ReconcileDurationSeconds < 0 || observation.ReconcileDurationSeconds > 600 {
		writeError(w, http.StatusBadRequest, "invalid reconcile duration")
		return
	}
	now := time.Now().UTC()
	observation.ID = uuid.NewString()
	observation.AssignmentUID = assignment.UID
	observation.ServerID = assignment.ServerID
	observation.NodeID = assignment.NodeID
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = now
	}
	observation.CreatedAt = now
	observation.UpdatedAt = now
	if err := h.store.UpsertWorkloadObservation(r.Context(), &observation); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist workload observation")
		return
	}
	h.apiMetrics.ObserveWorkloadReconcile(
		node.ID,
		assignment.ServerID,
		time.Duration(observation.ReconcileDurationSeconds*float64(time.Second)),
		assignment.Generation-observation.ObservedGeneration,
		observation.LastError != "",
	)
	writeJSON(w, http.StatusOK, map[string]int{"acceptedGeneration": observation.ObservedGeneration})
}

func (h *Handler) pollAgentTunnel(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("X-Node-Token")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		token = r.Header.Get("Authorization")
		token = strings.TrimPrefix(token, "Bearer ")
	}

	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing agent token")
		return
	}

	node, err := h.store.GetComputeNodeByToken(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid agent token")
		return
	}

	if h.gateway == nil {
		writeError(w, http.StatusServiceUnavailable, "stream gateway not initialized")
		return
	}

	reqChan := h.gateway.SubscribeNodeRequests(node.ID)

	select {
	case req, ok := <-reqChan:
		if !ok {
			writeError(w, http.StatusGone, "tunnel subscription closed")
			return
		}
		writeJSON(w, http.StatusOK, req)
	case <-time.After(25 * time.Second):
		w.WriteHeader(http.StatusNoContent)
	case <-r.Context().Done():
		return
	}
}

func (h *Handler) connectAgentTunnel(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("X-Node-Token")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		token = r.Header.Get("Authorization")
		token = strings.TrimPrefix(token, "Bearer ")
	}

	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing agent token")
		return
	}

	_, err := h.store.GetComputeNodeByToken(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid agent token")
		return
	}

	streamID := r.URL.Query().Get("streamId")
	if streamID == "" {
		writeError(w, http.StatusBadRequest, "missing streamId")
		return
	}

	if h.gateway == nil {
		writeError(w, http.StatusServiceUnavailable, "stream gateway not initialized")
		return
	}

	_ = h.gateway.HandleAgentTunnelConnect(w, r, streamID)
}

type agentLogsUploadRequest struct {
	Lines []string `json:"lines"`
}

func (h *Handler) agentUploadLogs(w http.ResponseWriter, r *http.Request) {
	node, ok := h.authenticateAgentNode(w, r)
	if !ok {
		return
	}
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		writeError(w, http.StatusBadRequest, "missing server id")
		return
	}
	server, err := h.store.GetGameServer(r.Context(), serverID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if server.NodeID != node.ID {
		writeError(w, http.StatusForbidden, "server belongs to another node")
		return
	}

	var req agentLogsUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	received := 0
	if len(req.Lines) > 0 {
		h.agentLogsMu.Lock()
		current := h.agentLogs[serverID]
		newLines := logSnapshotDelta(current, req.Lines)
		received = len(newLines)
		current = append(current, newLines...)
		if len(current) > 500 {
			current = current[len(current)-500:]
		}
		h.agentLogs[serverID] = current

		// Broadcast to all active SSE subscribers for this server
		if subs, ok := h.agentLogChans[serverID]; ok {
			for _, line := range newLines {
				for ch := range subs {
					select {
					case ch <- line:
					default:
					}
				}
			}
		}
		h.agentLogsMu.Unlock()
	}

	writeJSON(w, http.StatusOK, map[string]int{"received": received})
}

func logSnapshotDelta(current, snapshot []string) []string {
	maxOverlap := len(current)
	if len(snapshot) < maxOverlap {
		maxOverlap = len(snapshot)
	}
	for overlap := maxOverlap; overlap > 0; overlap-- {
		matches := true
		for i := 0; i < overlap; i++ {
			if current[len(current)-overlap+i] != snapshot[i] {
				matches = false
				break
			}
		}
		if matches {
			return snapshot[overlap:]
		}
	}
	return snapshot
}
