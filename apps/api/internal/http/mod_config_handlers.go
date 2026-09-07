package http

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modruntime"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
)

const maxModConfigBytes = modruntime.MaxConfigBytes

type modConfigFileResponse = modruntime.ConfigFile

func (h *Handler) listModConfigs(w http.ResponseWriter, r *http.Request) {
	server, ok := h.modConfigServer(w, r, false)
	if !ok {
		return
	}
	items, err := h.modRuntime.ListConfigs(r.Context(), server)
	if err != nil {
		writeModConfigError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) getModConfig(w http.ResponseWriter, r *http.Request) {
	server, ok := h.modConfigServer(w, r, false)
	if !ok {
		return
	}
	item, err := h.modRuntime.ReadConfig(r.Context(), server, chi.URLParam(r, "name"))
	if err != nil {
		writeModConfigError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) saveModConfig(w http.ResponseWriter, r *http.Request) {
	unlock := h.lockServerMutation(chi.URLParam(r, "id"))
	defer unlock()
	server, ok := h.modConfigServer(w, r, true)
	if !ok {
		return
	}
	var payload struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxModConfigBytes+1024)).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	item, err := h.modRuntime.WriteConfig(r.Context(), server, chi.URLParam(r, "name"), []byte(payload.Content))
	if err != nil {
		writeModConfigError(w, err, http.StatusBadRequest)
		return
	}
	h.recordActivity(r.Context(), server.ID, "mod_config.saved", "Saved mod config "+item.Name, map[string]any{"name": item.Name})
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) uploadModConfig(w http.ResponseWriter, r *http.Request) {
	unlock := h.lockServerMutation(chi.URLParam(r, "id"))
	defer unlock()
	server, ok := h.modConfigServer(w, r, true)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxModConfigBytes+64*1024)
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "mod config file is required")
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxModConfigBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "unable to read mod config")
		return
	}
	item, err := h.modRuntime.WriteConfig(r.Context(), server, header.Filename, content)
	if err != nil {
		writeModConfigError(w, err, http.StatusBadRequest)
		return
	}
	h.recordActivity(r.Context(), server.ID, "mod_config.uploaded", "Uploaded mod config "+item.Name, map[string]any{"name": item.Name})
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) deleteModConfig(w http.ResponseWriter, r *http.Request) {
	unlock := h.lockServerMutation(chi.URLParam(r, "id"))
	defer unlock()
	server, ok := h.modConfigServer(w, r, true)
	if !ok {
		return
	}
	name, err := h.modRuntime.DeleteConfig(r.Context(), server, chi.URLParam(r, "name"))
	if err != nil {
		writeModConfigError(w, err, http.StatusInternalServerError)
		return
	}
	h.recordActivity(r.Context(), server.ID, "mod_config.deleted", "Deleted mod config "+name, map[string]any{"name": name})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) modConfigServer(w http.ResponseWriter, r *http.Request, mutate bool) (domain.GameServer, bool) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return domain.GameServer{}, false
	}
	if mutate && h.gameUpdateLocked(r.Context(), server.ID) {
		writeError(w, http.StatusConflict, "server maintenance is in progress")
		return domain.GameServer{}, false
	}
	return server, true
}

func writeModConfigError(w http.ResponseWriter, err error, fallback int) {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		writeError(w, http.StatusNotFound, "mod config not found")
	case errors.Is(err, modruntime.ErrConfigConflict):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, provider.ErrUnsupported), errors.Is(err, modruntime.ErrInvalidConfig):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, fallback, err.Error())
	}
}
