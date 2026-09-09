package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestServerRoutesEnforceTenantMembership(t *testing.T) {
	router, db, _ := newTestRouter(t)
	ctx := context.Background()
	node := domain.ComputeNode{ID: "node-local", IsLocal: true, CPUCores: 8, MemoryTotalMB: 8192}
	if err := db.CreateComputeNode(ctx, &node); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"tenant-a", "tenant-b"} {
		account := domain.AdminAccount{ID: id, Username: id, Role: domain.RoleMember}
		if err := db.CreateAdminAccount(ctx, &account); err != nil {
			t.Fatal(err)
		}
		org := domain.Organization{ID: id, Slug: id}
		if err := db.CreateOrganization(ctx, &org, id); err != nil {
			t.Fatal(err)
		}
		server := domain.GameServer{ID: id, Name: id, OrganizationID: id, Spec: domain.ServerSpec{Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 1024}}, Status: domain.ServerRuntimeStatus{Phase: domain.PhaseStopped}}
		if err := db.CreateGameServer(ctx, &server); err != nil {
			t.Fatal(err)
		}
	}
	session := domain.Session{ID: "scope-session", AccountID: "tenant-a", TokenHash: hashSessionToken("scope-token"), ExpiresAt: time.Now().Add(time.Hour)}
	if err := db.CreateSession(ctx, &session); err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "scope-token"})
		req.Header.Set("X-Organization-ID", "tenant-b")
		timeout, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req.WithContext(timeout))
		return rec
	}
	params := regexp.MustCompile(`\{[^}]+\}`)
	checked := 0
	if err := chi.Walk(router.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !strings.HasPrefix(route, "/api/servers/{id}") {
			return nil
		}
		path := strings.Replace(route, "{id}", "tenant-b", 1)
		path = params.ReplaceAllString(path, "probe")
		t.Run(method+" "+route, func(t *testing.T) {
			got := request(method, path, `{}`)
			if got.Code != http.StatusNotFound {
				t.Fatalf("cross-tenant route returned %d: %s", got.Code, got.Body.String())
			}
		})
		checked++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"overview", "metrics", "server-load", "events", "platform"} {
		if got := request(http.MethodGet, "/api/monitoring/"+path, ""); got.Code != http.StatusForbidden {
			t.Fatalf("platform monitoring %s: %d", path, got.Code)
		}
	}
	for _, path := range []string{"/api/nodes", "/api/nodes/node-local", "/api/nodes/node-local/servers", "/api/regions/default/status", "/api/regions/default/nodes", "/api/regions/default/deployments"} {
		if got := request(http.MethodGet, path, ""); got.Code != http.StatusForbidden {
			t.Fatalf("tenant node access %s: %d", path, got.Code)
		}
	}
	if checked < 40 {
		t.Fatalf("unexpected route coverage: %d", checked)
	}
	for _, path := range []string{"/api/servers", "/api/servers?page=1&search=tenant&sort=status"} {
		got := request(http.MethodGet, path, "")
		if got.Code != http.StatusOK || strings.Contains(got.Body.String(), `"id":"tenant-b"`) || !strings.Contains(got.Body.String(), `"id":"tenant-a"`) {
			t.Fatalf("scoped list: %d %s", got.Code, got.Body.String())
		}
	}
	for _, path := range []string{"/api/servers?organizationId=tenant-a", "/api/servers?page=1&organizationId=tenant-a"} {
		got := request(http.MethodGet, path, "")
		if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"id":"tenant-a"`) || strings.Contains(got.Body.String(), `"id":"tenant-b"`) {
			t.Fatalf("explicit organization scope: %d %s", got.Code, got.Body.String())
		}
	}
	for _, path := range []string{"/api/servers?organizationId=tenant-b", "/api/servers?page=1&organizationId=tenant-b"} {
		got := request(http.MethodGet, path, "")
		if got.Code != http.StatusOK || strings.Contains(got.Body.String(), `"id":"tenant-b"`) {
			t.Fatalf("foreign organization scope: %d %s", got.Code, got.Body.String())
		}
	}
	if got := request(http.MethodGet, "/api/servers/tenant-a", ""); got.Code != http.StatusOK {
		t.Fatalf("own server: %d", got.Code)
	}
	if got := request(http.MethodPost, "/api/servers", `{"organizationId":"tenant-b"}`); got.Code != http.StatusNotFound {
		t.Fatalf("foreign creation: %d %s", got.Code, got.Body.String())
	}
	got := request(http.MethodPost, "/api/servers", `{"resources":{"cpuLimitCores":1,"memoryLimitMb":1024},"name":"Owned server","providerKey":"terraria-vanilla","config":{}}`)
	var created domain.GameServer
	if err := json.Unmarshal(got.Body.Bytes(), &created); err != nil || got.Code != http.StatusCreated || created.OrganizationID != "tenant-a" {
		t.Fatalf("owned creation: %d %s %v", got.Code, got.Body.String(), err)
	}

	if err := db.UpdateTenantQuota(ctx, domain.TenantQuota{OrganizationID: "tenant-a", MaxServers: 2, MaxCPUCores: 2, MaxMemoryMB: 2048, MaxStorageGB: 10}); err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodPost, "/api/servers", `{"resources":{"cpuLimitCores":1,"memoryLimitMb":1024},"providerKey":"terraria-vanilla","config":{}}`); got.Code != http.StatusConflict {
		t.Fatalf("quota creation: %d %s", got.Code, got.Body.String())
	}

	if got := request(http.MethodPut, "/api/servers/"+created.ID+"/config", `{"resources":{"cpuLimitCores":2,"memoryLimitMb":1024},"config":{}}`); got.Code != http.StatusConflict {
		t.Fatalf("quota config update: %d %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodPost, "/api/servers", `{"providerKey":"terraria-vanilla","config":{}}`); got.Code != http.StatusBadRequest {
		t.Fatalf("unlimited creation: %d %s", got.Code, got.Body.String())
	}
	if err := db.RemoveOrganizationMember(ctx, "tenant-a", "tenant-a"); err != nil {
		t.Fatal(err)
	}
	viewer := domain.OrganizationMember{ID: "viewer-a", OrganizationID: "tenant-a", UserID: "tenant-a", Role: domain.RoleViewer}
	if err := db.AddOrganizationMember(ctx, &viewer); err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodPost, "/api/servers/tenant-a/start", `{}`); got.Code != http.StatusForbidden {
		t.Fatalf("viewer mutation: %d", got.Code)
	}
	if got := request(http.MethodGet, "/api/servers/tenant-a", ""); got.Code != http.StatusOK {
		t.Fatalf("viewer read: %d", got.Code)
	}
	if err := db.RemoveOrganizationMember(ctx, "tenant-a", "tenant-a"); err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodGet, "/api/servers/tenant-a", ""); got.Code != http.StatusNotFound {
		t.Fatalf("revoked read: %d", got.Code)
	}
}
