package http

import (
	"errors"
	"net/http"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

// Instance-owned mods need source read permission in addition to target write
// permission. Legacy unassigned library ownership is a separate migration.
func (h *Handler) modTransferSource(w http.ResponseWriter, r *http.Request, item domain.ModFile) (string, bool) {
	if item.InstanceID == "unassigned" {
		return "", true
	}
	source, err := h.store.GetGameServer(r.Context(), item.InstanceID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "mod source not found")
		return "", false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to resolve mod source")
		return "", false
	}
	account, ok := accountFromContext(r.Context())
	if !ok || domain.NormalizeAccountRole(account.Role) == domain.RoleAdmin {
		return source.OrganizationID, true
	}
	_, err = h.store.GetUserOrganization(r.Context(), account.ID, source.OrganizationID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "mod not found")
		return "", false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to authorize mod source")
		return "", false
	}
	return source.OrganizationID, true
}
