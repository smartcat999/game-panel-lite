package http

import (
	"encoding/json"
	"net/http"
	"strings"

	modsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/mod"
)

const maxModBatchSize = 100

type modBatchRequest struct {
	IDs []string `json:"ids"`
}

type modBatchResult struct {
	ID    string `json:"id"`
	Error string `json:"error,omitempty"`
}

type modBatchResponse struct {
	Succeeded []modBatchResult `json:"succeeded"`
	Failed    []modBatchResult `json:"failed"`
}

func (h *Handler) batchDeleteGlobalMods(w http.ResponseWriter, r *http.Request) {
	ids, ok := decodeModBatchRequest(w, r)
	if !ok {
		return
	}
	response := modBatchResponse{
		Succeeded: make([]modBatchResult, 0, len(ids)),
		Failed:    make([]modBatchResult, 0),
	}
	for _, id := range ids {
		item, err := h.store.GetMod(r.Context(), id)
		if err != nil {
			response.Failed = append(response.Failed, modBatchResult{ID: id, Error: "mod not found"})
			continue
		}
		if item.InstanceID != "unassigned" {
			response.Failed = append(response.Failed, modBatchResult{ID: id, Error: "global mod delete only supports unassigned library mods"})
			continue
		}
		if item.Source != "workshop" {
			path, _ := modsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.ProviderKey, item.FileName)
			if err := removeStoredFile(path); err != nil {
				response.Failed = append(response.Failed, modBatchResult{ID: id, Error: err.Error()})
				continue
			}
		}
		if err := h.store.DeleteMod(r.Context(), item.ID); err != nil {
			response.Failed = append(response.Failed, modBatchResult{ID: id, Error: err.Error()})
			continue
		}
		response.Succeeded = append(response.Succeeded, modBatchResult{ID: id})
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) batchDeleteModPacks(w http.ResponseWriter, r *http.Request) {
	ids, ok := decodeModBatchRequest(w, r)
	if !ok {
		return
	}
	response := modBatchResponse{
		Succeeded: make([]modBatchResult, 0, len(ids)),
		Failed:    make([]modBatchResult, 0),
	}
	for _, id := range ids {
		if _, err := h.store.GetModPack(r.Context(), id); err != nil {
			response.Failed = append(response.Failed, modBatchResult{ID: id, Error: "mod pack not found"})
			continue
		}
		if err := h.store.DeleteModPack(r.Context(), id); err != nil {
			response.Failed = append(response.Failed, modBatchResult{ID: id, Error: err.Error()})
			continue
		}
		response.Succeeded = append(response.Succeeded, modBatchResult{ID: id})
	}
	writeJSON(w, http.StatusOK, response)
}

func decodeModBatchRequest(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	var payload modBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	ids := uniqueNonEmptyStrings(payload.IDs)
	for index := range ids {
		ids[index] = strings.TrimSpace(ids[index])
	}
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "select at least one item")
		return nil, false
	}
	if len(ids) > maxModBatchSize {
		writeError(w, http.StatusBadRequest, "batch size cannot exceed 100 items")
		return nil, false
	}
	return ids, true
}
