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
	dockerMonitor  *runtime.DockerMonitor
	runtimeFactory func(string) (runtime.Adapter, error)
}

func NewHandler(
	cfg config.Config,
	logger *slog.Logger,
	store *store.Store,
	providers *provider.Registry,
	adapter *runtime.SwitchableAdapter,
	dockerMonitor *runtime.DockerMonitor,
	runtimeFactory func(string) (runtime.Adapter, error),
) *Handler {
	return &Handler{
		cfg:            cfg,
		logger:         logger,
		store:          store,
		provider:       providers,
		runtime:        adapter,
		dockerMonitor:  dockerMonitor,
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
	r.Get("/api/settings", h.getSettings)
	r.Put("/api/settings", h.updateSettings)
	r.Get("/api/activity", h.listActivity)
	r.Get("/api/servers", h.listServers)
	r.Post("/api/servers", h.createServer)
	r.Get("/api/servers/{id}", h.getServer)
	r.Put("/api/servers/{id}/config", h.updateServerConfig)
	r.Post("/api/servers/{id}/start", h.startServer)
	r.Post("/api/servers/{id}/stop", h.stopServer)
	r.Post("/api/servers/{id}/restart", h.restartServer)
	r.Post("/api/servers/{id}/command", h.sendServerCommand)
	r.Delete("/api/servers/{id}", h.deleteServer)
	r.Get("/api/servers/{id}/logs", h.serverLogs)
	r.Get("/api/servers/{id}/logs/snapshot", h.serverLogSnapshot)
	r.Get("/api/servers/{id}/stats", h.serverStats)
	r.Get("/api/worlds", h.listWorlds)
	r.Post("/api/worlds/import", h.importWorld)
	r.Post("/api/worlds/{id}/assign", h.assignWorld)
	r.Post("/api/worlds/{id}/duplicate", h.duplicateWorld)
	r.Post("/api/worlds/{id}/migrate", h.migrateWorld)
	r.Get("/api/worlds/{id}/download", h.downloadWorld)
	r.Delete("/api/worlds/{id}", h.deleteWorld)
	r.Get("/api/backups", h.listBackups)
	r.Post("/api/servers/{id}/backups", h.createBackup)
	r.Get("/api/backups/{id}/download", h.downloadBackup)
	r.Post("/api/backups/{id}/migrate", h.migrateBackup)
	r.Post("/api/backups/{id}/restore", h.restoreBackup)
	r.Delete("/api/backups/{id}", h.deleteBackup)
	r.Get("/api/servers/{id}/mods", h.listMods)
	r.Post("/api/servers/{id}/mods/upload", h.uploadMod)
	r.Patch("/api/servers/{id}/mods/{modId}", h.updateMod)
	r.Delete("/api/servers/{id}/mods/{modId}", h.deleteMod)
	r.Get("/api/mods", h.listGlobalMods)
	r.Post("/api/mods/upload", h.uploadGlobalMod)
	r.Post("/api/mods/{id}/assign", h.assignMod)
	r.Delete("/api/mods/{id}", h.deleteGlobalMod)
	r.Get("/api/terraria/presets", h.presets)
	r.Get("/api/terraria/versions", h.versions)
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
	item, created, err := h.upsertModRecord(r.Context(), server.ID, header.Filename, size)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "mod.uploaded", fmt.Sprintf("Uploaded mod %s to %s", item.FileName, server.Name))
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, item)
}

func (h *Handler) updateMod(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	item, err := h.store.GetMod(r.Context(), chi.URLParam(r, "modId"))
	if err != nil || item.InstanceID != server.ID {
		writeError(w, http.StatusNotFound, "mod not found")
		return
	}
	var payload struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if payload.Enabled == nil {
		writeError(w, http.StatusBadRequest, "enabled is required")
		return
	}
	item.Enabled = *payload.Enabled
	if err := h.store.SaveMod(r.Context(), &item); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "mod.updated", fmt.Sprintf("Updated mod %s", item.FileName))
	writeJSON(w, http.StatusOK, item)
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
	h.recordActivity(r.Context(), item.InstanceID, "mod.deleted", fmt.Sprintf("Deleted mod %s", item.FileName))
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) listGlobalMods(w http.ResponseWriter, r *http.Request) {
	mods, err := h.store.ListMods(r.Context(), "unassigned")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, mods)
}

