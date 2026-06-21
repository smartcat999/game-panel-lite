package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
	worldsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/world"
)

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
		server, err := h.store.GetGameServer(r.Context(), instanceID)
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
	h.recordActivity(r.Context(), instanceID, "world.imported", fmt.Sprintf("Imported world %s", item.Name), activityWorldPayload(item, nil))
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
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
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
		name = serverWorldName(server)
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
	h.recordActivity(r.Context(), server.ID, "world.snapshot.created", fmt.Sprintf("Saved world snapshot %s from %s", item.Name, server.Name), activityWorldPayload(item, &server))
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
	resource, err := h.store.GetGameServer(r.Context(), payload.InstanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if isGameServerLockedForMutation(resource) {
		writeError(w, http.StatusConflict, "stop the server before assigning a world")
		return
	}
	if !worldCompatibleWithServer(item, resource) {
		writeError(w, http.StatusConflict, "world snapshot is not compatible with this server type")
		return
	}
	gameProvider, ok := h.provider.Get(resource.ProviderKey)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown provider")
		return
	}
	configPayload, _, err := decodeProviderConfigPayload(gameProvider, nil, resource.Spec.Config)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	summary, err := providerConfigSummary(gameProvider, configPayload)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.materializeWorldForRuntime(item, resource); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.clearActiveWorlds(r.Context(), payload.InstanceID, item.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	item.ActiveInstanceID = payload.InstanceID
	item.UpdatedAt = time.Now()
	resource.Spec.Config = configPayload
	resource.Spec.Network.Port = summary.Port
	resource.Spec.Generation++
	if resource.Spec.Generation <= 0 {
		resource.Spec.Generation = 1
	}
	resource.Status.Phase = domain.PhasePending
	resource.UpdatedAt = time.Now()
	if resource.Spec.SourceWorldID == "" {
		resource.Spec.SourceWorldID = item.ID
		resource.Spec.SourceWorldName = item.Name
	}
	if err := h.store.SaveGameServer(r.Context(), &resource); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.store.SaveWorld(r.Context(), &item); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), payload.InstanceID, "world.assigned", fmt.Sprintf("Assigned world %s to %s", item.Name, resource.Name), activityWorldPayload(item, &resource))
	writeJSON(w, http.StatusOK, h.hydrateWorldResource(r.Context(), item))
}

func (h *Handler) hydrateWorldResource(ctx context.Context, world domain.World) domain.World {
	if world.ProviderKey == "" && world.InstanceID != "" && world.InstanceID != "unassigned" {
		if server, err := h.store.GetGameServer(ctx, world.InstanceID); err == nil {
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

func (h *Handler) materializeWorldForRuntime(world domain.World, server domain.GameServer) error {
	sourcePath, err := worldsvc.NewService(h.cfg.DataDir).Path(world.InstanceID, world.FileName)
	if err != nil {
		return err
	}
	dataDir, err := serverDataDir(server)
	if err != nil {
		return err
	}
	for _, relPath := range h.runtimeWorldPathCandidates(server) {
		targetPath := filepath.Join(dataDir, relPath)
		if err := copyStoredFile(sourcePath, targetPath); err != nil {
			return err
		}
	}
	return nil
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

func worldCompatibleWithServer(world domain.World, server domain.GameServer) bool {
	return world.ProviderKey == "" || world.ProviderKey == server.ProviderKey
}

func (h *Handler) upsertWorldSnapshotRecord(ctx context.Context, server domain.GameServer, name string, fileName string, size int64) (domain.World, bool, error) {
	gameProvider, ok := h.provider.Get(server.ProviderKey)
	if !ok {
		return domain.World{}, false, fmt.Errorf("unknown provider")
	}
	configPayload, configPayloadJSON, err := decodeProviderConfigPayload(gameProvider, nil, server.Spec.Config)
	if err != nil {
		return domain.World{}, false, err
	}
	if existing, err := h.store.GetWorldByInstanceAndFile(ctx, server.ID, fileName); err == nil {
		existing.Name = name
		existing.SizeBytes = size
		existing.ProviderKey = server.ProviderKey
		existing.Source = "server_snapshot"
		existing.Config = configPayload
		existing.ConfigPayload = configPayload
		existing.ConfigPayloadJSON = configPayloadJSON
		if existing.ActiveInstanceID == server.ID {
			existing.ActiveInstanceID = ""
		}
		existing.UpdatedAt = time.Now()
		return existing, false, h.store.SaveWorld(ctx, &existing)
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.World{}, false, err
	}
	item := domain.World{
		ID:                uuid.NewString(),
		InstanceID:        server.ID,
		ProviderKey:       server.ProviderKey,
		Name:              name,
		FileName:          fileName,
		SizeBytes:         size,
		Source:            "server_snapshot",
		Config:            configPayload,
		ConfigPayloadJSON: configPayloadJSON,
		ConfigPayload:     configPayload,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	return item, true, h.store.CreateWorld(ctx, &item)
}

func (h *Handler) currentRuntimeWorldPath(server domain.GameServer) (string, error) {
	dataDir, err := serverDataDir(server)
	if err != nil {
		return "", err
	}
	for _, relPath := range h.runtimeWorldPathCandidates(server) {
		path := filepath.Join(dataDir, relPath)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("current world file has not been created yet")
}

func (h *Handler) runtimeWorldPathCandidates(server domain.GameServer) []string {
	summary, err := h.configSummaryForServer(server)
	worldName := serverWorldName(server)
	if err == nil && strings.TrimSpace(summary.WorldName) != "" {
		worldName = summary.WorldName
	}
	worldFile := worldName + ".wld"
	candidates := []string{}
	if server.ProviderKey == domain.ProviderTerrariaVanilla || server.ProviderKey == domain.ProviderTerrariaTModLoader {
		config, err := terraria.ConfigFromPayload(server.Spec.Config, terraria.Config{WorldName: worldName})
		if err != nil {
			config = terraria.Config{WorldName: worldName}
		}
		candidates = append(candidates, terraria.RuntimeWorldFiles(server.ProviderKey, config)...)
	}
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
		server, err := h.store.GetGameServer(r.Context(), item.ActiveInstanceID)
		if err == nil && serverWorldName(server) == item.Name {
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
	h.recordActivity(r.Context(), item.ActiveInstanceID, "world.deleted", fmt.Sprintf("Deleted world %s", item.Name), activityWorldPayload(item, nil))
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) worldTemplateInUse(ctx context.Context, worldID string) (bool, error) {
	servers, err := h.store.ListGameServers(ctx)
	if err != nil {
		return false, err
	}
	for _, server := range servers {
		if server.Spec.SourceWorldID == worldID {
			return true, nil
		}
	}
	return false, nil
}
