package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/deploymentstatus"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

func TestInstanceViewsEnforceProductResponsibilities(t *testing.T) {
	router, db, _ := newTestRouterWithAdapter(t, runtime.NewMockAdapter())
	ctx := context.Background()
	for _, account := range []domain.AdminAccount{
		{ID: "api-view-owner", Username: "api-view-owner", Role: domain.RoleMember, PlatformRole: domain.PlatformRoleUser},
		{ID: "api-view-outsider", Username: "api-view-outsider", Role: domain.RoleMember, PlatformRole: domain.PlatformRoleUser},
		{ID: "api-view-operator", Username: "api-view-operator", Role: domain.RoleMember, PlatformRole: domain.PlatformRoleAdmin},
	} {
		if err := db.CreateAdminAccount(ctx, &account); err != nil {
			t.Fatal(err)
		}
	}
	organization := domain.Organization{ID: "api-view-tenant", Slug: "api-view-tenant", Name: "API view tenant"}
	if err := db.CreateOrganization(ctx, &organization, "api-view-owner"); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateTenantQuota(ctx, domain.TenantQuota{OrganizationID: organization.ID, MaxServers: 1, MaxCPUCores: 2, MaxMemoryMB: 2048, MaxStorageGB: 10}); err != nil {
		t.Fatal(err)
	}
	region, err := db.RegisterRegion(ctx, "api-view-east", "API view east")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetRegionAcceptingCreates(ctx, region.ID, region.Version, true); err != nil {
		t.Fatal(err)
	}
	created, err := db.CreateGlobalServer(ctx, "api-view-owner", instances.CreateRequest{OrganizationID: organization.ID, Name: "API logical server", RegionID: region.ID, IdempotencyKey: "api-view-create", Specification: instances.Specification{
		ProviderKey: "example-provider", GameVersion: "1.0.0", ConfigSchemaVersion: 1,
		Configuration: instances.ProtectedConfiguration{KeyID: "api-view-key", Ciphertext: []byte("api-secret-ciphertext")},
		Resources:     instances.Resources{CPU: 1, MemoryMB: 512},
	}})
	if err != nil {
		t.Fatal(err)
	}
	observedAt := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.RecordDeploymentStatus(ctx, region.ID, deploymentstatus.Event{SchemaVersion: 1, EventID: "api-view-status", RegionID: region.ID, OrganizationID: organization.ID, OperationID: created.Operation.ID, ServerID: created.Server.ID, RevisionID: created.Revision.ID, TaskID: "api-private-task", NodeID: "api-private-node", PlacementEpoch: 1, SpecGeneration: 1, IntentVersion: 1, Fence: 1, ActualState: "running", Outcome: "succeeded", RuntimeID: "api-private-runtime", ObservedAtMS: observedAt.UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	cookies := map[string]*http.Cookie{}
	for _, accountID := range []string{"api-view-owner", "api-view-outsider", "api-view-operator"} {
		token := accountID + "-token"
		if err := db.CreateSession(ctx, &domain.Session{ID: accountID + "-session", AccountID: accountID, TokenHash: hashSessionToken(token), ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
		cookies[accountID] = &http.Cookie{Name: sessionCookieName, Value: token}
	}
	request := func(path, accountID string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if accountID != "" {
			req.AddCookie(cookies[accountID])
		}
		router.ServeHTTP(recorder, req)
		return recorder
	}
	if got := request("/api/instances?organizationId="+organization.ID, ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated tenant list: %d %s", got.Code, got.Body.String())
	}
	if got := request("/api/instances", "api-view-owner"); got.Code != http.StatusBadRequest {
		t.Fatalf("implicit tenant list: %d %s", got.Code, got.Body.String())
	}
	if got := request("/api/instances?organizationId="+organization.ID, "api-view-outsider"); got.Code != http.StatusNotFound {
		t.Fatalf("foreign tenant list: %d %s", got.Code, got.Body.String())
	}
	tenant := request("/api/instances?organizationId="+organization.ID, "api-view-owner")
	if tenant.Code != http.StatusOK || strings.Contains(tenant.Body.String(), "api-private-node") || strings.Contains(tenant.Body.String(), "api-private-runtime") || strings.Contains(tenant.Body.String(), "api-secret-ciphertext") {
		t.Fatalf("tenant response leaked infrastructure or config: %d %s", tenant.Code, tenant.Body.String())
	}
	var page instanceViewPageResponse
	if err := json.Unmarshal(tenant.Body.Bytes(), &page); err != nil || len(page.Items) != 1 || page.Items[0].Deployment == nil || page.Items[0].Deployment.ActualState != "running" {
		t.Fatalf("tenant response: %+v %v", page, err)
	}
	if got := request("/api/platform/instances", "api-view-owner"); got.Code != http.StatusForbidden {
		t.Fatalf("tenant entered platform inventory: %d %s", got.Code, got.Body.String())
	}
	platform := request("/api/platform/instances/"+created.Server.ID, "api-view-operator")
	if platform.Code != http.StatusOK || !strings.Contains(platform.Body.String(), "api-private-node") || strings.Contains(platform.Body.String(), "api-private-runtime") || strings.Contains(platform.Body.String(), "api-secret-ciphertext") {
		t.Fatalf("platform response boundary: %d %s", platform.Code, platform.Body.String())
	}
	if got := request("/api/platform/instances?organizationId=x&organizationId=y", "api-view-operator"); got.Code != http.StatusBadRequest {
		t.Fatalf("duplicate query accepted: %d %s", got.Code, got.Body.String())
	}
}
