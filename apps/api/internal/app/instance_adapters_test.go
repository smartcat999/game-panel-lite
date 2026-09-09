package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/config"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instanceapp"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func TestBuildInstanceProvisionerUsesPublishedPlanAndProtectedWriter(t *testing.T) {
	root := t.TempDir()
	writeKeyring := func(name string, fill byte) string {
		path := filepath.Join(root, name)
		key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{fill}, 32))
		if err := os.WriteFile(path, []byte(`{"active":"v1","keys":{"v1":"`+key+`"}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	db, err := store.Open(filepath.Join(root, "global.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.SeedDefaultPrepaidPlans(ctx); err != nil {
		t.Fatal(err)
	}
	organization := domain.Organization{ID: "provisioner-tenant", Slug: "provisioner-tenant"}
	if err := db.CreateOrganization(ctx, &organization, "provisioner-owner"); err != nil {
		t.Fatal(err)
	}
	registry, err := provider.NewRegistry(terraria.NewVanillaProvider())
	if err != nil {
		t.Fatal(err)
	}
	provisioner, err := buildInstanceProvisioner(config.Config{
		ConfigurationKeyringPath: writeKeyring("configuration.json", 1),
		FingerprintKeyringPath:   writeKeyring("fingerprint.json", 2),
		LogicalConfigMaxBytes:    4096,
	}, db, registry)
	if err != nil {
		t.Fatal(err)
	}
	result, err := provisioner.Create(ctx, "provisioner-owner", instanceapp.CreateCommand{
		OrganizationID: organization.ID, Name: "planned-server", PlanID: "terraria-starter", PlanVersion: 1,
		IdempotencyKey: "planned-create", Configuration: []byte(`{}`),
	})
	if err != nil || result.Server.ID == "" || result.Placement.RegionID != "default" || result.Revision.Specification.Resources.CPU != 2 || result.Revision.Specification.Resources.MemoryMB != 4096 {
		t.Fatalf("planned create: %+v %v", result, err)
	}
	if result.Revision.Specification.Configuration.KeyID != "v1" || len(result.Revision.Specification.Configuration.Ciphertext) == 0 {
		t.Fatalf("configuration was not protected: %+v", result.Revision.Specification.Configuration)
	}
}

func TestBuildInstanceProvisionerRequiresBothKeyrings(t *testing.T) {
	registry, err := provider.NewRegistry(terraria.NewVanillaProvider())
	if err != nil {
		t.Fatal(err)
	}
	if provisioner, err := buildInstanceProvisioner(config.Config{}, nil, registry); err != nil || provisioner != nil {
		t.Fatalf("disabled provisioner: %v %v", provisioner, err)
	}
	if _, err := buildInstanceProvisioner(config.Config{ConfigurationKeyringPath: "configuration.json"}, nil, registry); err == nil {
		t.Fatal("accepted configuration keyring without fingerprint keyring")
	}
	if _, err := buildInstanceProvisioner(config.Config{FingerprintKeyringPath: "fingerprint.json"}, nil, registry); err == nil {
		t.Fatal("accepted fingerprint keyring without configuration keyring")
	}
}
