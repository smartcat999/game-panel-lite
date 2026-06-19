package http

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	backupsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/config"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/metrics"
	modsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/mod"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modcatalog"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/monitoring"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/observability"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/dst"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/minecraft"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/palworld"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/safety"
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
	apiMetrics     *metrics.Registry

	runtimeImageJobsMu sync.Mutex
	runtimeImageJobs   map[string]domain.RuntimeImageStatus
}

type resourceLimitPayload struct {
	CPULimitCores float64 `json:"cpuLimitCores,omitempty"`
	MemoryLimitMB int     `json:"memoryLimitMb,omitempty"`
}

const serverLifecycleTimeout = 15 * time.Minute
const staleLifecyclePendingAfter = 10 * time.Minute

func NewHandler(
	cfg config.Config,
	logger *slog.Logger,
	store *store.Store,
	providers *provider.Registry,
	adapter *runtime.SwitchableAdapter,
	dockerMonitor *runtime.DockerMonitor,
	runtimeFactory func(string) (runtime.Adapter, error),
	apiMetrics *metrics.Registry,
) *Handler {
	if apiMetrics == nil {
		apiMetrics = metrics.NewRegistry()
	}
	return &Handler{
		cfg:              cfg,
		logger:           logger,
		store:            store,
		provider:         providers,
		runtime:          adapter,
		dockerMonitor:    dockerMonitor,
		runtimeFactory:   runtimeFactory,
		apiMetrics:       apiMetrics,
		runtimeImageJobs: map[string]domain.RuntimeImageStatus{},
	}
}

