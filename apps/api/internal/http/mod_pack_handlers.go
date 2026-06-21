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
