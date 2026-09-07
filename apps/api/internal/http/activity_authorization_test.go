package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestActivityHistoryIsWorkspaceScoped(t *testing.T) {
	router, db, _ := newTestRouter(t)
	ctx := context.Background()
	account := domain.AdminAccount{ID: "activity-reader", Username: "activity-reader", Role: domain.RoleMember}
	if err := db.CreateAdminAccount(ctx, &account); err != nil {
		t.Fatal(err)
	}
	org := domain.Organization{ID: "activity-owned-space", Slug: "activity-owned-space"}
	if err := db.CreateOrganization(ctx, &org, account.ID); err != nil {
		t.Fatal(err)
	}
	server := domain.GameServer{ID: "activity-instance", OrganizationID: org.ID, Spec: domain.ServerSpec{DesiredState: domain.DesiredStopped}, Status: domain.ServerRuntimeStatus{Phase: domain.PhaseStopped}}
	if err := db.CreateGameServer(ctx, &server); err != nil {
		t.Fatal(err)
	}
	for _, event := range []domain.ActivityEvent{
		{ID: "owned-event", InstanceID: server.ID, Message: "visible-owned"},
		{ID: "foreign-event", InstanceID: server.ID, OrganizationID: "foreign-space", Message: "secret-foreign"},
		{ID: "platform-event", Message: "secret-platform"},
	} {
		if err := db.CreateActivity(ctx, &event); err != nil {
			t.Fatal(err)
		}
	}
	session := domain.Session{ID: "activity-session", AccountID: account.ID, TokenHash: hashSessionToken("activity-token"), ExpiresAt: time.Now().Add(time.Hour)}
	if err := db.CreateSession(ctx, &session); err != nil {
		t.Fatal(err)
	}
	request := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "activity-token"})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	for _, path := range []string{"/api/activity?organizationId=foreign-space", "/api/servers/activity-instance/events"} {
		got := request(path)
		if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "visible-owned") || strings.Contains(got.Body.String(), "secret-") {
			t.Fatalf("history %s: %d %s", path, got.Code, got.Body.String())
		}
	}
	h := &Handler{store: db}
	snapshots := h.serverWatchEvents(context.WithValue(ctx, authAccountContextKey, account), server)
	for _, event := range snapshots {
		if strings.Contains(event.Message, "secret-") {
			t.Fatalf("watch leaked foreign history: %+v", event)
		}
	}
	if err := db.RemoveOrganizationMember(ctx, org.ID, account.ID); err != nil {
		t.Fatal(err)
	}
	if got := request("/api/activity"); got.Code != http.StatusOK || strings.TrimSpace(got.Body.String()) != "[]" {
		t.Fatalf("revoked history: %d %s", got.Code, got.Body.String())
	}
}
