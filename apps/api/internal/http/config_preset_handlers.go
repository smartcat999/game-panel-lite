package http

import (
	"context"
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

type configPresetPayload struct {
	OrganizationID string               `json:"organizationId,omitempty"`
	Name           string               `json:"name"`
	ProviderKey    domain.ProviderKey   `json:"providerKey"`
	Config         json.RawMessage      `json:"config"`
	Version        string               `json:"version"`
	Resources      resourceLimitPayload `json:"resources,omitempty"`
	ModPackID      string               `json:"modPackId,omitempty"`
	ModIDs         []string             `json:"modIds"`
}

type configPresetBatchDeletePayload struct {
	IDs []string `json:"ids"`
}

type configPresetBatchDeleteResult struct {
	Succeeded []map[string]string `json:"succeeded"`
	Failed    []map[string]string `json:"failed"`
}

func (h *Handler) listConfigPresets(w http.ResponseWriter, r *http.Request) {
	list := h.store.ListConfigPresets
	if actor := allocationActor(r); actor != "" {
		list = func(ctx context.Context) ([]domain.ConfigPreset, error) {
			return h.store.ListUserConfigPresets(ctx, actor)
		}
	}
	presets, err := list(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range presets {
		presets[i], err = h.gameConfig.PublicPreset(presets[i])
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, presets)
}

func (h *Handler) getConfigPreset(w http.ResponseWriter, r *http.Request) {
	preset, err := h.visibleConfigPreset(r, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "config preset not found")
		return
	}
	preset, err = h.gameConfig.PublicPreset(preset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
	organizationID, status, err := h.creationOrganization(r, preset.OrganizationID)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	preset.OrganizationID = organizationID
	preset.ID = uuid.NewString()
	preset.CreatedAt = time.Now()
	preset.UpdatedAt = preset.CreatedAt
	if err := h.store.CreateOwnedConfigPreset(r.Context(), allocationActor(r), &preset); err != nil {
		writePresetWriteError(w, err)
		return
	}
	hydratePresetConfigPayload(&preset)
	writeJSON(w, http.StatusCreated, preset)
}

func (h *Handler) updateConfigPreset(w http.ResponseWriter, r *http.Request) {
	existing, err := h.visibleConfigPreset(r, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "config preset not found")
		return
	}
	preset, err := h.buildConfigPreset(r, existing.ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if preset.OrganizationID != "" && preset.OrganizationID != existing.OrganizationID {
		writeError(w, http.StatusBadRequest, "preset workspace cannot be changed")
		return
	}
	preset.OrganizationID = existing.OrganizationID
	preset.CreatedAt = existing.CreatedAt
	preset.UpdatedAt = time.Now()
	if err := h.store.SaveOwnedConfigPreset(r.Context(), allocationActor(r), existing, preset); err != nil {
		writePresetWriteError(w, err)
		return
	}
	hydratePresetConfigPayload(&preset)
	writeJSON(w, http.StatusOK, preset)
}

func (h *Handler) deleteConfigPreset(w http.ResponseWriter, r *http.Request) {
	existing, err := h.visibleConfigPreset(r, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "config preset not found")
		return
	}
	if err := h.store.DeleteOwnedConfigPreset(r.Context(), allocationActor(r), existing); err != nil {
		writePresetWriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) batchDeleteConfigPresets(w http.ResponseWriter, r *http.Request) {
	var payload configPresetBatchDeletePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	ids := uniqueNonEmptyStrings(payload.IDs)
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "at least one config preset id is required")
		return
	}
	if len(ids) > 100 {
		writeError(w, http.StatusBadRequest, "cannot delete more than 100 config presets at once")
		return
	}
	result := configPresetBatchDeleteResult{Succeeded: []map[string]string{}, Failed: []map[string]string{}}
	for _, id := range ids {
		existing, err := h.visibleConfigPreset(r, id)
		if err != nil {
			result.Failed = append(result.Failed, map[string]string{"id": id, "error": "config preset not found"})
			continue
		}
		if err := h.store.DeleteOwnedConfigPreset(r.Context(), allocationActor(r), existing); err != nil {
			result.Failed = append(result.Failed, map[string]string{"id": id, "error": err.Error()})
			continue
		}
		result.Succeeded = append(result.Succeeded, map[string]string{"id": id})
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) buildConfigPreset(r *http.Request, id string) (domain.ConfigPreset, error) {
	var payload configPresetPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return domain.ConfigPreset{}, fmt.Errorf("invalid JSON body")
	}
	if allocationActor(r) != "" && (payload.ModPackID != "" || len(uniqueNonEmptyStrings(payload.ModIDs)) > 0) {
		return domain.ConfigPreset{}, fmt.Errorf("tenant preset mod references require a workspace-owned mod library")
	}
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.Name == "" {
		return domain.ConfigPreset{}, fmt.Errorf("preset name is required")
	}
	gameProvider, ok := h.provider.Get(payload.ProviderKey)
	if !ok {
		return domain.ConfigPreset{}, fmt.Errorf("unknown provider")
	}
	configPayload, configPayloadJSON, err := h.gameConfig.Normalize(gameProvider.Key(), gameProvider.CatalogMetadata().ConfigVersion, payload.Config, nil)
	if err != nil {
		return domain.ConfigPreset{}, err
	}
	if payload.Version == "" {
		payload.Version = gameProvider.Versions()[0]
	}
	if !providerSupportsVersion(gameProvider, payload.Version) {
		return domain.ConfigPreset{}, fmt.Errorf("unsupported provider version")
	}
	if err := h.gameConfig.Validate(gameProvider.Key(), configPayload); err != nil {
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
	modIDs := uniqueNonEmptyStrings(payload.ModIDs)
	if len(modIDs) > 0 && !h.providerSupportsMods(payload.ProviderKey) {
		return domain.ConfigPreset{}, fmt.Errorf("mods are not supported for this provider")
	}
	for _, modID := range modIDs {
		item, err := h.store.GetMod(r.Context(), modID)
		if err != nil || item.InstanceID != "unassigned" {
			return domain.ConfigPreset{}, fmt.Errorf("mod not found")
		}
		if item.ProviderKey != "" && item.ProviderKey != payload.ProviderKey {
			return domain.ConfigPreset{}, fmt.Errorf("mod is not compatible with this provider")
		}
	}
	modIDsJSON, err := json.Marshal(modIDs)
	if err != nil {
		return domain.ConfigPreset{}, err
	}
	configPayload, configPayloadJSON, err = h.gameConfig.PublicConfig(gameProvider.Key(), configPayload)
	if err != nil {
		return domain.ConfigPreset{}, err
	}
	return domain.ConfigPreset{
		OrganizationID: strings.TrimSpace(payload.OrganizationID), ID: id, Name: payload.Name, GameKey: gameProvider.GameKey(), ProviderKey: payload.ProviderKey,
		Version: payload.Version, Config: configPayload, ConfigPayloadJSON: configPayloadJSON, ConfigPayload: configPayload,
		CPULimitCores: resources.CPULimitCores, MemoryLimitMB: resources.MemoryLimitMB, ModPackID: payload.ModPackID,
		ModIDsJSON: string(modIDsJSON), ModIDs: modIDs,
	}, nil
}

func (h *Handler) visibleConfigPreset(r *http.Request, id string) (domain.ConfigPreset, error) {
	if actor := allocationActor(r); actor != "" {
		return h.store.GetUserConfigPreset(r.Context(), actor, id)
	}
	return h.store.GetConfigPreset(r.Context(), id)
}

func writePresetWriteError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrInvalidModLibrary) {
		writeError(w, http.StatusBadRequest, "mod references must belong to the preset workspace and provider")
		return
	}
	writeAllocationError(w, err)
}