func (h *Handler) uploadGlobalMod(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "mod file is required")
		return
	}
	defer file.Close()
	_, size, err := modsvc.NewService(h.cfg.DataDir).Upload("unassigned", header.Filename, file)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	item, created, err := h.upsertModRecord(r.Context(), "unassigned", header.Filename, size)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, item)
}

func (h *Handler) assignMod(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.GetMod(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "mod not found")
		return
	}
	var payload struct {
		InstanceID string `json:"instanceId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.InstanceID == "" {
		writeError(w, http.StatusBadRequest, "instanceId is required")
		return
	}
	targetServer, err := h.store.GetServer(r.Context(), payload.InstanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if targetServer.ProviderKey != domain.ProviderTerrariaTModLoader {
		writeError(w, http.StatusBadRequest, "mods are only supported for tModLoader servers")
		return
	}
	sourcePath, _ := modsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
	src, err := os.Open(sourcePath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "mod file not found")
		return
	}
	defer src.Close()
	_, size, err := modsvc.NewService(h.cfg.DataDir).Upload(targetServer.ID, item.FileName, src)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	assigned, created, err := h.upsertModRecord(r.Context(), targetServer.ID, item.FileName, size)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !created {
		h.recordActivity(r.Context(), targetServer.ID, "mod.assigned", fmt.Sprintf("Updated assigned mod %s for %s", item.FileName, targetServer.Name))
		writeJSON(w, http.StatusOK, assigned)
		return
	}
	h.recordActivity(r.Context(), targetServer.ID, "mod.assigned", fmt.Sprintf("Assigned mod %s to %s", item.FileName, targetServer.Name))
	writeJSON(w, http.StatusCreated, assigned)
}

func (h *Handler) upsertModRecord(ctx context.Context, instanceID string, fileName string, size int64) (domain.ModFile, bool, error) {
	if existing, err := h.store.GetModByInstanceAndFile(ctx, instanceID, fileName); err == nil {
		existing.SizeBytes = size
		existing.Enabled = true
		return existing, false, h.store.SaveMod(ctx, &existing)
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.ModFile{}, false, err
	}
	item := domain.ModFile{ID: uuid.NewString(), InstanceID: instanceID, FileName: fileName, SizeBytes: size, Enabled: true, CreatedAt: time.Now()}
	return item, true, h.store.CreateMod(ctx, &item)
}

func (h *Handler) deleteGlobalMod(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.GetMod(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "mod not found")
		return
	}
	if item.InstanceID != "unassigned" {
		writeError(w, http.StatusBadRequest, "global mod delete only supports unassigned library mods")
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
	svc := worldsvc.NewService(h.cfg.DataDir)
	visible := make([]domain.World, 0, len(worlds))
	for _, world := range worlds {
		path, err := svc.Path(world.InstanceID, world.FileName)
		if err != nil {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			h.logger.Warn("world file missing, pruning orphaned record", "worldId", world.ID, "path", path)
			_ = h.store.DeleteWorld(r.Context(), world.ID)
			continue
		}
		visible = append(visible, world)
	}
	writeJSON(w, http.StatusOK, visible)
}

func (h *Handler) importWorld(w http.ResponseWriter, r *http.Request) {
	instanceID := r.FormValue("instanceId")
	if instanceID == "" {
		instanceID = "unassigned"
	}
	if instanceID != "unassigned" {
		if _, err := h.store.GetServer(r.Context(), instanceID); err != nil {
			writeError(w, http.StatusNotFound, "server not found")
			return
		}
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
	item, created, err := h.upsertWorldRecord(r.Context(), instanceID, header.Filename[:len(header.Filename)-len(filepath.Ext(header.Filename))], header.Filename, size)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), instanceID, "world.imported", fmt.Sprintf("Imported world %s", item.Name))
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, item)
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
	if _, err := os.Stat(path); err != nil {
		writeError(w, http.StatusNotFound, "world file not found on disk")
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
	server, err := h.store.GetServer(r.Context(), payload.InstanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if server.Status == domain.StatusRunning || server.Status == domain.StatusRestarting {
		writeError(w, http.StatusConflict, "stop the server before assigning a world")
		return
	}
	if item.InstanceID != payload.InstanceID {
		writeError(w, http.StatusConflict, "world must be imported or migrated to this server before assignment")
		return
	}
	nextConfig := server.Config
	nextConfig.WorldName = item.Name
	if err := h.applyServerConfig(r.Context(), &server, nextConfig); err != nil {
		writeError(w, statusCodeForRuntimeError(err), err.Error())
		return
	}
	if err := h.clearActiveWorlds(r.Context(), payload.InstanceID, item.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	item.ActiveInstanceID = payload.InstanceID
	item.UpdatedAt = time.Now()
	if err := h.store.SaveServer(r.Context(), &server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.store.SaveWorld(r.Context(), &item); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), payload.InstanceID, "world.assigned", fmt.Sprintf("Assigned world %s to %s", item.Name, server.Name))
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) clearActiveWorlds(ctx context.Context, instanceID string, keepWorldID string) error {
	worlds, err := h.store.ListWorlds(ctx)
	if err != nil {
		return err
	}
	for _, world := range worlds {
		if world.ID == keepWorldID || world.ActiveInstanceID != instanceID {
			continue
		}
		world.ActiveInstanceID = ""
		world.UpdatedAt = time.Now()
		if err := h.store.SaveWorld(ctx, &world); err != nil {
			return err
		}
	}
	return nil
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
	copy, created, err := h.upsertWorldRecord(r.Context(), item.InstanceID, payload.Name, payload.FileName, size)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), item.ActiveInstanceID, "world.duplicated", fmt.Sprintf("Duplicated world %s", item.Name))
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, copy)
}

func (h *Handler) migrateWorld(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.GetWorld(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "world not found")
		return
	}
	var payload struct {
		InstanceID string `json:"instanceId"`
		FileName   string `json:"fileName"`
		Name       string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.InstanceID == "" {
		writeError(w, http.StatusBadRequest, "instanceId is required")
		return
	}
	if _, err := h.store.GetServer(r.Context(), payload.InstanceID); err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if payload.FileName == "" {
		payload.FileName = item.FileName
	}
	if payload.Name == "" {
		payload.Name = item.Name
	}
	_, size, err := worldsvc.NewService(h.cfg.DataDir).Migrate(item.InstanceID, item.FileName, payload.InstanceID, payload.FileName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	migrated, created, err := h.upsertWorldRecord(r.Context(), payload.InstanceID, payload.Name, payload.FileName, size)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), payload.InstanceID, "world.migrated", fmt.Sprintf("Migrated world %s", migrated.Name))
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, migrated)
}

func (h *Handler) upsertWorldRecord(ctx context.Context, instanceID string, name string, fileName string, size int64) (domain.World, bool, error) {
	if existing, err := h.store.GetWorldByInstanceAndFile(ctx, instanceID, fileName); err == nil {
		existing.Name = name
		existing.SizeBytes = size
		existing.UpdatedAt = time.Now()
		return existing, false, h.store.SaveWorld(ctx, &existing)
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.World{}, false, err
	}
	item := domain.World{ID: uuid.NewString(), InstanceID: instanceID, Name: name, FileName: fileName, SizeBytes: size, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	return item, true, h.store.CreateWorld(ctx, &item)
}

func (h *Handler) deleteWorld(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.GetWorld(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "world not found")
		return
	}
	if item.ActiveInstanceID != "" {
		server, err := h.store.GetServer(r.Context(), item.ActiveInstanceID)
		if err == nil && server.WorldName == item.Name {
			writeError(w, http.StatusConflict, "switch the server to another world before deleting the active world")
			return
		}
	}
	path, _ := worldsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
	_ = os.Remove(path)
	_ = h.store.DeleteWorld(r.Context(), item.ID)
	h.recordActivity(r.Context(), item.ActiveInstanceID, "world.deleted", fmt.Sprintf("Deleted world %s", item.Name))
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) listBackups(w http.ResponseWriter, r *http.Request) {
	backups, err := h.store.ListBackups(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	svc := backupsvc.NewService(h.cfg.DataDir)
	visible := make([]domain.Backup, 0, len(backups))
	for _, b := range backups {
		path, err := svc.Path(b.InstanceID, b.FileName)
		if err != nil {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			h.logger.Warn("backup file missing, pruning orphaned record", "backupId", b.ID, "path", path)
			_ = h.store.DeleteBackup(r.Context(), b.ID)
			continue
		}
		visible = append(visible, b)
	}
	writeJSON(w, http.StatusOK, visible)
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
	h.recordActivity(r.Context(), server.ID, "backup.created", fmt.Sprintf("Created backup %s for %s", item.FileName, server.Name))
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
	if _, err := os.Stat(path); err != nil {
		writeError(w, http.StatusNotFound, "backup file not found on disk")
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
	restorePath, _ := backupsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
	if _, err := os.Stat(restorePath); err != nil {
		writeError(w, http.StatusNotFound, "backup file not found on disk")
		return
	}
	if err := backupsvc.NewService(h.cfg.DataDir).Restore(item.InstanceID, item.FileName, server.DataDir); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "backup.restored", fmt.Sprintf("Restored backup %s for %s", item.FileName, server.Name))
	writeJSON(w, http.StatusOK, map[string]string{"status": "restored", "backupId": item.ID})
}

func (h *Handler) migrateBackup(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.GetBackup(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	var payload struct {
		InstanceID string `json:"instanceId"`
		FileName   string `json:"fileName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.InstanceID == "" {
		writeError(w, http.StatusBadRequest, "instanceId is required")
		return
	}
	targetServer, err := h.store.GetServer(r.Context(), payload.InstanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if payload.FileName == "" {
		payload.FileName = item.FileName
	}
	_, size, err := backupsvc.NewService(h.cfg.DataDir).Migrate(item.InstanceID, item.FileName, payload.InstanceID, payload.FileName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	migrated, created, err := h.upsertBackupRecord(r.Context(), payload.InstanceID, payload.FileName, targetServer.WorldName, size, item.Type)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), payload.InstanceID, "backup.migrated", fmt.Sprintf("Migrated backup %s", migrated.FileName))
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, migrated)
}

