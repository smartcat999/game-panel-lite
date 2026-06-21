package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

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
	configPayload, configPayloadJSON, err := decodeProviderConfigPayload(gameProvider, payload.Config, nil)
	if err != nil {
		return domain.ConfigPreset{}, err
	}
	if payload.Version == "" {
		payload.Version = gameProvider.Versions()[0]
	}
	if !providerSupportsVersion(gameProvider, payload.Version) {
		return domain.ConfigPreset{}, fmt.Errorf("unsupported provider version")
	}
	if err := validateProviderConfigPayload(gameProvider, configPayload); err != nil {
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
	configPayloadJSON, err = sanitizePresetConfigPayload(gameProvider, configPayloadJSON)
	if err != nil {
		return domain.ConfigPreset{}, err
	}
	configPayload, err = configPayloadMap(configPayloadJSON)
	if err != nil {
		return domain.ConfigPreset{}, err
	}
	return domain.ConfigPreset{
		ID: id, Name: payload.Name, GameKey: gameProvider.GameKey(), ProviderKey: payload.ProviderKey,
		Version: payload.Version, Config: configPayload, ConfigPayloadJSON: configPayloadJSON, ConfigPayload: configPayload,
		CPULimitCores: resources.CPULimitCores, MemoryLimitMB: resources.MemoryLimitMB, ModPackID: payload.ModPackID,
	}, nil
}