func (h *Handler) Register(r chi.Router) {
	r.Use(h.cors)
	r.Use(h.apiMetrics.Middleware)
	r.Get("/healthz", h.health)
	r.With(h.optionalAuth).Get("/api/auth/bootstrap", h.authBootstrap)
	r.Post("/api/auth/setup", h.setupAdmin)
	r.Post("/api/auth/login", h.login)
	r.Post("/api/auth/logout", h.logout)
	r.Get("/metrics", h.prometheusMetrics)
	r.Get("/api/public/servers/{token}", h.getPublicServerShare)
	r.Group(func(r chi.Router) {
		r.Use(h.requireAuth)
		r.Get("/api/auth/me", h.currentAccount)
		r.Post("/api/auth/password", h.changePassword)
		r.Get("/api/version", h.version)
		r.Get("/api/runtime/docker", h.dockerStatus)
		r.Get("/api/runtime/stats", h.runtimeStats)
		r.Get("/api/observability/metrics", h.observabilityMetrics)
		r.Get("/api/observability/prometheus", h.prometheusMetrics)
		monitoring.NewHandler(monitoring.NewService(
			h.store,
			monitoring.NewPrometheusClient(h.cfg.PrometheusURL, h.cfg.PrometheusQueryTimeout, h.apiMetrics),
		)).Register(r)
		r.Post("/api/runtime/images/prepare", h.prepareRuntimeImage)
		r.Get("/api/settings", h.getSettings)
		r.Put("/api/settings/public-host", h.updatePublicHost)
		r.Get("/api/activity", h.listActivity)
		r.Get("/api/games", h.listGames)
		r.Get("/api/games/{gameKey}", h.getGame)
		r.Get("/api/games/{gameKey}/versions", h.gameVersions)
		r.Get("/api/config-presets", h.listConfigPresets)
		r.Post("/api/config-presets", h.createConfigPreset)
		r.Get("/api/config-presets/{id}", h.getConfigPreset)
		r.Put("/api/config-presets/{id}", h.updateConfigPreset)
		r.Delete("/api/config-presets/{id}", h.deleteConfigPreset)
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
		r.Get("/api/worlds/{id}/download", h.downloadWorld)
		r.Delete("/api/worlds/{id}", h.deleteWorld)
		r.Get("/api/backups", h.listBackups)
		r.Post("/api/servers/{id}/world-snapshots", h.createWorldSnapshot)
		r.Post("/api/servers/{id}/backups", h.createBackup)
		r.Get("/api/backups/{id}/download", h.downloadBackup)
		r.Post("/api/backups/{id}/restore", h.restoreBackup)
		r.Delete("/api/backups/{id}", h.deleteBackup)
		r.Get("/api/servers/{id}/saves", h.listServerSaves)
		r.Post("/api/servers/{id}/saves/snapshot", h.createServerSaveSnapshot)
		r.Get("/api/servers/{id}/saves/{saveId}/download", h.downloadServerSave)
		r.Post("/api/servers/{id}/saves/{saveId}/restore", h.restoreServerSave)
		r.Get("/api/servers/{id}/players", h.listServerPlayers)
		r.Post("/api/servers/{id}/players/{player}/kick", h.kickServerPlayer)
		r.Post("/api/servers/{id}/players/{player}/ban", h.banServerPlayer)
		r.Get("/api/servers/{id}/whitelist", h.getServerWhitelist)
		r.Post("/api/servers/{id}/whitelist/{player}", h.addServerWhitelistPlayer)
		r.Delete("/api/servers/{id}/whitelist/{player}", h.removeServerWhitelistPlayer)
		r.Get("/api/servers/{id}/join-info", h.getServerJoinInfo)
		r.Get("/api/servers/{id}/share", h.getServerShare)
		r.Post("/api/servers/{id}/share", h.enableServerShare)
		r.Delete("/api/servers/{id}/share", h.disableServerShare)
		r.Get("/api/servers/{id}/mods", h.listMods)
		r.Post("/api/servers/{id}/mods/upload", h.uploadMod)
		r.Post("/api/servers/{id}/mods/workshop", h.importWorkshopMods)
		r.Patch("/api/servers/{id}/mods/{modId}", h.updateMod)
		r.Delete("/api/servers/{id}/mods/{modId}", h.deleteMod)
		r.Get("/api/mods", h.listGlobalMods)
		r.Get("/api/mods/recommended", h.listRecommendedMods)
		r.Post("/api/mods/upload", h.uploadGlobalMod)
		r.Post("/api/mods/workshop", h.importGlobalWorkshopMods)
		r.Post("/api/mods/{id}/assign", h.assignMod)
		r.Delete("/api/mods/{id}", h.deleteGlobalMod)
		r.Get("/api/mod-packs", h.listModPacks)
		r.Post("/api/mod-packs", h.createModPack)
		r.Patch("/api/mod-packs/{id}", h.updateModPack)
		r.Delete("/api/mod-packs/{id}", h.deleteModPack)
		r.Get("/api/terraria/presets", h.presets)
		r.Get("/api/terraria/versions", h.versions)
		r.Post("/api/terraria/config/preview", h.configPreview)
	})
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
	visible, err := h.visibleServerMods(r.Context(), server, mods)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, visible)
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
	if isServerBusyForModMutation(server.Status) {
		writeError(w, http.StatusConflict, "server lifecycle action already in progress")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "mod file is required")
		return
	}
	defer file.Close()
	if !isTModPackage(header.Filename) {
		writeError(w, http.StatusBadRequest, "only .tmod files can be uploaded as mods")
		return
	}
	path, size, err := modsvc.NewService(h.cfg.DataDir).Upload(server.ID, header.Filename, file)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	metadata, err := modsvc.Inspect(path)
	if err != nil {
		h.logger.Warn("failed to parse tmod metadata", "file", header.Filename, "error", err)
	}
	item, created, err := h.upsertModRecord(r.Context(), server.ID, header.Filename, size, metadata)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.materializeModForRuntime(item, server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.syncRuntimeEnabledMods(r.Context(), server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dependencies, err := h.ensureModDependencies(r.Context(), server, []domain.ModFile{item})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(dependencies) > 0 {
		if err := h.syncRuntimeEnabledMods(r.Context(), server); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	h.recordActivity(r.Context(), server.ID, "mod.uploaded", fmt.Sprintf("Uploaded mod %s to %s", item.FileName, server.Name))
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, item)
}

func (h *Handler) importWorkshopMods(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if server.ProviderKey != domain.ProviderTerrariaTModLoader {
		writeError(w, http.StatusBadRequest, "workshop mods are only supported for tModLoader servers")
		return
	}
	if isServerBusyForModMutation(server.Status) {
		writeError(w, http.StatusConflict, "server lifecycle action already in progress")
		return
	}
	if h.workshopSyncUnsupported() {
		writeError(w, http.StatusConflict, "workshop mod sync is not supported on ARM Docker hosts; upload .tmod files instead")
		return
	}
	workshopIDs, err := decodeWorkshopIDs(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	items := make([]domain.ModFile, 0, len(workshopIDs))
	for _, workshopID := range workshopIDs {
		item, _, err := h.upsertWorkshopModRecord(r.Context(), server.ID, workshopID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, item)
	}
	dependencies, err := h.ensureModDependencies(r.Context(), server, items)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items = append(items, dependencies...)
	if err := h.syncRuntimeEnabledMods(r.Context(), server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "mod.workshop_imported", fmt.Sprintf("Imported %d workshop mod IDs for %s", len(workshopIDs), server.Name))
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) importGlobalWorkshopMods(w http.ResponseWriter, r *http.Request) {
	if h.workshopSyncUnsupported() {
		writeError(w, http.StatusConflict, "workshop mod import is not supported on ARM Docker hosts; upload .tmod files instead")
		return
	}
	workshopIDs, err := decodeWorkshopIDs(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	items := make([]domain.ModFile, 0, len(workshopIDs))
	for _, workshopID := range workshopIDs {
		item, _, err := h.createWorkshopModRecord(r.Context(), "unassigned", workshopID)
		if err != nil {
			if errors.Is(err, errWorkshopModExists) {
				writeError(w, http.StatusConflict, fmt.Sprintf("workshop mod %s already exists in mod library", workshopID))
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, item)
	}
	h.recordActivity(r.Context(), "", "mod.workshop_imported", fmt.Sprintf("Imported %d workshop mod IDs into mod library", len(workshopIDs)))
	writeJSON(w, http.StatusOK, items)
}

func decodeWorkshopIDs(r *http.Request) ([]string, error) {
	var payload struct {
		WorkshopIDs []string `json:"workshopIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("invalid JSON body")
	}
	workshopIDs := uniqueNonEmptyStrings(payload.WorkshopIDs)
	if len(workshopIDs) == 0 {
		return nil, fmt.Errorf("select at least one workshop item")
	}
	for _, id := range workshopIDs {
		if !isDigitsOnly(id) {
			return nil, fmt.Errorf("workshop IDs must contain digits only")
		}
	}
	return workshopIDs, nil
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
	if isServerBusyForModMutation(server.Status) {
		writeError(w, http.StatusConflict, "server lifecycle action already in progress")
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
	if err := h.syncRuntimeEnabledMods(r.Context(), server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "mod.updated", fmt.Sprintf("Updated mod %s", item.FileName))
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) deleteMod(w http.ResponseWriter, r *http.Request) {
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
	if isServerBusyForModMutation(server.Status) {
		writeError(w, http.StatusConflict, "server lifecycle action already in progress")
		return
	}
	if item.Source != "workshop" {
		if err := h.removeRuntimeMod(item, server); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		path, _ := modsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
		if err := removeStoredFile(path); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := h.store.DeleteMod(r.Context(), item.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.syncRuntimeEnabledMods(r.Context(), server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), item.InstanceID, "mod.deleted", fmt.Sprintf("Deleted mod %s", item.FileName))
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) listGlobalMods(w http.ResponseWriter, r *http.Request) {
	mods, err := h.store.ListMods(r.Context(), "unassigned")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	visible, err := h.visibleMods(r.Context(), mods)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, visible)
}

type recommendedModResponse struct {
	modcatalog.RecommendedMod
	GameKey     domain.GameKey     `json:"gameKey"`
	ProviderKey domain.ProviderKey `json:"providerKey"`
	InLibrary   bool               `json:"inLibrary"`
	ModID       string             `json:"modId,omitempty"`
}

func (h *Handler) listRecommendedMods(w http.ResponseWriter, r *http.Request) {
	items, err := modcatalog.RecommendedTModLoaderMods()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	mods, err := h.store.ListMods(r.Context(), "unassigned")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	byWorkshopID := make(map[string]domain.ModFile, len(mods))
	for _, item := range mods {
		if item.Source == "workshop" && item.WorkshopID != "" {
			byWorkshopID[item.WorkshopID] = item
		}
	}
	response := make([]recommendedModResponse, 0, len(items))
	for _, item := range items {
		entry := recommendedModResponse{
			RecommendedMod: item,
			GameKey:        domain.GameTerraria,
			ProviderKey:    domain.ProviderTerrariaTModLoader,
		}
		if mod, ok := byWorkshopID[item.WorkshopID]; ok {
			entry.InLibrary = true
			entry.ModID = mod.ID
		}
		response = append(response, entry)
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) uploadGlobalMod(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "mod file is required")
		return
	}
	defer file.Close()
	if !isTModPackage(header.Filename) {
		writeError(w, http.StatusBadRequest, "only .tmod files can be uploaded as mods")
		return
	}
	path, size, err := modsvc.NewService(h.cfg.DataDir).Upload("unassigned", header.Filename, file)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	metadata, err := modsvc.Inspect(path)
	if err != nil {
		h.logger.Warn("failed to parse tmod metadata", "file", header.Filename, "error", err)
	}
	item, created, err := h.upsertModRecord(r.Context(), "unassigned", header.Filename, size, metadata)
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
	if isServerBusyForModMutation(targetServer.Status) {
		writeError(w, http.StatusConflict, "server lifecycle action already in progress")
		return
	}
	if item.Source == "workshop" {
		if h.workshopSyncUnsupported() {
			writeError(w, http.StatusConflict, "workshop mods are not supported on ARM Docker hosts; upload the .tmod file instead")
			return
		}
		assigned, created, err := h.upsertWorkshopModRecord(r.Context(), targetServer.ID, item.WorkshopID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := h.syncRuntimeEnabledMods(r.Context(), targetServer); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		dependencies, err := h.ensureModDependencies(r.Context(), targetServer, []domain.ModFile{assigned})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if len(dependencies) > 0 {
			if err := h.syncRuntimeEnabledMods(r.Context(), targetServer); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if !created {
			h.recordActivity(r.Context(), targetServer.ID, "mod.assigned", fmt.Sprintf("Updated assigned mod %s for %s", item.FileName, targetServer.Name))
			writeJSON(w, http.StatusOK, assigned)
			return
		}
		h.recordActivity(r.Context(), targetServer.ID, "mod.assigned", fmt.Sprintf("Assigned mod %s to %s", item.FileName, targetServer.Name))
		writeJSON(w, http.StatusCreated, assigned)
		return
	}
	size, err := h.copyLibraryModToServerCache(item, targetServer.ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	assigned, created, err := h.upsertModRecord(r.Context(), targetServer.ID, item.FileName, size, metadataFromMod(item))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.materializeModForRuntime(assigned, targetServer); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.syncRuntimeEnabledMods(r.Context(), targetServer); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dependencies, err := h.ensureModDependencies(r.Context(), targetServer, []domain.ModFile{assigned})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(dependencies) > 0 {
		if err := h.syncRuntimeEnabledMods(r.Context(), targetServer); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if !created {
		h.recordActivity(r.Context(), targetServer.ID, "mod.assigned", fmt.Sprintf("Updated assigned mod %s for %s", item.FileName, targetServer.Name))
		writeJSON(w, http.StatusOK, assigned)
		return
	}
	h.recordActivity(r.Context(), targetServer.ID, "mod.assigned", fmt.Sprintf("Assigned mod %s to %s", item.FileName, targetServer.Name))
	writeJSON(w, http.StatusCreated, assigned)
}

func (h *Handler) copyLibraryModToServerCache(item domain.ModFile, targetInstanceID string) (int64, error) {
	svc := modsvc.NewService(h.cfg.DataDir)
	sourcePath, err := svc.Path(item.InstanceID, item.FileName)
	if err != nil {
		return 0, err
	}
	src, err := os.Open(sourcePath)
	if err != nil {
		return 0, fmt.Errorf("mod file not found")
	}
	defer src.Close()
	_, size, err := svc.Upload(targetInstanceID, item.FileName, src)
	return size, err
}

func (h *Handler) upsertModRecord(ctx context.Context, instanceID string, fileName string, size int64, metadata modsvc.Metadata) (domain.ModFile, bool, error) {
	if existing, err := h.store.GetModByInstanceAndFile(ctx, instanceID, fileName); err == nil {
		existing.SizeBytes = size
		existing.Enabled = true
		if existing.Source == "" {
			existing.Source = "upload"
		}
		applyTModMetadata(&existing, metadata)
		hydrateModMetadata(&existing)
		return existing, false, h.store.SaveMod(ctx, &existing)
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.ModFile{}, false, err
	}
	item := domain.ModFile{ID: uuid.NewString(), InstanceID: instanceID, FileName: fileName, Source: "upload", SizeBytes: size, Enabled: true, CreatedAt: time.Now()}
	applyTModMetadata(&item, metadata)
	hydrateModMetadata(&item)
	return item, true, h.store.CreateMod(ctx, &item)
}

func applyTModMetadata(item *domain.ModFile, metadata modsvc.Metadata) {
	if metadata.Name != "" {
		item.ModName = metadata.Name
		item.Title = metadata.Name
	}
	if metadata.Version != "" {
		item.ModVersion = metadata.Version
	}
	if metadata.TModLoaderVersion != "" {
		item.TModVersion = metadata.TModLoaderVersion
	}
}

func metadataFromMod(item domain.ModFile) modsvc.Metadata {
	return modsvc.Metadata{
		Name:              item.Title,
		Version:           item.ModVersion,
		TModLoaderVersion: item.TModVersion,
	}
}

func (h *Handler) upsertWorkshopModRecord(ctx context.Context, instanceID string, workshopID string) (domain.ModFile, bool, error) {
	fileName := "workshop-" + workshopID
	if existing, err := h.store.GetModByInstanceAndWorkshopID(ctx, instanceID, workshopID); err == nil {
		existing.Source = "workshop"
		existing.WorkshopID = workshopID
		existing.Enabled = true
		applyRecommendedModMetadata(&existing, workshopID)
		return existing, false, h.store.SaveMod(ctx, &existing)
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.ModFile{}, false, err
	}
	if existing, err := h.store.GetModByInstanceAndFile(ctx, instanceID, fileName); err == nil {
		existing.Source = "workshop"
		existing.WorkshopID = workshopID
		existing.Enabled = true
		applyRecommendedModMetadata(&existing, workshopID)
		return existing, false, h.store.SaveMod(ctx, &existing)
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.ModFile{}, false, err
	}
	item := domain.ModFile{
		ID:         uuid.NewString(),
		InstanceID: instanceID,
		FileName:   fileName,
		Source:     "workshop",
		WorkshopID: workshopID,
		SizeBytes:  int64(len(workshopID) + 1),
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	applyRecommendedModMetadata(&item, workshopID)
	return item, true, h.store.CreateMod(ctx, &item)
}

func (h *Handler) ensureModDependencies(ctx context.Context, server domain.GameServerInstance, roots []domain.ModFile) ([]domain.ModFile, error) {
	if server.ProviderKey != domain.ProviderTerrariaTModLoader || len(roots) == 0 {
		return nil, nil
	}
	added := make([]domain.ModFile, 0)
	queue := append([]domain.ModFile(nil), roots...)
	seen := make(map[string]struct{}, len(queue))
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		key := modIdentity(item)
		if key != "" {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
		}
		for _, dependencyName := range modDependencies(item) {
			dependency, created, err := h.ensureModDependency(ctx, server, dependencyName)
			if err != nil {
				return nil, err
			}
			if created {
				added = append(added, dependency)
			}
			queue = append(queue, dependency)
		}
	}
	return added, nil
}

func (h *Handler) ensureModDependency(ctx context.Context, server domain.GameServerInstance, dependencyName string) (domain.ModFile, bool, error) {
	dependencyName = strings.TrimSpace(dependencyName)
	if dependencyName == "" {
		return domain.ModFile{}, false, nil
	}
	if existing, ok, err := h.findServerModByModName(ctx, server.ID, dependencyName); err != nil || ok {
		return existing, false, err
	}
	if library, ok, err := h.findLibraryModByModName(ctx, dependencyName); err != nil || ok {
		if err != nil {
			return domain.ModFile{}, false, err
		}
		if library.Source == "workshop" {
			assigned, created, err := h.upsertWorkshopModRecord(ctx, server.ID, library.WorkshopID)
			return assigned, created, err
		}
		size, err := h.copyLibraryModToServerCache(library, server.ID)
		if err != nil {
			return domain.ModFile{}, false, err
		}
		assigned, created, err := h.upsertModRecord(ctx, server.ID, library.FileName, size, metadataFromMod(library))
		if err != nil {
			return domain.ModFile{}, false, err
		}
		if err := h.materializeModForRuntime(assigned, server); err != nil {
			return domain.ModFile{}, false, err
		}
		return assigned, created, nil
	}
	recommended, ok := modcatalog.RecommendedTModLoaderModByModName(dependencyName)
	if !ok || recommended.WorkshopID == "" {
		return domain.ModFile{}, false, fmt.Errorf("missing dependency %s in mod library", dependencyName)
	}
	if h.workshopSyncUnsupported() {
		return domain.ModFile{}, false, fmt.Errorf("missing dependency %s in mod library; upload the .tmod dependency file first", dependencyName)
	}
	assigned, created, err := h.upsertWorkshopModRecord(ctx, server.ID, recommended.WorkshopID)
	return assigned, created, err
}

func (h *Handler) findServerModByModName(ctx context.Context, instanceID string, modName string) (domain.ModFile, bool, error) {
	mods, err := h.store.ListMods(ctx, instanceID)
	if err != nil {
		return domain.ModFile{}, false, err
	}
	for _, item := range mods {
		if modIdentity(item) == modName {
			return item, true, nil
		}
	}
	return domain.ModFile{}, false, nil
}

func (h *Handler) findLibraryModByModName(ctx context.Context, modName string) (domain.ModFile, bool, error) {
	mods, err := h.store.ListMods(ctx, "unassigned")
	if err != nil {
		return domain.ModFile{}, false, err
	}
	for _, item := range mods {
		if modIdentity(item) == modName {
			return item, true, nil
		}
	}
	return domain.ModFile{}, false, nil
}

func modIdentity(item domain.ModFile) string {
	if item.WorkshopID != "" {
		if recommended, ok := modcatalog.RecommendedTModLoaderModByWorkshopID(item.WorkshopID); ok && strings.TrimSpace(recommended.ModName) != "" {
			return recommended.ModName
		}
	}
	for _, value := range []string{item.ModName, item.Title, strings.TrimSuffix(item.FileName, filepath.Ext(item.FileName))} {
		value = strings.TrimSpace(value)
		if value != "" && !strings.HasPrefix(value, "workshop-") {
			return value
		}
	}
	return ""
}

func modDependencies(item domain.ModFile) []string {
	if len(item.Dependencies) > 0 {
		return uniqueNonEmptyStrings(item.Dependencies)
	}
	if strings.TrimSpace(item.DependenciesJSON) != "" {
		var values []string
		if err := json.Unmarshal([]byte(item.DependenciesJSON), &values); err == nil {
			return uniqueNonEmptyStrings(values)
		}
	}
	if item.WorkshopID != "" {
		if recommended, ok := modcatalog.RecommendedTModLoaderModByWorkshopID(item.WorkshopID); ok {
			return uniqueNonEmptyStrings(recommended.Dependencies)
		}
	}
	if recommended, ok := modcatalog.RecommendedTModLoaderModByModName(modIdentity(item)); ok {
		return uniqueNonEmptyStrings(recommended.Dependencies)
	}
	return nil
}

var errWorkshopModExists = errors.New("workshop mod already exists")

func (h *Handler) createWorkshopModRecord(ctx context.Context, instanceID string, workshopID string) (domain.ModFile, bool, error) {
	if _, err := h.store.GetModByInstanceAndWorkshopID(ctx, instanceID, workshopID); err == nil {
		return domain.ModFile{}, false, errWorkshopModExists
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.ModFile{}, false, err
	}
	if _, err := h.store.GetModByInstanceAndFile(ctx, instanceID, "workshop-"+workshopID); err == nil {
		return domain.ModFile{}, false, errWorkshopModExists
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.ModFile{}, false, err
	}
	return h.upsertWorkshopModRecord(ctx, instanceID, workshopID)
}

func applyRecommendedModMetadata(item *domain.ModFile, workshopID string) {
	recommended, ok := modcatalog.RecommendedTModLoaderModByWorkshopID(workshopID)
	if !ok {
		return
	}
	tags, _ := json.Marshal(recommended.Tags)
	dependencies, _ := json.Marshal(uniqueNonEmptyStrings(recommended.Dependencies))
	item.ModName = recommended.ModName
	item.Title = recommended.Title
	item.CreatorSteamID = recommended.CreatorSteamID
	item.PreviewURL = recommended.PreviewURL
	item.Description = recommended.Description
	item.TagsJSON = string(tags)
	item.DependenciesJSON = string(dependencies)
	item.Subscriptions = recommended.Subscriptions
	item.Favorited = recommended.Favorited
	item.Views = recommended.Views
	item.UpdatedAtSteam = recommended.TimeUpdated
	if recommended.FileSize > 0 {
		item.SizeBytes = recommended.FileSize
	}
	hydrateModMetadata(item)
}

func hydrateModMetadata(item *domain.ModFile) {
	hydrateModGameMetadata(item)
	if item.TagsJSON != "" {
		_ = json.Unmarshal([]byte(item.TagsJSON), &item.Tags)
	}
	if item.DependenciesJSON != "" {
		_ = json.Unmarshal([]byte(item.DependenciesJSON), &item.Dependencies)
	}
	if item.Source == "workshop" && item.Title == "" && item.WorkshopID != "" {
		item.Title = "Workshop " + item.WorkshopID
	}
	if item.ModName == "" {
		item.ModName = modIdentity(*item)
	}
	if len(item.Dependencies) == 0 {
		item.Dependencies = modDependencies(*item)
	}
}

func hydrateModGameMetadata(item *domain.ModFile) {
	if item.GameKey == "" {
		item.GameKey = domain.GameTerraria
	}
	if item.ProviderKey == "" {
		item.ProviderKey = domain.ProviderTerrariaTModLoader
	}
}

func (h *Handler) materializeModForRuntime(item domain.ModFile, server domain.GameServerInstance) error {
	if item.Source == "workshop" {
		return nil
	}
	sourcePath, err := modsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
	if err != nil {
		return err
	}
	for _, relPath := range terraria.RuntimeModFiles(server.ProviderKey, item.FileName) {
		targetPath := filepath.Join(server.DataDir, relPath)
		if err := copyStoredFile(sourcePath, targetPath); err != nil {
			return err
		}
	}
	return nil
}

func ensureRuntimeDataDir(path string) error {
	if err := os.MkdirAll(path, 0o777); err != nil {
		return err
	}
	return os.Chmod(path, 0o777)
}

func writeRuntimeDataFile(targetPath string, content []byte) error {
	if err := ensureRuntimeDataDir(filepath.Dir(targetPath)); err != nil {
		return err
	}
	return os.WriteFile(targetPath, content, 0o666)
}

func (h *Handler) removeRuntimeMod(item domain.ModFile, server domain.GameServerInstance) error {
	for _, relPath := range terraria.RuntimeModFiles(server.ProviderKey, item.FileName) {
		if err := removeStoredFile(filepath.Join(server.DataDir, relPath)); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) syncRuntimeEnabledMods(ctx context.Context, server domain.GameServerInstance) error {
	if server.ProviderKey != domain.ProviderTerrariaTModLoader {
		return nil
	}
	mods, err := h.store.ListMods(ctx, server.ID)
	if err != nil {
		return err
	}
	enabled := make([]string, 0, len(mods))
	workshopIDs := make([]string, 0, len(mods))
	for _, item := range mods {
		if !item.Enabled {
			continue
		}
		if item.Source == "workshop" && item.WorkshopID != "" {
			workshopIDs = append(workshopIDs, item.WorkshopID)
			if name := modIdentity(item); name != "" {
				enabled = append(enabled, name)
			}
			continue
		}
		if isTModPackage(item.FileName) {
			if name := modIdentity(item); name != "" {
				enabled = append(enabled, name)
			}
		}
	}
	sort.Strings(enabled)
	sort.Strings(workshopIDs)
	payload, err := json.MarshalIndent(enabled, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	for _, relPath := range terraria.RuntimeModFiles(server.ProviderKey, "enabled.json") {
		targetPath := filepath.Join(server.DataDir, relPath)
		if err := writeRuntimeDataFile(targetPath, payload); err != nil {
			return err
		}
	}
	if err := h.writeRuntimeInstallFile(server, workshopIDs); err != nil {
		return err
	}
	return nil
}

func (h *Handler) writeRuntimeInstallFile(server domain.GameServerInstance, workshopIDs []string) error {
	content := ""
	if len(workshopIDs) > 0 {
		content = strings.Join(workshopIDs, "\n") + "\n"
	}
	for _, relPath := range terraria.RuntimeModFiles(server.ProviderKey, "install.txt") {
		targetPath := filepath.Join(server.DataDir, relPath)
		if err := writeRuntimeDataFile(targetPath, []byte(content)); err != nil {
			return err
		}
	}
	return nil
}

func isTModPackage(fileName string) bool {
	return strings.EqualFold(filepath.Ext(fileName), ".tmod")
}

func (h *Handler) visibleMods(ctx context.Context, mods []domain.ModFile) ([]domain.ModFile, error) {
	svc := modsvc.NewService(h.cfg.DataDir)
	visible := make([]domain.ModFile, 0, len(mods))
	for _, item := range mods {
		if item.Source == "workshop" {
			hydrateModMetadata(&item)
			visible = append(visible, item)
			continue
		}
		path, err := svc.Path(item.InstanceID, item.FileName)
		if err != nil {
			continue
		}
		if item.FileName == "install.txt" && item.Source == "" {
			items, err := h.migrateLegacyWorkshopInstall(ctx, item, path)
			if err != nil {
				return nil, err
			}
			visible = append(visible, items...)
			continue
		}
		if _, err := os.Stat(path); err != nil {
			h.logger.Warn("mod file missing, pruning orphaned record", "modId", item.ID, "path", path)
			if err := h.store.DeleteMod(ctx, item.ID); err != nil {
				return nil, err
			}
			continue
		}
		hydrateModMetadata(&item)
		visible = append(visible, item)
	}
	return visible, nil
}

func (h *Handler) visibleServerMods(ctx context.Context, server domain.GameServerInstance, mods []domain.ModFile) ([]domain.ModFile, error) {
	visible, err := h.visibleMods(ctx, mods)
	if err != nil {
		return nil, err
	}
	runtimeEnabled, err := readRuntimeEnabledMods(server)
	if err != nil {
		h.logger.Warn("failed to read runtime enabled mods", "server", server.ID, "error", err)
		return visible, nil
	}
	for index := range visible {
		visible[index].GameKey = server.GameKey
		visible[index].ProviderKey = server.ProviderKey
		present := runtimeModPresent(server, visible[index])
		visible[index].RuntimePresent = &present
		if runtimeEnabled == nil {
			continue
		}
		enabled := false
		if _, ok := runtimeEnabled[modIdentity(visible[index])]; ok {
			enabled = true
		}
		visible[index].RuntimeEnabled = &enabled
	}
	return visible, nil
}

func runtimeModPresent(server domain.GameServerInstance, item domain.ModFile) bool {
	if server.ProviderKey != domain.ProviderTerrariaTModLoader || strings.TrimSpace(server.DataDir) == "" {
		return true
	}
	candidates := []string{filepath.Join(server.DataDir, "Mods", item.FileName)}
	if identity := modIdentity(item); identity != "" {
		candidates = append(candidates, filepath.Join(server.DataDir, "Mods", identity+".tmod"))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func readRuntimeEnabledMods(server domain.GameServerInstance) (map[string]struct{}, error) {
	if server.ProviderKey != domain.ProviderTerrariaTModLoader || strings.TrimSpace(server.DataDir) == "" {
		return nil, nil
	}
	path := filepath.Join(server.DataDir, "Mods", "enabled.json")
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var values []string
	if err := json.Unmarshal(content, &values); err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result[value] = struct{}{}
		}
	}
	return result, nil
}

func (h *Handler) migrateLegacyWorkshopInstall(ctx context.Context, item domain.ModFile, path string) ([]domain.ModFile, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, h.store.DeleteMod(ctx, item.ID)
		}
		return nil, err
	}
	workshopIDs := workshopIDsFromInstallContent(string(content))
	items := make([]domain.ModFile, 0, len(workshopIDs))
	for _, workshopID := range workshopIDs {
		mod, _, err := h.upsertWorkshopModRecord(ctx, item.InstanceID, workshopID)
		if err != nil {
			return nil, err
		}
		items = append(items, mod)
	}
	if err := h.store.DeleteMod(ctx, item.ID); err != nil {
		return nil, err
	}
	if err := removeStoredFile(path); err != nil {
		return nil, err
	}
	return items, nil
}

func workshopIDsFromInstallContent(content string) []string {
	ids := make([]string, 0)
	seen := make(map[string]struct{})
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		id := strings.TrimSpace(scanner.Text())
		if id == "" || !isDigitsOnly(id) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
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
	if item.Source != "workshop" {
		path, _ := modsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
		if err := removeStoredFile(path); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := h.store.DeleteMod(r.Context(), item.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type modPackResponse struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	GameKey     domain.GameKey     `json:"gameKey,omitempty"`
	ProviderKey domain.ProviderKey `json:"providerKey,omitempty"`
	ModIDs      []string           `json:"modIds"`
	Mods        []domain.ModFile   `json:"mods"`
	CreatedAt   time.Time          `json:"createdAt"`
	UpdatedAt   time.Time          `json:"updatedAt"`
}

func (h *Handler) listModPacks(w http.ResponseWriter, r *http.Request) {
	packs, err := h.store.ListModPacks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	response := make([]modPackResponse, 0, len(packs))
	for _, pack := range packs {
		item, err := h.modPackResponse(r.Context(), pack)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		response = append(response, item)
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) createModPack(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		ModIDs      []string `json:"modIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	name, description, modIDsJSON, err := h.modPackPayload(r.Context(), payload.Name, payload.Description, payload.ModIDs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pack := domain.ModPack{
		ID:          uuid.NewString(),
		Name:        name,
		Description: description,
		ModIDsJSON:  string(modIDsJSON),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := h.store.CreateModPack(r.Context(), &pack); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	response, err := h.modPackResponse(r.Context(), pack)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, response)
}

func (h *Handler) updateModPack(w http.ResponseWriter, r *http.Request) {
	pack, err := h.store.GetModPack(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "mod pack not found")
		return
	}
	var payload struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		ModIDs      []string `json:"modIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	name, description, modIDsJSON, err := h.modPackPayload(r.Context(), payload.Name, payload.Description, payload.ModIDs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pack.Name = name
	pack.Description = description
	pack.ModIDsJSON = string(modIDsJSON)
	pack.UpdatedAt = time.Now()
	if err := h.store.SaveModPack(r.Context(), &pack); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	response, err := h.modPackResponse(r.Context(), pack)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) modPackPayload(ctx context.Context, rawName string, rawDescription string, rawModIDs []string) (string, string, []byte, error) {
	name := strings.TrimSpace(rawName)
	if name == "" {
		return "", "", nil, fmt.Errorf("mod pack name is required")
	}
	modIDs := uniqueNonEmptyStrings(rawModIDs)
	if len(modIDs) == 0 {
		return "", "", nil, fmt.Errorf("select at least one mod")
	}
	for _, modID := range modIDs {
		item, err := h.store.GetMod(ctx, modID)
		if err != nil {
			return "", "", nil, fmt.Errorf("mod not found")
		}
		if item.InstanceID != "unassigned" || (!isTModPackage(item.FileName) && item.Source != "workshop") {
			return "", "", nil, fmt.Errorf("mod packs can only use global mod library items")
		}
	}
	modIDsJSON, err := json.Marshal(modIDs)
	if err != nil {
		return "", "", nil, err
	}
	return name, strings.TrimSpace(rawDescription), modIDsJSON, nil
}

func (h *Handler) deleteModPack(w http.ResponseWriter, r *http.Request) {
	if _, err := h.store.GetModPack(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusNotFound, "mod pack not found")
		return
	}
	if err := h.store.DeleteModPack(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) modPackResponse(ctx context.Context, pack domain.ModPack) (modPackResponse, error) {
	modIDs, err := decodeModPackIDs(pack.ModIDsJSON)
	if err != nil {
		return modPackResponse{}, err
	}
	mods := make([]domain.ModFile, 0, len(modIDs))
	for _, modID := range modIDs {
		item, err := h.store.GetMod(ctx, modID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			return modPackResponse{}, err
		}
		if item.InstanceID != "unassigned" {
			continue
		}
		hydrateModMetadata(&item)
		mods = append(mods, item)
	}
	gameKey, providerKey := modPackGameMetadata(mods)
	return modPackResponse{
		ID:          pack.ID,
		Name:        pack.Name,
		Description: pack.Description,
		GameKey:     gameKey,
		ProviderKey: providerKey,
		ModIDs:      modIDs,
		Mods:        mods,
		CreatedAt:   pack.CreatedAt,
		UpdatedAt:   pack.UpdatedAt,
	}, nil
}

func modPackGameMetadata(mods []domain.ModFile) (domain.GameKey, domain.ProviderKey) {
	if len(mods) == 0 {
		return "", ""
	}
	gameKey := mods[0].GameKey
	providerKey := mods[0].ProviderKey
	for _, item := range mods[1:] {
		if item.GameKey != gameKey {
			gameKey = ""
		}
		if item.ProviderKey != providerKey {
			providerKey = ""
		}
	}
	return gameKey, providerKey
}

func decodeModPackIDs(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return []string{}, nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(value), &ids); err != nil {
		return nil, err
	}
	return uniqueNonEmptyStrings(ids), nil
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func isDigitsOnly(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
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
		visible = append(visible, h.hydrateWorldResource(r.Context(), world))
	}
	writeJSON(w, http.StatusOK, visible)
}

func (h *Handler) importWorld(w http.ResponseWriter, r *http.Request) {
	instanceID := r.FormValue("instanceId")
	if instanceID == "" {
		instanceID = "unassigned"
	}
	var providerKey domain.ProviderKey
	if instanceID != "unassigned" {
		server, err := h.store.GetServer(r.Context(), instanceID)
		if err != nil {
			writeError(w, http.StatusNotFound, "server not found")
			return
		}
		providerKey = server.ProviderKey
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
	item.ProviderKey = providerKey
	if err := h.store.SaveWorld(r.Context(), &item); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	item = h.hydrateWorldResource(r.Context(), item)
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
		h.logger.Warn("world file missing during download, pruning orphaned record", "worldId", item.ID, "path", path)
		_ = h.store.DeleteWorld(r.Context(), item.ID)
		writeError(w, http.StatusNotFound, "world file not found on disk")
		return
	}
	http.ServeFile(w, r, path)
}

func (h *Handler) createWorldSnapshot(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	var payload struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		name = server.WorldName
	}
	fileName := safeWorldSnapshotFileName(name)
	sourcePath, err := h.currentRuntimeWorldPath(server)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "world file not found on disk")
		return
	}
	defer source.Close()
	_, size, err := worldsvc.NewService(h.cfg.DataDir).Import(server.ID, fileName, source)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	item, created, err := h.upsertWorldSnapshotRecord(r.Context(), server, name, fileName, size)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "world.snapshot.created", fmt.Sprintf("Saved world snapshot %s from %s", item.Name, server.Name))
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, item)
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
	if isServerLockedForMutation(server.Status) {
		writeError(w, http.StatusConflict, "stop the server before assigning a world")
		return
	}
	if !worldCompatibleWithServer(item, server) {
		writeError(w, http.StatusConflict, "world snapshot is not compatible with this server type")
		return
	}
	nextConfig := server.Config
	configPayloadJSON, err := providerConfigPayloadJSON(server.ProviderKey, nextConfig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.applyServerConfig(r.Context(), &server, nextConfig, configPayloadJSON, nil, nil); err != nil {
		writeError(w, statusCodeForRuntimeError(err), err.Error())
		return
	}
	if err := h.materializeWorldForRuntime(item, server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.clearActiveWorlds(r.Context(), payload.InstanceID, item.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	item.ActiveInstanceID = payload.InstanceID
	item.UpdatedAt = time.Now()
	if server.SourceWorldID == "" {
		server.SourceWorldID = item.ID
		server.SourceWorldName = item.Name
	}
	if err := h.store.SaveServer(r.Context(), &server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.store.SaveWorld(r.Context(), &item); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), payload.InstanceID, "world.assigned", fmt.Sprintf("Assigned world %s to %s", item.Name, server.Name))
	writeJSON(w, http.StatusOK, h.hydrateWorldResource(r.Context(), item))
}

func (h *Handler) hydrateWorldResource(ctx context.Context, world domain.World) domain.World {
	if world.ProviderKey == "" && world.InstanceID != "" && world.InstanceID != "unassigned" {
		if server, err := h.store.GetServer(ctx, world.InstanceID); err == nil {
			world.ProviderKey = server.ProviderKey
			world.GameKey = server.GameKey
		}
	}
	if world.GameKey == "" && world.ProviderKey != "" {
		if gameProvider, ok := h.provider.Get(world.ProviderKey); ok {
			world.GameKey = gameProvider.GameKey()
		}
	}
	return world
}

func (h *Handler) materializeWorldForRuntime(world domain.World, server domain.GameServerInstance) error {
	sourcePath, err := worldsvc.NewService(h.cfg.DataDir).Path(world.InstanceID, world.FileName)
	if err != nil {
		return err
	}
	for _, relPath := range terraria.RuntimeWorldFiles(server.ProviderKey, server.Config) {
		targetPath := filepath.Join(server.DataDir, relPath)
		if err := copyStoredFile(sourcePath, targetPath); err != nil {
			return err
		}
	}
	return nil
}

func copyStoredFile(sourcePath string, targetPath string) error {
	if err := ensureRuntimeDataDir(filepath.Dir(targetPath)); err != nil {
		return err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.CreateTemp(filepath.Dir(targetPath), "."+filepath.Base(targetPath)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := target.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if _, err := io.Copy(target, source); err != nil {
		_ = target.Close()
		return err
	}
	if err := target.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o666); err != nil {
		return err
	}
	return os.Rename(tmpName, targetPath)
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

func worldCompatibleWithServer(world domain.World, server domain.GameServerInstance) bool {
	return world.ProviderKey == "" || world.ProviderKey == server.ProviderKey
}

func (h *Handler) upsertWorldSnapshotRecord(ctx context.Context, server domain.GameServerInstance, name string, fileName string, size int64) (domain.World, bool, error) {
	if existing, err := h.store.GetWorldByInstanceAndFile(ctx, server.ID, fileName); err == nil {
		existing.Name = name
		existing.SizeBytes = size
		existing.ProviderKey = server.ProviderKey
		existing.Source = "server_snapshot"
		existing.Config = server.Config
		if existing.ActiveInstanceID == server.ID {
			existing.ActiveInstanceID = ""
		}
		existing.UpdatedAt = time.Now()
		return existing, false, h.store.SaveWorld(ctx, &existing)
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.World{}, false, err
	}
	item := domain.World{
		ID:          uuid.NewString(),
		InstanceID:  server.ID,
		ProviderKey: server.ProviderKey,
		Name:        name,
		FileName:    fileName,
		SizeBytes:   size,
		Source:      "server_snapshot",
		Config:      server.Config,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	return item, true, h.store.CreateWorld(ctx, &item)
}

func (h *Handler) currentRuntimeWorldPath(server domain.GameServerInstance) (string, error) {
	for _, relPath := range runtimeWorldPathCandidates(server) {
		path := filepath.Join(server.DataDir, relPath)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("current world file has not been created yet")
}

func runtimeWorldPathCandidates(server domain.GameServerInstance) []string {
	worldFile := server.Config.WorldName + ".wld"
	candidates := append([]string{}, terraria.RuntimeWorldFiles(server.ProviderKey, server.Config)...)
	candidates = append(candidates, worldFile, filepath.Join("Worlds", worldFile), filepath.Join("worlds", worldFile))
	seen := map[string]bool{}
	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		clean := filepath.Clean(candidate)
		if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." || seen[clean] {
			continue
		}
		seen[clean] = true
		result = append(result, clean)
	}
	return result
}

func safeWorldSnapshotFileName(name string) string {
	clean := strings.Trim(strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, name), "-.")
	if clean == "" {
		clean = "world-snapshot"
	}
	if !strings.HasSuffix(strings.ToLower(clean), ".wld") {
		clean += ".wld"
	}
	return clean
}

func (h *Handler) deleteWorld(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.GetWorld(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "world not found")
		return
	}
	if inUse, err := h.worldTemplateInUse(r.Context(), item.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if inUse {
		writeError(w, http.StatusConflict, "world template is used by one or more servers")
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
	if err := removeStoredFile(path); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.store.DeleteWorld(r.Context(), item.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), item.ActiveInstanceID, "world.deleted", fmt.Sprintf("Deleted world %s", item.Name))
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) worldTemplateInUse(ctx context.Context, worldID string) (bool, error) {
	servers, err := h.store.ListServers(ctx)
	if err != nil {
		return false, err
	}
	for _, server := range servers {
		if server.SourceWorldID == worldID {
			return true, nil
		}
	}
	return false, nil
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
		visible = append(visible, h.hydrateBackupResource(r.Context(), b))
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
	writeJSON(w, http.StatusCreated, h.hydrateBackupResource(r.Context(), item))
}

func (h *Handler) hydrateBackupResource(ctx context.Context, backup domain.Backup) domain.Backup {
	if backup.InstanceID == "" {
		return backup
	}
	server, err := h.store.GetServer(ctx, backup.InstanceID)
	if err != nil {
		return backup
	}
	backup.GameKey = server.GameKey
	backup.ProviderKey = server.ProviderKey
	return backup
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
		h.logger.Warn("backup file missing during download, pruning orphaned record", "backupId", item.ID, "path", path)
		_ = h.store.DeleteBackup(r.Context(), item.ID)
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
	if isServerLockedForMutation(server.Status) {
		writeError(w, http.StatusConflict, "stop the server before restoring a backup")
		return
	}
	missing, err := h.pruneMissingBackupSource(r.Context(), item)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if missing {
		writeError(w, http.StatusNotFound, "backup file not found on disk")
		return
	}
	if err := backupsvc.NewService(h.cfg.DataDir).Restore(item.InstanceID, item.FileName, server.DataDir); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.syncRestoredServerConfig(r.Context(), &server); err != nil {
		writeError(w, statusCodeForRuntimeError(err), err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "backup.restored", fmt.Sprintf("Restored backup %s for %s", item.FileName, server.Name))
	writeJSON(w, http.StatusOK, map[string]string{"status": "restored", "backupId": item.ID})
}

func (h *Handler) saveDisplayName(providerKey domain.ProviderKey) string {
	gameProvider, ok := h.provider.Get(providerKey)
	if !ok {
		return "save"
	}
	if saveProvider, ok := gameProvider.(provider.SaveMetadataProvider); ok {
		return saveProvider.SaveDisplayName()
	}
	return "save"
}

func (h *Handler) listServerSaves(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	backups, err := h.store.ListBackupsByInstance(r.Context(), server.ID)
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
			_ = h.store.DeleteBackup(r.Context(), b.ID)
			continue
		}
		visible = append(visible, h.hydrateBackupResource(r.Context(), b))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"saveDisplayName": h.saveDisplayName(server.ProviderKey),
		"saves":           visible,
	})
}

func (h *Handler) createServerSaveSnapshot(w http.ResponseWriter, r *http.Request) {
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
	saveName := h.saveDisplayName(server.ProviderKey)
	h.recordActivity(r.Context(), server.ID, "save.snapshot.created", fmt.Sprintf("Created %s snapshot %s for %s", saveName, item.FileName, server.Name))
	writeJSON(w, http.StatusCreated, map[string]any{
		"saveDisplayName": saveName,
		"save":            h.hydrateBackupResource(r.Context(), item),
	})
}

func (h *Handler) downloadServerSave(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	saveID := chi.URLParam(r, "saveId")
	item, err := h.store.GetBackup(r.Context(), saveID)
	if err != nil {
		writeError(w, http.StatusNotFound, "save snapshot not found")
		return
	}
	if item.InstanceID != instanceID {
		writeError(w, http.StatusNotFound, "save snapshot not found")
		return
	}
	path, err := backupsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := os.Stat(path); err != nil {
		_ = h.store.DeleteBackup(r.Context(), item.ID)
		writeError(w, http.StatusNotFound, "save snapshot file not found on disk")
		return
	}
	http.ServeFile(w, r, path)
}

func (h *Handler) restoreServerSave(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	saveID := chi.URLParam(r, "saveId")
	item, err := h.store.GetBackup(r.Context(), saveID)
	if err != nil {
		writeError(w, http.StatusNotFound, "save snapshot not found")
		return
	}
	if item.InstanceID != instanceID {
		writeError(w, http.StatusNotFound, "save snapshot not found")
		return
	}
	server, err := h.store.GetServer(r.Context(), instanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if isServerLockedForMutation(server.Status) {
		writeError(w, http.StatusConflict, "stop the server before restoring a save snapshot")
		return
	}
	missing, err := h.pruneMissingBackupSource(r.Context(), item)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if missing {
		writeError(w, http.StatusNotFound, "save snapshot file not found on disk")
		return
	}
	if err := backupsvc.NewService(h.cfg.DataDir).Restore(item.InstanceID, item.FileName, server.DataDir); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.syncRestoredServerConfig(r.Context(), &server); err != nil {
		writeError(w, statusCodeForRuntimeError(err), err.Error())
		return
	}
	saveName := h.saveDisplayName(server.ProviderKey)
	h.recordActivity(r.Context(), server.ID, "save.snapshot.restored", fmt.Sprintf("Restored %s snapshot %s for %s", saveName, item.FileName, server.Name))
	writeJSON(w, http.StatusOK, map[string]string{"status": "restored", "saveId": item.ID})
}

func (h *Handler) requirePlayerCapability(server domain.GameServerInstance, capabilityCheck func(domain.ProviderCapabilities) bool, action string) (provider.PlayerCommandProvider, error) {
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
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
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
	if server.Status != domain.StatusRunning {
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

func (h *Handler) recentServerLogLines(ctx context.Context, server domain.GameServerInstance) ([]string, error) {
	snapshotServer := server
	snapshotServer.Status = domain.StatusStopped
	stream, err := h.runtime.Logs(ctx, snapshotServer)
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
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
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
	if server.Status != domain.StatusRunning {
		writeError(w, http.StatusConflict, "server must be running to kick players")
		return
	}
	server, _, err = h.ensureRuntimeContainer(r.Context(), server)
	if err != nil {
		writeError(w, statusCodeForRuntimeError(err), err.Error())
		return
	}
	command := commandProvider.KickCommand(player)
	if err := h.runtime.SendCommand(r.Context(), server, command); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "player.kicked", fmt.Sprintf("Kicked player %s from %s", player, server.Name))
	writeJSON(w, http.StatusOK, map[string]string{"status": "kicked", "player": player})
}

func (h *Handler) banServerPlayer(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
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
	if server.Status != domain.StatusRunning {
		writeError(w, http.StatusConflict, "server must be running to ban players")
		return
	}
	server, _, err = h.ensureRuntimeContainer(r.Context(), server)
	if err != nil {
		writeError(w, statusCodeForRuntimeError(err), err.Error())
		return
	}
	command := commandProvider.BanCommand(player)
	if err := h.runtime.SendCommand(r.Context(), server, command); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "player.banned", fmt.Sprintf("Banned player %s from %s", player, server.Name))
	writeJSON(w, http.StatusOK, map[string]string{"status": "banned", "player": player})
}

func (h *Handler) getServerWhitelist(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	gameProvider, ok := h.provider.Get(server.ProviderKey)
	if !ok || !gameProvider.Capabilities().Whitelist {
		writeJSON(w, http.StatusOK, map[string]any{"supported": false, "running": server.Status == domain.StatusRunning})
		return
	}
	_, ok = gameProvider.(provider.WhitelistCommandProvider)
	writeJSON(w, http.StatusOK, map[string]any{"supported": ok, "running": server.Status == domain.StatusRunning})
}

func (h *Handler) addServerWhitelistPlayer(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
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
	if server.Status != domain.StatusRunning {
		writeError(w, http.StatusConflict, "server must be running to edit the whitelist")
		return
	}
	server, _, err = h.ensureRuntimeContainer(r.Context(), server)
	if err != nil {
		writeError(w, statusCodeForRuntimeError(err), err.Error())
		return
	}
	if err := h.runtime.SendCommand(r.Context(), server, commandProvider.WhitelistAddCommand(player)); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "player.whitelisted", fmt.Sprintf("Added player %s to %s whitelist", player, server.Name))
	writeJSON(w, http.StatusOK, map[string]string{"status": "added", "player": player})
}

func (h *Handler) removeServerWhitelistPlayer(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
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
	if server.Status != domain.StatusRunning {
		writeError(w, http.StatusConflict, "server must be running to edit the whitelist")
		return
	}
	server, _, err = h.ensureRuntimeContainer(r.Context(), server)
	if err != nil {
		writeError(w, statusCodeForRuntimeError(err), err.Error())
		return
	}
	if err := h.runtime.SendCommand(r.Context(), server, commandProvider.WhitelistRemoveCommand(player)); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "player.whitelist.removed", fmt.Sprintf("Removed player %s from %s whitelist", player, server.Name))
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed", "player": player})
}

func (h *Handler) requireWhitelistCapability(server domain.GameServerInstance) (provider.WhitelistCommandProvider, error) {
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

func (h *Handler) syncRestoredServerConfig(ctx context.Context, server *domain.GameServerInstance) error {
	configBytes, err := os.ReadFile(filepath.Join(server.DataDir, "serverconfig.txt"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	nextConfig, err := terraria.ParseServerConfig(server.Config, string(configBytes))
	if err != nil {
		return err
	}
	nextConfig = normalizeProviderRuntimeConfig(server.ProviderKey, nextConfig)
	configPayloadJSON, err := providerConfigPayloadJSON(server.ProviderKey, nextConfig)
	if err != nil {
		return err
	}
	server.WorldName = nextConfig.WorldName
	server.Port = nextConfig.Port
	server.MaxPlayers = nextConfig.MaxPlayers
	server.Password = nextConfig.Password
	server.Config = nextConfig
	server.ConfigPayloadJSON = configPayloadJSON
	hydrateServerConfigPayload(server)
	server.UpdatedAt = time.Now()
	if server.ContainerID != "" {
		if h.runtimeStatusAvailable() {
			if _, err := h.runtime.Inspect(ctx, *server); err == nil {
				if err := h.runtime.Remove(ctx, *server); err != nil {
					return err
				}
			}
		}
		server.ContainerID = ""
	}
	return h.store.SaveServer(ctx, server)
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

func (h *Handler) pruneMissingBackupSource(ctx context.Context, item domain.Backup) (bool, error) {
	path, err := backupsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			h.logger.Warn("backup file missing during mutation, pruning orphaned record", "backupId", item.ID, "path", path)
			return true, h.store.DeleteBackup(ctx, item.ID)
		}
		return false, err
	}
	return false, nil
}

func (h *Handler) deleteBackup(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.GetBackup(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	path, _ := backupsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
	if err := removeStoredFile(path); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.store.DeleteBackup(r.Context(), item.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), item.InstanceID, "backup.deleted", fmt.Sprintf("Deleted backup %s", item.FileName))
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func removeStoredFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (h *Handler) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
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

type prepareRuntimeImageRequest struct {
	ProviderKey domain.ProviderKey `json:"providerKey"`
	Version     string             `json:"version,omitempty"`
}

type runtimeInstallRef struct {
	ProviderKey domain.ProviderKey `json:"providerKey"`
	Version     string             `json:"version"`
	Image       string             `json:"image"`
}

type runtimeInstallMarker struct {
	ProviderKey domain.ProviderKey `json:"providerKey"`
	Version     string             `json:"version"`
	Image       string             `json:"image"`
	InstalledAt time.Time          `json:"installedAt"`
}

func (h *Handler) prepareRuntimeImage(w http.ResponseWriter, r *http.Request) {
	var payload prepareRuntimeImageRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	gameProvider, ok := h.provider.Get(payload.ProviderKey)
	if !ok {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}
	if err := h.requireProviderRuntimeSupported(payload.ProviderKey); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err := h.requireRuntimeAvailable(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	version := normalizeStoredProviderVersion(gameProvider, payload.Version)
	image := gameProvider.ImageFor(version)
	ref := runtimeInstallRef{ProviderKey: payload.ProviderKey, Version: version, Image: image}
	if status := h.runtimeInstallStatus(r.Context(), ref); status.Status == runtime.ImageStatusReady {
		writeJSON(w, http.StatusOK, status)
		return
	} else if status.Status == runtime.ImageStatusPreparing {
		writeJSON(w, http.StatusAccepted, status)
		return
	}
	status := domain.RuntimeImageStatus{Image: image, Status: runtime.ImageStatusPreparing, UpdatedAt: time.Now()}
	h.setRuntimeImageJob(status)
	go h.prepareRuntimeImageAsync(ref)
	writeJSON(w, http.StatusAccepted, status)
}

func (h *Handler) prepareRuntimeImageAsync(ref runtimeInstallRef) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	lastProgress := 0
	status := domain.RuntimeImageStatus{Image: ref.Image, Progress: lastProgress, UpdatedAt: time.Now()}
	if err := h.runtime.PrepareImageWithProgress(ctx, ref.Image, func(progress runtime.ImagePrepareProgress) {
		nextProgress := clampRuntimeImageProgress(progress.Progress)
		if nextProgress < lastProgress {
			nextProgress = lastProgress
		}
		lastProgress = nextProgress
		h.setRuntimeImageJob(domain.RuntimeImageStatus{
			Image:     ref.Image,
			Status:    runtime.ImageStatusPreparing,
			Message:   progress.Message,
			Progress:  nextProgress,
			UpdatedAt: time.Now(),
		})
	}); err != nil {
		status.Status = runtime.ImageStatusFailed
		status.Message = err.Error()
		status.Progress = lastProgress
		if h.logger != nil {
			h.logger.Warn("runtime image prepare failed", "image", ref.Image, "provider", ref.ProviderKey, "version", ref.Version, "error", err)
		}
	} else {
		if err := h.writeRuntimeInstallMarker(ref); err != nil {
			status.Status = runtime.ImageStatusFailed
			status.Message = err.Error()
			status.Progress = lastProgress
			if h.logger != nil {
				h.logger.Warn("runtime install marker write failed", "image", ref.Image, "provider", ref.ProviderKey, "version", ref.Version, "error", err)
			}
		} else {
			status.Status = runtime.ImageStatusReady
			status.Progress = 100
			if h.logger != nil {
				h.logger.Info("runtime image prepared", "image", ref.Image, "provider", ref.ProviderKey, "version", ref.Version)
			}
		}
	}
	status.UpdatedAt = time.Now()
	h.setRuntimeImageJob(status)
}

func clampRuntimeImageProgress(progress int) int {
	if progress < 0 {
		return 0
	}
	if progress > 100 {
		return 100
	}
	return progress
}

func (h *Handler) runtimeInstallStatus(ctx context.Context, ref runtimeInstallRef) domain.RuntimeImageStatus {
	if job, ok := h.getRuntimeImageJob(ref.Image); ok && job.Status == runtime.ImageStatusPreparing {
		return job
	}
	imageStatus := h.runtime.ImageStatus(ctx, ref.Image)
	if imageStatus.Status == runtime.ImageStatusReady {
		if markerStatus, ok := h.runtimeInstallMarkerStatus(ref); ok {
			if markerStatus.Status == runtime.ImageStatusReady {
				imageStatus.UpdatedAt = markerStatus.UpdatedAt
			}
		} else if err := h.writeRuntimeInstallMarker(ref); err != nil {
			imageStatus.Message = err.Error()
		}
		return imageStatus
	}
	if job, ok := h.getRuntimeImageJob(ref.Image); ok && job.Status == runtime.ImageStatusFailed {
		return job
	}
	return imageStatus
}

func (h *Handler) runtimeInstallMarkerStatus(ref runtimeInstallRef) (domain.RuntimeImageStatus, bool) {
	path := h.runtimeInstallMarkerPath(ref)
	stat, err := os.Stat(path)
	if err == nil {
		return domain.RuntimeImageStatus{Image: ref.Image, Status: runtime.ImageStatusReady, UpdatedAt: stat.ModTime()}, true
	}
	if errors.Is(err, os.ErrNotExist) {
		return domain.RuntimeImageStatus{}, false
	}
	return domain.RuntimeImageStatus{Image: ref.Image, Status: runtime.ImageStatusFailed, Message: err.Error(), UpdatedAt: time.Now()}, true
}

func (h *Handler) writeRuntimeInstallMarker(ref runtimeInstallRef) error {
	if ref.ProviderKey == "" || strings.TrimSpace(ref.Version) == "" || strings.TrimSpace(ref.Image) == "" {
		return fmt.Errorf("runtime install marker is missing provider, version, or image")
	}
	path := h.runtimeInstallMarkerPath(ref)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	marker := runtimeInstallMarker{
		ProviderKey: ref.ProviderKey,
		Version:     ref.Version,
		Image:       ref.Image,
		InstalledAt: time.Now(),
	}
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (h *Handler) runtimeInstallMarkerPath(ref runtimeInstallRef) string {
	return filepath.Join(h.cfg.DataDir, "runtime-installs", safeRuntimeInstallPathPart(string(ref.ProviderKey)), safeRuntimeInstallPathPart(ref.Version)+".json")
}

func safeRuntimeInstallPathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "_"
	}
	var builder strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
			builder.WriteRune(char)
		case char >= 'A' && char <= 'Z':
			builder.WriteRune(char)
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
		case char == '.', char == '-', char == '_':
			builder.WriteRune(char)
		default:
			builder.WriteByte('_')
		}
	}
	return builder.String()
}

func (h *Handler) getRuntimeImageJob(image string) (domain.RuntimeImageStatus, bool) {
	h.runtimeImageJobsMu.Lock()
	defer h.runtimeImageJobsMu.Unlock()
	status, ok := h.runtimeImageJobs[image]
	return status, ok
}

func (h *Handler) setRuntimeImageJob(status domain.RuntimeImageStatus) {
	if status.Image == "" {
		return
	}
	h.runtimeImageJobsMu.Lock()
	defer h.runtimeImageJobsMu.Unlock()
	h.runtimeImageJobs[status.Image] = status
}

func (h *Handler) workshopSyncUnsupported() bool {
	architecture := strings.ToLower(strings.TrimSpace(h.dockerMonitor.Status().Architecture))
	return strings.HasPrefix(architecture, "arm") || strings.Contains(architecture, "aarch64")
}

func (h *Handler) providerRuntimeUnsupported(providerKey domain.ProviderKey) bool {
	if providerKey != domain.ProviderDST || h.dockerMonitor == nil {
		return false
	}
	architecture := strings.ToLower(strings.TrimSpace(h.dockerMonitor.Status().Architecture))
	return strings.HasPrefix(architecture, "arm") || strings.Contains(architecture, "aarch64")
}

func (h *Handler) requireProviderRuntimeSupported(providerKey domain.ProviderKey) error {
	if h.providerRuntimeUnsupported(providerKey) {
		return fmt.Errorf("Don't Starve Together server runtime is currently supported only on amd64 Docker hosts")
	}
	return nil
}

func (h *Handler) runtimeStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.runtime.HostStats(r.Context())
	if err != nil {
		stats = runtime.HostStats{}
	}
	if used, usageErr := dataDirUsageBytes(h.cfg.DataDir); usageErr == nil {
		stats.StorageUsedBytes = used
	}
	writeJSON(w, http.StatusOK, stats)
}

func dataDirUsageBytes(root string) (int64, error) {
	if strings.TrimSpace(root) == "" {
		return 0, nil
	}
	var total int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total, err
}

func (h *Handler) observabilityMetrics(w http.ResponseWriter, r *http.Request) {
	snapshot, err := observability.NewService(h.store, h.runtime).Snapshot(r.Context(), h.runtimeStatusAvailable())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (h *Handler) prometheusMetrics(w http.ResponseWriter, r *http.Request) {
	body, err := observability.NewService(h.store, h.runtime).PrometheusText(r.Context(), h.runtimeStatusAvailable())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.apiMetrics != nil {
		body += "\n" + h.apiMetrics.PrometheusText()
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"host":       h.cfg.Host,
		"port":       h.cfg.Port,
		"dataDir":    h.cfg.DataDir,
		"dbPath":     h.cfg.DBPath,
		"dockerHost": h.cfg.DockerHost,
		"publicHost": h.resolvePublicHost(),
	})
}

func (h *Handler) updatePublicHost(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		PublicHost string `json:"publicHost"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	host := strings.TrimSpace(payload.PublicHost)
	if host != "" {
		if strings.ContainsAny(host, " \t\n\r/") || len(host) > 253 {
			writeError(w, http.StatusBadRequest, "public host must be a valid hostname or IP")
			return
		}
	}
	if host == "" {
		host = ""
	}
	if err := h.store.SetSetting(r.Context(), "publicHost", host); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), "", "settings.publicHost", fmt.Sprintf("Updated public host to %q", host))
	writeJSON(w, http.StatusOK, map[string]string{"publicHost": h.resolvePublicHost()})
}

func (h *Handler) getServerJoinInfo(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	hydrateServerConfigPayload(&server)
	h.attachServerJoinInfo(&server)
	writeJSON(w, http.StatusOK, server.JoinInfo)
}

type serverShareResponse struct {
	Enabled         bool      `json:"enabled"`
	Token           string    `json:"token,omitempty"`
	SharePath       string    `json:"sharePath,omitempty"`
	IncludePassword bool      `json:"includePassword"`
	CreatedAt       time.Time `json:"createdAt,omitempty"`
	UpdatedAt       time.Time `json:"updatedAt,omitempty"`
}

type publicServerShareResponse struct {
	Name        string                `json:"name"`
	GameKey     domain.GameKey        `json:"gameKey"`
	ProviderKey domain.ProviderKey    `json:"providerKey"`
	Status      domain.ServerStatus   `json:"status"`
	Players     int                   `json:"players"`
	MaxPlayers  int                   `json:"maxPlayers"`
	JoinInfo    domain.ServerJoinInfo `json:"joinInfo"`
}

func (h *Handler) getServerShare(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	share, err := h.store.GetServerShareByInstance(r.Context(), server.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusOK, serverShareResponse{Enabled: false})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, shareResponse(share))
}