func (h *Handler) upsertBackupRecord(ctx context.Context, instanceID string, fileName string, worldName string, size int64, backupType string) (domain.Backup, bool, error) {
	if existing, err := h.store.GetBackupByInstanceAndFile(ctx, instanceID, fileName); err == nil {
		existing.WorldName = worldName
		existing.SizeBytes = size
		existing.Type = backupType
		return existing, false, h.store.SaveBackup(ctx, &existing)
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.Backup{}, false, err
	}
	item := domain.Backup{ID: uuid.NewString(), InstanceID: instanceID, FileName: fileName, WorldName: worldName, SizeBytes: size, Type: backupType, CreatedAt: time.Now()}
	return item, true, h.store.CreateBackup(ctx, &item)
}

func (h *Handler) deleteBackup(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.GetBackup(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	_ = h.store.DeleteBackup(r.Context(), item.ID)
	path, _ := backupsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
	_ = os.Remove(path)
	h.recordActivity(r.Context(), item.InstanceID, "backup.deleted", fmt.Sprintf("Deleted backup %s", item.FileName))
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
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
	writeJSON(w, http.StatusOK, h.dockerMonitor.Status())
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
	writeJSON(w, http.StatusOK, h.dockerMonitor.Refresh(r.Context()))
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"host":       h.cfg.Host,
		"port":       h.cfg.Port,
		"dataDir":    h.cfg.DataDir,
		"dbPath":     h.cfg.DBPath,
		"dockerHost": h.cfg.DockerHost,
	})
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		DockerHost string `json:"dockerHost"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	host := strings.TrimSpace(payload.DockerHost)
	if host == "" {
		writeError(w, http.StatusBadRequest, "dockerHost is required")
		return
	}
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
	h.dockerMonitor.Refresh(r.Context())
	h.getSettings(w, r)
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
	for index := range servers {
		servers[index] = h.refreshServerStatus(r.Context(), servers[index])
	}
	writeJSON(w, http.StatusOK, servers)
}

func (h *Handler) listActivity(w http.ResponseWriter, r *http.Request) {
	events, err := h.store.ListActivity(r.Context(), 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, events)
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
	server = h.refreshServerStatus(r.Context(), server)
	writeJSON(w, http.StatusOK, server)
}

func (h *Handler) updateServerConfig(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if server.Status == domain.StatusRunning || server.Status == domain.StatusRestarting {
		writeError(w, http.StatusConflict, "stop the server before updating config")
		return
	}
	var payload struct {
		Config domain.TerrariaConfig `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := h.applyServerConfig(r.Context(), &server, payload.Config); err != nil {
		writeError(w, statusCodeForRuntimeError(err), err.Error())
		return
	}
	if err := h.store.SaveServer(r.Context(), &server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.config.updated", fmt.Sprintf("Updated config for %s", server.Name))
	writeJSON(w, http.StatusOK, server)
}

