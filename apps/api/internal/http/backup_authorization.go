package http

import (
	"errors"
	"net/http"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

// Global backup URLs must resolve through the same instance membership as
// nested save URLs, before touching any file or attempting cleanup/restoration.
func (h *Handler) backupForRequest(w http.ResponseWriter, r *http.Request, id string) (domain.Backup, bool) {
	account, customer := accountFromContext(r.Context())
	customer = customer && !domain.IsPlatformAdmin(account)
	var item domain.Backup
	var err error
	if customer {
		item, err = h.store.GetUserBackup(r.Context(), account.ID, id)
	} else {
		item, err = h.store.GetBackup(r.Context(), id)
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "backup not found")
		return item, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read backup")
		return item, false
	}
	if customer && r.Method != http.MethodGet && r.Method != http.MethodHead {
		role, err := h.store.ServerMembershipRole(r.Context(), account.ID, item.InstanceID)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "backup not found")
			return item, false
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to authorize backup")
			return item, false
		}
		if !tenantRoleCanWrite(role) {
			writeError(w, http.StatusForbidden, "workspace write permission required")
			return item, false
		}
	}
	return item, true
}
