package runtime

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/smartcat999/game-panel-cloud/pkg/models"
)

func WriteModelError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, models.ErrUnauthorized):
		WriteError(w, http.StatusUnauthorized, err.Error())
	case errors.Is(err, models.ErrForbidden):
		WriteError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, models.ErrNotFound):
		WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, models.ErrAlreadyExists), errors.Is(err, models.ErrConflict):
		WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, models.ErrBadGateway):
		WriteError(w, http.StatusBadGateway, err.Error())
	default:
		WriteError(w, http.StatusBadRequest, err.Error())
	}
}

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]any{
		"apiVersion": "v1",
		"kind":       "Status",
		"status":     "Failure",
		"code":       status,
		"message":    message,
	})
}
