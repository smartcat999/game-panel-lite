package nodeapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/serviceauth"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

type NodeExecution interface {
	Acquire(context.Context, string, int64, string) (*workload.AuthorizedAssignment, error)
	Renew(context.Context, string, string, int64, string, int64) (workload.LeaseGrant, error)
	Release(context.Context, string, string, int64, string, int64) error
	Observe(context.Context, string, string, int64, workload.Observation) error
}

func mountExecutionRoutes(router chi.Router, execution NodeExecution, identities *serviceauth.Nodes, maxBytes int64) {
	authenticate := func(w http.ResponseWriter, r *http.Request) (string, bool) {
		w.Header().Set("Cache-Control", "no-store")
		nodeID, err := identities.Authenticate(r)
		if err != nil {
			http.Error(w, "node identity required", http.StatusUnauthorized)
			return "", false
		}
		if r.URL.RawQuery != "" {
			http.Error(w, "unexpected query", http.StatusBadRequest)
			return "", false
		}
		return nodeID, true
	}
	decode := func(w http.ResponseWriter, r *http.Request, value any) bool {
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			http.Error(w, "JSON required", http.StatusUnsupportedMediaType)
			return false
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		defer r.Body.Close()
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(value); err != nil {
			var limit *http.MaxBytesError
			if errors.As(err, &limit) {
				http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			} else {
				http.Error(w, "invalid execution request", http.StatusBadRequest)
			}
			return false
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			http.Error(w, "invalid execution request", http.StatusBadRequest)
			return false
		}
		return true
	}
	writeFailure := func(w http.ResponseWriter, err error) {
		if errors.Is(err, regional.ErrExecutionUnavailable) || errors.Is(err, regional.ErrExecutionLeaseLost) {
			http.Error(w, "execution authority unavailable", http.StatusConflict)
			return
		}
		http.Error(w, "execution authority temporarily unavailable", http.StatusServiceUnavailable)
	}

	router.Post("/internal/node/assignments/claim", func(w http.ResponseWriter, r *http.Request) {
		nodeID, ok := authenticate(w, r)
		if !ok {
			return
		}
		var request workload.RegionalAssignmentRequest
		if !decode(w, r, &request) {
			return
		}
		if request.SessionEpoch < 1 || request.HolderID == "" || len(request.HolderID) > 128 {
			http.Error(w, "invalid execution request", http.StatusBadRequest)
			return
		}
		grant, err := execution.Acquire(r.Context(), nodeID, request.SessionEpoch, request.HolderID)
		if err != nil {
			writeFailure(w, err)
			return
		}
		if grant == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(grant)
	})

	router.Post("/internal/node/assignments/{taskID}/lease", func(w http.ResponseWriter, r *http.Request) {
		nodeID, ok := authenticate(w, r)
		if !ok {
			return
		}
		var request workload.RegionalLeaseRequest
		if !decode(w, r, &request) {
			return
		}
		if request.SessionEpoch < 1 || request.HolderID == "" || len(request.HolderID) > 128 || request.Fence < 1 {
			http.Error(w, "invalid execution request", http.StatusBadRequest)
			return
		}
		taskID := chi.URLParam(r, "taskID")
		switch request.Action {
		case "renew":
			grant, err := execution.Renew(r.Context(), nodeID, taskID, request.SessionEpoch, request.HolderID, request.Fence)
			if err != nil {
				writeFailure(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(grant)
		case "release":
			if err := execution.Release(r.Context(), nodeID, taskID, request.SessionEpoch, request.HolderID, request.Fence); err != nil {
				writeFailure(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "invalid lease action", http.StatusBadRequest)
		}
	})

	router.Post("/internal/node/assignments/{taskID}/observation", func(w http.ResponseWriter, r *http.Request) {
		nodeID, ok := authenticate(w, r)
		if !ok {
			return
		}
		var report workload.RegionalObservationReport
		if !decode(w, r, &report) {
			return
		}
		if report.SessionEpoch < 1 || report.Observation.LeaseHolderID == "" || report.Observation.LeaseFence < 1 {
			http.Error(w, "invalid execution request", http.StatusBadRequest)
			return
		}
		if err := execution.Observe(r.Context(), nodeID, chi.URLParam(r, "taskID"), report.SessionEpoch, report.Observation); err != nil {
			writeFailure(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
