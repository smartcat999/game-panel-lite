package controlapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/serviceauth"
)

type AssetReader interface {
	ResolveRegionalAsset(context.Context, string, instances.RevisionAvailable, instances.AssetVersion) (assets.PublishedVersion, error)
}
type Reader interface {
	RevisionReader
	AssetReader
}

func NewHandler(reader Reader, identities *serviceauth.Regions, maxBytes int64) (http.Handler, error) {
	revisions, err := NewRevisionHandler(reader, identities, maxBytes)
	if err != nil {
		return nil, err
	}
	assetHandler, err := NewAssetHandler(reader, identities, maxBytes)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/internal/region/revisions/", revisions)
	mux.Handle("/internal/region/assets/", assetHandler)
	return mux, nil
}

func NewAssetHandler(reader AssetReader, identities *serviceauth.Regions, maxBytes int64) (http.Handler, error) {
	if reader == nil || identities == nil || maxBytes < 1 {
		return nil, errors.New("invalid asset API configuration")
	}
	router := chi.NewRouter()
	router.Post("/internal/region/assets/resolve", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		region, err := identities.Authenticate(r)
		if err != nil {
			http.Error(w, "service identity required", http.StatusUnauthorized)
			return
		}
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(query) != 2 || len(query["assetId"]) != 1 || len(query["version"]) != 1 || query.Get("assetId") == "" || query.Get("version") == "" {
			http.Error(w, "invalid asset reference", http.StatusBadRequest)
			return
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			http.Error(w, "JSON required", http.StatusUnsupportedMediaType)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
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
		version, err := reader.ResolveRegionalAsset(r.Context(), region, event, instances.AssetVersion{AssetID: query.Get("assetId"), Version: query.Get("version")})
		if errors.Is(err, regional.ErrRevisionUnavailable) {
			http.Error(w, "asset unavailable", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "asset service unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(version)
	})
	return router, nil
}
