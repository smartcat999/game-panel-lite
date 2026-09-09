package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/identity"
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
}

type Handler struct {
	identity  IdentityModule
	workspace WorkspaceModule
}

func NewHandler(identityModule IdentityModule, workspaceModule WorkspaceModule) http.Handler {
	handler := Handler{identity: identityModule, workspace: workspaceModule}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/session", handler.getSession)
	mux.HandleFunc("GET /v1/user-preferences", handler.getPreferences)
	mux.HandleFunc("PATCH /v1/user-preferences", handler.updatePreferences)
	mux.HandleFunc("GET /v1/workspaces", handler.listWorkspaces)
	mux.HandleFunc("POST /v1/workspace-selection", handler.selectWorkspace)
	mux.HandleFunc("GET /v1/workspaces/{workspaceId}/members", handler.listMembers)
	mux.HandleFunc("GET /v1/platform/regions", handler.listPlatformRegions)
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
	userID, ok := h.authenticate(w, request)
	if !ok {
		return
	}
	if !h.identity.IsPlatformOperator(request.Context(), userID) {
		writeError(w, http.StatusForbidden, "platform_authority_required")
		return
	}
	writeJSON(w, http.StatusOK, []any{})
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
