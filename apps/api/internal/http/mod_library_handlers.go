package http

import (
	"errors"
	"mime"
	"net/http"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	modfiles "github.com/smartcat999/game-panel-lite/apps/api/internal/mod"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modlibrary"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func (h *Handler) listMyLibraryMods(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	items, err := h.store.ListUserLibraryMods(r.Context(), account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workspace mods")
		return
	}
	writeJSON(w, http.StatusOK, items)
}
func (h *Handler) uploadMyLibraryMod(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/octet-stream" {
		writeError(w, http.StatusUnsupportedMediaType, "send the file as application/octet-stream")
		return
	}
	query := r.URL.Query()
	item, err := h.modLibrary.Upload(r.Context(), account.ID, query.Get("organizationId"), domain.ProviderKey(query.Get("providerKey")), query.Get("fileName"), r.Body, h.cfg.ModUploadLimit())
	if err == nil {
		writeJSON(w, http.StatusCreated, item)
		return
	}
	switch {
	case errors.Is(err, modlibrary.ErrRetainedUpload):
		h.logger.Error("library upload requires reconciliation", "uploadId", item.ID, "organizationId", item.OrganizationID, "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "upload failed; retained file requires reconciliation", "uploadId": item.ID})
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "workspace not found")
	case errors.Is(err, store.ErrWorkspaceWriteDenied):
		writeError(w, http.StatusForbidden, "workspace write permission required")
	case errors.Is(err, modfiles.ErrUploadTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "mod upload exceeds configured size limit")
	case errors.Is(err, modlibrary.ErrInvalidUpload):
		writeError(w, http.StatusBadRequest, "invalid mod upload or package metadata")
	case errors.Is(err, store.ErrInvalidModLibrary):
		writeError(w, http.StatusConflict, "mod file already exists in this workspace")
	default:
		h.logger.Error("library upload failed", "uploadId", item.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to upload mod")
	}
}

// Legacy global mutations remain an administrator tool until each operation is
// migrated to workspace storage and reference authorization.
func (h *Handler) requireLegacyLibraryAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allocationActor(r) != "" {
			writeError(w, http.StatusForbidden, "legacy mod library requires platform administrator access")
			return
		}
		next.ServeHTTP(w, r)
	})
}
