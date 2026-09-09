package http

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regions"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

// listRegions returns the bounded global Region directory. It never infers
// Regions from legacy nodes because Node ownership belongs to each Region.
func (h *Handler) listRegions(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, regions.ErrInvalidRegion.Error())
			return
		}
		limit = parsed
	}
	entries, err := h.store.ListRegions(r.Context(), strings.TrimSpace(r.URL.Query().Get("after")), limit)
	if err != nil {
		if err == regions.ErrInvalidRegion {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to list regions")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// getRegionStatus returns the last asynchronously received operational
// projection. It never queries a Region database from the request path.
func (h *Handler) getRegionStatus(w http.ResponseWriter, r *http.Request) {
	regionID := chi.URLParam(r, "id")
	if _, err := h.store.GetRegion(r.Context(), regionID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "region not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load region")
		return
	}
	snapshot, err := h.store.GetRegionStatus(r.Context(), regionID)
	if errors.Is(err, store.ErrNotFound) {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load Region status")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, snapshot)
}
