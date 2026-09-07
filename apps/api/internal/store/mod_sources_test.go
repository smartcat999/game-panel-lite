package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestModSources(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "sources.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testModSources(t, db)
}

func testModSources(t *testing.T, db *Store) {
	ctx := context.Background()
	org := domain.Organization{ID: "source-workspace", Slug: "source-workspace"}
	if err := db.CreateOrganization(ctx, &org, "source-owner"); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateTenantQuota(ctx, domain.TenantQuota{OrganizationID: org.ID, MaxServers: 10, MaxCPUCores: 10, MaxMemoryMB: 10240}); err != nil {
		t.Fatal(err)
	}
	target := domain.GameServer{ID: "source-target", OrganizationID: org.ID, ProviderKey: domain.ProviderTerrariaTModLoader, Spec: domain.ServerSpec{Generation: 1, Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 1024}}}
	mods := []domain.ModFile{
		{ID: "source-own", InstanceID: "unassigned", OrganizationID: org.ID},
		{ID: "source-foreign", InstanceID: "unassigned", OrganizationID: "other-workspace"},
		{ID: "source-legacy", InstanceID: "unassigned"},
		{ID: "source-installed", InstanceID: target.ID},
		{ID: "source-other-instance", InstanceID: "another-instance", OrganizationID: org.ID},
		{ID: "source-other-provider", InstanceID: "unassigned", OrganizationID: org.ID, ProviderKey: domain.ProviderDST},
	}
	for i := range mods {
		if mods[i].ProviderKey == "" {
			mods[i].ProviderKey = target.ProviderKey
		}
		mods[i].FileName = mods[i].ID + ".tmod"
		if err := db.CreateMod(ctx, &mods[i]); err != nil {
			t.Fatal(err)
		}
	}
	target.Spec.ModIDs = []string{mods[0].ID}
	if err := db.CreateAllocatedGameServer(ctx, "source-owner", &target); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{mods[0].ID, mods[3].ID} {
		item, err := db.GetModForServer(ctx, target, id)
		if err != nil || item.ID != id {
			t.Fatalf("allowed %s: %+v %v", id, item, err)
		}
	}
	for _, id := range []string{mods[1].ID, mods[2].ID, mods[4].ID, "missing"} {
		if _, err := db.GetModForServer(ctx, target, id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("foreign source %s: %v", id, err)
		}
	}
	if _, err := db.GetModForServer(ctx, target, mods[5].ID); !errors.Is(err, ErrInvalidModLibrary) {
		t.Fatalf("provider: %v", err)
	}
	list, err := db.ListLibraryModsForServer(ctx, target)
	if err != nil || len(list) != 2 {
		t.Fatalf("library: %+v %v", list, err)
	}
	for _, item := range list {
		if item.OrganizationID != org.ID || item.InstanceID != "unassigned" {
			t.Fatalf("leaked: %+v", item)
		}
	}
	for _, id := range []string{mods[1].ID, mods[2].ID, mods[4].ID, mods[5].ID, "missing"} {
		after := target
		after.Spec.ModIDs = []string{mods[0].ID, id}
		if err := db.SaveAllocatedGameServer(ctx, "source-owner", target, after); !errors.Is(err, ErrInvalidModLibrary) {
			t.Fatalf("save %s: %v", id, err)
		}
		if err := db.SaveGameServer(ctx, &after); !errors.Is(err, ErrInvalidModLibrary) {
			t.Fatalf("legacy save %s: %v", id, err)
		}
		after.ID = "source-invalid-create"
		if err := db.CreateAllocatedGameServer(ctx, "source-owner", &after); !errors.Is(err, ErrInvalidModLibrary) {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	if err := db.DeleteOwnedLibraryMod(ctx, "source-owner", mods[0]); !errors.Is(err, ErrInvalidModLibrary) {
		t.Fatalf("deleted desired source: %v", err)
	}
	after := target
	after.Spec.Generation++
	after.Spec.ModIDs = nil
	if err := db.SaveAllocatedGameServer(ctx, "source-owner", target, after); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetModForServer(ctx, target, mods[0].ID); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("stale get: %v", err)
	}
	if _, err := db.ListLibraryModsForServer(ctx, target); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("stale list: %v", err)
	}
	if err := db.DeleteOwnedLibraryMod(ctx, "source-owner", mods[0]); err != nil {
		t.Fatalf("delete unreferenced: %v", err)
	}
	// The reference and delete operations must serialize under the same workspace lock.
	for i := 0; i < 3; i++ {
		item := mods[0]
		item.ID = fmt.Sprintf("source-race-mod-%d", i)
		item.FileName = item.ID + ".tmod"
		if err := db.CreateOwnedLibraryMod(ctx, "source-owner", &item); err != nil {
			t.Fatal(err)
		}
		instance := after
		instance.ID = fmt.Sprintf("source-race-server-%d", i)
		instance.Spec.ModIDs = []string{item.ID}
		start := make(chan struct{})
		created := make(chan error, 1)
		deleted := make(chan error, 1)
		go func() { <-start; created <- db.CreateAllocatedGameServer(ctx, "source-owner", &instance) }()
		go func() { <-start; deleted <- db.DeleteOwnedLibraryMod(ctx, "source-owner", item) }()
		close(start)
		createErr, deleteErr := <-created, <-deleted
		if createErr == nil {
			if !errors.Is(deleteErr, ErrInvalidModLibrary) {
				t.Fatalf("reference lost race: %v", deleteErr)
			}
		} else if deleteErr != nil || !errors.Is(createErr, ErrInvalidModLibrary) {
			t.Fatalf("delete/create race: %v / %v", deleteErr, createErr)
		}
	}

}
