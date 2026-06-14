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
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	backupsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/config"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	modsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/mod"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
	worldsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/world"
)

type Handler struct {
	cfg            config.Config
	logger         *slog.Logger
	store          *store.Store
	provider       *provider.Registry
	runtime        *runtime.SwitchableAdapter
	runtimeFactory func(string) (runtime.Adapter, error)
}

func NewHandler(
	cfg config.Config,
	logger *slog.Logger,
	store *store.Store,
	providers *provider.Registry,
	adapter *runtime.SwitchableAdapter,
	runtimeFactory func(string) (runtime.Adapter, error),
) *Handler {
	return &Handler{
		cfg:            cfg,
		logger:         logger,
		store:          store,
		provider:       providers,
		runtime:        adapter,
		runtimeFactory: runtimeFactory,
	}
}

func (h *Handler) Register(r chi.Router) {
	r.Use(h.cors)
	r.Get("/healthz", h.health)
	r.Get("/api/version", h.version)
	r.Get("/api/runtime/docker", h.dockerStatus)
	r.Get("/api/runtime/docker/hosts", h.dockerHosts)
	r.Post("/api/runtime/docker/host", h.applyDockerHost)
	r.Get("/api/servers", h.listServers)
	r.Post("/api/servers", h.createServer)
	r.Get("/api/servers/{id}", h.getServer)
	r.Post("/api/servers/{id}/start", h.startServer)
	r.Post("/api/servers/{id}/stop", h.stopServer)
	r.Post("/api/servers/{id}/restart", h.restartServer)
	r.Delete("/api/servers/{id}", h.deleteServer)
	r.Get("/api/servers/{id}/logs", h.serverLogs)
	r.Get("/api/worlds", h.listWorlds)
	r.Post("/api/worlds/import", h.importWorld)
	r.Post("/api/worlds/{id}/assign", h.assignWorld)
	r.Post("/api/worlds/{id}/duplicate", h.duplicateWorld)
	r.Get("/api/worlds/{id}/download", h.downloadWorld)
	r.Delete("/api/worlds/{id}", h.deleteWorld)
	r.Get("/api/backups", h.listBackups)
	r.Post("/api/servers/{id}/backups", h.createBackup)
	r.Get("/api/backups/{id}/download", h.downloadBackup)
	r.Post("/api/backups/{id}/restore", h.restoreBackup)
	r.Delete("/api/backups/{id}", h.deleteBackup)
	r.Get("/api/servers/{id}/mods", h.listMods)
	r.Post("/api/servers/{id}/mods/upload", h.uploadMod)
	r.Delete("/api/servers/{id}/mods/{modId}", h.deleteMod)
	r.Get("/api/terraria/presets", h.presets)
	r.Post("/api/terraria/config/preview", h.configPreview)
}

func (h *Handler) listMods(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	mods, err := h.store.ListMods(r.Context(), server.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, mods)
}

func (h *Handler) uploadMod(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if server.ProviderKey != domain.ProviderTerrariaTModLoader {
		writeError(w, http.StatusBadRequest, "mods are only supported for tModLoader servers")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "mod file is required")
		return
	}
	defer file.Close()
	_, size, err := modsvc.NewService(h.cfg.DataDir).Upload(server.ID, header.Filename, file)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	item := domain.ModFile{ID: uuid.NewString(), InstanceID: server.ID, FileName: header.Filename, SizeBytes: size, Enabled: true, CreatedAt: time.Now()}
	if err := h.store.CreateMod(r.Context(), &item); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) deleteMod(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.GetMod(r.Context(), chi.URLParam(r, "modId"))
	if err != nil {
		writeError(w, http.StatusNotFound, "mod not found")
		return
	}
	path, _ := modsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
	_ = os.Remove(path)
	_ = h.store.DeleteMod(r.Context(), item.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) listWorlds(w http.ResponseWriter, r *http.Request) {
	worlds, err := h.store.ListWorlds(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, worlds)
}

func (h *Handler) importWorld(w http.ResponseWriter, r *http.Request) {
	instanceID := r.FormValue("instanceId")
	if instanceID == "" {
		instanceID = "unassigned"
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "world file is required")
		return
	}
	defer file.Close()
	_, size, err := worldsvc.NewService(h.cfg.DataDir).Import(instanceID, header.Filename, file)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	item := domain.World{ID: uuid.NewString(), InstanceID: instanceID, Name: header.Filename[:len(header.Filename)-len(filepath.Ext(header.Filename))], FileName: header.Filename, SizeBytes: size, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := h.store.CreateWorld(r.Context(), &item); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) downloadWorld(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.GetWorld(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "world not found")
		return
	}
	path, err := worldsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	http.ServeFile(w, r, path)
}

