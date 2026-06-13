package http

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/config"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type Handler struct {
	cfg      config.Config
	logger   *slog.Logger
	store    *store.Store
	provider *provider.Registry
	runtime  runtime.Adapter
}

func NewHandler(cfg config.Config, logger *slog.Logger, store *store.Store, providers *provider.Registry, adapter runtime.Adapter) *Handler {
	return &Handler{cfg: cfg, logger: logger, store: store, provider: providers, runtime: adapter}
}

func (h *Handler) Register(r chi.Router) {
	r.Use(h.cors)
	r.Get("/healthz", h.health)
	r.Get("/api/version", h.version)
	r.Get("/api/runtime/docker", h.dockerStatus)
	r.Get("/api/servers", h.listServers)
	r.Post("/api/servers", h.createServer)
	r.Get("/api/servers/{id}", h.getServer)
	r.Post("/api/servers/{id}/start", h.startServer)
	r.Post("/api/servers/{id}/stop", h.stopServer)
	r.Post("/api/servers/{id}/restart", h.restartServer)
	r.Delete("/api/servers/{id}", h.deleteServer)
	r.Get("/api/servers/{id}/logs", h.serverLogs)
	r.Get("/api/terraria/presets", h.presets)
	r.Post("/api/terraria/config/preview", h.configPreview)
}

func (h *Handler) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"name": "GamePanel Lite", "version": "0.1.0"})
}

func (h *Handler) dockerStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.runtime.Check(r.Context()))
}

func (h *Handler) listServers(w http.ResponseWriter, r *http.Request) {
	servers, err := h.store.ListServers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, servers)
}

func (h *Handler) getServer(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
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

func (h *Handler) createServer(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name        string                `json:"name"`
		ProviderKey domain.ProviderKey    `json:"providerKey"`
		Config      domain.TerrariaConfig `json:"config"`
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
	if payload.Name == "" {
		payload.Name = payload.Config.ServerName
	}
	configText, err := gameProvider.RenderConfig(payload.Config)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	id := uuid.NewString()
	dataDir := filepath.Join(h.cfg.DataDir, "instances", id)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	server := domain.GameServerInstance{
		ID: id, Name: payload.Name, GameKey: "terraria", ProviderKey: payload.ProviderKey,
		Status: domain.StatusStopped, WorldName: payload.Config.WorldName, Port: payload.Config.Port,
		MaxPlayers: payload.Config.MaxPlayers, Password: payload.Config.Password, DataDir: dataDir,
		Config: payload.Config, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	containerID, err := h.runtime.Create(r.Context(), runtime.ContainerSpec{InstanceID: id, Name: payload.Name, Image: gameProvider.Image(), Port: payload.Config.Port, DataDir: dataDir, ConfigText: configText})
	if err != nil {
		h.logger.Warn("container create failed; keeping server record stopped", "error", err)
	} else {
		server.ContainerID = containerID
	}
	if err := h.store.CreateServer(r.Context(), &server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, server)
}

func (h *Handler) startServer(w http.ResponseWriter, r *http.Request) {
	h.transitionServer(w, r, domain.StatusRunning, h.runtime.Start)
}

func (h *Handler) stopServer(w http.ResponseWriter, r *http.Request) {
	h.transitionServer(w, r, domain.StatusStopped, h.runtime.Stop)
}

func (h *Handler) restartServer(w http.ResponseWriter, r *http.Request) {
	h.transitionServer(w, r, domain.StatusRunning, h.runtime.Restart)
}

func (h *Handler) transitionServer(w http.ResponseWriter, r *http.Request, status domain.ServerStatus, action func(context.Context, domain.GameServerInstance) error) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if server.ContainerID != "" {
		if err := action(r.Context(), server); err != nil {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
	}
	server.Status = status
	server.UpdatedAt = time.Now()
	if err := h.store.SaveServer(r.Context(), &server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, server)
}

func (h *Handler) deleteServer(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if server.ContainerID != "" {
		_ = h.runtime.Remove(r.Context(), server)
	}
	if err := h.store.DeleteServer(r.Context(), server.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) serverLogs(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	stream, err := h.runtime.Logs(r.Context(), server)
	if err != nil {
		_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		return
	}
	defer stream.Close()
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		_, _ = fmt.Fprintf(w, "event: log\ndata: %s\n\n", scanner.Text())
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}
}

func (h *Handler) presets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, terraria.Presets)
}

func (h *Handler) configPreview(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Config terrariaPreviewConfig `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	rendered, err := terraria.RenderServerConfig(payload.Config.ToDomain())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"serverconfig": rendered})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
