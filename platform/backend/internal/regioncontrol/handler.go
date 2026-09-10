package regioncontrol

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/identity"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionexecution"
)

type IdentityModule interface {
	RestoreSession(context.Context, string) (identity.Session, error)
	IsRegionOperator(context.Context, contract.UserID, contract.RegionID) bool
}

type RegionModule interface {
	Overview(context.Context) regionexecution.Overview
	Nodes(context.Context) []regionexecution.Node
	Deployments(context.Context) []regionexecution.RegionalDeployment
	Tasks(context.Context) []regionexecution.RegionalTask
	Capacity(context.Context) regionexecution.Capacity
	Audits(context.Context) []regionexecution.AuditRecord
	Reconcile(context.Context, time.Time)
	OverridePlacement(context.Context, contract.UserID, contract.RegionalDeploymentID, contract.NodeID, string, time.Time) (regionexecution.Reservation, error)
}

type Handler struct {
	identity IdentityModule
	region   RegionModule
}

func NewHandler(identityModule IdentityModule, region RegionModule) http.Handler {
	h := Handler{identity: identityModule, region: region}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/regions/{regionId}/overview", h.overview)
	mux.HandleFunc("GET /v1/regions/{regionId}/nodes", h.nodes)
	mux.HandleFunc("GET /v1/regions/{regionId}/deployments", h.deployments)
	mux.HandleFunc("GET /v1/regions/{regionId}/tasks", h.tasks)
	mux.HandleFunc("GET /v1/regions/{regionId}/capacity", h.capacity)
	mux.HandleFunc("GET /v1/regions/{regionId}/storage", h.storage)
	mux.HandleFunc("GET /v1/regions/{regionId}/monitoring", h.monitoring)
	mux.HandleFunc("GET /v1/regions/{regionId}/audit", h.audit)
	mux.HandleFunc("POST /v1/regions/{regionId}/deployments/{deploymentId}/placement-override", h.override)
	return mux
}

func (h Handler) overview(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorize(w, r); ok {
		writeJSON(w, http.StatusOK, h.region.Overview(r.Context()))
	}
}
func (h Handler) nodes(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorize(w, r); ok {
		writeJSON(w, http.StatusOK, h.region.Nodes(r.Context()))
	}
}
func (h Handler) deployments(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorize(w, r); ok {
		writeJSON(w, http.StatusOK, h.region.Deployments(r.Context()))
	}
}
func (h Handler) tasks(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorize(w, r); ok {
		writeJSON(w, http.StatusOK, h.region.Tasks(r.Context()))
	}
}
func (h Handler) capacity(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorize(w, r); ok {
		writeJSON(w, http.StatusOK, h.region.Capacity(r.Context()))
	}
}
func (h Handler) storage(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorize(w, r); ok {
		writeJSON(w, http.StatusOK, map[string]any{"directTransfer": true, "configured": true})
	}
}
func (h Handler) monitoring(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorize(w, r); ok {
		stale := 0
		for _, node := range h.region.Nodes(r.Context()) {
			if node.State == regionexecution.NodeStale {
				stale++
			}
		}
		writeJSON(w, http.StatusOK, map[string]int{"inboxLag": 0, "outboxLag": 0, "staleNodes": stale})
	}
}
func (h Handler) audit(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorize(w, r); ok {
		writeJSON(w, http.StatusOK, h.region.Audits(r.Context()))
	}
}

func (h Handler) override(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.authorize(w, r)
	if !ok {
		return
	}
	var input struct {
		NodeID contract.NodeID `json:"nodeId"`
		Reason string          `json:"reason"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_json"})
		return
	}
	reservation, err := h.region.OverridePlacement(r.Context(), actor, contract.RegionalDeploymentID(r.PathValue("deploymentId")), input.NodeID, input.Reason, time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "placement_override_rejected"})
		return
	}
	writeJSON(w, http.StatusOK, reservation)
}

func (h Handler) authorize(w http.ResponseWriter, r *http.Request) (contract.UserID, bool) {
	regionID := contract.RegionID(r.PathValue("regionId"))
	if h.region.Overview(r.Context()).RegionID != regionID {
		writeJSON(w, http.StatusNotFound, map[string]string{"code": "region_not_found"})
		return "", false
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	session, err := h.identity.RestoreSession(r.Context(), token)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"code": "session_required"})
		return "", false
	}
	if !h.identity.IsRegionOperator(r.Context(), session.UserID, regionID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"code": "region_authority_required"})
		return "", false
	}
	return session.UserID, true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