func (h *Handler) assignWorld(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.GetWorld(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "world not found")
		return
	}
	var payload struct {
		InstanceID string `json:"instanceId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.InstanceID == "" {
		writeError(w, http.StatusBadRequest, "instanceId is required")
		return
	}
	if _, err := h.store.GetServer(r.Context(), payload.InstanceID); err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	item.ActiveInstanceID = payload.InstanceID
	item.UpdatedAt = time.Now()
	if err := h.store.SaveWorld(r.Context(), &item); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) duplicateWorld(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.GetWorld(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "world not found")
		return
	}
	var payload struct {
		FileName string `json:"fileName"`
		Name     string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)
	if payload.FileName == "" {
		payload.FileName = "copy-" + item.FileName
	}
	if payload.Name == "" {
		payload.Name = item.Name + " Copy"
	}
	_, size, err := worldsvc.NewService(h.cfg.DataDir).Duplicate(item.InstanceID, item.FileName, payload.FileName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	copy := domain.World{ID: uuid.NewString(), InstanceID: item.InstanceID, Name: payload.Name, FileName: payload.FileName, SizeBytes: size, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := h.store.CreateWorld(r.Context(), &copy); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, copy)
}

func (h *Handler) deleteWorld(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.GetWorld(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "world not found")
		return
	}
	path, _ := worldsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
	_ = os.Remove(path)
	_ = h.store.DeleteWorld(r.Context(), item.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) listBackups(w http.ResponseWriter, r *http.Request) {
	backups, err := h.store.ListBackups(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, backups)
}

func (h *Handler) createBackup(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	path, size, err := backupsvc.NewService(h.cfg.DataDir).Create(server.ID, server.DataDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	item := domain.Backup{ID: uuid.NewString(), InstanceID: server.ID, FileName: filepath.Base(path), WorldName: server.WorldName, SizeBytes: size, Type: "Manual", CreatedAt: time.Now()}
	if err := h.store.CreateBackup(r.Context(), &item); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) downloadBackup(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.GetBackup(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	path, err := backupsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	http.ServeFile(w, r, path)
}

func (h *Handler) restoreBackup(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.GetBackup(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	server, err := h.store.GetServer(r.Context(), item.InstanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if server.Status == domain.StatusRunning || server.Status == domain.StatusRestarting {
		writeError(w, http.StatusConflict, "stop the server before restoring a backup")
		return
	}
	if err := backupsvc.NewService(h.cfg.DataDir).Restore(item.InstanceID, item.FileName, server.DataDir); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "restored", "backupId": item.ID})
}

func (h *Handler) deleteBackup(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.GetBackup(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	path, _ := backupsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
	_ = os.Remove(path)
	_ = h.store.DeleteBackup(r.Context(), item.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
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

func (h *Handler) dockerHosts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"currentHost": h.cfg.DockerHost,
		"candidates":  config.DockerHostCandidates(h.cfg.DockerHost),
	})
}

func (h *Handler) applyDockerHost(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Host string `json:"host"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	host := strings.TrimSpace(payload.Host)
	if !isAllowedDockerHost(host) {
		writeError(w, http.StatusBadRequest, "docker host must start with unix://, tcp://, or npipe://")
		return
	}
	adapter, err := h.runtimeFactory(host)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.runtime.Set(adapter)
	h.cfg.DockerHost = host
	writeJSON(w, http.StatusOK, h.runtime.Check(r.Context()))
}

func isAllowedDockerHost(host string) bool {
	return strings.HasPrefix(host, "unix://") ||
		strings.HasPrefix(host, "tcp://") ||
		strings.HasPrefix(host, "npipe://")
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
