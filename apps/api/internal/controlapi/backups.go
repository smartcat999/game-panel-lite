package controlapi

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/backupingress"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/serviceauth"
)

type BackupChecker interface {
	CheckRegionalBackup(context.Context, string, backup.Requested) error
}

// NewBackupHandler exposes a point-in-time check, never an execution grant.
func NewBackupHandler(checker BackupChecker, identities *serviceauth.Regions, maxBytes int64) (http.Handler, error) {
	if checker == nil || identities == nil || maxBytes < 1 {
		return nil, errors.New("invalid backup check configuration")
	}
	router := chi.NewRouter()
	router.Post("/internal/region/backups/check", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		region, err := identities.Authenticate(r)
		if err != nil {
			http.Error(w, "service identity required", http.StatusUnauthorized)
			return
		}
		if r.URL.RawQuery != "" {
			http.Error(w, "unexpected query", http.StatusBadRequest)
			return
		}
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
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
				http.Error(w, "invalid request", http.StatusBadRequest)
			}
			return
		}
		request, err := backupingress.DecodeRequest(body)
		if err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if region != request.RegionID {
			http.Error(w, "region mismatch", http.StatusForbidden)
			return
		}
		err = checker.CheckRegionalBackup(r.Context(), region, request)
		if errors.Is(err, backup.ErrRequestUnavailable) {
			http.Error(w, "backup request unavailable", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "backup check unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return router, nil
}
