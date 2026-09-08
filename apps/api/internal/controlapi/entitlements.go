package controlapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/entitlements"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/serviceauth"
)

type EntitlementReader interface {
	RegionalRunEntitlement(context.Context, string, instances.RevisionAvailable, int64) (entitlements.Record, error)
}

func NewEntitlementHandler(reader EntitlementReader, identities *serviceauth.Regions, maxBytes int64) (http.Handler, error) {
	if reader == nil || identities == nil || maxBytes < 1 {
		return nil, errors.New("invalid entitlement API configuration")
	}
	router := chi.NewRouter()
	router.Post("/internal/region/entitlements/resolve", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		region, err := identities.Authenticate(r)
		if err != nil {
			http.Error(w, "service identity required", http.StatusUnauthorized)
			return
		}
		query, err := url.ParseQuery(r.URL.RawQuery)
		intent, parseErr := strconv.ParseInt(query.Get("intentVersion"), 10, 64)
		if err != nil || parseErr != nil || intent < 1 || len(query) != 1 || len(query["intentVersion"]) != 1 || strconv.FormatInt(intent, 10) != query.Get("intentVersion") {
			http.Error(w, "invalid intent version", http.StatusBadRequest)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			http.Error(w, "JSON required", http.StatusUnsupportedMediaType)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			var limit *http.MaxBytesError
			if errors.As(err, &limit) {
				http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			} else {
				http.Error(w, "invalid event", http.StatusBadRequest)
			}
			return
		}
		event, err := regional.DecodeRevisionNotification(body)
		if err != nil {
			http.Error(w, "invalid event", http.StatusBadRequest)
			return
		}
		if event.RegionID != region {
			http.Error(w, "region mismatch", http.StatusForbidden)
			return
		}
		snapshot, err := reader.RegionalRunEntitlement(r.Context(), region, event, intent)
		if errors.Is(err, entitlements.ErrUnavailable) {
			http.Error(w, "entitlement unavailable", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "entitlement service unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snapshot)
	})
	return router, nil
}
