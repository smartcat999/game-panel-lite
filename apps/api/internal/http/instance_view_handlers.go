package http

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instanceview"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type instanceOperationResponse struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

type instanceDeploymentResponse struct {
	OperationID string    `json:"operationId"`
	ActualState string    `json:"actualState"`
	Outcome     string    `json:"outcome"`
	ObservedAt  time.Time `json:"observedAt"`
}

type instanceViewResponse struct {
	ID                  string                      `json:"id"`
	OrganizationID      string                      `json:"organizationId"`
	Name                string                      `json:"name"`
	ProviderKey         string                      `json:"providerKey"`
	GameVersion         string                      `json:"gameVersion"`
	ConfigSchemaVersion int                         `json:"configSchemaVersion"`
	CPU                 float64                     `json:"cpu"`
	MemoryMB            int64                       `json:"memoryMb"`
	DesiredState        string                      `json:"desiredState"`
	RegionID            string                      `json:"regionId"`
	RevisionID          string                      `json:"revisionId"`
	SpecGeneration      int64                       `json:"specGeneration"`
	IntentVersion       int64                       `json:"intentVersion"`
	PlacementEpoch      int64                       `json:"placementEpoch"`
	CreatedAt           time.Time                   `json:"createdAt"`
	LatestOperation     *instanceOperationResponse  `json:"latestOperation,omitempty"`
	Deployment          *instanceDeploymentResponse `json:"deployment,omitempty"`
	NodeID              string                      `json:"nodeId,omitempty"`
	TaskID              string                      `json:"taskId,omitempty"`
}

type instanceViewPageResponse struct {
	Items      []instanceViewResponse `json:"items"`
	NextCursor string                 `json:"nextCursor,omitempty"`
}

func (h *Handler) listTenantInstances(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	organizationID, after, limit, ok := parseInstanceViewPage(w, r, true)
	if !ok {
		return
	}
	page, err := h.instanceViews.ListTenantInstanceViews(r.Context(), account.ID, organizationID, after, limit)
	if err != nil {
		writeInstanceViewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tenantInstancePageResponse(page))
}

func (h *Handler) getTenantInstance(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	organizationID, ok := singleInstanceQuery(w, r, "organizationId", true)
	if !ok {
		return
	}
	if len(r.URL.Query()) != 1 {
		writeError(w, http.StatusBadRequest, "unsupported instance query parameter")
		return
	}
	record, err := h.instanceViews.GetTenantInstanceView(r.Context(), account.ID, organizationID, chi.URLParam(r, "id"))
	if err != nil {
		writeInstanceViewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tenantInstanceResponse(record))
}

func (h *Handler) listPlatformInstances(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	organizationID, after, limit, ok := parseInstanceViewPage(w, r, false)
	if !ok {
		return
	}
	page, err := h.instanceViews.ListPlatformInstanceViews(r.Context(), account.ID, organizationID, after, limit)
	if err != nil {
		writeInstanceViewError(w, err)
		return
	}
	items := make([]instanceViewResponse, len(page.Items))
	for i := range page.Items {
		items[i] = platformInstanceResponse(page.Items[i])
	}
	writeJSON(w, http.StatusOK, instanceViewPageResponse{Items: items, NextCursor: page.NextCursor})
}

func (h *Handler) getPlatformInstance(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if len(r.URL.Query()) != 0 {
		writeError(w, http.StatusBadRequest, "unsupported instance query parameter")
		return
	}
	record, err := h.instanceViews.GetPlatformInstanceView(r.Context(), account.ID, chi.URLParam(r, "id"))
	if err != nil {
		writeInstanceViewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, platformInstanceResponse(record))
}

func parseInstanceViewPage(w http.ResponseWriter, r *http.Request, requireOrganization bool) (string, string, int, bool) {
	for key := range r.URL.Query() {
		if key != "organizationId" && key != "after" && key != "limit" {
			writeError(w, http.StatusBadRequest, "unsupported instance query parameter")
			return "", "", 0, false
		}
	}
	organizationID, ok := singleInstanceQuery(w, r, "organizationId", requireOrganization)
	if !ok {
		return "", "", 0, false
	}
	after, ok := singleInstanceQuery(w, r, "after", false)
	if !ok {
		return "", "", 0, false
	}
	rawLimit, ok := singleInstanceQuery(w, r, "limit", false)
	if !ok {
		return "", "", 0, false
	}
	limit := 50
	if rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(w, http.StatusBadRequest, "instance limit must be between 1 and 100")
			return "", "", 0, false
		}
		limit = parsed
	}
	return organizationID, after, limit, true
}

func singleInstanceQuery(w http.ResponseWriter, r *http.Request, key string, required bool) (string, bool) {
	values, present := r.URL.Query()[key]
	if !present {
		if required {
			writeError(w, http.StatusBadRequest, key+" is required")
			return "", false
		}
		return "", true
	}
	if len(values) != 1 || values[0] != strings.TrimSpace(values[0]) || len(values[0]) > 128 || (required && values[0] == "") {
		writeError(w, http.StatusBadRequest, "invalid "+key)
		return "", false
	}
	return values[0], true
}

func tenantInstancePageResponse(page instanceview.Page[instanceview.Record]) instanceViewPageResponse {
	items := make([]instanceViewResponse, len(page.Items))
	for i := range page.Items {
		items[i] = tenantInstanceResponse(page.Items[i])
	}
	return instanceViewPageResponse{Items: items, NextCursor: page.NextCursor}
}

func tenantInstanceResponse(record instanceview.Record) instanceViewResponse {
	response := instanceViewResponse{
		ID: record.ID, OrganizationID: record.OrganizationID, Name: record.Name, ProviderKey: record.ProviderKey,
		GameVersion: record.GameVersion, ConfigSchemaVersion: record.ConfigSchemaVersion,
		CPU: record.Resources.CPU, MemoryMB: record.Resources.MemoryMB, DesiredState: record.DesiredState,
		RegionID: record.RegionID, RevisionID: record.RevisionID, SpecGeneration: record.SpecGeneration,
		IntentVersion: record.IntentVersion, PlacementEpoch: record.PlacementEpoch, CreatedAt: record.CreatedAt,
	}
	if record.LatestOperation != nil {
		response.LatestOperation = &instanceOperationResponse{ID: record.LatestOperation.ID, Kind: record.LatestOperation.Kind, Status: record.LatestOperation.Status, CreatedAt: record.LatestOperation.CreatedAt}
	}
	if record.Deployment != nil {
		response.Deployment = &instanceDeploymentResponse{OperationID: record.Deployment.OperationID, ActualState: record.Deployment.ActualState, Outcome: record.Deployment.Outcome, ObservedAt: record.Deployment.ObservedAt}
	}
	return response
}

func platformInstanceResponse(record instanceview.PlatformRecord) instanceViewResponse {
	response := tenantInstanceResponse(record.Record)
	response.NodeID, response.TaskID = record.NodeID, record.TaskID
	return response
}

func writeInstanceViewError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, instanceview.ErrInvalidQuery):
		writeError(w, http.StatusBadRequest, "invalid instance query")
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "instance not found")
	case errors.Is(err, instanceview.ErrOperatorRequired):
		writeError(w, http.StatusForbidden, "platform operator required")
	default:
		writeError(w, http.StatusInternalServerError, "instance view unavailable")
	}
}