func (h *Handler) createServer(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name        string                `json:"name"`
		ProviderKey domain.ProviderKey    `json:"providerKey"`
		Config      domain.TerrariaConfig `json:"config"`
		Version     string                `json:"version"`
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
	if payload.Version == "" {
		payload.Version = gameProvider.Versions()[0]
	}
	if !providerSupportsVersion(gameProvider, payload.Version) {
		writeError(w, http.StatusBadRequest, "unsupported provider version")
		return
	}
	if err := gameProvider.ValidateConfig(payload.Config); err != nil {
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
		Config: payload.Config, Version: payload.Version, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := h.store.CreateServer(r.Context(), &server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.created", fmt.Sprintf("Created server %s", server.Name))
	writeJSON(w, http.StatusCreated, server)
}

func (h *Handler) startServer(w http.ResponseWriter, r *http.Request) {
	server, err := h.serverWithRuntimeContainer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, statusCodeForRuntimeError(err), err.Error())
		return
	}
	if err := h.runtime.Start(r.Context(), server); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	server.Status = domain.StatusRunning
	server.UpdatedAt = time.Now()
	if err := h.store.SaveServer(r.Context(), &server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.started", fmt.Sprintf("Started server %s", server.Name))
	writeJSON(w, http.StatusOK, server)
}

func (h *Handler) stopServer(w http.ResponseWriter, r *http.Request) {
	h.transitionServer(w, r, domain.StatusStopped, h.runtime.Stop)
}

func (h *Handler) restartServer(w http.ResponseWriter, r *http.Request) {
	server, err := h.serverWithRuntimeContainer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, statusCodeForRuntimeError(err), err.Error())
		return
	}
	if err := h.runtime.Restart(r.Context(), server); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	server.Status = domain.StatusRunning
	server.UpdatedAt = time.Now()
	if err := h.store.SaveServer(r.Context(), &server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.restarted", fmt.Sprintf("Restarted server %s", server.Name))
	writeJSON(w, http.StatusOK, server)
}

