package http

import (
	"errors"
	"net/http"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func (h *Handler) worldForRequest(w http.ResponseWriter, r *http.Request, id string) (domain.World, bool) {
	account, customer := accountFromContext(r.Context())
	customer = customer && !domain.IsPlatformAdmin(account)
	var item domain.World
	var err error
	if customer {
		item, err = h.store.GetUserWorld(r.Context(), account.ID, id)
	} else {
		item, err = h.store.GetWorld(r.Context(), id)
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "world not found")
		return item, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read world")
		return item, false
	}
	if customer && r.Method != http.MethodGet && r.Method != http.MethodHead {
		_, status, err := h.creationOrganization(r, item.OrganizationID)
		if err != nil {
			writeError(w, status, err.Error())
			return item, false
		}
	}
	return item, true
}
