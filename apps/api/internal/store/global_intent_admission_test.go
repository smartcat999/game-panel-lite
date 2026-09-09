package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regions"
)

func TestGlobalIntentAdmissionChecksTenantRegionAndAssets(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "admission.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	organization := domain.Organization{ID: "admission-tenant", Slug: "admission-tenant"}
	if err := db.CreateOrganization(ctx, &organization, "admission-owner"); err != nil {
		t.Fatal(err)
	}
	region, err := db.RegisterRegion(ctx, "admission-east", "Admission east")
	if err != nil {
		t.Fatal(err)
	}
	admission, err := NewGlobalIntentAdmission(db)
	if err != nil {
		t.Fatal(err)
	}
	request := instances.CreateRequest{OrganizationID: organization.ID, Name: "server", RegionID: region.ID, IdempotencyKey: "create", Specification: instances.Specification{
		ProviderKey: "provider", GameVersion: "1", ConfigSchemaVersion: 1, Resources: instances.Resources{CPU: 1, MemoryMB: 128},
	}}
	if err := admission.CheckCreate(ctx, "admission-owner", request); !errors.Is(err, regions.ErrRegionUnavailable) {
		t.Fatalf("closed Region accepted: %v", err)
	}
	if err := db.SetRegionAcceptingCreates(ctx, region.ID, region.Version, true); err != nil {
		t.Fatal(err)
	}
	if err := admission.CheckCreate(ctx, "outsider", request); !errors.Is(err, ErrWorkspaceWriteDenied) {
		t.Fatalf("foreign member accepted: %v", err)
	}
	request.Specification.Assets = []instances.AssetVersion{{AssetID: "foreign", Version: "1"}}
	if err := admission.CheckCreate(ctx, "admission-owner", request); !errors.Is(err, assets.ErrUnavailable) {
		t.Fatalf("unavailable asset accepted: %v", err)
	}
	request.Specification.Assets = nil
	if err := admission.CheckCreate(ctx, "admission-owner", request); err != nil {
		t.Fatalf("valid admission rejected: %v", err)
	}
}
