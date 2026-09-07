package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	servers "github.com/smartcat999/game-panel-lite/apps/api/internal/server"
)

func TestWorkspaceModInstallationRequest(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	ctx := context.Background()
	for _, id := range []string{"install-alice", "install-bob", "install-viewer"} {
		account := domain.AdminAccount{ID: id, Username: id, Role: domain.RoleMember}
		if err := db.CreateAdminAccount(ctx, &account); err != nil {
			t.Fatal(err)
		}
		session := domain.Session{ID: id, AccountID: id, TokenHash: hashSessionToken(id), ExpiresAt: time.Now().Add(time.Hour)}
		if err := db.CreateSession(ctx, &session); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"install-alice", "install-bob"} {
		org := domain.Organization{ID: id, Slug: id}
		if err := db.CreateOrganization(ctx, &org, id); err != nil {
			t.Fatal(err)
		}
	}
	viewer := domain.OrganizationMember{OrganizationID: "install-alice", UserID: "install-viewer", Role: domain.RoleViewer}
	if err := db.AddOrganizationMember(ctx, &viewer); err != nil {
		t.Fatal(err)
	}
	target := domain.GameServer{ID: "install-target", OrganizationID: "install-alice", ProviderKey: domain.ProviderTerrariaTModLoader, Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredStopped, Runtime: domain.ServerRuntimeSpec{DataDir: filepath.Join(cfg.DataDir, "instances", "install-target")}, Config: map[string]any{"password": "private-secret"}}, Status: domain.ServerRuntimeStatus{Phase: domain.PhaseStopped, ActualState: domain.ActualStopped}}
	if err := db.CreateGameServer(ctx, &target); err != nil {
		t.Fatal(err)
	}
	remote := target
	remote.ID = "remote-target"
	remote.NodeID = "remote-node"
	if err := db.CreateGameServer(ctx, &remote); err != nil {
		t.Fatal(err)
	}
	running := target
	running.ID = "running-target"
	running.Spec.DesiredState = domain.DesiredRunning
	running.Status.Phase = domain.PhaseRunning
	if err := db.CreateGameServer(ctx, &running); err != nil {
		t.Fatal(err)
	}
	fixture := tmodFixture("IntentMod", "1", "2024")
	for _, scope := range []string{"install-alice", "install-bob", ""} {
		item := domain.ModFile{ID: scope + "-mod", InstanceID: "unassigned", OrganizationID: scope, ProviderKey: target.ProviderKey, FileName: "same.tmod", Source: "upload"}
		if scope != "" {
			if _, err := newTestModService(t, cfg.DataDir).PutLibrary(ctx, item, bytes.NewReader(fixture), int64(len(fixture))); err != nil {
				t.Fatal(err)
			}
		}
		if err := db.CreateMod(ctx, &item); err != nil {
			t.Fatal(err)
		}
	}
	request := func(user, id, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/api/servers/"+id+"/mods/installation-requests", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if user != "" {
			r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: user})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	for _, tc := range []struct {
		user, target, body string
		status             int
	}{
		{"", "install-target", `{"modId":"install-alice-mod","generation":1}`, 401},
		{"install-bob", "install-target", `{"modId":"install-alice-mod","generation":1}`, 404},
		{"install-viewer", "install-target", `{"modId":"install-alice-mod","generation":1}`, 403},
		{"install-alice", "install-target", `{"modId":"install-bob-mod","generation":1}`, 404},
		{"install-alice", "install-target", `{"modId":"-mod","generation":1}`, 404},
		{"install-alice", "install-target", `{"modId":"install-alice-mod","generation":2}`, 409},
		{"install-alice", "install-target", `{"modId":"install-alice-mod"}`, 400},
		{"install-alice", "install-target", `{"modId":"install-alice-mod","generation":1,"organizationId":"install-bob"}`, 400},
		{"install-alice", "install-target", `{"modId":"install-alice-mod","generation":1} {}`, 400},
		{"install-alice", "remote-target", `{"modId":"install-alice-mod","generation":1}`, 409},
		{"install-alice", "running-target", `{"modId":"install-alice-mod","generation":1}`, 409},
	} {
		if got := request(tc.user, tc.target, tc.body); got.Code != tc.status {
			t.Fatalf("%+v: %d %s", tc, got.Code, got.Body.String())
		}
	}
	for _, generation := range []string{"1", "2"} {
		response := request("install-alice", target.ID, `{"modId":"install-alice-mod","generation":`+generation+`}`)
		var result struct {
			Generation int      `json:"generation"`
			State      string   `json:"state"`
			ModIDs     []string `json:"modIds"`
		}
		if response.Code != 202 || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Generation != 2 || result.State != "requested" || len(result.ModIDs) != 1 {
			t.Fatalf("request: %d %s", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "private-secret") {
			t.Fatal("response leaked configuration")
		}
	}
	if got := request("install-alice", target.ID, `{"modId":"install-alice-mod","generation":1}`); got.Code != 409 {
		t.Fatalf("stale replay: %d", got.Code)
	}
	saved, err := db.GetGameServer(ctx, target.ID)
	if err != nil || saved.Spec.Generation != 2 || saved.Spec.DesiredState != domain.DesiredStopped || saved.Status.Phase != domain.PhaseStopped || saved.Spec.Config["password"] != "private-secret" {
		t.Fatalf("saved intent: %+v %v", saved, err)
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "mods", target.ID)); !os.IsNotExist(err) {
		t.Fatalf("HTTP request wrote files: %v", err)
	}
	registry, err := provider.NewRegistry(terraria.NewTModLoaderProvider())
	if err != nil {
		t.Fatal(err)
	}
	if err := servers.NewRuntimeModPlanner(cfg.DataDir, db, registry).PlanMods(ctx, saved); err != nil {
		t.Fatalf("plan requested source: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(saved.Spec.Runtime.DataDir, "Mods", "same.tmod"))
	if err != nil || !bytes.Equal(content, fixture) {
		t.Fatalf("materialized requested source: %v", err)
	}

}
