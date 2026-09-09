package http

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regions"
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
