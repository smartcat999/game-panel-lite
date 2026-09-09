package http

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regions"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

var errInvalidRegionOperationsQuery = errors.New("invalid Region operations query")

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

// listRegionNodes routes an authenticated platform operator to the owning
// Region. Node details are never copied into the global database.
func (h *Handler) listRegionNodes(w http.ResponseWriter, r *http.Request) {
	regionID := chi.URLParam(r, "id")
	if _, err := h.store.GetRegion(r.Context(), regionID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "region not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load region")
		return
	}
	after, limit, err := regionOperationsPageQuery(r, 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid node query")
		return
	}
	if h.regionOps == nil {
		writeError(w, http.StatusServiceUnavailable, "region operations unavailable")
		return
	}
	page, err := h.regionOps.ListRegionalNodes(r.Context(), regionID, after, limit)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "region operations unavailable")
		return
	}
	if page.RegionID != regionID || page.Validate() != nil {
		writeError(w, http.StatusBadGateway, "invalid region operations response")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) listRegionDeployments(w http.ResponseWriter, r *http.Request) {
	regionID := chi.URLParam(r, "id")
	if _, err := h.store.GetRegion(r.Context(), regionID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "region not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load region")
		return
	}
	after, limit, err := regionOperationsPageQuery(r, 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid deployment query")
		return
	}
	if h.regionOps == nil {
		writeError(w, http.StatusServiceUnavailable, "region operations unavailable")
		return
	}
	page, err := h.regionOps.ListRegionalDeployments(r.Context(), regionID, after, limit)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "region operations unavailable")
		return
	}
	if page.RegionID != regionID || page.Validate() != nil {
		writeError(w, http.StatusBadGateway, "invalid region operations response")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, page)
}

func regionOperationsPageQuery(r *http.Request, fallback int) (string, int, error) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return "", 0, err
	}
	for key, values := range query {
		if (key != "after" && key != "limit") || len(values) != 1 {
			return "", 0, errInvalidRegionOperationsQuery
		}
	}
	after := query.Get("after")
	if len(after) > 128 || after != strings.TrimSpace(after) {
		return "", 0, errInvalidRegionOperationsQuery
	}
	limit := fallback
	if raw := query.Get("limit"); raw != "" {
		if raw != strings.TrimSpace(raw) {
			return "", 0, errInvalidRegionOperationsQuery
		}
		limit, err = strconv.Atoi(raw)
		if err != nil {
			return "", 0, err
		}
	}
	if limit < 1 || limit > 200 {
		return "", 0, errInvalidRegionOperationsQuery
	}
	return after, limit, nil
}
