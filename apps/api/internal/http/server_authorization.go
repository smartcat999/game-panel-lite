package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func tenantRoleCanWrite(role domain.Role) bool {
	return role == domain.RoleOwner || role == domain.RoleAdmin || role == domain.RoleMember
}

// Platform administration and pre-setup self-hosted access retain their existing
// policy. Authenticated customers must have a current membership for every route.
func (h *Handler) requireServerAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := accountFromContext(r.Context())
		if !ok || domain.NormalizeAccountRole(account.Role) == domain.RoleAdmin {
			next.ServeHTTP(w, r)
			return
		}
		role, err := h.store.ServerMembershipRole(r.Context(), account.ID, chi.URLParam(r, "id"))
		if errors.Is(err, store.ErrNotFound) || (err == nil && !tenantRoleCanWrite(role) && role != domain.RoleViewer) {
			writeError(w, http.StatusNotFound, "server not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to authorize server")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && !tenantRoleCanWrite(role) {
			writeError(w, http.StatusForbidden, "workspace write permission required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) creationOrganization(r *http.Request, requested string) (string, int, error) {
	requested = strings.TrimSpace(requested)
	account, ok := accountFromContext(r.Context())
	if !ok || domain.NormalizeAccountRole(account.Role) == domain.RoleAdmin {
		if requested == "" {
			return "", 0, nil
		}
		if _, err := h.store.GetOrganization(r.Context(), requested); err != nil {
			return "", http.StatusNotFound, errors.New("workspace not found")
		}
		return requested, 0, nil
	}
	if requested == "" {
		orgs, err := h.store.ListUserOrganizations(r.Context(), account.ID)
		if err != nil {
			return "", http.StatusInternalServerError, errors.New("failed to resolve workspace")
		}
		if len(orgs) != 1 {
			return "", http.StatusBadRequest, errors.New("select an organizationId for server creation")
		}
		requested = orgs[0].ID
	}
	member, err := h.store.GetOrganizationMember(r.Context(), requested, account.ID)
	if errors.Is(err, store.ErrNotFound) {
		return "", http.StatusNotFound, errors.New("workspace not found")
	}
	if err != nil {
		return "", http.StatusInternalServerError, errors.New("failed to authorize workspace")
	}
	if !tenantRoleCanWrite(member.Role) {
		return "", http.StatusForbidden, errors.New("workspace write permission required")
	}
	return requested, 0, nil
}

func (h *Handler) serverTransferAllowed(w http.ResponseWriter, r *http.Request, server domain.GameServer, sourceOrg string) bool {
	account, ok := accountFromContext(r.Context())
	if !ok || domain.NormalizeAccountRole(account.Role) == domain.RoleAdmin {
		return true
	}
	if server.OrganizationID == "" {
		writeError(w, http.StatusNotFound, "server not found")
		return false
	}
	_, status, err := h.creationOrganization(r, server.OrganizationID)
	if err != nil {
		writeError(w, status, err.Error())
		return false
	}
	if sourceOrg != "" && sourceOrg != server.OrganizationID {
		writeError(w, http.StatusForbidden, "source and target must belong to the same workspace")
		return false
	}
	return true
}