func (h *Handler) enableServerShare(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	var payload struct {
		IncludePassword bool `json:"includePassword"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	now := time.Now()
	share, err := h.store.GetServerShareByInstance(r.Context(), server.ID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		share = domain.ServerShare{
			Token:      strings.ReplaceAll(uuid.NewString(), "-", ""),
			InstanceID: server.ID,
			CreatedAt:  now,
		}
	}
	share.IncludePassword = payload.IncludePassword
	share.UpdatedAt = now
	if err := h.store.SaveServerShare(r.Context(), &share); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.share.enabled", fmt.Sprintf("Enabled share page for %s", server.Name))
	writeJSON(w, http.StatusOK, shareResponse(share))
}

func (h *Handler) disableServerShare(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if err := h.store.DeleteServerShareByInstance(r.Context(), server.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.share.disabled", fmt.Sprintf("Disabled share page for %s", server.Name))
	writeJSON(w, http.StatusOK, serverShareResponse{Enabled: false})
}

func (h *Handler) getPublicServerShare(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(chi.URLParam(r, "token"))
	if token == "" {
		writeError(w, http.StatusNotFound, "share page not found")
		return
	}
	share, err := h.store.GetServerShareByToken(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusNotFound, "share page not found")
		return
	}
	server, err := h.store.GetServer(r.Context(), share.InstanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "share page not found")
		return
	}
	server = h.refreshServerStatus(r.Context(), server)
	h.attachServerJoinInfo(&server)
	if !share.IncludePassword {
		server.JoinInfo.Password = ""
		server.JoinInfo.InviteText = stripInvitePassword(server.JoinInfo.InviteText)
	}
	writeJSON(w, http.StatusOK, publicServerShareResponse{
		Name:        server.Name,
		GameKey:     server.GameKey,
		ProviderKey: server.ProviderKey,
		Status:      server.Status,
		Players:     server.PlayersOnline,
		MaxPlayers:  server.MaxPlayers,
		JoinInfo:    server.JoinInfo,
	})
}

func shareResponse(share domain.ServerShare) serverShareResponse {
	return serverShareResponse{
		Enabled:         true,
		Token:           share.Token,
		SharePath:       "/share/" + share.Token,
		IncludePassword: share.IncludePassword,
		CreatedAt:       share.CreatedAt,
		UpdatedAt:       share.UpdatedAt,
	}
}

func stripInvitePassword(invite string) string {
	if invite == "" {
		return invite
	}
	index := strings.Index(strings.ToLower(invite), " password:")
	if index < 0 {
		return invite
	}
	return strings.TrimSpace(invite[:index])
}

func (h *Handler) listGames(w http.ResponseWriter, r *http.Request) {
	games := h.provider.Games()
	h.applyRuntimeGameAvailability(games)
	h.attachRuntimeImageStatuses(r.Context(), games)
	servers, err := h.store.ListServers(r.Context())
	if err == nil {
		counts := map[domain.GameKey]int{}
		for _, server := range servers {
			counts[server.GameKey]++
		}
		for index := range games {
			games[index].ServerCount = counts[games[index].Key]
		}
	}
	writeJSON(w, http.StatusOK, games)
}

func (h *Handler) getGame(w http.ResponseWriter, r *http.Request) {
	game, ok := h.provider.Game(domain.GameKey(chi.URLParam(r, "gameKey")))
	if !ok {
		writeError(w, http.StatusNotFound, "game not found")
		return
	}
	games := []domain.GameCatalogEntry{game}
	h.applyRuntimeGameAvailability(games)
	h.attachRuntimeImageStatuses(r.Context(), games)
	writeJSON(w, http.StatusOK, games[0])
}

func (h *Handler) applyRuntimeGameAvailability(games []domain.GameCatalogEntry) {
	if !h.providerRuntimeUnsupported(domain.ProviderDST) {
		return
	}
	for index := range games {
		if games[index].Key == domain.GameDST {
			games[index].Status = "unsupported"
		}
	}
}

func (h *Handler) attachRuntimeImageStatuses(ctx context.Context, games []domain.GameCatalogEntry) {
	for gameIndex := range games {
		for providerIndex := range games[gameIndex].Providers {
			providerCatalog := &games[gameIndex].Providers[providerIndex]
			gameProvider, ok := h.provider.Get(providerCatalog.Key)
			if !ok {
				continue
			}
			version := providerCatalog.RecommendedVersion
			if version == "" {
				version = normalizeStoredProviderVersion(gameProvider, "")
			}
			image := gameProvider.ImageFor(version)
			if h.providerRuntimeUnsupported(providerCatalog.Key) {
				providerCatalog.RuntimeImage = domain.RuntimeImageStatus{
					Image:     image,
					Status:    runtime.ImageStatusUnsupported,
					Message:   "server runtime is not supported on this Docker architecture",
					UpdatedAt: time.Now(),
				}
				continue
			}
			providerCatalog.RuntimeImage = h.runtimeInstallStatus(ctx, runtimeInstallRef{ProviderKey: providerCatalog.Key, Version: version, Image: image})
		}
	}
}

func (h *Handler) gameVersions(w http.ResponseWriter, r *http.Request) {
	game, ok := h.provider.Game(domain.GameKey(chi.URLParam(r, "gameKey")))
	if !ok {
		writeError(w, http.StatusNotFound, "game not found")
		return
	}
	versions := map[domain.ProviderKey][]string{}
	for _, item := range game.Providers {
		versions[item.Key] = item.Versions
	}
	writeJSON(w, http.StatusOK, versions)
}

func (h *Handler) listServers(w http.ResponseWriter, r *http.Request) {
	servers, err := h.store.ListServers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for index := range servers {
		servers[index] = h.refreshServerStatus(r.Context(), servers[index])
		h.attachServerJoinInfo(&servers[index])
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
	h.attachServerJoinInfo(&server)
	writeJSON(w, http.StatusOK, server)
}

func (h *Handler) updateServerConfig(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if isLifecyclePending(server.Status) {
		writeError(w, http.StatusConflict, "server lifecycle action already in progress")
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
	nextConfig, configPayloadJSON, err := decodeProviderRuntimeConfig(server.ProviderKey, payload.Config, server.Config)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.applyServerConfig(r.Context(), &server, nextConfig, configPayloadJSON, payload.HostPort, payload.Resources); err != nil {
		writeError(w, statusCodeForRuntimeError(err), err.Error())
		return
	}
	if err := h.store.SaveServer(r.Context(), &server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.config.updated", fmt.Sprintf("Updated config for %s", server.Name))
	h.attachServerJoinInfo(&server)
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
	config, configPayloadJSON, err := decodeProviderRuntimeConfig(payload.ProviderKey, payload.Config, gameProvider.DefaultConfig())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if payload.Name == "" {
		payload.Name = config.ServerName
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
	imageStatus := h.runtimeInstallStatus(r.Context(), runtimeInstallRef{
		ProviderKey: payload.ProviderKey,
		Version:     payload.Version,
		Image:       gameProvider.ImageFor(payload.Version),
	})
	if imageStatus.Status != runtime.ImageStatusReady {
		writeError(w, http.StatusConflict, "server runtime is not installed; install it from Game Library first")
		return
	}
	if err := gameProvider.ValidateConfig(config); err != nil {
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
	server := domain.GameServerInstance{
		ID: id, Name: payload.Name, GameKey: gameProvider.GameKey(), ProviderKey: payload.ProviderKey,
		Status: domain.StatusStopped, WorldName: config.WorldName, Port: config.Port,
		MaxPlayers: config.MaxPlayers, Password: config.Password, DataDir: dataDir, HostPort: hostPort,
		CPULimitCores: resources.CPULimitCores, MemoryLimitMB: resources.MemoryLimitMB,
		Config: config, ConfigPayloadJSON: configPayloadJSON, Version: payload.Version, ConfigRevision: 1, AppliedConfigRevision: 1,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	hydrateServerConfigPayload(&server)
	if err := h.store.CreateServer(r.Context(), &server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.created", fmt.Sprintf("Created server %s", server.Name))
	h.attachServerJoinInfo(&server)
	writeJSON(w, http.StatusCreated, server)
}

type configPresetPayload struct {
	Name        string               `json:"name"`
	ProviderKey domain.ProviderKey   `json:"providerKey"`
	Config      json.RawMessage      `json:"config"`
	Version     string               `json:"version"`
	Resources   resourceLimitPayload `json:"resources,omitempty"`
	ModPackID   string               `json:"modPackId,omitempty"`
}

func (h *Handler) listConfigPresets(w http.ResponseWriter, r *http.Request) {
	presets, err := h.store.ListConfigPresets(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, presets)
}

func (h *Handler) getConfigPreset(w http.ResponseWriter, r *http.Request) {
	preset, err := h.store.GetConfigPreset(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "config preset not found")
		return
	}
	writeJSON(w, http.StatusOK, preset)
}

func (h *Handler) createConfigPreset(w http.ResponseWriter, r *http.Request) {
	preset, err := h.buildConfigPreset(r, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	preset.ID = uuid.NewString()
	preset.CreatedAt = time.Now()
	preset.UpdatedAt = preset.CreatedAt
	if err := h.store.CreateConfigPreset(r.Context(), &preset); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	hydratePresetConfigPayload(&preset)
	writeJSON(w, http.StatusCreated, preset)
}

func (h *Handler) updateConfigPreset(w http.ResponseWriter, r *http.Request) {
	existing, err := h.store.GetConfigPreset(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "config preset not found")
		return
	}
	preset, err := h.buildConfigPreset(r, existing.ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	preset.CreatedAt = existing.CreatedAt
	preset.UpdatedAt = time.Now()
	if err := h.store.SaveConfigPreset(r.Context(), &preset); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	hydratePresetConfigPayload(&preset)
	writeJSON(w, http.StatusOK, preset)
}

func (h *Handler) deleteConfigPreset(w http.ResponseWriter, r *http.Request) {
	if _, err := h.store.GetConfigPreset(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusNotFound, "config preset not found")
		return
	}
	if err := h.store.DeleteConfigPreset(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) buildConfigPreset(r *http.Request, id string) (domain.ConfigPreset, error) {
	var payload configPresetPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return domain.ConfigPreset{}, fmt.Errorf("invalid JSON body")
	}
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.Name == "" {
		return domain.ConfigPreset{}, fmt.Errorf("preset name is required")
	}
	gameProvider, ok := h.provider.Get(payload.ProviderKey)
	if !ok {
		return domain.ConfigPreset{}, fmt.Errorf("unknown provider")
	}
	config, configPayloadJSON, err := decodeProviderRuntimeConfig(payload.ProviderKey, payload.Config, gameProvider.DefaultConfig())
	if err != nil {
		return domain.ConfigPreset{}, err
	}
	if payload.Version == "" {
		payload.Version = gameProvider.Versions()[0]
	}
	if !providerSupportsVersion(gameProvider, payload.Version) {
		return domain.ConfigPreset{}, fmt.Errorf("unsupported provider version")
	}
	if err := gameProvider.ValidateConfig(config); err != nil {
		return domain.ConfigPreset{}, err
	}
	resources, err := normalizeResourceLimits(payload.Resources)
	if err != nil {
		return domain.ConfigPreset{}, err
	}
	if payload.ModPackID != "" {
		if _, err := h.store.GetModPack(r.Context(), payload.ModPackID); err != nil {
			return domain.ConfigPreset{}, fmt.Errorf("mod pack not found")
		}
	}
	config = sanitizePresetConfig(gameProvider.Key(), config)
	configPayloadJSON, err = sanitizePresetConfigPayload(gameProvider, configPayloadJSON)
	if err != nil {
		return domain.ConfigPreset{}, err
	}
	return domain.ConfigPreset{
		ID: id, Name: payload.Name, GameKey: gameProvider.GameKey(), ProviderKey: payload.ProviderKey,
		Version: payload.Version, Config: config, ConfigPayloadJSON: configPayloadJSON,
		CPULimitCores: resources.CPULimitCores, MemoryLimitMB: resources.MemoryLimitMB, ModPackID: payload.ModPackID,
	}, nil
}

func sanitizePresetConfig(providerKey domain.ProviderKey, config domain.TerrariaConfig) domain.TerrariaConfig {
	config.Password = ""
	switch providerKey {
	case domain.ProviderPalworld, domain.ProviderDST:
		config.MOTD = ""
	}
	return config
}

func (h *Handler) startServer(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if isLifecyclePending(server.Status) {
		writeError(w, http.StatusConflict, "server lifecycle action already in progress")
		return
	}
	server.Status = domain.StatusStarting
	server.LastError = ""
	server.UpdatedAt = time.Now()
	if err := h.store.SaveServer(r.Context(), &server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.start.queued", fmt.Sprintf("Queued start for server %s", server.Name))
	go h.runServerLifecycle(server.ID, h.runStartServer)
	h.attachServerJoinInfo(&server)
	writeJSON(w, http.StatusAccepted, server)
}

func (h *Handler) stopServer(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if isLifecyclePending(server.Status) {
		writeError(w, http.StatusConflict, "server lifecycle action already in progress")
		return
	}
	if server.ContainerID != "" {
		if err := h.requireRuntimeAvailable(r.Context()); err != nil {
			writeError(w, statusCodeForRuntimeError(err), err.Error())
			return
		}
	}
	server.Status = domain.StatusStopping
	server.LastError = ""
	server.UpdatedAt = time.Now()
	if err := h.store.SaveServer(r.Context(), &server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.stop.queued", fmt.Sprintf("Queued stop for server %s", server.Name))
	go h.runServerLifecycle(server.ID, h.runStopServer)
	h.attachServerJoinInfo(&server)
	writeJSON(w, http.StatusAccepted, server)
}

func (h *Handler) restartServer(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if isLifecyclePending(server.Status) {
		writeError(w, http.StatusConflict, "server lifecycle action already in progress")
		return
	}
	server.Status = domain.StatusRestarting
	server.LastError = ""
	server.UpdatedAt = time.Now()
	if err := h.store.SaveServer(r.Context(), &server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.restart.queued", fmt.Sprintf("Queued restart for server %s", server.Name))
	go h.runServerLifecycle(server.ID, h.runRestartServer)
	h.attachServerJoinInfo(&server)
	writeJSON(w, http.StatusAccepted, server)
}

func (h *Handler) runStartServer(ctx context.Context, id string) {
	server, err := h.store.GetServer(ctx, id)
	if err != nil {
		h.markServerLifecycleFailed(ctx, id, "server.start.failed", errors.New("server not found"))
		return
	}
	h.recordActivity(ctx, server.ID, "server.start.container.prepare", fmt.Sprintf("Preparing runtime container for server %s", server.Name))
	server, recreated, err := h.ensureRuntimeContainer(ctx, server)
	if err != nil {
		h.markServerLifecycleFailed(ctx, id, "server.start.failed", err)
		return
	}
	if recreated {
		h.recordActivity(ctx, server.ID, "server.start.container.created", fmt.Sprintf("Created runtime container for server %s", server.Name))
	} else {
		h.recordActivity(ctx, server.ID, "server.start.container.ready", fmt.Sprintf("Runtime container ready for server %s", server.Name))
	}
	h.recordActivity(ctx, server.ID, "server.start.runtime.starting", fmt.Sprintf("Starting runtime container for server %s", server.Name))
	if err := h.runtime.Start(ctx, server); err != nil {
		h.markServerLifecycleFailed(ctx, id, "server.start.failed", err)
		return
	}
	server.Status = domain.StatusRunning
	server.LastError = ""
	server.AppliedConfigRevision = server.ConfigRevision
	server.UpdatedAt = time.Now()
	if err := h.store.SaveServer(ctx, &server); err != nil {
		h.logger.Warn("failed to persist async server start", "server", id, "error", err)
		return
	}
	h.recordActivity(ctx, server.ID, "server.started", fmt.Sprintf("Started server %s", server.Name))
}

func (h *Handler) runServerLifecycle(id string, run func(context.Context, string)) {
	ctx, cancel := context.WithTimeout(context.Background(), serverLifecycleTimeout)
	defer cancel()
	run(ctx, id)
}

func (h *Handler) runRestartServer(ctx context.Context, id string) {
	server, err := h.store.GetServer(ctx, id)
	if err != nil {
		h.markServerLifecycleFailed(ctx, id, "server.restart.failed", errors.New("server not found"))
		return
	}
	server, err = h.recreateRuntimeOnNextStart(ctx, server)
	if err != nil {
		h.markServerLifecycleFailed(ctx, id, "server.restart.failed", err)
		return
	}
	h.recordActivity(ctx, server.ID, "server.restart.container.prepare", fmt.Sprintf("Preparing runtime container for server %s", server.Name))
	server, recreated, err := h.ensureRuntimeContainer(ctx, server)
	if err != nil {
		h.markServerLifecycleFailed(ctx, id, "server.restart.failed", err)
		return
	}
	if recreated {
		h.recordActivity(ctx, server.ID, "server.restart.container.created", fmt.Sprintf("Created runtime container for server %s", server.Name))
	} else {
		h.recordActivity(ctx, server.ID, "server.restart.container.ready", fmt.Sprintf("Runtime container ready for server %s", server.Name))
	}
	h.recordActivity(ctx, server.ID, "server.restart.runtime.starting", fmt.Sprintf("Starting runtime container for server %s", server.Name))
	if err := h.runtime.Start(ctx, server); err != nil {
		h.markServerLifecycleFailed(ctx, id, "server.restart.failed", err)
		return
	}
	server.Status = domain.StatusRunning
	server.LastError = ""
	server.AppliedConfigRevision = server.ConfigRevision
	server.UpdatedAt = time.Now()
	if err := h.store.SaveServer(ctx, &server); err != nil {
		h.logger.Warn("failed to persist async server restart", "server", id, "error", err)
		return
	}
	h.recordActivity(ctx, server.ID, "server.restarted", fmt.Sprintf("Restarted server %s", server.Name))
}

func (h *Handler) recreateRuntimeOnNextStart(ctx context.Context, server domain.GameServerInstance) (domain.GameServerInstance, error) {
	if server.ContainerID == "" {
		return server, nil
	}
	if err := h.requireRuntimeAvailable(ctx); err != nil {
		return domain.GameServerInstance{}, err
	}
	if _, err := h.runtime.Inspect(ctx, server); err == nil {
		if err := h.runtime.Remove(ctx, server); err != nil {
			return domain.GameServerInstance{}, err
		}
	} else {
		h.logger.Warn("runtime container missing before recreate; clearing stale container", "server", server.ID, "container", server.ContainerID, "error", err)
	}
	server.ContainerID = ""
	server.UpdatedAt = time.Now()
	if err := h.store.SaveServer(ctx, &server); err != nil {
		return domain.GameServerInstance{}, err
	}
	return server, nil
}

func (h *Handler) runStopServer(ctx context.Context, id string) {
	server, err := h.store.GetServer(ctx, id)
	if err != nil {
		h.markServerLifecycleFailed(ctx, id, "server.stop.failed", errors.New("server not found"))
		return
	}
	if server.ContainerID != "" {
		if err := h.requireRuntimeAvailable(ctx); err != nil {
			h.markServerLifecycleFailed(ctx, id, "server.stop.failed", err)
			return
		}
		if _, err := h.runtime.Inspect(ctx, server); err != nil {
			h.logger.Warn("runtime container missing during async stop; clearing stale container", "server", server.ID, "container", server.ContainerID, "error", err)
			server.ContainerID = ""
		} else if err := h.runtime.Stop(ctx, server); err != nil {
			h.markServerLifecycleFailed(ctx, id, "server.stop.failed", err)
			return
		}
	}
	server.Status = domain.StatusStopped
	server.LastError = ""
	server.UpdatedAt = time.Now()
	if err := h.store.SaveServer(ctx, &server); err != nil {
		h.logger.Warn("failed to persist async server stop", "server", id, "error", err)
		return
	}
	h.recordActivity(ctx, server.ID, "server.stopped", fmt.Sprintf("Stopped server %s", server.Name))
}

func (h *Handler) markServerLifecycleFailed(ctx context.Context, id string, activityType string, cause error) {
	server, err := h.store.GetServer(ctx, id)
	if err != nil {
		h.logger.Warn("failed to load server after lifecycle failure", "server", id, "error", err, "cause", cause)
		return
	}
	server.Status = domain.StatusErrored
	server.LastError = cause.Error()
	server.UpdatedAt = time.Now()
	if err := h.store.SaveServer(ctx, &server); err != nil {
		h.logger.Warn("failed to persist lifecycle failure", "server", id, "error", err, "cause", cause)
		return
	}
	h.recordActivity(ctx, server.ID, activityType, fmt.Sprintf("%s: %v", server.Name, cause))
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
	if err := h.requireProviderRuntimeSupported(server.ProviderKey); err != nil {
		return domain.GameServerInstance{}, false, err
	}
	if server.ContainerID != "" {
		if _, err := h.runtime.Inspect(ctx, server); err == nil {
			return server, false, nil
		}
		h.logger.Warn("runtime container missing; recreating from server data", "server", server.ID, "container", server.ContainerID)
		server.ContainerID = ""
		if err := h.store.SaveServer(ctx, &server); err != nil {
			return domain.GameServerInstance{}, false, err
		}
	}
	spec, err := h.runtimeSpecForServer(ctx, &server)
	if err != nil {
		return domain.GameServerInstance{}, false, err
	}
	containerID, err := h.runtime.Create(ctx, spec)
	if err != nil {
		return domain.GameServerInstance{}, false, err
	}
	server.ContainerID = containerID
	server.AppliedConfigRevision = server.ConfigRevision
	server.UpdatedAt = time.Now()
	if err := h.store.SaveServer(ctx, &server); err != nil {
		return domain.GameServerInstance{}, false, err
	}
	return server, true, nil
}

func (h *Handler) runtimeSpecForServer(ctx context.Context, server *domain.GameServerInstance) (runtime.ContainerSpec, error) {
	gameProvider, ok := h.provider.Get(server.ProviderKey)
	if !ok {
		return runtime.ContainerSpec{}, fmt.Errorf("unknown provider: %s", server.ProviderKey)
	}
	configText, options, err := runtimeConfigForServer(gameProvider, *server)
	if err != nil {
		return runtime.ContainerSpec{}, err
	}
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		return runtime.ContainerSpec{}, err
	}
	server.Version = normalizeStoredProviderVersion(gameProvider, server.Version)
	if server.HostPort == 0 {
		server.HostPort, err = h.allocateHostPort(ctx, server.ID)
		if err != nil {
			return runtime.ContainerSpec{}, err
		}
	}
	spec := runtime.ContainerSpec{
		InstanceID: server.ID,
		Name:       server.Name,
		Image:      gameProvider.ImageFor(server.Version),
		Port:       server.Port,
		HostPort:   server.HostPort,
		Resources: runtime.ContainerResources{
			CPULimitCores: server.CPULimitCores,
			MemoryLimitMB: server.MemoryLimitMB,
		},
		DataDir:    server.DataDir,
		ConfigText: configText,
		Options:    options,
	}
	return spec, nil
}

func runtimeConfigForServer(gameProvider provider.GameProvider, server domain.GameServerInstance) (string, runtime.ContainerOptions, error) {
	if serverRuntimeProvider, ok := gameProvider.(provider.ServerRuntimeProvider); ok {
		configText, err := serverRuntimeProvider.RenderServerConfig(server)
		if err != nil {
			return "", runtime.ContainerOptions{}, err
		}
		options, err := serverRuntimeProvider.RuntimeOptionsForServer(server)
		if err != nil {
			return "", runtime.ContainerOptions{}, err
		}
		return configText, options, nil
	}
	configText, err := gameProvider.RenderConfig(server.Config)
	if err != nil {
		return "", runtime.ContainerOptions{}, err
	}
	return configText, gameProvider.RuntimeOptions(server.Config), nil
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

func (h *Handler) applyServerConfig(ctx context.Context, server *domain.GameServerInstance, nextConfig domain.TerrariaConfig, configPayloadJSON string, hostPort *int, resourceLimits *resourceLimitPayload) error {
	if isLifecyclePending(server.Status) {
		return fmt.Errorf("server lifecycle action already in progress")
	}
	gameProvider, ok := h.provider.Get(server.ProviderKey)
	if !ok {
		return fmt.Errorf("unknown provider: %s", server.ProviderKey)
	}
	if nextConfig.ServerName == "" {
		nextConfig.ServerName = server.Name
	}
	nextConfig = normalizeProviderRuntimeConfig(server.ProviderKey, nextConfig)
	if configPayloadJSON == "" {
		var err error
		configPayloadJSON, err = providerConfigPayloadJSON(server.ProviderKey, nextConfig)
		if err != nil {
			return err
		}
	}
	nextHostPort := server.HostPort
	if hostPort != nil {
		nextHostPort = *hostPort
	}
	nextCPU := server.CPULimitCores
	nextMemory := server.MemoryLimitMB
	var err error
	if resourceLimits != nil {
		nextResources, err := normalizeResourceLimits(*resourceLimits)
		if err != nil {
			return err
		}
		nextCPU = nextResources.CPULimitCores
		nextMemory = nextResources.MemoryLimitMB
	}
	nextHostPort, err = h.resolveHostPort(ctx, nextHostPort, server.ID)
	if err != nil {
		return err
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
	if err := os.WriteFile(filepath.Join(server.DataDir, "serverconfig.txt"), []byte(configText), 0o644); err != nil {
		return err
	}
	for name, content := range gameProvider.RuntimeOptions(nextConfig).Files {
		if err := writeInstanceDataFile(server.DataDir, name, content); err != nil {
			return err
		}
	}
	if server.Status != domain.StatusRunning && server.ContainerID != "" {
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
	server.HostPort = nextHostPort
	server.CPULimitCores = nextCPU
	server.MemoryLimitMB = nextMemory
	server.MaxPlayers = nextConfig.MaxPlayers
	server.Password = nextConfig.Password
	server.Config = nextConfig
	server.ConfigPayloadJSON = configPayloadJSON
	hydrateServerConfigPayload(server)
	server.ConfigRevision++
	if server.ConfigRevision <= 0 {
		server.ConfigRevision = 1
	}
	if server.Status != domain.StatusRunning {
		server.AppliedConfigRevision = server.ConfigRevision
	}
	server.UpdatedAt = time.Now()
	return nil
}

func (h *Handler) refreshServerStatus(ctx context.Context, server domain.GameServerInstance) domain.GameServerInstance {
	if isLifecyclePending(server.Status) {
		if server.ContainerID == "" && time.Since(server.UpdatedAt) > staleLifecyclePendingAfter {
			server.Status = domain.StatusErrored
			server.LastError = "server lifecycle action timed out before a Docker container was created"
			server.UpdatedAt = time.Now()
			if err := h.store.SaveServer(ctx, &server); err != nil {
				h.logger.Warn("failed to persist stale lifecycle timeout", "server", server.ID, "error", err)
			}
		}
		return server
	}
	if server.ContainerID == "" {
		if server.Status == domain.StatusErrored && isRuntimeContainerMissingError(errors.New(server.LastError)) {
			server.Status = domain.StatusStopped
			server.LastError = ""
			server.UpdatedAt = time.Now()
			if err := h.store.SaveServer(ctx, &server); err != nil {
				h.logger.Warn("failed to persist stale runtime server recovery", "server", server.ID, "error", err)
			}
		}
		return server
	}
	if !h.runtimeStatusAvailable() {
		return server
	}
	status, err := h.runtime.Inspect(ctx, server)
	if err != nil {
		if isRuntimeContainerMissingError(err) {
			h.logger.Warn("runtime container missing during status refresh; marking server stopped", "server", server.ID, "container", server.ContainerID, "error", err)
			server.Status = domain.StatusStopped
			server.ContainerID = ""
			server.LastError = ""
			server.UpdatedAt = time.Now()
			if saveErr := h.store.SaveServer(ctx, &server); saveErr != nil {
				h.logger.Warn("failed to persist stale runtime server recovery", "server", server.ID, "error", saveErr, "cause", err)
			}
			return server
		}
		if status == domain.StatusErrored {
			server.Status = domain.StatusErrored
			server.LastError = err.Error()
			server.UpdatedAt = time.Now()
			if saveErr := h.store.SaveServer(ctx, &server); saveErr != nil {
				h.logger.Warn("failed to persist refreshed runtime server error", "server", server.ID, "error", saveErr, "cause", err)
			}
		} else {
			h.logger.Warn("failed to refresh runtime server status", "server", server.ID, "container", server.ContainerID, "error", err)
		}
		return server
	}
	if status == server.Status {
		return server
	}
	server.Status = status
	if status != domain.StatusErrored {
		server.LastError = ""
	}
	server.UpdatedAt = time.Now()
	if err := h.store.SaveServer(ctx, &server); err != nil {
		h.logger.Warn("failed to persist refreshed runtime server status", "server", server.ID, "status", status, "error", err)
	}
	return server
}

func isRuntimeContainerMissingError(err error) bool {
	if err == nil {
		return false
	}
	normalized := strings.ToLower(err.Error())
	return strings.Contains(normalized, "no docker container found") ||
		strings.Contains(normalized, "no such container") ||
		strings.Contains(normalized, "page not found")
}

func (h *Handler) runtimeStatusAvailable() bool {
	if h.dockerMonitor == nil {
		return true
	}
	return h.dockerMonitor.Status().Available
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

func isLifecyclePending(status domain.ServerStatus) bool {
	return status == domain.StatusStarting || status == domain.StatusStopping || status == domain.StatusRestarting || status == domain.StatusDeleting
}

func isServerLockedForMutation(status domain.ServerStatus) bool {
	return status == domain.StatusRunning || isLifecyclePending(status)
}

func isServerBusyForModMutation(status domain.ServerStatus) bool {
	return status == domain.StatusCreating || isLifecyclePending(status)
}

func (h *Handler) deleteServer(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if isLifecyclePending(server.Status) {
		writeError(w, http.StatusConflict, "server lifecycle action already in progress")
		return
	}
	server.Status = domain.StatusDeleting
	server.UpdatedAt = time.Now()
	if err := h.store.SaveServer(r.Context(), &server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.delete.queued", fmt.Sprintf("Queued delete for server %s", server.Name))
	go h.runServerLifecycle(server.ID, h.runDeleteServer)
	h.attachServerJoinInfo(&server)
	writeJSON(w, http.StatusAccepted, server)
}

func (h *Handler) runDeleteServer(ctx context.Context, id string) {
	server, err := h.store.GetServer(ctx, id)
	if err != nil {
		h.logger.Warn("failed to load server for async delete", "server", id, "error", err)
		return
	}
	if server.ContainerID != "" {
		if err := h.requireRuntimeAvailable(ctx); err != nil {
			h.markServerLifecycleFailed(ctx, server.ID, "server.delete.failed", err)
			return
		}
		if _, err := h.runtime.Inspect(ctx, server); err == nil {
			if err := h.runtime.Remove(ctx, server); err != nil {
				h.markServerLifecycleFailed(ctx, server.ID, "server.delete.failed", err)
				return
			}
		} else {
			h.logger.Warn("runtime container missing during server delete; deleting stale record", "server", server.ID, "container", server.ContainerID, "error", err)
		}
	}
	if err := h.cleanupOwnedServerResources(ctx, server); err != nil {
		h.markServerLifecycleFailed(ctx, server.ID, "server.delete.failed", err)
		return
	}
	if err := h.store.DeleteServerShareByInstance(ctx, server.ID); err != nil {
		h.markServerLifecycleFailed(ctx, server.ID, "server.delete.failed", err)
		return
	}
	if err := h.store.DeleteServer(ctx, server.ID); err != nil {
		h.markServerLifecycleFailed(ctx, server.ID, "server.delete.failed", err)
		return
	}
	h.recordActivity(ctx, server.ID, "server.deleted", fmt.Sprintf("Deleted server %s", server.Name))
}

func (h *Handler) cleanupOwnedServerResources(ctx context.Context, server domain.GameServerInstance) error {
	worlds, err := h.store.ListWorlds(ctx)
	if err != nil {
		return err
	}
	for _, item := range worlds {
		if item.InstanceID == server.ID {
			if item.Source == "server_snapshot" {
				if item.ActiveInstanceID == server.ID {
					item.ActiveInstanceID = ""
					if err := h.store.SaveWorld(ctx, &item); err != nil {
						return err
					}
				}
				continue
			}
			path, err := worldsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
			if err != nil {
				return err
			}
			if err := removeStoredFile(path); err != nil {
				return err
			}
			if err := h.store.DeleteWorld(ctx, item.ID); err != nil {
				return err
			}
			continue
		}
		if item.ActiveInstanceID == server.ID {
			item.ActiveInstanceID = ""
			if err := h.store.SaveWorld(ctx, &item); err != nil {
				return err
			}
		}
	}

	backups, err := h.store.ListBackups(ctx)
	if err != nil {
		return err
	}
	for _, item := range backups {
		if item.InstanceID != server.ID {
			continue
		}
		path, err := backupsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
		if err != nil {
			return err
		}
		if err := removeStoredFile(path); err != nil {
			return err
		}
		if err := h.store.DeleteBackup(ctx, item.ID); err != nil {
			return err
		}
	}

	mods, err := h.store.ListMods(ctx, server.ID)
	if err != nil {
		return err
	}
	for _, item := range mods {
		path, err := modsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
		if err != nil {
			return err
		}
		if err := removeStoredFile(path); err != nil {
			return err
		}
		if err := h.store.DeleteMod(ctx, item.ID); err != nil {
			return err
		}
	}

	instanceDir, err := safety.SafeJoin(h.cfg.DataDir, "instances", server.ID)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(instanceDir); err != nil {
		return err
	}
	return nil
}

func (h *Handler) serverStats(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if server.Status != domain.StatusRunning || !h.runtimeStatusAvailable() {
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

func normalizeTerrariaRuntimeConfig(config domain.TerrariaConfig) domain.TerrariaConfig {
	config.Port = terraria.DefaultInternalPort
	return terraria.NormalizeConfig(config)
}

func normalizeProviderRuntimeConfig(providerKey domain.ProviderKey, config domain.TerrariaConfig) domain.TerrariaConfig {
	switch providerKey {
	case domain.ProviderPalworld:
		return palworld.NormalizeConfig(config)
	case domain.ProviderDST:
		return dst.NormalizeConfig(config)
	case domain.ProviderMinecraft:
		return minecraft.NormalizeConfig(config)
	default:
		return normalizeTerrariaRuntimeConfig(config)
	}
}

func decodeProviderRuntimeConfig(providerKey domain.ProviderKey, raw json.RawMessage, fallback domain.TerrariaConfig) (domain.TerrariaConfig, string, error) {
	if isEmptyRawJSON(raw) {
		config := normalizeProviderRuntimeConfig(providerKey, fallback)
		payload, err := providerConfigPayloadJSON(providerKey, config)
		return config, payload, err
	}
	switch providerKey {
	case domain.ProviderDST:
		return decodeDSTRuntimeConfig(raw, fallback)
	case domain.ProviderPalworld:
		return decodePalworldRuntimeConfig(raw, fallback)
	case domain.ProviderMinecraft:
		return decodeMinecraftRuntimeConfig(raw, fallback)
	default:
		return decodeTerrariaRuntimeConfig(raw, fallback)
	}
}

func decodeTerrariaRuntimeConfig(raw json.RawMessage, fallback domain.TerrariaConfig) (domain.TerrariaConfig, string, error) {
	config := fallback
	if err := json.Unmarshal(raw, &config); err != nil {
		return domain.TerrariaConfig{}, "", fmt.Errorf("invalid config payload")
	}
	config = normalizeTerrariaRuntimeConfig(config)
	payload, err := providerConfigPayloadJSON(domain.ProviderTerrariaVanilla, config)
	return config, payload, err
}

func decodePalworldRuntimeConfig(raw json.RawMessage, fallback domain.TerrariaConfig) (domain.TerrariaConfig, string, error) {
	var payload struct {
		ServerName     string `json:"serverName"`
		SaveName       string `json:"saveName"`
		WorldName      string `json:"worldName"`
		MaxPlayers     int    `json:"maxPlayers"`
		ServerPassword string `json:"serverPassword"`
		Password       string `json:"password"`
		AdminPassword  string `json:"adminPassword"`
		MOTD           string `json:"motd"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return domain.TerrariaConfig{}, "", fmt.Errorf("invalid config payload")
	}
	config := fallback
	if payload.ServerName != "" {
		config.ServerName = payload.ServerName
	}
	if payload.SaveName != "" {
		config.WorldName = payload.SaveName
	} else if payload.WorldName != "" {
		config.WorldName = payload.WorldName
	}
	if payload.MaxPlayers > 0 {
		config.MaxPlayers = payload.MaxPlayers
	}
	if payload.ServerPassword != "" {
		config.Password = payload.ServerPassword
	} else if payload.Password != "" {
		config.Password = payload.Password
	}
	if payload.AdminPassword != "" {
		config.MOTD = payload.AdminPassword
	} else if payload.MOTD != "" {
		config.MOTD = payload.MOTD
	}
	config = palworld.NormalizeConfig(config)
	configPayloadJSON, err := providerConfigPayloadJSON(domain.ProviderPalworld, config)
	return config, configPayloadJSON, err
}

