package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	modsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/mod"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modcatalog"
)

func (h *Handler) listMods(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
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
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if server.ProviderKey != domain.ProviderTerrariaTModLoader {
		writeError(w, http.StatusBadRequest, "mods are only supported for tModLoader servers")
		return
	}
	if isGameServerBusyForModMutation(server) {
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
	h.recordActivity(r.Context(), server.ID, "mod.uploaded", fmt.Sprintf("Uploaded mod %s to %s", item.FileName, server.Name), activityModPayload(item, &server))
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, item)
}

func (h *Handler) importWorkshopMods(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if server.ProviderKey != domain.ProviderTerrariaTModLoader {
		writeError(w, http.StatusBadRequest, "workshop mods are only supported for tModLoader servers")
		return
	}
	if isGameServerBusyForModMutation(server) {
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
	h.recordActivity(r.Context(), server.ID, "mod.workshop_imported", fmt.Sprintf("Imported %d workshop mod IDs for %s", len(workshopIDs), server.Name), map[string]any{
		"serverId":      server.ID,
		"serverName":    server.Name,
		"gameKey":       server.GameKey,
		"providerKey":   server.ProviderKey,
		"workshopIds":   workshopIDs,
		"workshopCount": len(workshopIDs),
	})
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
	h.recordActivity(r.Context(), "", "mod.workshop_imported", fmt.Sprintf("Imported %d workshop mod IDs into mod library", len(workshopIDs)), map[string]any{
		"workshopIds":   workshopIDs,
		"workshopCount": len(workshopIDs),
	})
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
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	item, err := h.store.GetMod(r.Context(), chi.URLParam(r, "modId"))
	if err != nil || item.InstanceID != server.ID {
		writeError(w, http.StatusNotFound, "mod not found")
		return
	}
	if isGameServerBusyForModMutation(server) {
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
	h.recordActivity(r.Context(), server.ID, "mod.updated", fmt.Sprintf("Updated mod %s", item.FileName), activityModPayload(item, &server))
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) deleteMod(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	item, err := h.store.GetMod(r.Context(), chi.URLParam(r, "modId"))
	if err != nil || item.InstanceID != server.ID {
		writeError(w, http.StatusNotFound, "mod not found")
		return
	}
	if isGameServerBusyForModMutation(server) {
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
	h.recordActivity(r.Context(), item.InstanceID, "mod.deleted", fmt.Sprintf("Deleted mod %s", item.FileName), activityModPayload(item, &server))
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
	targetServer, err := h.store.GetGameServer(r.Context(), payload.InstanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if targetServer.ProviderKey != domain.ProviderTerrariaTModLoader {
		writeError(w, http.StatusBadRequest, "mods are only supported for tModLoader servers")
		return
	}
	if isGameServerBusyForModMutation(targetServer) {
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
			h.recordActivity(r.Context(), targetServer.ID, "mod.assigned", fmt.Sprintf("Updated assigned mod %s for %s", item.FileName, targetServer.Name), activityModPayload(assigned, &targetServer))
			writeJSON(w, http.StatusOK, assigned)
			return
		}
		h.recordActivity(r.Context(), targetServer.ID, "mod.assigned", fmt.Sprintf("Assigned mod %s to %s", item.FileName, targetServer.Name), activityModPayload(assigned, &targetServer))
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
		h.recordActivity(r.Context(), targetServer.ID, "mod.assigned", fmt.Sprintf("Updated assigned mod %s for %s", item.FileName, targetServer.Name), activityModPayload(assigned, &targetServer))
		writeJSON(w, http.StatusOK, assigned)
		return
	}
	h.recordActivity(r.Context(), targetServer.ID, "mod.assigned", fmt.Sprintf("Assigned mod %s to %s", item.FileName, targetServer.Name), activityModPayload(assigned, &targetServer))
	writeJSON(w, http.StatusCreated, assigned)
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
