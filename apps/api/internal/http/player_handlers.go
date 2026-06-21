package http

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
)

func (h *Handler) requirePlayerCapability(server domain.GameServer, capabilityCheck func(domain.ProviderCapabilities) bool, action string) (provider.PlayerCommandProvider, error) {
	gameProvider, ok := h.provider.Get(server.ProviderKey)
	if !ok {
		return nil, fmt.Errorf("unknown provider")
	}
	if !capabilityCheck(gameProvider.Capabilities()) {
		return nil, fmt.Errorf("%s is not supported by this game", action)
	}
	commandProvider, ok := gameProvider.(provider.PlayerCommandProvider)
	if !ok {
		return nil, fmt.Errorf("%s is not supported by this game", action)
	}
	return commandProvider, nil
}

func (h *Handler) listServerPlayers(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	gameProvider, ok := h.provider.Get(server.ProviderKey)
	if !ok || !gameProvider.Capabilities().PlayerList {
		writeJSON(w, http.StatusOK, map[string]any{"supported": false, "players": []domain.Player{}})
		return
	}
	playerProvider, ok := gameProvider.(provider.PlayerListProvider)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"supported": false, "players": []domain.Player{}})
		return
	}
	if server.Status.Phase != domain.PhaseRunning {
		writeJSON(w, http.StatusOK, map[string]any{"supported": true, "players": []domain.Player{}})
		return
	}
	lines, err := h.recentServerLogLines(r.Context(), server)
	if err != nil {
		h.logger.Warn("failed to read player log output", "server", server.ID, "error", err)
		writeJSON(w, http.StatusOK, map[string]any{"supported": true, "players": []domain.Player{}})
		return
	}
	players := playerProvider.ParsePlayerListOutput(lines)
	if players == nil {
		players = []domain.Player{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"supported": true, "players": players})
}

func (h *Handler) recentServerLogLines(ctx context.Context, server domain.GameServer) ([]string, error) {
	stream, err := h.runtime.LogSnapshotWorkload(ctx, server.Status.RuntimeID)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	lines := make([]string, 0, 120)
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > 120 {
			lines = lines[len(lines)-120:]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func (h *Handler) kickServerPlayer(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	commandProvider, err := h.requirePlayerCapability(server, func(c domain.ProviderCapabilities) bool { return c.KickPlayer }, "kick")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	player := strings.TrimSpace(chi.URLParam(r, "player"))
	if player == "" {
		writeError(w, http.StatusBadRequest, "player name is required")
		return
	}
	if server.Status.Phase != domain.PhaseRunning {
		writeError(w, http.StatusConflict, "server must be running to kick players")
		return
	}
	server, err = h.requireResourceRuntimeAttached(r.Context(), server)
	if err != nil {
		writeError(w, statusCodeForRuntimeError(err), err.Error())
		return
	}
	command := commandProvider.KickCommand(player)
	if err := h.runtime.SendCommandWorkload(r.Context(), server.Status.RuntimeID, command); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "player.kicked", fmt.Sprintf("Kicked player %s from %s", player, server.Name), activityPlayerPayload(server, player))
	writeJSON(w, http.StatusOK, map[string]string{"status": "kicked", "player": player})
}

func (h *Handler) banServerPlayer(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	commandProvider, err := h.requirePlayerCapability(server, func(c domain.ProviderCapabilities) bool { return c.BanPlayer }, "ban")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	player := strings.TrimSpace(chi.URLParam(r, "player"))
	if player == "" {
		writeError(w, http.StatusBadRequest, "player name is required")
		return
	}
	if server.Status.Phase != domain.PhaseRunning {
		writeError(w, http.StatusConflict, "server must be running to ban players")
		return
	}
	server, err = h.requireResourceRuntimeAttached(r.Context(), server)
	if err != nil {
		writeError(w, statusCodeForRuntimeError(err), err.Error())
		return
	}
	command := commandProvider.BanCommand(player)
	if err := h.runtime.SendCommandWorkload(r.Context(), server.Status.RuntimeID, command); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "player.banned", fmt.Sprintf("Banned player %s from %s", player, server.Name), activityPlayerPayload(server, player))
	writeJSON(w, http.StatusOK, map[string]string{"status": "banned", "player": player})
}

func (h *Handler) getServerWhitelist(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	gameProvider, ok := h.provider.Get(server.ProviderKey)
	if !ok || !gameProvider.Capabilities().Whitelist {
		writeJSON(w, http.StatusOK, map[string]any{"supported": false, "running": server.Status.Phase == domain.PhaseRunning})
		return
	}
	_, ok = gameProvider.(provider.WhitelistCommandProvider)
	writeJSON(w, http.StatusOK, map[string]any{"supported": ok, "running": server.Status.Phase == domain.PhaseRunning})
}

func (h *Handler) addServerWhitelistPlayer(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	commandProvider, err := h.requireWhitelistCapability(server)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	player := strings.TrimSpace(chi.URLParam(r, "player"))
	if player == "" {
		writeError(w, http.StatusBadRequest, "player name is required")
		return
	}
	if server.Status.Phase != domain.PhaseRunning {
		writeError(w, http.StatusConflict, "server must be running to edit the whitelist")
		return
	}
	server, err = h.requireResourceRuntimeAttached(r.Context(), server)
	if err != nil {
		writeError(w, statusCodeForRuntimeError(err), err.Error())
		return
	}
	if err := h.runtime.SendCommandWorkload(r.Context(), server.Status.RuntimeID, commandProvider.WhitelistAddCommand(player)); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "player.whitelisted", fmt.Sprintf("Added player %s to %s whitelist", player, server.Name), activityPlayerPayload(server, player))
	writeJSON(w, http.StatusOK, map[string]string{"status": "added", "player": player})
}

func (h *Handler) removeServerWhitelistPlayer(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	commandProvider, err := h.requireWhitelistCapability(server)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	player := strings.TrimSpace(chi.URLParam(r, "player"))
	if player == "" {
		writeError(w, http.StatusBadRequest, "player name is required")
		return
	}
	if server.Status.Phase != domain.PhaseRunning {
		writeError(w, http.StatusConflict, "server must be running to edit the whitelist")
		return
	}
	server, err = h.requireResourceRuntimeAttached(r.Context(), server)
	if err != nil {
		writeError(w, statusCodeForRuntimeError(err), err.Error())
		return
	}
	if err := h.runtime.SendCommandWorkload(r.Context(), server.Status.RuntimeID, commandProvider.WhitelistRemoveCommand(player)); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "player.whitelist.removed", fmt.Sprintf("Removed player %s from %s whitelist", player, server.Name), activityPlayerPayload(server, player))
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed", "player": player})
}

func (h *Handler) requireWhitelistCapability(server domain.GameServer) (provider.WhitelistCommandProvider, error) {
	gameProvider, ok := h.provider.Get(server.ProviderKey)
	if !ok {
		return nil, fmt.Errorf("unknown provider")
	}
	if !gameProvider.Capabilities().Whitelist {
		return nil, fmt.Errorf("whitelist is not supported by this game")
	}
	commandProvider, ok := gameProvider.(provider.WhitelistCommandProvider)
	if !ok {
		return nil, fmt.Errorf("whitelist is not supported by this game")
	}
	return commandProvider, nil
}