func decodeDSTRuntimeConfig(raw json.RawMessage, fallback domain.TerrariaConfig) (domain.TerrariaConfig, string, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return domain.TerrariaConfig{}, "", fmt.Errorf("invalid config payload")
	}
	config := dst.ConfigFromPayload(payload, fallback)
	configPayloadJSON, err := json.Marshal(dst.EnrichPayloadFromConfig(config, payload))
	if err != nil {
		return domain.TerrariaConfig{}, "", err
	}
	return config, string(configPayloadJSON), nil
}

func decodeMinecraftRuntimeConfig(raw json.RawMessage, fallback domain.TerrariaConfig) (domain.TerrariaConfig, string, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return domain.TerrariaConfig{}, "", fmt.Errorf("invalid config payload")
	}
	config := minecraft.ConfigFromPayload(payload, fallback)
	configPayloadJSON, err := json.Marshal(minecraft.EnrichPayloadFromConfig(config, payload))
	if err != nil {
		return domain.TerrariaConfig{}, "", err
	}
	return config, string(configPayloadJSON), nil
}

func stringPayload(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func providerConfigPayloadJSON(providerKey domain.ProviderKey, config domain.TerrariaConfig) (string, error) {
	if providerKey == domain.ProviderDST {
		buf, err := json.Marshal(dst.PayloadFromConfig(config, "survival"))
		return string(buf), err
	}
	if providerKey == domain.ProviderMinecraft {
		buf, err := json.Marshal(minecraft.PayloadFromConfig(config, nil))
		return string(buf), err
	}
	if providerKey == domain.ProviderPalworld {
		payload := map[string]any{
			"serverName":    config.ServerName,
			"saveName":      config.WorldName,
			"maxPlayers":    config.MaxPlayers,
			"adminPassword": config.MOTD,
		}
		if config.Password != "" {
			payload["serverPassword"] = config.Password
		}
		buf, err := json.Marshal(payload)
		return string(buf), err
	}
	buf, err := json.Marshal(config)
	return string(buf), err
}

func sanitizePresetConfigPayload(gameProvider provider.GameProvider, configPayloadJSON string) (string, error) {
	if strings.TrimSpace(configPayloadJSON) == "" {
		return "", nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(configPayloadJSON), &payload); err != nil {
		return "", err
	}
	for _, key := range []string{"password", "serverPassword", "adminPassword", "clusterToken"} {
		delete(payload, key)
	}
	for _, field := range gameProvider.ConfigSchema() {
		if field.Type == "password" {
			delete(payload, field.Name)
		}
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

func isEmptyRawJSON(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed == "" || trimmed == "null"
}

func hydratePresetConfigPayload(preset *domain.ConfigPreset) {
	if preset == nil || strings.TrimSpace(preset.ConfigPayloadJSON) == "" {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(preset.ConfigPayloadJSON), &payload); err == nil {
		preset.ConfigPayload = payload
	}
}

func hydrateServerConfigPayload(server *domain.GameServerInstance) {
	if server == nil || strings.TrimSpace(server.ConfigPayloadJSON) == "" {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(server.ConfigPayloadJSON), &payload); err == nil {
		server.ConfigPayload = payload
	}
}

func (h *Handler) attachServerJoinInfo(server *domain.GameServerInstance) {
	if server == nil {
		return
	}
	gameProvider, ok := h.provider.Get(server.ProviderKey)
	if ok {
		if joinProvider, ok := gameProvider.(provider.JoinInfoProvider); ok {
			server.JoinInfo = joinProvider.JoinInfo(*server)
			h.applyPublicHostToJoinInfo(&server.JoinInfo, *server)
			return
		}
	}
	server.JoinInfo = defaultJoinInfo(*server)
	h.applyPublicHostToJoinInfo(&server.JoinInfo, *server)
}

func (h *Handler) resolvePublicHost() string {
	host, err := h.store.GetSetting(context.Background(), "publicHost")
	if err == nil && strings.TrimSpace(host) != "" {
		return strings.TrimSpace(host)
	}
	if strings.TrimSpace(h.cfg.PublicHost) != "" {
		return strings.TrimSpace(h.cfg.PublicHost)
	}
	return "127.0.0.1"
}

func (h *Handler) applyPublicHostToJoinInfo(info *domain.ServerJoinInfo, server domain.GameServerInstance) {
	host := h.resolvePublicHost()
	if host == "" || host == info.Address {
		return
	}
	old := info.Address
	info.Address = host
	info.InviteText = strings.ReplaceAll(info.InviteText, old+":"+fmt.Sprintf("%d", info.Port), host+":"+fmt.Sprintf("%d", info.Port))
	info.InviteText = strings.ReplaceAll(info.InviteText, old, host)
}

func defaultJoinInfo(server domain.GameServerInstance) domain.ServerJoinInfo {
	port := server.HostPort
	if port == 0 {
		port = server.Port
	}
	address := "127.0.0.1"
	invite := fmt.Sprintf("Join %s at %s:%d", server.Name, address, port)
	if server.Password != "" {
		invite += " password: " + server.Password
	}
	return domain.ServerJoinInfo{
		Address:    address,
		Port:       port,
		Password:   server.Password,
		InviteText: invite,
	}
}

func (h *Handler) resolveHostPort(ctx context.Context, requested int, excludeInstanceID string) (int, error) {
	if requested == 0 {
		return h.allocateHostPort(ctx, excludeInstanceID)
	}
	if requested < 1024 || requested > 65535 {
		return 0, fmt.Errorf("external port must be between 1024 and 65535")
	}
	if err := h.ensureHostPortAvailable(ctx, requested, excludeInstanceID); err != nil {
		return 0, err
	}
	return requested, nil
}

func (h *Handler) ensureHostPortAvailable(ctx context.Context, hostPort int, excludeInstanceID string) error {
	servers, err := h.store.ListServers(ctx)
	if err != nil {
		return err
	}
	for _, server := range servers {
		if server.ID != excludeInstanceID && server.HostPort == hostPort {
			return fmt.Errorf("external port %d is already used", hostPort)
		}
	}
	return nil
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
	if h.apiMetrics != nil {
		h.apiMetrics.AddSSEConnection("server_logs", 1)
		defer h.apiMetrics.AddSSEConnection("server_logs", -1)
	}
	stream, err := h.runtime.Logs(r.Context(), server)
	if err != nil {
		if strings.TrimSpace(server.LastError) != "" {
			_, _ = fmt.Fprintf(w, "event: log\ndata: %s\n\n", server.LastError)
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

func (h *Handler) updatePlayersFromLogLine(ctx context.Context, server domain.GameServerInstance, recentLines []string, line string) {
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
	latest, err := h.store.GetServer(ctx, server.ID)
	if err != nil {
		h.logger.Warn("failed to load server for player activity update", "server", server.ID, "error", err)
		return
	}
	nextCount := latest.PlayersOnline
	switch event {
	case domain.PlayerJoined:
		nextCount++
		if latest.MaxPlayers > 0 && nextCount > latest.MaxPlayers {
			nextCount = latest.MaxPlayers
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
	server, err := h.store.GetServer(ctx, serverID)
	if err != nil {
		h.logger.Warn("failed to load server for player count update", "server", serverID, "error", err)
		return
	}
	if count < 0 {
		count = 0
	}
	if server.MaxPlayers > 0 && count > server.MaxPlayers {
		count = server.MaxPlayers
	}
	if server.PlayersOnline == count {
		return
	}
	server.PlayersOnline = count
	server.UpdatedAt = time.Now()
	if err := h.store.SaveServer(ctx, &server); err != nil {
		h.logger.Warn("failed to persist player count", "server", serverID, "error", err)
	}
}

func (h *Handler) serverLogSnapshot(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if server.Status != domain.StatusRunning && !h.runtimeStatusAvailable() {
		writeJSON(w, http.StatusOK, map[string][]string{"lines": []string{}})
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
	stream, err := h.runtime.LogSnapshot(r.Context(), server)
	if err != nil {
		if strings.TrimSpace(server.LastError) != "" {
			writeJSON(w, http.StatusOK, map[string][]string{"lines": []string{server.LastError}})
			return
		}
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

func normalizeStoredProviderVersion(gameProvider provider.GameProvider, version string) string {
	version = strings.TrimSpace(version)
	if providerSupportsVersion(gameProvider, version) {
		return version
	}
	versions := gameProvider.Versions()
	if len(versions) == 0 {
		return version
	}
	return versions[0]
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
	return os.WriteFile(target, []byte(content), 0o644)
}