func (h *Handler) sendServerCommand(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
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
	if server.Status != domain.StatusRunning {
		writeError(w, http.StatusConflict, "server must be running to send commands")
		return
	}
	server, recreated, err := h.ensureRuntimeContainer(r.Context(), server)
	if err != nil {
		writeError(w, statusCodeForRuntimeError(err), err.Error())
		return
	}
	server, err = h.startRecreatedRunningContainer(r.Context(), server, recreated)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err := h.runtime.SendCommand(r.Context(), server, command); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (h *Handler) transitionServer(w http.ResponseWriter, r *http.Request, status domain.ServerStatus, action func(context.Context, domain.GameServerInstance) error) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if server.ContainerID != "" {
		if err := h.requireRuntimeAvailable(r.Context()); err != nil {
			writeError(w, statusCodeForRuntimeError(err), err.Error())
			return
		}
		if _, err := h.runtime.Inspect(r.Context(), server); err != nil {
			h.logger.Warn("runtime container missing during state transition; clearing stale container", "server", server.ID, "container", server.ContainerID, "error", err)
			server.ContainerID = ""
		} else {
			if err := action(r.Context(), server); err != nil {
				writeError(w, http.StatusServiceUnavailable, err.Error())
				return
			}
		}
	}
	server.Status = status
	server.UpdatedAt = time.Now()
	if err := h.store.SaveServer(r.Context(), &server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	activityType := "server.updated"
	message := fmt.Sprintf("Updated server %s", server.Name)
	if status == domain.StatusStopped {
		activityType = "server.stopped"
		message = fmt.Sprintf("Stopped server %s", server.Name)
	}
	h.recordActivity(r.Context(), server.ID, activityType, message)
	writeJSON(w, http.StatusOK, server)
}

func (h *Handler) serverWithRuntimeContainer(ctx context.Context, id string) (domain.GameServerInstance, error) {
	server, err := h.store.GetServer(ctx, id)
	if err != nil {
		return domain.GameServerInstance{}, errors.New("server not found")
	}
	server, _, err = h.ensureRuntimeContainer(ctx, server)
	return server, err
}

func (h *Handler) ensureRuntimeContainer(ctx context.Context, server domain.GameServerInstance) (domain.GameServerInstance, bool, error) {
	if err := h.requireRuntimeAvailable(ctx); err != nil {
		return domain.GameServerInstance{}, false, err
	}
	if server.ContainerID != "" {
		if _, err := h.runtime.Inspect(ctx, server); err == nil {
			return server, false, nil
		}
		h.logger.Warn("runtime container missing; recreating from server data", "server", server.ID, "container", server.ContainerID)
		server.ContainerID = ""
	}
	gameProvider, ok := h.provider.Get(server.ProviderKey)
	if !ok {
		return domain.GameServerInstance{}, false, fmt.Errorf("unknown provider: %s", server.ProviderKey)
	}
	configText, err := gameProvider.RenderConfig(server.Config)
	if err != nil {
		return domain.GameServerInstance{}, false, err
	}
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		return domain.GameServerInstance{}, false, err
	}
	if server.HostPort == 0 {
		server.HostPort, err = h.allocateHostPort(ctx, server.ID)
		if err != nil {
			return domain.GameServerInstance{}, false, err
		}
	}
	containerID, err := h.runtime.Create(ctx, runtime.ContainerSpec{
		InstanceID: server.ID,
		Name:       server.Name,
		Image:      gameProvider.ImageFor(server.Version),
		Port:       server.Port,
		HostPort:   server.HostPort,
		DataDir:    server.DataDir,
		ConfigText: configText,
		Options:    gameProvider.RuntimeOptions(server.Config),
	})
	if err != nil {
		return domain.GameServerInstance{}, false, err
	}
	server.ContainerID = containerID
	server.UpdatedAt = time.Now()
	if err := h.store.SaveServer(ctx, &server); err != nil {
		return domain.GameServerInstance{}, false, err
	}
	return server, true, nil
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

func (h *Handler) startRecreatedRunningContainer(ctx context.Context, server domain.GameServerInstance, recreated bool) (domain.GameServerInstance, error) {
	if !recreated || server.Status != domain.StatusRunning {
		return server, nil
	}
	if err := h.runtime.Start(ctx, server); err != nil {
		return domain.GameServerInstance{}, err
	}
	server.Status = domain.StatusRunning
	server.UpdatedAt = time.Now()
	if err := h.store.SaveServer(ctx, &server); err != nil {
		return domain.GameServerInstance{}, err
	}
	return server, nil
}

func (h *Handler) applyServerConfig(ctx context.Context, server *domain.GameServerInstance, nextConfig domain.TerrariaConfig) error {
	if server.Status == domain.StatusRunning || server.Status == domain.StatusRestarting {
		return fmt.Errorf("stop the server before updating config")
	}
	gameProvider, ok := h.provider.Get(server.ProviderKey)
	if !ok {
		return fmt.Errorf("unknown provider: %s", server.ProviderKey)
	}
	if nextConfig.ServerName == "" {
		nextConfig.ServerName = server.Name
	}
	if err := gameProvider.ValidateConfig(nextConfig); err != nil {
		return err
	}
	configText, err := gameProvider.RenderConfig(nextConfig)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(server.DataDir, "serverconfig.txt"), []byte(configText), 0o600); err != nil {
		return err
	}
	for name, content := range gameProvider.RuntimeOptions(nextConfig).Files {
		if err := writeInstanceDataFile(server.DataDir, name, content); err != nil {
			return err
		}
	}
	if server.ContainerID != "" {
		if _, err := h.runtime.Inspect(ctx, *server); err == nil {
			if err := h.runtime.Remove(ctx, *server); err != nil {
				return err
			}
		}
		server.ContainerID = ""
	}
	server.Name = nextConfig.ServerName
	server.WorldName = nextConfig.WorldName
	server.Port = nextConfig.Port
	server.MaxPlayers = nextConfig.MaxPlayers
	server.Password = nextConfig.Password
	server.Config = nextConfig
	server.UpdatedAt = time.Now()
	return nil
}

