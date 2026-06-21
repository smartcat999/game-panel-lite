package http

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
	serverctrl "github.com/smartcat999/game-panel-lite/apps/api/internal/server"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func (h *Handler) listServers(w http.ResponseWriter, r *http.Request) {
	servers, err := h.store.ListGameServers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, servers)
}

func (h *Handler) getServer(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, server)
}

func (h *Handler) updateServerConfig(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	var payload struct {
		Config    json.RawMessage       `json:"config"`
		HostPort  *int                  `json:"hostPort,omitempty"`
		Resources *resourceLimitPayload `json:"resources,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	gameProvider, ok := h.provider.Get(server.ProviderKey)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown provider")
		return
	}
	configPayload, _, err := decodeProviderConfigPayload(gameProvider, payload.Config, server.Spec.Config)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateProviderConfigPayload(gameProvider, configPayload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	summary, err := providerConfigSummary(gameProvider, configPayload)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if payload.HostPort != nil {
		hostPort, err := h.resolveHostPort(r.Context(), *payload.HostPort, server.ID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		server.Spec.Network.HostPort = hostPort
	}
	if payload.Resources != nil {
		resources, err := normalizeResourceLimits(*payload.Resources)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		server.Spec.Resources.CPULimitCores = resources.CPULimitCores
		server.Spec.Resources.MemoryLimitMB = resources.MemoryLimitMB
	}
	server.Spec.Config = configPayload
	server.Spec.Network.Port = summary.Port
	server.Spec.Generation++
	if server.Spec.Generation <= 0 {
		server.Spec.Generation = 1
	}
	server.Status.Phase = domain.PhasePending
	server.UpdatedAt = time.Now()
	if err := h.store.SaveGameServer(r.Context(), &server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.config.updated", fmt.Sprintf("Updated config for %s", server.Name), activityServerPayload(server))
	writeJSON(w, http.StatusOK, server)
}

func (h *Handler) createServer(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name        string               `json:"name"`
		ProviderKey domain.ProviderKey   `json:"providerKey"`
		Config      json.RawMessage      `json:"config"`
		HostPort    int                  `json:"hostPort,omitempty"`
		Version     string               `json:"version"`
		Resources   resourceLimitPayload `json:"resources,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	gameProvider, ok := h.provider.Get(payload.ProviderKey)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown provider")
		return
	}
	if err := h.requireProviderRuntimeSupported(payload.ProviderKey); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	configPayload, _, err := decodeProviderConfigPayload(gameProvider, payload.Config, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	summary, err := providerConfigSummary(gameProvider, configPayload)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if payload.Name == "" {
		payload.Name = summary.ServerName
	}
	if payload.Version == "" {
		payload.Version = gameProvider.Versions()[0]
	}
	if !providerSupportsVersion(gameProvider, payload.Version) {
		writeError(w, http.StatusBadRequest, "unsupported provider version")
		return
	}
	if err := h.requireRuntimeAvailable(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	runtimeRef := runtimeInstallRef{
		ProviderKey: payload.ProviderKey,
		Version:     payload.Version,
		Image:       gameProvider.ImageFor(payload.Version),
	}
	imageStatus := h.runtimeInstallStatus(r.Context(), runtimeRef)
	if imageStatus.Status != runtime.ImageStatusReady {
		writeError(w, http.StatusConflict, "server runtime is not installed; install it from Game Library first")
		return
	}
	if err := h.requireRuntimeImageReady(r.Context(), runtimeRef); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err := validateProviderConfigPayload(gameProvider, configPayload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	id := uuid.NewString()
	dataDir := filepath.Join(h.cfg.DataDir, "instances", id)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	hostPort, err := h.resolveHostPort(r.Context(), payload.HostPort, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	resources, err := normalizeResourceLimits(payload.Resources)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	now := time.Now()
	server := domain.GameServer{
		ID:          id,
		Name:        payload.Name,
		GameKey:     gameProvider.GameKey(),
		ProviderKey: payload.ProviderKey,
		Spec: domain.ServerSpec{
			Generation:   1,
			DesiredState: domain.DesiredStopped,
			Version:      payload.Version,
			Config:       configPayload,
			Resources: domain.ServerResources{
				CPULimitCores: resources.CPULimitCores,
				MemoryLimitMB: resources.MemoryLimitMB,
			},
			Network: domain.ServerNetworkSpec{
				Port:     summary.Port,
				HostPort: hostPort,
			},
			Runtime: domain.ServerRuntimeSpec{DataDir: dataDir},
		},
		Status: domain.ServerRuntimeStatus{
			Phase:            domain.PhasePending,
			ActualState:      domain.ActualMissing,
			LastTransitionAt: now,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := h.store.CreateGameServer(r.Context(), &server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.created", fmt.Sprintf("Created server %s", server.Name), activityServerPayload(server))
	writeJSON(w, http.StatusCreated, server)
}

func (h *Handler) startServer(w http.ResponseWriter, r *http.Request) {
	server, err := serverctrl.NewService(h.store).RequestStart(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.start.queued", fmt.Sprintf("Queued start for server %s", server.Name), activityServerPayload(server))
	writeJSON(w, http.StatusAccepted, server)
}

func (h *Handler) stopServer(w http.ResponseWriter, r *http.Request) {
	server, err := serverctrl.NewService(h.store).RequestStop(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.stop.queued", fmt.Sprintf("Queued stop for server %s", server.Name), activityServerPayload(server))
	writeJSON(w, http.StatusAccepted, server)
}

func (h *Handler) restartServer(w http.ResponseWriter, r *http.Request) {
	server, err := serverctrl.NewService(h.store).RequestRestart(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.restart.queued", fmt.Sprintf("Queued restart for server %s", server.Name), activityServerPayload(server))
	writeJSON(w, http.StatusAccepted, server)
}

func (h *Handler) sendServerCommand(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	var payload struct {
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	command := strings.TrimSpace(payload.Command)
	if command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}
	if len(command) > 200 {
		writeError(w, http.StatusBadRequest, "command is too long")
		return
	}
	if server.Status.Phase != domain.PhaseRunning {
		writeError(w, http.StatusConflict, "server must be running to send commands")
		return
	}
	server, err = h.requireResourceRuntimeAttached(r.Context(), server)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err := h.runtime.SendCommandWorkload(r.Context(), server.Status.RuntimeID, command); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (h *Handler) deleteServer(w http.ResponseWriter, r *http.Request) {
	server, err := serverctrl.NewService(h.store).RequestDelete(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.delete.queued", fmt.Sprintf("Queued delete for server %s", server.Name), activityServerPayload(server))
	writeJSON(w, http.StatusAccepted, server)
}

func (h *Handler) serverStats(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if server.Status.Phase != domain.PhaseRunning || !h.runtimeStatusAvailable() {
		writeJSON(w, http.StatusOK, runtime.WorkloadStats{})
		return
	}
	stats, err := h.runtime.StatsWorkload(r.Context(), server.Status.RuntimeID)
	if err != nil {
		h.logger.Warn("failed to get container stats", "server", server.ID, "error", err)
		writeJSON(w, http.StatusOK, runtime.WorkloadStats{})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) serverLogs(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if server.Status.Phase == domain.PhaseRunning {
		server, err = h.requireResourceRuntimeAttached(r.Context(), server)
		if err != nil {
			writeError(w, statusCodeForRuntimeError(err), err.Error())
			return
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	if h.apiMetrics != nil {
		h.apiMetrics.AddSSEConnection("server_logs", 1)
		defer h.apiMetrics.AddSSEConnection("server_logs", -1)
	}
	stream, err := h.runtime.LogsWorkload(r.Context(), server.Status.RuntimeID, true)
	if err != nil {
		if strings.TrimSpace(server.Status.LastError) != "" {
			_, _ = fmt.Fprintf(w, "event: log\ndata: %s\n\n", server.Status.LastError)
			if h.apiMetrics != nil {
				h.apiMetrics.AddSSEEvent("server_logs", "log")
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		if h.apiMetrics != nil {
			h.apiMetrics.AddSSEEvent("server_logs", "error")
		}
		return
	}
	defer stream.Close()
	scanner := bufio.NewScanner(stream)
	recentLines := make([]string, 0, 120)
	for scanner.Scan() {
		line := scanner.Text()
		recentLines = append(recentLines, line)
		if len(recentLines) > 120 {
			recentLines = recentLines[len(recentLines)-120:]
		}
		h.updatePlayersFromLogLine(r.Context(), server, recentLines, line)
		_, _ = fmt.Fprintf(w, "event: log\ndata: %s\n\n", line)
		if h.apiMetrics != nil {
			h.apiMetrics.AddSSEEvent("server_logs", "log")
		}
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}
}

func (h *Handler) updatePlayersFromLogLine(ctx context.Context, server domain.GameServer, recentLines []string, line string) {
	gameProvider, ok := h.provider.Get(server.ProviderKey)
	if !ok {
		return
	}
	if playerProvider, ok := gameProvider.(provider.PlayerListProvider); ok && looksLikePlayerListOutput(line) {
		if players := playerProvider.ParsePlayerListOutput(recentLines); players != nil {
			h.savePlayersOnline(ctx, server.ID, len(players))
			return
		}
	}
	activityProvider, ok := gameProvider.(provider.PlayerActivityProvider)
	if !ok {
		return
	}
	event, ok := activityProvider.ParsePlayerLogEvent(line)
	if !ok {
		return
	}
	latest, err := h.store.GetGameServer(ctx, server.ID)
	if err != nil {
		h.logger.Warn("failed to load server for player activity update", "server", server.ID, "error", err)
		return
	}
	nextCount := latest.Status.PlayersOnline
	switch event {
	case domain.PlayerJoined:
		nextCount++
		if maxPlayers := maxPlayersFromServer(latest); maxPlayers > 0 && nextCount > maxPlayers {
			nextCount = maxPlayers
		}
	case domain.PlayerLeft:
		nextCount--
		if nextCount < 0 {
			nextCount = 0
		}
	default:
		return
	}
	h.savePlayersOnline(ctx, server.ID, nextCount)
}

func looksLikePlayerListOutput(line string) bool {
	trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), ":"))
	lower := strings.ToLower(trimmed)
	return strings.Contains(lower, "players connected") ||
		strings.Contains(lower, "player connected") ||
		strings.Contains(lower, "no players") ||
		strings.Contains(trimmed, "玩家已连接") ||
		strings.Contains(trimmed, "无玩家连接")
}

func (h *Handler) savePlayersOnline(ctx context.Context, serverID string, count int) {
	server, err := h.store.GetGameServer(ctx, serverID)
	if err != nil {
		h.logger.Warn("failed to load server for player count update", "server", serverID, "error", err)
		return
	}
	if count < 0 {
		count = 0
	}
	if maxPlayers := maxPlayersFromServer(server); maxPlayers > 0 && count > maxPlayers {
		count = maxPlayers
	}
	if server.Status.PlayersOnline == count {
		return
	}
	server.Status.PlayersOnline = count
	server.UpdatedAt = time.Now()
	if err := h.store.SaveGameServer(ctx, &server); err != nil {
		h.logger.Warn("failed to persist player count", "server", serverID, "error", err)
	}
}

func maxPlayersFromServer(server domain.GameServer) int {
	value, ok := server.Spec.Config["maxPlayers"]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return 0
}

func (h *Handler) serverLogSnapshot(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if server.Status.Phase != domain.PhaseRunning && !h.runtimeStatusAvailable() {
		writeJSON(w, http.StatusOK, map[string][]string{"lines": []string{}})
		return
	}
	stream, err := h.runtime.LogSnapshotWorkload(r.Context(), server.Status.RuntimeID)
	if err != nil {
		if strings.TrimSpace(server.Status.LastError) != "" {
			writeJSON(w, http.StatusOK, map[string][]string{"lines": []string{server.Status.LastError}})
			return
		}
		if server.Status.Phase != domain.PhaseRunning {
			h.logger.Warn("stopped server log snapshot unavailable; returning empty history", "server", server.ID, "error", err)
			writeJSON(w, http.StatusOK, map[string][]string{"lines": []string{}})
			return
		}
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	defer stream.Close()
	lines := make([]string, 0, 120)
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > 300 {
			lines = lines[len(lines)-300:]
		}
	}
	if err := scanner.Err(); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"lines": lines})
}

func (h *Handler) requireResourceRuntimeAttached(ctx context.Context, server domain.GameServer) (domain.GameServer, error) {
	if err := h.requireRuntimeAvailable(ctx); err != nil {
		return domain.GameServer{}, err
	}
	if err := h.requireProviderRuntimeSupported(server.ProviderKey); err != nil {
		return domain.GameServer{}, err
	}
	if server.Status.RuntimeID == "" {
		return domain.GameServer{}, fmt.Errorf("server runtime is not ready")
	}
	if _, err := h.runtime.InspectWorkload(ctx, server.Status.RuntimeID); err != nil {
		return domain.GameServer{}, err
	}
	return server, nil
}

func normalizeResourceLimits(input resourceLimitPayload) (resourceLimitPayload, error) {
	if input.CPULimitCores < 0 {
		return resourceLimitPayload{}, fmt.Errorf("CPU limit cannot be negative")
	}
	if input.CPULimitCores > 0 && (input.CPULimitCores < 0.25 || input.CPULimitCores > 64) {
		return resourceLimitPayload{}, fmt.Errorf("CPU limit must be between 0.25 and 64 cores")
	}
	if input.MemoryLimitMB < 0 {
		return resourceLimitPayload{}, fmt.Errorf("memory limit cannot be negative")
	}
	if input.MemoryLimitMB > 0 && (input.MemoryLimitMB < 256 || input.MemoryLimitMB > 262144) {
		return resourceLimitPayload{}, fmt.Errorf("memory limit must be between 256 MB and 262144 MB")
	}
	return input, nil
}

func (h *Handler) requireRuntimeAvailable(ctx context.Context) error {
	if h.dockerMonitor == nil {
		return nil
	}
	status := h.dockerMonitor.Refresh(ctx)
	if status.Available {
		return nil
	}
	message := strings.TrimSpace(status.Message)
	if message == "" {
		message = "Docker daemon is not available"
	}
	return fmt.Errorf("Docker runtime unavailable: %s", message)
}
