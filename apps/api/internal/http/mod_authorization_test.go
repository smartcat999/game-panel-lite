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

func TestModAssignmentChecksBothInstanceWorkspaces(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	ctx := context.Background()
	account := domain.AdminAccount{ID: "mod-user", Username: "mod-user", Role: domain.RoleMember}
	if err := db.CreateAdminAccount(ctx, &account); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"mod-org-a", "mod-org-b"} {
		org := domain.Organization{ID: id, Slug: id}
		owner := "other-user"
		if id == "mod-org-a" {
			owner = account.ID
		}
		if err := db.CreateOrganization(ctx, &org, owner); err != nil {
			t.Fatal(err)
		}
		server := domain.GameServer{ID: id, OrganizationID: id, ProviderKey: domain.ProviderTerrariaTModLoader, Spec: domain.ServerSpec{DesiredState: domain.DesiredStopped}, Status: domain.ServerRuntimeStatus{Phase: domain.PhaseStopped}}
		if err := db.CreateGameServer(ctx, &server); err != nil {
			t.Fatal(err)
		}
		mod := domain.ModFile{ID: id, InstanceID: id, ProviderKey: domain.ProviderTerrariaTModLoader, FileName: "private.tmod"}
		if err := db.CreateMod(ctx, &mod); err != nil {
			t.Fatal(err)
		}
	}
	library := domain.ModFile{ID: "legacy-library", InstanceID: "unassigned", ProviderKey: domain.ProviderTerrariaTModLoader, FileName: "library.tmod"}
	if err := db.CreateMod(ctx, &library); err != nil {
		t.Fatal(err)
	}
	session := domain.Session{ID: "mod-transfer-session", AccountID: account.ID, TokenHash: hashSessionToken("mod-transfer-token"), ExpiresAt: time.Now().Add(time.Hour)}
	if err := db.CreateSession(ctx, &session); err != nil {
		t.Fatal(err)
	}
	assign := func(source, target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/mods/"+source+"/assign", strings.NewReader(`{"instanceId":"`+target+`"}`))
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "mod-transfer-token"})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	for _, tc := range []struct{ source, target string }{{"mod-org-b", "mod-org-a"}, {"mod-org-a", "mod-org-b"}, {"legacy-library", "mod-org-b"}} {
		got := assign(tc.source, tc.target)
		if got.Code != http.StatusNotFound {
			t.Fatalf("foreign transfer %s -> %s: %d %s", tc.source, tc.target, got.Code, got.Body.String())
		}
	}
	owned := testServer("same-workspace-target", cfg.DataDir)
	owned.ProviderKey = domain.ProviderTerrariaTModLoader
	createTestServer(t, db, owned)
	resource, err := db.GetGameServer(ctx, owned.ID)
	if err != nil {
		t.Fatal(err)
	}
	resource.OrganizationID = "mod-org-a"
	if err := db.SaveGameServer(ctx, &resource); err != nil {
		t.Fatal(err)
	}
	if _, _, err := newTestModService(t, cfg.DataDir).Upload("mod-org-a", domain.ProviderTerrariaTModLoader, "private.tmod", strings.NewReader("owned-mod")); err != nil {
		t.Fatal(err)
	}
	if got := assign("mod-org-a", owned.ID); got.Code != http.StatusCreated {
		t.Fatalf("same-workspace transfer: %d %s", got.Code, got.Body.String())
	}
	member := domain.OrganizationMember{ID: "second-workspace-member", OrganizationID: "mod-org-b", UserID: account.ID, Role: domain.RoleMember}
	if err := db.AddOrganizationMember(ctx, &member); err != nil {
		t.Fatal(err)
	}
	if got := assign("mod-org-a", "mod-org-b"); got.Code != http.StatusForbidden {
		t.Fatalf("both memberships cross-space transfer: %d %s", got.Code, got.Body.String())
	}
	if err := db.RemoveOrganizationMember(ctx, "mod-org-a", account.ID); err != nil {
		t.Fatal(err)
	}
	viewer := domain.OrganizationMember{ID: "mod-viewer", OrganizationID: "mod-org-a", UserID: account.ID, Role: domain.RoleViewer}
	if err := db.AddOrganizationMember(ctx, &viewer); err != nil {
		t.Fatal(err)
	}
	if got := assign("legacy-library", "mod-org-a"); got.Code != http.StatusForbidden {
		t.Fatalf("read-only target: %d %s", got.Code, got.Body.String())
	}
	mods, err := db.ListMods(ctx, "mod-org-b")
	if err != nil || len(mods) != 1 || mods[0].ID != "mod-org-b" {
		t.Fatalf("foreign mods changed: %+v %v", mods, err)
	}
}
