package http

import (
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
	_, _ = h.store.EnsureDefaultOrganization(r.Context())
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

func (h *Handler) listOrganizationMembers(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	members, err := h.store.ListOrganizationMembers(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list organization members: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func (h *Handler) addOrganizationMember(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
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
	account, err := h.store.GetAdminAccount(r.Context(), req.UserID)
	if err != nil {
		account, err = h.store.GetAdminAccountByUsername(r.Context(), req.UserID)
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
	if _, err := h.store.GetOrganizationMember(r.Context(), orgID, account.ID); err == nil {
		writeError(w, http.StatusConflict, "user is already an organization member")
		return
	}

	member := domain.OrganizationMember{
		ID:             uuid.NewString(),
		OrganizationID: orgID,
		UserID:         account.ID,
		Role:           req.Role,
		CreatedAt:      time.Now().UTC(),
	}
	if err := h.store.AddOrganizationMember(r.Context(), &member); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add member: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, member)
}

func (h *Handler) removeOrganizationMember(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	userID := chi.URLParam(r, "userId")
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
	if req.MaxServers <= 0 {
		req.MaxServers = 10
	}
	if req.MaxCPUCores <= 0 {
		req.MaxCPUCores = 16.0
	}
	if req.MaxMemoryMB <= 0 {
		req.MaxMemoryMB = 32768
	}
	if req.MaxStorageGB <= 0 {
		req.MaxStorageGB = 100
	}

	quota := domain.TenantQuota{
		OrganizationID: orgID,
		MaxServers:     req.MaxServers,
		MaxCPUCores:    req.MaxCPUCores,
		MaxMemoryMB:    req.MaxMemoryMB,
		MaxStorageGB:   req.MaxStorageGB,
	}

	if err := h.store.UpdateTenantQuota(r.Context(), quota); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update quota: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, quota)
}
