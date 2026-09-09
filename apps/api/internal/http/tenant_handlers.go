package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type createOrganizationRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
	Plan string `json:"plan,omitempty"`
}

type addMemberRequest struct {
	UserID string      `json:"userId"`
	Role   domain.Role `json:"role"`
}

func (h *Handler) listOrganizations(w http.ResponseWriter, r *http.Request) {
	orgs, err := h.store.ListOrganizations(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list organizations: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, orgs)
}

func (h *Handler) getOrganization(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	org, err := h.store.GetOrganization(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "organization not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get organization: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, org)
}

func (h *Handler) createOrganization(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createOrganizationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request payload")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "organization name is required")
		return
	}
	if req.Slug == "" {
		req.Slug = uuid.NewString()[:8]
	}
	if req.Plan == "" {
		req.Plan = "starter"
	}

	org := domain.Organization{
		ID:        uuid.NewString(),
		Name:      req.Name,
		Slug:      req.Slug,
		Plan:      req.Plan,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := h.store.CreateOrganization(r.Context(), &org, account.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create organization: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, org)
}

type organizationMemberDTO struct {
	domain.OrganizationMember
	Username string `json:"username,omitempty"`
}

type createInvitationRequest struct {
	Role       domain.Role `json:"role"`
	MaxUses    int         `json:"maxUses"`
	ExpireDays int         `json:"expireDays"`
}

func (h *Handler) canManageOrganization(ctx context.Context, orgID string, account domain.AdminAccount) bool {
	if domain.NormalizeAccountRole(account.Role) == domain.RoleAdmin {
		return true
	}
	member, err := h.store.GetOrganizationMember(ctx, orgID, account.ID)
	if err != nil {
		return false
	}
	return member.Role == domain.RoleOwner || member.Role == domain.RoleAdmin
}

func (h *Handler) canViewOrganization(ctx context.Context, orgID string, account domain.AdminAccount) bool {
	if domain.NormalizeAccountRole(account.Role) == domain.RoleAdmin {
		return true
	}
	_, err := h.store.GetOrganizationMember(ctx, orgID, account.ID)
	return err == nil
}

func (h *Handler) listOrganizationMembers(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	account, ok := accountFromContext(r.Context())
	if !ok || !h.canViewOrganization(r.Context(), orgID, account) {
		writeError(w, http.StatusForbidden, "not authorized to view organization members")
		return
	}
	members, err := h.store.ListOrganizationMembers(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list organization members: "+err.Error())
		return
	}
	enriched := make([]organizationMemberDTO, 0, len(members))
	for _, m := range members {
		dto := organizationMemberDTO{OrganizationMember: m}
		if u, err := h.store.GetAdminAccount(r.Context(), m.UserID); err == nil {
			dto.Username = u.Username
		}
		enriched = append(enriched, dto)
	}
	writeJSON(w, http.StatusOK, enriched)
}

func (h *Handler) addOrganizationMember(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	account, ok := accountFromContext(r.Context())
	if !ok || !h.canManageOrganization(r.Context(), orgID, account) {
		writeError(w, http.StatusForbidden, "not authorized to manage organization members")
		return
	}
	var req addMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request payload")
		return
	}
	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, "userId is required")
		return
	}
	if _, err := h.store.GetOrganization(r.Context(), orgID); err != nil {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}
	targetUser, err := h.store.GetAdminAccount(r.Context(), req.UserID)
	if err != nil {
		targetUser, err = h.store.GetAdminAccountByUsername(r.Context(), req.UserID)
	}
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if req.Role == "" {
		req.Role = domain.RoleMember
	}
	if req.Role != domain.RoleAdmin && req.Role != domain.RoleMember && req.Role != domain.RoleViewer {
		writeError(w, http.StatusBadRequest, "invalid role, allowed: admin, member, viewer")
		return
	}
	if _, err := h.store.GetOrganizationMember(r.Context(), orgID, targetUser.ID); err == nil {
		writeError(w, http.StatusConflict, "user is already an organization member")
		return
	}

	member := domain.OrganizationMember{
		ID:             uuid.NewString(),
		OrganizationID: orgID,
		UserID:         targetUser.ID,
		Role:           req.Role,
		CreatedAt:      time.Now().UTC(),
	}
	if err := h.store.AddOrganizationMember(r.Context(), &member); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add member: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, organizationMemberDTO{
		OrganizationMember: member,
		Username:           targetUser.Username,
	})
}

