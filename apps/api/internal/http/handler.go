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
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	backupsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/config"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	modsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/mod"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modcatalog"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
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
	r.Get("/api/runtime/stats", h.runtimeStats)
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
	r.Get("/api/worlds/{id}/download", h.downloadWorld)
	r.Delete("/api/worlds/{id}", h.deleteWorld)
	r.Get("/api/backups", h.listBackups)
	r.Post("/api/servers/{id}/world-snapshots", h.createWorldSnapshot)
	r.Post("/api/servers/{id}/backups", h.createBackup)
	r.Get("/api/backups/{id}/download", h.downloadBackup)
	r.Post("/api/backups/{id}/restore", h.restoreBackup)
	r.Delete("/api/backups/{id}", h.deleteBackup)
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
	visible, err := h.visibleMods(r.Context(), mods)
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
	if err := h.syncRuntimeEnabledMods(r.Context(), server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "mod.workshop_imported", fmt.Sprintf("Imported %d workshop mod IDs for %s", len(workshopIDs), server.Name))
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) importGlobalWorkshopMods(w http.ResponseWriter, r *http.Request) {
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
	InLibrary bool   `json:"inLibrary"`
	ModID     string `json:"modId,omitempty"`
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
		entry := recommendedModResponse{RecommendedMod: item}
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
		assigned, created, err := h.upsertWorkshopModRecord(r.Context(), targetServer.ID, item.WorkshopID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := h.syncRuntimeEnabledMods(r.Context(), targetServer); err != nil {
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
		return existing, false, h.store.SaveMod(ctx, &existing)
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.ModFile{}, false, err
	}
	item := domain.ModFile{ID: uuid.NewString(), InstanceID: instanceID, FileName: fileName, Source: "upload", SizeBytes: size, Enabled: true, CreatedAt: time.Now()}
	applyTModMetadata(&item, metadata)
	return item, true, h.store.CreateMod(ctx, &item)
}

func applyTModMetadata(item *domain.ModFile, metadata modsvc.Metadata) {
	if metadata.Name != "" {
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
	item.Title = recommended.Title
	item.CreatorSteamID = recommended.CreatorSteamID
	item.PreviewURL = recommended.PreviewURL
	item.Description = recommended.Description
	item.TagsJSON = string(tags)
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
	if item.TagsJSON != "" {
		_ = json.Unmarshal([]byte(item.TagsJSON), &item.Tags)
	}
	if item.Source == "workshop" && item.Title == "" && item.WorkshopID != "" {
		item.Title = "Workshop " + item.WorkshopID
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
			continue
		}
		if isTModPackage(item.FileName) {
			enabled = append(enabled, strings.TrimSuffix(item.FileName, filepath.Ext(item.FileName)))
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
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(targetPath, payload, 0o600); err != nil {
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
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(targetPath, []byte(content), 0o600); err != nil {
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
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	ModIDs      []string         `json:"modIds"`
	Mods        []domain.ModFile `json:"mods"`
	CreatedAt   time.Time        `json:"createdAt"`
	UpdatedAt   time.Time        `json:"updatedAt"`
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
		mods = append(mods, item)
	}
	return modPackResponse{
		ID:          pack.ID,
		Name:        pack.Name,
		Description: pack.Description,
		ModIDs:      modIDs,
		Mods:        mods,
		CreatedAt:   pack.CreatedAt,
		UpdatedAt:   pack.UpdatedAt,
	}, nil
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
		visible = append(visible, world)
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
	if err := h.applyServerConfig(r.Context(), &server, nextConfig, nil); err != nil {
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
	writeJSON(w, http.StatusOK, item)
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
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
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
	if err := os.Chmod(tmpName, 0o600); err != nil {
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
	nextConfig = normalizeTerrariaRuntimeConfig(nextConfig)
	server.WorldName = nextConfig.WorldName
	server.Port = nextConfig.Port
	server.MaxPlayers = nextConfig.MaxPlayers
	server.Password = nextConfig.Password
	server.Config = nextConfig
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

func (h *Handler) runtimeStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.runtime.HostStats(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, runtime.HostStats{})
		return
	}
	writeJSON(w, http.StatusOK, stats)
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
	if err := config.PersistDockerHost(host); err != nil {
		h.logger.Warn("failed to persist docker host", "error", err)
	}
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
	if err := config.PersistDockerHost(host); err != nil {
		h.logger.Warn("failed to persist docker host", "error", err)
	}
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
	if isLifecyclePending(server.Status) {
		writeError(w, http.StatusConflict, "server lifecycle action already in progress")
		return
	}
	var payload struct {
		Config   domain.TerrariaConfig `json:"config"`
		HostPort *int                  `json:"hostPort,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := h.applyServerConfig(r.Context(), &server, payload.Config, payload.HostPort); err != nil {
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
		HostPort    int                   `json:"hostPort,omitempty"`
		Version     string                `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	payload.Config = normalizeTerrariaRuntimeConfig(payload.Config)
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
	hostPort, err := h.resolveHostPort(r.Context(), payload.HostPort, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	server := domain.GameServerInstance{
		ID: id, Name: payload.Name, GameKey: "terraria", ProviderKey: payload.ProviderKey,
		Status: domain.StatusStopped, WorldName: payload.Config.WorldName, Port: payload.Config.Port,
		MaxPlayers: payload.Config.MaxPlayers, Password: payload.Config.Password, DataDir: dataDir, HostPort: hostPort,
		Config: payload.Config, Version: payload.Version, ConfigRevision: 1, AppliedConfigRevision: 1,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := h.store.CreateServer(r.Context(), &server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.created", fmt.Sprintf("Created server %s", server.Name))
	writeJSON(w, http.StatusCreated, server)
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
	go h.runStartServer(context.Background(), server.ID)
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
	go h.runStopServer(context.Background(), server.ID)
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
	go h.runRestartServer(context.Background(), server.ID)
	writeJSON(w, http.StatusAccepted, server)
}

func (h *Handler) runStartServer(ctx context.Context, id string) {
	server, err := h.store.GetServer(ctx, id)
	if err != nil {
		h.markServerLifecycleFailed(ctx, id, "server.start.failed", errors.New("server not found"))
		return
	}
	server, _, err = h.ensureRuntimeContainer(ctx, server)
	if err != nil {
		h.markServerLifecycleFailed(ctx, id, "server.start.failed", err)
		return
	}
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
	server, _, err = h.ensureRuntimeContainer(ctx, server)
	if err != nil {
		h.markServerLifecycleFailed(ctx, id, "server.restart.failed", err)
		return
	}
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
	configText, err := gameProvider.RenderConfig(server.Config)
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
		DataDir:    server.DataDir,
		ConfigText: configText,
		Options:    gameProvider.RuntimeOptions(server.Config),
	}
	return spec, nil
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

func (h *Handler) applyServerConfig(ctx context.Context, server *domain.GameServerInstance, nextConfig domain.TerrariaConfig, hostPort *int) error {
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
	nextConfig = normalizeTerrariaRuntimeConfig(nextConfig)
	nextHostPort := server.HostPort
	if hostPort != nil {
		nextHostPort = *hostPort
	}
	var err error
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
	if err := os.WriteFile(filepath.Join(server.DataDir, "serverconfig.txt"), []byte(configText), 0o600); err != nil {
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
	server.MaxPlayers = nextConfig.MaxPlayers
	server.Password = nextConfig.Password
	server.Config = nextConfig
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
		return server
	}
	if server.ContainerID == "" {
		return server
	}
	if !h.runtimeStatusAvailable() {
		return server
	}
	status, err := h.runtime.Inspect(ctx, server)
	if err != nil {
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
	go h.runDeleteServer(context.Background(), server.ID)
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
	stream, err := h.runtime.Logs(r.Context(), server)
	if err != nil {
		if strings.TrimSpace(server.LastError) != "" {
			_, _ = fmt.Fprintf(w, "event: log\ndata: %s\n\n", server.LastError)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
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
	return os.WriteFile(target, []byte(content), 0o600)
}
