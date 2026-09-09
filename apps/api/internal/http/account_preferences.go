package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

type accountPreferencesResponse struct {
	Locale string `json:"locale"`
	Theme  string `json:"theme"`
}

func (h *Handler) getAccountPreferences(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	preferences, err := h.store.GetAccountPreferences(r.Context(), account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, accountPreferencesResponse{Locale: preferences.Locale, Theme: preferences.Theme})
}

func (h *Handler) updateAccountPreferences(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var payload accountPreferencesResponse
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	payload.Locale = strings.TrimSpace(payload.Locale)
	payload.Theme = strings.TrimSpace(payload.Theme)
	if payload.Locale != "zh" && payload.Locale != "en" {
		writeError(w, http.StatusBadRequest, "locale must be zh or en")
		return
	}
	if payload.Theme != "light" && payload.Theme != "dark" && payload.Theme != "system" {
		writeError(w, http.StatusBadRequest, "theme must be light, dark, or system")
		return
	}
	preferences := domain.AccountPreferences{
		AccountID: account.ID,
		Locale:    payload.Locale,
		Theme:     payload.Theme,
	}
	if err := h.store.SaveAccountPreferences(r.Context(), preferences); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, payload)
}
