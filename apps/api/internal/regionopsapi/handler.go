// Package regionopsapi exposes Region-owned operational reads to an
// authenticated global control plane. It is separate from the Node Agent API.
package regionopsapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/serviceauth"
)

var errInvalidPageQuery = errors.New("invalid regional operations page query")

type Reader interface {
	RegionID() string
	ListRegionalNodeOperations(context.Context, string, int, time.Duration) (regional.NodeOperationsPage, error)
	ListRegionalDeploymentOperations(context.Context, string, int) (regional.DeploymentOperationsPage, error)
}

func NewHandler(reader Reader, identities *serviceauth.GlobalControls, freshness time.Duration) (http.Handler, error) {
	if reader == nil || identities == nil || reader.RegionID() == "" || freshness < time.Second || freshness > time.Hour {
		return nil, errors.New("invalid regional operations API configuration")
	}
	router := chi.NewRouter()
	router.Get("/internal/operations/nodes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if err := identities.Authenticate(r); err != nil {
			http.Error(w, "global control identity required", http.StatusUnauthorized)
			return
		}
		after, limit, err := pageQuery(r, 50)
		if err != nil {
			http.Error(w, "invalid node query", http.StatusBadRequest)
			return
		}
		page, err := reader.ListRegionalNodeOperations(r.Context(), after, limit, freshness)
		if errors.Is(err, regional.ErrInvalidNodeOperations) {
			http.Error(w, "invalid node query", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "regional node operations unavailable", http.StatusServiceUnavailable)
			return
		}
		if page.RegionID != reader.RegionID() || page.Validate() != nil {
			http.Error(w, "regional node operations unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(page)
	})
	router.Get("/internal/operations/deployments", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if err := identities.Authenticate(r); err != nil {
			http.Error(w, "global control identity required", http.StatusUnauthorized)
			return
		}
		after, limit, err := pageQuery(r, 50)
		if err != nil {
			http.Error(w, "invalid deployment query", http.StatusBadRequest)
			return
		}
		page, err := reader.ListRegionalDeploymentOperations(r.Context(), after, limit)
		if errors.Is(err, regional.ErrInvalidDeploymentOperations) {
			http.Error(w, "invalid deployment query", http.StatusBadRequest)
			return
		}
		if err != nil || page.RegionID != reader.RegionID() || page.Validate() != nil {
			http.Error(w, "regional deployment operations unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(page)
	})
	return router, nil
}

func pageQuery(r *http.Request, fallback int) (string, int, error) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return "", 0, err
	}
	for key, values := range query {
		if (key != "after" && key != "limit") || len(values) != 1 {
			return "", 0, errInvalidPageQuery
		}
	}
	after := query.Get("after")
	if len(after) > 128 || after != strings.TrimSpace(after) {
		return "", 0, errInvalidPageQuery
	}
	limit := fallback
	if raw := query.Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil {
			return "", 0, err
		}
	}
	if limit < 1 || limit > 200 {
		return "", 0, errInvalidPageQuery
	}
	return after, limit, nil
}
