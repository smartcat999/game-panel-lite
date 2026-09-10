package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/backupcontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/commerce"
	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/globalproduct"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/identity"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instancecontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regiondirectory"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/workspace"
)

type IdentityModule interface {
	RestoreSession(context.Context, string) (identity.Session, error)
	UserPreferences(context.Context, contract.UserID) (identity.Preferences, error)
	UpdateUserPreferences(context.Context, contract.UserID, identity.Preferences) (identity.Preferences, error)
	IsPlatformOperator(context.Context, contract.UserID) bool
	UsersByID(context.Context, []contract.UserID) []identity.User
}

type WorkspaceModule interface {
	ListForUser(context.Context, contract.UserID) []workspace.Workspace
	Select(context.Context, contract.UserID, contract.WorkspaceID) error
	Selected(context.Context, contract.UserID) (contract.WorkspaceID, bool)
	Members(context.Context, contract.UserID, contract.WorkspaceID) ([]workspace.Membership, error)
	All(context.Context) []workspace.Workspace
}

type ProductModule interface {
	CreateCheckout(context.Context, globalproduct.CreateCommand, time.Time) (globalproduct.CheckoutResult, error)
	ActivateVerifiedPayment(context.Context, contract.OrderID, string, bool, time.Time) (globalproduct.ActivationResult, error)
	Regions(context.Context) ([]regiondirectory.Region, error)
	Plans(context.Context) ([]commerce.PlanVersion, error)
	Instances(context.Context, contract.WorkspaceID) ([]instancecontrol.LogicalInstance, error)
	Instance(context.Context, contract.WorkspaceID, contract.LogicalInstanceID) (instancecontrol.Detail, error)
	Orders(context.Context, contract.WorkspaceID) ([]commerce.Order, error)
	AllInstances(context.Context) ([]instancecontrol.LogicalInstance, error)
	AllOrders(context.Context) ([]commerce.Order, error)
	RequestBackup(context.Context, backupcontrol.CreateCommand, time.Time) (backupcontrol.Request, error)
	Backups(context.Context, contract.WorkspaceID) ([]backupcontrol.Request, error)
}

type Handler struct {
	identity  IdentityModule
	workspace WorkspaceModule
	product   ProductModule
}

