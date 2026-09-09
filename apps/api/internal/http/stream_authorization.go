package http

import (
	"context"
	"net/http"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

// Idle streams recheck persisted authorization as well as active streams. The
// query deadline bounds time spent checking an unavailable authorization store.
const streamAuthorizationInterval = 5 * time.Second
const streamAuthorizationTimeout = 2 * time.Second

func (h *Handler) streamAuthorized(r *http.Request, serverID string, permission domain.Permission) bool {
	account, ok := h.accountFromRequest(r)
	if initial, authenticated := accountFromContext(r.Context()); authenticated && (!ok || initial.ID != account.ID) {
		return false
	}
	if !ok {
		initialized, err := h.store.HasAdminAccount(r.Context())
		return err == nil && !initialized
	}
	if !domain.RoleHasPermission(account.Role, permission) {
		return false
	}
	if domain.IsPlatformAdmin(account) {
		return true
	}
	role, err := h.store.ServerMembershipRole(r.Context(), account.ID, serverID)
	return err == nil && (tenantRoleCanWrite(role) || role == domain.RoleViewer)
}

func (h *Handler) watchStreamAuthorization(w http.ResponseWriter, r *http.Request, serverID string, permission domain.Permission, interval time.Duration) (*http.Request, func()) {
	ctx, cancel := context.WithCancel(r.Context())
	r = r.WithContext(ctx)
	done := make(chan struct{})
	writeDone := make(chan struct{})
	stopWrite := context.AfterFunc(ctx, func() { defer close(writeDone); _ = http.NewResponseController(w).SetWriteDeadline(time.Now()) })
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			queryCtx, stop := context.WithTimeout(ctx, streamAuthorizationTimeout)
			allowed := h.streamAuthorized(r.WithContext(queryCtx), serverID, permission)
			stop()
			if !allowed {
				cancel()
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return r, func() {
		if !stopWrite() {
			<-writeDone
		}
		cancel()
		<-done
	}
}
