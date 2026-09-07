package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modlibrary"
)

func (h *Handler) requestModInstallation(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var request struct {
		ModID      string `json:"modId"`
		Generation int    `json:"generation"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid installation request")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid installation request")
		return
	}
	server, err := h.modInstaller.Request(r.Context(), account.ID, chi.URLParam(r, "id"), request.ModID, request.Generation)
	if err != nil {
		switch {
		case errors.Is(err, modlibrary.ErrInvalidInstallation):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, modlibrary.ErrInstallationConflict), errors.Is(err, modlibrary.ErrRemoteInstallation):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeAllocationError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"serverId": server.ID, "generation": server.Spec.Generation, "modIds": server.Spec.ModIDs, "state": "requested"})
}
