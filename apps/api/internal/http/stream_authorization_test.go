package http

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

func TestIdleServerStreamsCloseAfterRevocation(t *testing.T) {
	for _, mode := range []string{"watch", "remote_logs", "local_logs"} {
		t.Run(mode, func(t *testing.T) {
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			adapter := pipeLogAdapter{availableMockAdapter: availableMockAdapter{MockAdapter: runtime.NewMockAdapter()}, reader: reader}
			router, db, _ := newTestRouterWithAdapter(t, adapter)
			path := "logs"
			nodeID := "remote-stream-node"
			if mode == "watch" {
				path = "watch"
			}
			if mode == "local_logs" {
				nodeID = "node-local"
				go func() { _, _ = io.WriteString(writer, "ready\n") }()
			}
			ctx := context.Background()
			account := domain.AdminAccount{ID: "stream-user", Username: "stream-user", Role: domain.RoleMember}
			if err := db.CreateAdminAccount(ctx, &account); err != nil {
				t.Fatal(err)
			}
			org := domain.Organization{ID: "stream-org", Slug: "stream-org"}
			if err := db.CreateOrganization(ctx, &org, account.ID); err != nil {
				t.Fatal(err)
			}
			resource := domain.GameServer{ID: "stream-server", OrganizationID: org.ID, NodeID: nodeID, Spec: domain.ServerSpec{DesiredState: domain.DesiredStopped}, Status: domain.ServerRuntimeStatus{Phase: domain.PhaseStopped}}
			if err := db.CreateGameServer(ctx, &resource); err != nil {
				t.Fatal(err)
			}
			session := domain.Session{ID: "stream-session", AccountID: account.ID, TokenHash: hashSessionToken("stream-token"), ExpiresAt: time.Now().Add(time.Hour)}
			if err := db.CreateSession(ctx, &session); err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(router)
			defer srv.Close()
			client := srv.Client()
			client.Timeout = streamAuthorizationInterval + streamAuthorizationTimeout + 3*time.Second
			req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/servers/stream-server/"+path, nil)
			req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "stream-token"})
			response, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("stream startup: %d", response.StatusCode)
			}
			finished := make(chan error, 1)
			go func() { _, err := io.Copy(io.Discard, response.Body); finished <- err }()
			select {
			case err := <-finished:
				t.Fatalf("stream closed before revocation: %v", err)
			case <-time.After(100 * time.Millisecond):
			}
			if path == "watch" {
				if err := db.RemoveOrganizationMember(ctx, org.ID, account.ID); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := db.DeleteSession(ctx, session.ID); err != nil {
					t.Fatal(err)
				}
			}
			started := time.Now()
			err = <-finished
			// A cancelled response can end cleanly or terminate its chunked body. A
			// client timeout means the server failed to close the idle stream in time.
			if time.Since(started) > streamAuthorizationInterval+streamAuthorizationTimeout+time.Second {
				t.Fatalf("idle stream did not close promptly: %v", err)
			}
		})
	}
}

func TestStreamAuthorizationUsesCurrentRoleAndExpiresSessions(t *testing.T) {
	_, db, _ := newTestRouter(t)
	ctx := context.Background()
	h := &Handler{store: db}
	account := domain.AdminAccount{ID: "auth-stream", Username: "auth-stream", Role: domain.RoleAdmin}
	if err := db.CreateAdminAccount(ctx, &account); err != nil {
		t.Fatal(err)
	}
	session := domain.Session{ID: "auth-stream-session", AccountID: account.ID, TokenHash: hashSessionToken("token"), ExpiresAt: time.Now().Add(time.Hour)}
	if err := db.CreateSession(ctx, &session); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "token"})
	if !h.streamAuthorized(req, "server", domain.PermissionServerConfigure) {
		t.Fatal("admin stream denied")
	}
	account.Role = domain.RoleViewer
	if err := db.SaveAdminAccount(ctx, &account); err != nil {
		t.Fatal(err)
	}
	if h.streamAuthorized(req, "server", domain.PermissionServerConfigure) {
		t.Fatal("demoted account kept log permission")
	}
	if err := db.DeleteSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	session.ExpiresAt = time.Now().Add(-time.Second)
	if err := db.CreateSession(ctx, &session); err != nil {
		t.Fatal(err)
	}
	if h.streamAuthorized(req, "server", domain.PermissionServerView) {
		t.Fatal("expired session remained authorized")
	}
	if err := db.DeleteAdminAccount(ctx, account.ID); err != nil {
		t.Fatal(err)
	}
	authenticatedRequest := req.WithContext(context.WithValue(ctx, authAccountContextKey, account))
	if h.streamAuthorized(authenticatedRequest, "server", domain.PermissionServerView) {
		t.Fatal("deleted account fell back to anonymous access")
	}

}

type pipeLogAdapter struct {
	availableMockAdapter
	reader *io.PipeReader
}

func (a pipeLogAdapter) LogsWorkload(context.Context, string, bool) (io.ReadCloser, error) {
	return a.reader, nil
}
