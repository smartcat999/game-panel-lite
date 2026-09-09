package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/commerce"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/gameconfig"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instanceapp"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regions"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type createTenantInstanceRequest struct {
	OrganizationID string          `json:"organizationId"`
	Name           string          `json:"name"`
	PlanID         string          `json:"planId"`
	PlanVersion    int64           `json:"planVersion"`
	GameVersion    string          `json:"gameVersion,omitempty"`
	IdempotencyKey string          `json:"idempotencyKey,omitempty"`
	Configuration  json.RawMessage `json:"configuration"`
}

type createTenantInstanceResponse struct {
	InstanceID      string `json:"instanceId"`
	OperationID     string `json:"operationId"`
	OperationStatus string `json:"operationStatus"`
	RevisionID      string `json:"revisionId"`
	SpecGeneration  int64  `json:"specGeneration"`
	IntentVersion   int64  `json:"intentVersion"`
	RegionID        string `json:"regionId"`
}

func (h *Handler) createTenantInstance(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if h.instanceCommands == nil {
		writeError(w, http.StatusServiceUnavailable, "logical instance creation is not configured")
		return
	}
	maxConfigurationBytes := h.cfg.LogicalConfigMaxBytes
	if maxConfigurationBytes <= 0 || maxConfigurationBytes > 4<<20 {
		maxConfigurationBytes = 1 << 20
	}
	var request createTenantInstanceRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, int64(maxConfigurationBytes)+(16<<10)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || len(request.Configuration) == 0 || len(request.Configuration) > maxConfigurationBytes {
		writeError(w, http.StatusBadRequest, "invalid logical instance request")
		return
	}
	headerKey := r.Header.Get("Idempotency-Key")
	if request.IdempotencyKey != "" && headerKey != "" && request.IdempotencyKey != headerKey {
		writeError(w, http.StatusBadRequest, "conflicting idempotency keys")
		return
	}
	if request.IdempotencyKey == "" {
		request.IdempotencyKey = headerKey
	}
	result, err := h.instanceCommands.Create(r.Context(), account.ID, instanceapp.CreateCommand{
		OrganizationID: request.OrganizationID,
		Name:           request.Name,
		PlanID:         request.PlanID,
		PlanVersion:    request.PlanVersion,
		GameVersion:    request.GameVersion,
		IdempotencyKey: request.IdempotencyKey,
		Configuration:  request.Configuration,
	})
	if err != nil {
		writeInstanceCommandError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, createTenantInstanceResponse{
		InstanceID: result.Server.ID, OperationID: result.Operation.ID, OperationStatus: result.Operation.Status,
		RevisionID: result.Revision.ID, SpecGeneration: result.Server.SpecGeneration, IntentVersion: result.Server.IntentVersion,
		RegionID: result.Placement.RegionID,
	})
}

func writeInstanceCommandError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, instanceapp.ErrInvalidCreateCommand), errors.Is(err, instances.ErrInvalidIntent), errors.Is(err, gameconfig.ErrInvalidLogicalConfiguration):
		writeError(w, http.StatusBadRequest, "invalid logical instance request")
	case errors.Is(err, store.ErrWorkspaceWriteDenied):
		writeError(w, http.StatusForbidden, "workspace write permission required")
	case errors.Is(err, commerce.ErrPlanUnavailable), errors.Is(err, commerce.ErrInvalidPlan), errors.Is(err, regions.ErrRegionUnavailable),
		errors.Is(err, assets.ErrUnavailable), errors.Is(err, store.ErrQuotaExceeded), errors.Is(err, store.ErrFiniteResourcesRequired):
		writeError(w, http.StatusConflict, "logical instance is not currently admissible")
	case errors.Is(err, instances.ErrIdempotencyConflict), errors.Is(err, instances.ErrVersionConflict):
		writeError(w, http.StatusConflict, "logical instance request conflicts with existing state")
	default:
		writeError(w, http.StatusInternalServerError, "logical instance creation unavailable")
	}
}