func (h *Handler) removeOrganizationMember(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	userID := chi.URLParam(r, "userId")
	account, ok := accountFromContext(r.Context())
	if !ok || !h.canManageOrganization(r.Context(), orgID, account) {
		writeError(w, http.StatusForbidden, "not authorized to manage organization members")
		return
	}
	member, err := h.store.GetOrganizationMember(r.Context(), orgID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "organization member not found")
		return
	}
	if member.Role == domain.RoleOwner {
		writeError(w, http.StatusBadRequest, "organization owner cannot be removed")
		return
	}
	if err := h.store.RemoveOrganizationMember(r.Context(), orgID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove member: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) createOrganizationInvitation(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	account, ok := accountFromContext(r.Context())
	if !ok || !h.canManageOrganization(r.Context(), orgID, account) {
		writeError(w, http.StatusForbidden, "not authorized to create invitations for this organization")
		return
	}
	var req createInvitationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "invalid request payload")
		return
	}
	if req.Role == "" {
		req.Role = domain.RoleMember
	}
	if req.Role != domain.RoleMember && req.Role != domain.RoleViewer {
		writeError(w, http.StatusBadRequest, "invalid role for invitation, allowed: member, viewer")
		return
	}
	if req.ExpireDays <= 0 {
		req.ExpireDays = 7
	}
	invite := domain.OrganizationInvitation{
		OrganizationID: orgID,
		InviterUserID:  account.ID,
		Role:           req.Role,
		MaxUses:        req.MaxUses,
		ExpiresAt:      time.Now().UTC().Add(time.Duration(req.ExpireDays) * 24 * time.Hour),
	}
	if err := h.store.CreateOrganizationInvitation(r.Context(), &invite); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create invitation: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, invite)
}

func (h *Handler) listOrganizationInvitations(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	account, ok := accountFromContext(r.Context())
	if !ok || !h.canManageOrganization(r.Context(), orgID, account) {
		writeError(w, http.StatusForbidden, "not authorized to list invitations for this organization")
		return
	}
	invites, err := h.store.ListOrganizationInvitations(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list invitations: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, invites)
}

func (h *Handler) revokeOrganizationInvitation(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	inviteID := chi.URLParam(r, "inviteId")
	account, ok := accountFromContext(r.Context())
	if !ok || !h.canManageOrganization(r.Context(), orgID, account) {
		writeError(w, http.StatusForbidden, "not authorized to revoke invitations for this organization")
		return
	}
	if err := h.store.RevokeOrganizationInvitation(r.Context(), orgID, inviteID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "invitation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to revoke invitation: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getInvitationInfo(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	summary, err := h.store.GetOrganizationInvitationSummary(r.Context(), token)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "invitation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get invitation summary: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (h *Handler) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	token := chi.URLParam(r, "token")
	member, err := h.store.AcceptOrganizationInvitation(r.Context(), token, account.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "invitation not found")
			return
		}
		if errors.Is(err, store.ErrInvitationExpired) || errors.Is(err, store.ErrInvitationRevoked) || errors.Is(err, store.ErrInvitationMaxUses) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to accept invitation: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "accepted",
		"organizationId": member.OrganizationID,
		"role":           member.Role,
	})
}

func (h *Handler) getOrganizationUsage(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	if orgID == "" {
		orgID = "default-org"
	}
	usage, err := h.store.GetTenantUsage(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get tenant usage: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, usage)
}

type updateQuotaRequest struct {
	MaxServers   int     `json:"maxServers"`
	MaxCPUCores  float64 `json:"maxCpuCores"`
	MaxMemoryMB  int     `json:"maxMemoryMb"`
	MaxStorageGB int     `json:"maxStorageGb"`
}

func (h *Handler) updateOrganizationQuota(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	if _, err := h.store.GetOrganization(r.Context(), orgID); err != nil {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}
	var req updateQuotaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request payload")
		return
	}

	quota := domain.TenantQuota{
		OrganizationID: orgID,
		MaxServers:     req.MaxServers,
		MaxCPUCores:    req.MaxCPUCores,
		MaxMemoryMB:    req.MaxMemoryMB,
		MaxStorageGB:   req.MaxStorageGB,
	}

	if err := h.store.UpdateTenantQuota(r.Context(), quota); err != nil {
		writeAllocationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, quota)
}