func NewHandler(identityModule IdentityModule, workspaceModule WorkspaceModule, productModules ...ProductModule) http.Handler {
	handler := Handler{identity: identityModule, workspace: workspaceModule}
	if len(productModules) > 0 {
		handler.product = productModules[0]
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/session", handler.getSession)
	mux.HandleFunc("GET /v1/user-preferences", handler.getPreferences)
	mux.HandleFunc("PATCH /v1/user-preferences", handler.updatePreferences)
	mux.HandleFunc("GET /v1/workspaces", handler.listWorkspaces)
	mux.HandleFunc("POST /v1/workspace-selection", handler.selectWorkspace)
	mux.HandleFunc("GET /v1/workspaces/{workspaceId}/members", handler.listMembers)
	mux.HandleFunc("GET /v1/platform/regions", handler.listPlatformRegions)
	mux.HandleFunc("GET /v1/regions", handler.listRegions)
	mux.HandleFunc("GET /v1/plans", handler.listPlans)
	mux.HandleFunc("POST /v1/instances", handler.createInstance)
	mux.HandleFunc("GET /v1/workspaces/{workspaceId}/instances", handler.listInstances)
	mux.HandleFunc("GET /v1/workspaces/{workspaceId}/instances/{instanceId}", handler.getInstance)
	mux.HandleFunc("GET /v1/workspaces/{workspaceId}/orders", handler.listOrders)
	mux.HandleFunc("GET /v1/workspaces/{workspaceId}/backups", handler.listBackups)
	mux.HandleFunc("POST /v1/workspaces/{workspaceId}/backups", handler.createBackup)
	mux.HandleFunc("GET /v1/platform/workspaces", handler.listPlatformWorkspaces)
	mux.HandleFunc("GET /v1/platform/plans", handler.listPlatformPlans)
	mux.HandleFunc("GET /v1/platform/orders", handler.listPlatformOrders)
	mux.HandleFunc("GET /v1/platform/instances", handler.listPlatformInstances)
	mux.HandleFunc("POST /v1/platform/payments/verified", handler.activatePayment)
	return mux
}

func (h Handler) getSession(w http.ResponseWriter, request *http.Request) {
	userID, ok := h.authenticate(w, request)
	if !ok {
		return
	}
	workspaces := h.workspace.ListForUser(request.Context(), userID)
	workspaceIDs := make([]contract.WorkspaceID, 0, len(workspaces))
	for _, item := range workspaces {
		workspaceIDs = append(workspaceIDs, item.ID)
	}
	selectedWorkspaceID, _ := h.workspace.Selected(request.Context(), userID)
	writeJSON(w, http.StatusOK, map[string]any{
		"userId":              userID,
		"workspaceIds":        workspaceIDs,
		"selectedWorkspaceId": selectedWorkspaceID,
		"platformOperator":    h.identity.IsPlatformOperator(request.Context(), userID),
	})
}

func (h Handler) getPreferences(w http.ResponseWriter, request *http.Request) {
	userID, ok := h.authenticate(w, request)
	if !ok {
		return
	}
	preferences, err := h.identity.UserPreferences(request.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user_not_found")
		return
	}
	writeJSON(w, http.StatusOK, preferences)
}

func (h Handler) updatePreferences(w http.ResponseWriter, request *http.Request) {
	userID, ok := h.authenticate(w, request)
	if !ok {
		return
	}
	current, err := h.identity.UserPreferences(request.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user_not_found")
		return
	}
	var update struct {
		Locale   *string `json:"locale"`
		Theme    *string `json:"theme"`
		TimeZone *string `json:"timeZone"`
	}
	if err := json.NewDecoder(request.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if update.Locale != nil {
		current.Locale = *update.Locale
	}
	if update.Theme != nil {
		current.Theme = *update.Theme
	}
	if update.TimeZone != nil {
		current.TimeZone = *update.TimeZone
	}
	preferences, err := h.identity.UpdateUserPreferences(request.Context(), userID, current)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_preferences")
		return
	}
	writeJSON(w, http.StatusOK, preferences)
}

func (h Handler) listWorkspaces(w http.ResponseWriter, request *http.Request) {
	userID, ok := h.authenticate(w, request)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, h.workspace.ListForUser(request.Context(), userID))
}

