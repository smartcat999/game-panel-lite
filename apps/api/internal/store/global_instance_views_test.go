package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/deploymentstatus"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instanceview"
)

func TestGlobalInstanceViewsSeparateTenantAndPlatformData(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "views.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	for _, account := range []domain.AdminAccount{
		{ID: "view-owner", Username: "view-owner", Role: domain.RoleMember, PlatformRole: domain.PlatformRoleUser},
		{ID: "view-outsider", Username: "view-outsider", Role: domain.RoleMember, PlatformRole: domain.PlatformRoleUser},
		{ID: "view-operator", Username: "view-operator", Role: domain.RoleMember, PlatformRole: domain.PlatformRoleAdmin},
	} {
		if err := db.CreateAdminAccount(ctx, &account); err != nil {
			t.Fatal(err)
		}
	}
	organization := domain.Organization{ID: "view-tenant", Slug: "view-tenant", Name: "View tenant"}
	if err := db.CreateOrganization(ctx, &organization, "view-owner"); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateTenantQuota(ctx, domain.TenantQuota{OrganizationID: organization.ID, MaxServers: 3, MaxCPUCores: 4, MaxMemoryMB: 4096, MaxStorageGB: 10}); err != nil {
		t.Fatal(err)
	}
	region, err := db.RegisterRegion(ctx, "view-east", "View east")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetRegionAcceptingCreates(ctx, region.ID, region.Version, true); err != nil {
		t.Fatal(err)
	}
	request := instances.CreateRequest{OrganizationID: organization.ID, Name: "Visible logical server", RegionID: "view-east", IdempotencyKey: "view-create-1", Specification: instances.Specification{
		ProviderKey: "example-provider", GameVersion: "1.0.0", ConfigSchemaVersion: 1,
		Configuration: instances.ProtectedConfiguration{KeyID: "view-key", Ciphertext: []byte("secret-ciphertext")},
		Resources:     instances.Resources{CPU: 1, MemoryMB: 512},
	}}
	created, err := db.CreateGlobalServer(ctx, "view-owner", request)
	if err != nil {
		t.Fatal(err)
	}
	request.Name, request.IdempotencyKey = "Second logical server", "view-create-2"
	if _, err := db.CreateGlobalServer(ctx, "view-owner", request); err != nil {
		t.Fatal(err)
	}
	event := deploymentstatus.Event{SchemaVersion: 1, EventID: "view-status", RegionID: "view-east", OrganizationID: organization.ID, OperationID: created.Operation.ID, ServerID: created.Server.ID, RevisionID: created.Revision.ID, TaskID: "view-task", NodeID: "private-node", PlacementEpoch: 1, SpecGeneration: 1, IntentVersion: 1, Fence: 1, ActualState: "running", Outcome: "succeeded", RuntimeID: "private-runtime", ObservedAtMS: time.Now().UTC().UnixMilli()}
	if err := db.RecordDeploymentStatus(ctx, "view-east", event); err != nil {
		t.Fatal(err)
	}
	tenant, err := db.ListTenantInstanceViews(ctx, "view-owner", organization.ID, "", 1)
	if err != nil || len(tenant.Items) != 1 || tenant.NextCursor == "" {
		t.Fatalf("tenant page: %+v %v", tenant, err)
	}
	remaining, err := db.ListTenantInstanceViews(ctx, "view-owner", organization.ID, tenant.NextCursor, 2)
	if err != nil || len(remaining.Items) != 1 || remaining.NextCursor != "" {
		t.Fatalf("tenant continuation: %+v %v", remaining, err)
	}
	visible, err := db.GetTenantInstanceView(ctx, "view-owner", organization.ID, created.Server.ID)
	if err != nil || visible.Deployment == nil || visible.Deployment.ActualState != "running" || visible.LatestOperation == nil || visible.LatestOperation.Status != "succeeded" {
		t.Fatalf("tenant detail: %+v %v", visible, err)
	}
	encoded, err := json.Marshal(visible)
	if err != nil || strings.Contains(string(encoded), "secret-ciphertext") || strings.Contains(string(encoded), "private-node") || strings.Contains(string(encoded), "private-runtime") {
		t.Fatalf("tenant read leaked protected or infrastructure data: %s %v", encoded, err)
	}
	if _, err := db.ListTenantInstanceViews(ctx, "view-outsider", organization.ID, "", 10); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign tenant read: %v", err)
	}
	if _, err := db.ListPlatformInstanceViews(ctx, "view-outsider", "", "", 10); !errors.Is(err, instanceview.ErrOperatorRequired) {
		t.Fatalf("non-operator platform read: %v", err)
	}
	platform, err := db.GetPlatformInstanceView(ctx, "view-operator", created.Server.ID)
	if err != nil || platform.NodeID != "private-node" || platform.TaskID != "view-task" || platform.OrganizationID != organization.ID {
		t.Fatalf("platform detail: %+v %v", platform, err)
	}
}