func (h *Handler) refreshServerStatus(ctx context.Context, server domain.GameServerInstance) domain.GameServerInstance {
	if server.ContainerID == "" {
		return server
	}
	status, err := h.runtime.Inspect(ctx, server)
	if err != nil {
		h.logger.Warn("failed to refresh runtime server status", "server", server.ID, "container", server.ContainerID, "error", err)
		return server
	}
	if status == server.Status {
		return server
	}
	server.Status = status
	server.UpdatedAt = time.Now()
	if err := h.store.SaveServer(ctx, &server); err != nil {
		h.logger.Warn("failed to persist refreshed runtime server status", "server", server.ID, "status", status, "error", err)
	}
	return server
}

func statusCodeForRuntimeError(err error) int {
	if err != nil && err.Error() == "server not found" {
		return http.StatusNotFound
	}
	if err != nil && (strings.Contains(err.Error(), "required") ||
		strings.Contains(err.Error(), "must be") ||
		strings.Contains(err.Error(), "cannot contain") ||
		strings.Contains(err.Error(), "invalid")) {
		return http.StatusBadRequest
	}
	if err != nil && strings.Contains(err.Error(), "stop the server") {
		return http.StatusConflict
	}
	if err != nil && strings.Contains(err.Error(), "unknown provider") {
		return http.StatusBadRequest
	}
	return http.StatusServiceUnavailable
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
	h.recordActivity(r.Context(), server.ID, "server.deleted", fmt.Sprintf("Deleted server %s", server.Name))
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) serverStats(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if server.Status != domain.StatusRunning {
		writeJSON(w, http.StatusOK, runtime.ContainerStats{})
		return
	}
	stats, err := h.runtime.Stats(r.Context(), server)
	if err != nil {
		h.logger.Warn("failed to get container stats", "server", server.ID, "error", err)
		writeJSON(w, http.StatusOK, runtime.ContainerStats{})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) allocateHostPort(ctx context.Context, excludeInstanceID string) (int, error) {
	servers, err := h.store.ListServers(ctx)
	if err != nil {
		return 0, err
	}
	used := map[int]bool{}
	for _, s := range servers {
		if s.ID != excludeInstanceID && s.HostPort > 0 {
			used[s.HostPort] = true
		}
	}
	port := 7777
	for port < 65535 {
		if !used[port] {
			return port, nil
		}
		port++
	}
	return 0, fmt.Errorf("no available host port in range 7777-65535")
}

func (h *Handler) recordActivity(ctx context.Context, instanceID, eventType, message string) {
	event := domain.ActivityEvent{
		ID:         uuid.NewString(),
		InstanceID: instanceID,
		Type:       eventType,
		Message:    message,
		CreatedAt:  time.Now(),
	}
	if err := h.store.CreateActivity(ctx, &event); err != nil {
		h.logger.Warn("failed to record activity", "error", err, "type", eventType)
	}
}

func (h *Handler) serverLogs(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if server.Status == domain.StatusRunning {
		var recreated bool
		server, recreated, err = h.ensureRuntimeContainer(r.Context(), server)
		if err != nil {
			writeError(w, statusCodeForRuntimeError(err), err.Error())
			return
		}
		server, err = h.startRecreatedRunningContainer(r.Context(), server, recreated)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
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

func (h *Handler) serverLogSnapshot(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if server.Status != domain.StatusRunning && server.ContainerID != "" {
		if _, err := h.runtime.Inspect(r.Context(), server); err != nil {
			h.logger.Warn("stopped server log snapshot found stale runtime container; clearing container id", "server", server.ID, "container", server.ContainerID, "error", err)
			server.ContainerID = ""
			server.UpdatedAt = time.Now()
			if saveErr := h.store.SaveServer(r.Context(), &server); saveErr != nil {
				writeError(w, http.StatusInternalServerError, saveErr.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string][]string{"lines": []string{}})
			return
		}
	}
	server.Status = domain.StatusStopped
	stream, err := h.runtime.Logs(r.Context(), server)
	if err != nil {
		if server.Status != domain.StatusRunning {
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

func (h *Handler) presets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, terraria.Presets)
}

func (h *Handler) versions(w http.ResponseWriter, r *http.Request) {
	out := map[string][]string{}
	for _, provider := range h.provider.List() {
		out[string(provider.Key())] = provider.Versions()
	}
	writeJSON(w, http.StatusOK, out)
}

func providerSupportsVersion(gameProvider provider.GameProvider, version string) bool {
	for _, supported := range gameProvider.Versions() {
		if supported == version {
			return true
		}
	}
	return false
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

func writeInstanceDataFile(dataDir string, name string, content string) error {
	clean := filepath.Clean(name)
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return fmt.Errorf("invalid instance data file path: %s", name)
	}
	target := filepath.Join(dataDir, clean)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(content), 0o600)
}