func (h Handler) selectWorkspace(w http.ResponseWriter, request *http.Request) {
	userID, ok := h.authenticate(w, request)
	if !ok {
		return
	}
	var command struct {
		WorkspaceID contract.WorkspaceID `json:"workspaceId"`
	}
	if err := json.NewDecoder(request.Body).Decode(&command); err != nil || command.WorkspaceID == "" {
		writeError(w, http.StatusBadRequest, "invalid_workspace_selection")
		return
	}
	if err := h.workspace.Select(request.Context(), userID, command.WorkspaceID); err != nil {
		writeWorkspaceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) listMembers(w http.ResponseWriter, request *http.Request) {
	userID, ok := h.authenticate(w, request)
	if !ok {
		return
	}
	members, err := h.workspace.Members(request.Context(), userID, contract.WorkspaceID(request.PathValue("workspaceId")))
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	userIDs := make([]contract.UserID, 0, len(members))
	for _, membership := range members {
		userIDs = append(userIDs, membership.UserID)
	}
	users := h.identity.UsersByID(request.Context(), userIDs)
	usersByID := make(map[contract.UserID]identity.User, len(users))
	for _, user := range users {
		usersByID[user.ID] = user
	}
	type memberView struct {
		MembershipID contract.MembershipID `json:"membershipId"`
		User         identity.User         `json:"user"`
		Role         workspace.Role        `json:"role"`
	}
	result := make([]memberView, 0, len(members))
	for _, membership := range members {
		result = append(result, memberView{MembershipID: membership.ID, User: usersByID[membership.UserID], Role: membership.Role})
	}
	writeJSON(w, http.StatusOK, result)
}

func (h Handler) listPlatformRegions(w http.ResponseWriter, request *http.Request) {
	if _, ok := h.requirePlatformOperator(w, request); !ok {
		return
	}
	if h.product == nil {
		writeJSON(w, http.StatusOK, []any{})
	} else {
		regions, err := h.product.Regions(request.Context())
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "product_unavailable")
			return
		}
		type platformRegion struct {
			ID               contract.RegionID `json:"id"`
			Code             string            `json:"code"`
			Name             string            `json:"name"`
			OperationalState string            `json:"operationalState"`
		}
		result := make([]platformRegion, 0, len(regions))
		for _, region := range regions {
			state := "disabled"
			if region.Available {
				state = "active"
			}
			result = append(result, platformRegion{ID: region.ID, Code: region.Code, Name: region.Name, OperationalState: state})
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func (h Handler) listRegions(w http.ResponseWriter, request *http.Request) {
	if _, ok := h.authenticate(w, request); !ok {
		return
	}
	regions, err := h.product.Regions(request.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "product_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, regions)
}

func (h Handler) listPlans(w http.ResponseWriter, request *http.Request) {
	if _, ok := h.authenticate(w, request); !ok {
		return
	}
	plans, err := h.product.Plans(request.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "product_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, plans)
}

func (h Handler) createInstance(w http.ResponseWriter, request *http.Request) {
	userID, ok := h.authenticate(w, request)
	if !ok {
		return
	}
	var input struct {
		WorkspaceID   contract.WorkspaceID   `json:"workspaceId"`
		PlanVersionID contract.PlanVersionID `json:"planVersionId"`
		RegionID      contract.RegionID      `json:"regionId"`
		Name          string                 `json:"name"`
		GameKey       string                 `json:"gameKey"`
		GameVersion   string                 `json:"gameVersion"`
		Configuration map[string]any         `json:"configuration"`
	}
	if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if _, err := h.workspace.Members(request.Context(), userID, input.WorkspaceID); err != nil {
		writeWorkspaceError(w, err)
		return
	}
	key := contract.IdempotencyKey(request.Header.Get("Idempotency-Key"))
	if key == "" {
		writeError(w, http.StatusBadRequest, "idempotency_key_required")
		return
	}
	result, err := h.product.CreateCheckout(request.Context(), globalproduct.CreateCommand{
		Identity:    contract.CommandIdentity{CommandID: contract.CommandID(request.Header.Get("X-Command-ID")), IdempotencyKey: key},
		WorkspaceID: input.WorkspaceID, PlanVersionID: input.PlanVersionID, RegionID: input.RegionID,
		Name: input.Name, GameKey: input.GameKey, GameVersion: input.GameVersion, Configuration: input.Configuration,
	}, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusBadRequest, "checkout_rejected")
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h Handler) listInstances(w http.ResponseWriter, request *http.Request) {
	userID, ok := h.authenticate(w, request)
	if !ok {
		return
	}
	workspaceID := contract.WorkspaceID(request.PathValue("workspaceId"))
	if _, err := h.workspace.Members(request.Context(), userID, workspaceID); err != nil {
		writeWorkspaceError(w, err)
		return
	}
	instances, err := h.product.Instances(request.Context(), workspaceID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "product_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, instances)
}

func (h Handler) getInstance(w http.ResponseWriter, request *http.Request) {
	userID, ok := h.authenticate(w, request)
	if !ok {
		return
	}
	workspaceID := contract.WorkspaceID(request.PathValue("workspaceId"))
	if _, err := h.workspace.Members(request.Context(), userID, workspaceID); err != nil {
		writeWorkspaceError(w, err)
		return
	}
	detail, err := h.product.Instance(request.Context(), workspaceID, contract.LogicalInstanceID(request.PathValue("instanceId")))
	if err != nil {
		writeError(w, http.StatusNotFound, "instance_not_found")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h Handler) listOrders(w http.ResponseWriter, request *http.Request) {
	userID, ok := h.authenticate(w, request)
	if !ok {
		return
	}
	workspaceID := contract.WorkspaceID(request.PathValue("workspaceId"))
	if _, err := h.workspace.Members(request.Context(), userID, workspaceID); err != nil {
		writeWorkspaceError(w, err)
		return
	}
	orders, err := h.product.Orders(request.Context(), workspaceID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "product_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

func (h Handler) listBackups(w http.ResponseWriter, request *http.Request) {
	userID, ok := h.authenticate(w, request)
	if !ok {
		return
	}
	workspaceID := contract.WorkspaceID(request.PathValue("workspaceId"))
	if _, err := h.workspace.Members(request.Context(), userID, workspaceID); err != nil {
		writeWorkspaceError(w, err)
		return
	}
	backups, err := h.product.Backups(request.Context(), workspaceID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "product_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, backups)
}

func (h Handler) createBackup(w http.ResponseWriter, request *http.Request) {
	userID, ok := h.authenticate(w, request)
	if !ok {
		return
	}
	workspaceID := contract.WorkspaceID(request.PathValue("workspaceId"))
	if _, err := h.workspace.Members(request.Context(), userID, workspaceID); err != nil {
		writeWorkspaceError(w, err)
		return
	}
	var input struct {
		LogicalInstanceID     contract.LogicalInstanceID `json:"logicalInstanceId"`
		RegionID              contract.RegionID          `json:"regionId"`
		Kind                  backupcontrol.Kind         `json:"kind"`
		SourceBackupRequestID contract.BackupRequestID   `json:"sourceBackupRequestId"`
	}
	if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	key := contract.IdempotencyKey(request.Header.Get("Idempotency-Key"))
	result, err := h.product.RequestBackup(request.Context(), backupcontrol.CreateCommand{Identity: contract.CommandIdentity{CommandID: contract.CommandID(request.Header.Get("X-Command-ID")), IdempotencyKey: key}, WorkspaceID: workspaceID, LogicalInstanceID: input.LogicalInstanceID, RegionID: input.RegionID, Kind: input.Kind, SourceBackupRequestID: input.SourceBackupRequestID}, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusBadRequest, "backup_request_rejected")
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (h Handler) listPlatformWorkspaces(w http.ResponseWriter, request *http.Request) {
	if _, ok := h.requirePlatformOperator(w, request); !ok {
		return
	}
	writeJSON(w, http.StatusOK, h.workspace.All(request.Context()))
}

func (h Handler) listPlatformPlans(w http.ResponseWriter, request *http.Request) {
	if _, ok := h.requirePlatformOperator(w, request); !ok {
		return
	}
	plans, err := h.product.Plans(request.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "product_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, plans)
}

func (h Handler) listPlatformOrders(w http.ResponseWriter, request *http.Request) {
	if _, ok := h.requirePlatformOperator(w, request); !ok {
		return
	}
	orders, err := h.product.AllOrders(request.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "product_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

func (h Handler) listPlatformInstances(w http.ResponseWriter, request *http.Request) {
	if _, ok := h.requirePlatformOperator(w, request); !ok {
		return
	}
	instances, err := h.product.AllInstances(request.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "product_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, instances)
}

func (h Handler) activatePayment(w http.ResponseWriter, request *http.Request) {
	if _, ok := h.requirePlatformOperator(w, request); !ok {
		return
	}
	var input struct {
		OrderID                contract.OrderID `json:"orderId"`
		ProviderNotificationID string           `json:"providerNotificationId"`
		Verified               bool             `json:"verified"`
	}
	if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	result, err := h.product.ActivateVerifiedPayment(request.Context(), input.OrderID, input.ProviderNotificationID, input.Verified, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusBadRequest, "payment_rejected")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h Handler) requirePlatformOperator(w http.ResponseWriter, request *http.Request) (contract.UserID, bool) {
	userID, ok := h.authenticate(w, request)
	if !ok {
		return "", false
	}
	if !h.identity.IsPlatformOperator(request.Context(), userID) {
		writeError(w, http.StatusForbidden, "platform_authority_required")
		return "", false
	}
	return userID, true
}

func (h Handler) authenticate(w http.ResponseWriter, request *http.Request) (contract.UserID, bool) {
	token := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		writeError(w, http.StatusUnauthorized, "session_required")
		return "", false
	}
	session, err := h.identity.RestoreSession(request.Context(), token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_session")
		return "", false
	}
	return session.UserID, true
}

func writeWorkspaceError(w http.ResponseWriter, err error) {
	if errors.Is(err, workspace.ErrWorkspaceNotFound) {
		writeError(w, http.StatusNotFound, "workspace_not_found")
		return
	}
	writeError(w, http.StatusForbidden, "workspace_access_forbidden")
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"code": code})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
