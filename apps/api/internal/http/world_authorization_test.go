package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestWorldLibrariesAndAssignmentAreTenantScoped(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	ctx := context.Background()
	if err := db.SetSetting(ctx, domain.SettingKeyAllowRegistration, "true"); err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{}
	worlds := []domain.World{}
	for _, name := range []string{"worldalice", "worldbruce"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"`+name+`","password":"secret123"}`)))
		if rec.Code != http.StatusCreated {
			t.Fatalf("register: %d %s", rec.Code, rec.Body.String())
		}
		cookie := authCookieFromRecorder(t, rec)
		cookies = append(cookies, cookie)
		req := newMultipartFileRequest(t, http.MethodPost, "/api/worlds/import", "file", "same.wld", []byte(name))
		req.AddCookie(cookie)
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		var world domain.World
		if err := json.Unmarshal(rec.Body.Bytes(), &world); err != nil || rec.Code != http.StatusCreated || world.OrganizationID == "" {
			t.Fatalf("private import: %d %s %v", rec.Code, rec.Body.String(), err)
		}
		worlds = append(worlds, world)
	}
	if worlds[0].ID == worlds[1].ID || worlds[0].OrganizationID == worlds[1].OrganizationID {
		t.Fatal("shared world identity")
	}
	request := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	for i, cookie := range cookies {
		got := request(http.MethodGet, "/api/worlds/"+worlds[i].ID+"/download", "", cookie)
		want := []string{"worldalice", "worldbruce"}[i]
		if got.Code != http.StatusOK || got.Body.String() != want {
			t.Fatalf("same filename isolation: %d %s", got.Code, got.Body.String())
		}
		got = request(http.MethodGet, "/api/worlds", "", cookie)
		if got.Code != http.StatusOK || strings.Contains(got.Body.String(), worlds[1-i].ID) || !strings.Contains(got.Body.String(), worlds[i].ID) {
			t.Fatalf("scoped list: %d %s", got.Code, got.Body.String())
		}
		for _, tc := range []struct{ method, suffix string }{{http.MethodGet, "/download"}, {http.MethodDelete, ""}, {http.MethodPost, "/assign"}} {
			got := request(tc.method, "/api/worlds/"+worlds[1-i].ID+tc.suffix, `{"instanceId":"other"}`, cookie)
			if got.Code != http.StatusNotFound {
				t.Fatalf("foreign world: %d %s", got.Code, got.Body.String())
			}
		}
	}
	// A valid source does not authorize a foreign target, including multipart import.
	foreign := domain.GameServer{ID: "foreign-world-target", OrganizationID: worlds[1].OrganizationID}
	if err := db.CreateGameServer(ctx, &foreign); err != nil {
		t.Fatal(err)
	}
	got := request(http.MethodPost, "/api/worlds/"+worlds[0].ID+"/assign", `{"instanceId":"foreign-world-target"}`, cookies[0])
	if got.Code != http.StatusNotFound {
		t.Fatalf("foreign assignment target: %d %s", got.Code, got.Body.String())
	}
	req := newMultipartFileRequest(t, http.MethodPost, "/api/worlds/import?instanceId=foreign-world-target", "file", "injected.wld", []byte("bad"))
	req.AddCookie(cookies[0])
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign import target: %d %s", rec.Code, rec.Body.String())
	}
	createTestServer(t, db, testServer("owned-world-target", cfg.DataDir))
	owned, err := db.GetGameServer(ctx, "owned-world-target")
	if err != nil {
		t.Fatal(err)
	}
	owned.OrganizationID = worlds[0].OrganizationID
	if err := db.SaveGameServer(ctx, &owned); err != nil {
		t.Fatal(err)
	}
	foreignActive := domain.World{ID: "foreign-active-reference", OrganizationID: worlds[1].OrganizationID, ActiveInstanceID: "owned-world-target"}
	if err := db.CreateWorld(ctx, &foreignActive); err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodPost, "/api/worlds/"+worlds[0].ID+"/assign", `{"instanceId":"owned-world-target"}`, cookies[0]); got.Code != http.StatusOK {
		t.Fatalf("own assignment: %d %s", got.Code, got.Body.String())
	}
	retained, err := db.GetWorld(ctx, foreignActive.ID)
	if err != nil || retained.ActiveInstanceID != foreignActive.ActiveInstanceID {
		t.Fatalf("foreign active reference mutated: %+v %v", retained, err)
	}
	account, err := db.GetAdminAccountByUsername(ctx, "worldalice")
	if err != nil {
		t.Fatal(err)
	}
	member := domain.OrganizationMember{ID: "shared-second-org", OrganizationID: worlds[1].OrganizationID, UserID: account.ID, Role: domain.RoleMember}
	if err := db.AddOrganizationMember(ctx, &member); err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodPost, "/api/worlds/"+worlds[0].ID+"/assign", `{"instanceId":"foreign-world-target"}`, cookies[0]); got.Code != http.StatusForbidden {
		t.Fatalf("cross-workspace assignment with both memberships: %d %s", got.Code, got.Body.String())
	}
	if err := db.RemoveOrganizationMember(ctx, worlds[0].OrganizationID, account.ID); err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodGet, "/api/worlds/"+worlds[0].ID+"/download", "", cookies[0]); got.Code != http.StatusNotFound {
		t.Fatalf("revoked: %d", got.Code)
	}
}
