package controlapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/serviceauth"
)

type RevisionReader interface {
	GetRegionalRevision(context.Context, string, instances.RevisionAvailable) (regional.RevisionSnapshot, error)
}

func NewRevisionHandler(reader RevisionReader, identities *serviceauth.Regions, maxBytes int64) (http.Handler, error) {
	if reader == nil || identities == nil || maxBytes < 1 {
		return nil, errors.New("invalid revision API configuration")
	}
	router := chi.NewRouter()
	router.Post("/internal/region/revisions/resolve", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		region, err := identities.Authenticate(r)
		if err != nil {
			http.Error(w, "service identity required", http.StatusUnauthorized)
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
		snapshot, err := reader.GetRegionalRevision(r.Context(), region, event)
		if errors.Is(err, regional.ErrRevisionUnavailable) {
			http.Error(w, "revision unavailable", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "revision service unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snapshot)
	})
	return router, nil
}
