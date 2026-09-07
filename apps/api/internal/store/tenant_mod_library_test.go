package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestTenantModLibrary(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testTenantModLibrary(t, db)
}
func testTenantModLibrary(t *testing.T, db *Store) {
	ctx := context.Background()
	for _, id := range []string{"library-a", "library-b"} {
		org := domain.Organization{ID: id, Slug: id}
		if err := db.CreateOrganization(ctx, &org, id); err != nil {
			t.Fatal(err)
		}
	}
	item := domain.ModFile{ID: "owned-library-mod", InstanceID: "unassigned", OrganizationID: "library-a", ProviderKey: domain.ProviderTerrariaTModLoader, FileName: "same.tmod", WorkshopID: "123", Source: "workshop", Enabled: true}
	if err := db.CreateOwnedLibraryMod(ctx, "library-b", &item); !errors.Is(err, ErrWorkspaceWriteDenied) {
		t.Fatalf("foreign create: %v", err)
	}
	if err := db.CreateOwnedLibraryMod(ctx, "library-a", &item); err != nil {
		t.Fatal(err)
	}
	duplicate := item
	duplicate.ID = "duplicate"
	if err := db.CreateOwnedLibraryMod(ctx, "library-a", &duplicate); !errors.Is(err, ErrInvalidModLibrary) {
		t.Fatalf("duplicate name/workshop: %v", err)
	}
	duplicate.FileName = "different.tmod"
	if err := db.CreateOwnedLibraryMod(ctx, "library-a", &duplicate); !errors.Is(err, ErrInvalidModLibrary) {
		t.Fatalf("duplicate workshop: %v", err)
	}
	foreign := item
	foreign.ID = "foreign-library-mod"
	foreign.OrganizationID = "library-b"
	if err := db.CreateOwnedLibraryMod(ctx, "library-b", &foreign); err != nil {
		t.Fatalf("same name in another space: %v", err)
	}
	otherProvider := item
	otherProvider.ID = "other-provider-mod"
	otherProvider.ProviderKey = domain.ProviderDST
	if err := db.CreateOwnedLibraryMod(ctx, "library-a", &otherProvider); err != nil {
		t.Fatalf("same identity in another provider: %v", err)
	}
	legacy := item
	legacy.ID = "legacy-library-mod"
	legacy.OrganizationID = ""
	if err := db.CreateMod(ctx, &legacy); err != nil {
		t.Fatal(err)
	}
	instance := item
	instance.ID = "instance-library-mod"
	instance.InstanceID = "server"
	if err := db.CreateMod(ctx, &instance); err != nil {
		t.Fatal(err)
	}
	list, err := db.ListUserLibraryMods(ctx, "library-a")
	if err != nil || len(list) != 2 {
		t.Fatalf("scoped library: %+v %v", list, err)
	}
	for _, id := range []string{legacy.ID, foreign.ID, instance.ID} {
		if _, err := db.GetUserLibraryMod(ctx, "library-a", id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("leaked %s: %v", id, err)
		}
	}
	global, err := db.ListMods(ctx, "unassigned")
	if err != nil || len(global) != 1 || global[0].ID != legacy.ID {
		t.Fatalf("legacy library mixed: %+v %v", global, err)
	}
	found, err := db.GetModByInstanceAndFile(ctx, "unassigned", "same.tmod")
	if err != nil || found.ID != legacy.ID {
		t.Fatalf("legacy identity mixed: %+v %v", found, err)
	}
	if _, err := db.GetMod(ctx, item.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("legacy get leaked owned item: %v", err)
	}
	pack := domain.ModPack{ID: "owned-mod-pack", OrganizationID: "library-a", Name: "Pack", ModIDsJSON: `["owned-library-mod"]`}
	for _, ids := range []string{`["foreign-library-mod"]`, `["legacy-library-mod"]`, `["instance-library-mod"]`, `["owned-library-mod","other-provider-mod"]`, `["owned-library-mod","owned-library-mod"]`, `[]`, `null`, `broken`} {
		invalid := pack
		invalid.ModIDsJSON = ids
		if err := db.CreateOwnedModPack(ctx, "library-a", &invalid); !errors.Is(err, ErrInvalidModLibrary) {
			t.Fatalf("invalid refs %s: %v", ids, err)
		}
	}
	if err := db.CreateOwnedModPack(ctx, "library-b", &pack); !errors.Is(err, ErrWorkspaceWriteDenied) {
		t.Fatalf("foreign pack create: %v", err)
	}
	if err := db.CreateOwnedModPack(ctx, "library-a", &pack); err != nil {
		t.Fatal(err)
	}
	packs, err := db.ListUserModPacks(ctx, "library-a")
	if err != nil || len(packs) != 1 {
		t.Fatalf("owned packs: %+v %v", packs, err)
	}
	if _, err := db.GetUserModPack(ctx, "library-b", pack.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign pack get: %v", err)
	}
	if packs, err := db.ListModPacks(ctx); err != nil || len(packs) != 0 {
		t.Fatalf("legacy pack leak: %+v %v", packs, err)
	}
	if err := db.DeleteOwnedLibraryMod(ctx, "library-a", item); !errors.Is(err, ErrInvalidModLibrary) {
		t.Fatalf("deleted referenced mod: %v", err)
	}
	after := item
	after.Enabled = false
	after.Title = "updated"
	after.TModVersion = "v2"
	if err := db.SaveOwnedLibraryMod(ctx, "library-b", item, after); !errors.Is(err, ErrWorkspaceWriteDenied) {
		t.Fatalf("foreign edit: %v", err)
	}
	if err := db.SaveOwnedLibraryMod(ctx, "library-a", item, after); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveOwnedLibraryMod(ctx, "library-a", item, after); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("stale edit: %v", err)
	}
	item, err = db.GetUserLibraryMod(ctx, "library-a", item.ID)
	if err != nil || item.Enabled || item.Title != "updated" || item.TModVersion != "v2" || item.Revision != 1 {
		t.Fatalf("saved metadata: %+v %v", item, err)
	}
	moved := item
	moved.OrganizationID = "library-b"
	if err := db.SaveOwnedLibraryMod(ctx, "library-a", item, moved); !errors.Is(err, ErrInvalidModLibrary) {
		t.Fatalf("ownership move: %v", err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- db.SaveOwnedModPack(ctx, "library-a", pack, pack) }()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrReconciliationSuperseded) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("accepted concurrent writers: %d", successes)
	}
	if err := db.DeleteOwnedModPack(ctx, "library-a", pack); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("stale pack delete: %v", err)
	}
	pack, err = db.GetUserModPack(ctx, "library-a", pack.ID)
	if err != nil {
		t.Fatal(err)
	}
	preset := domain.ConfigPreset{ID: "library-referencing-preset", OrganizationID: "library-a", ProviderKey: domain.ProviderTerrariaTModLoader, ModPackID: pack.ID, ModIDsJSON: `["owned-library-mod"]`}
	if err := db.CreateOwnedConfigPreset(ctx, "library-a", &preset); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteOwnedModPack(ctx, "library-a", pack); !errors.Is(err, ErrInvalidModLibrary) {
		t.Fatalf("referenced pack deletion: %v", err)
	}
	changed := pack
	changed.ModIDsJSON = `["other-provider-mod"]`
	if err := db.SaveOwnedModPack(ctx, "library-a", pack, changed); !errors.Is(err, ErrInvalidModLibrary) {
		t.Fatalf("changed referenced pack provider: %v", err)
	}
	invalidPreset := preset
	invalidPreset.ModIDsJSON = `["foreign-library-mod"]`
	if err := db.SaveOwnedConfigPreset(ctx, "library-a", preset, invalidPreset); !errors.Is(err, ErrInvalidModLibrary) {
		t.Fatalf("foreign preset reference: %v", err)
	}
	if err := db.DeleteOwnedConfigPreset(ctx, "library-a", preset); err != nil {
		t.Fatal(err)
	}
	viewer := domain.OrganizationMember{ID: "library-viewer", OrganizationID: "library-a", UserID: "library-viewer", Role: domain.RoleViewer}
	if err := db.AddOrganizationMember(ctx, &viewer); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetUserLibraryMod(ctx, viewer.UserID, item.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteOwnedModPack(ctx, viewer.UserID, pack); !errors.Is(err, ErrWorkspaceWriteDenied) {
		t.Fatalf("viewer delete: %v", err)
	}
	if err := db.SaveOwnedLibraryMod(ctx, viewer.UserID, item, item); !errors.Is(err, ErrWorkspaceWriteDenied) {
		t.Fatalf("viewer edit: %v", err)
	}
	if err := db.RemoveOrganizationMember(ctx, "library-a", "library-a"); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveOwnedLibraryMod(ctx, "library-a", item, item); !errors.Is(err, ErrWorkspaceWriteDenied) {
		t.Fatalf("revoked mod edit: %v", err)
	}
	if err := db.DeleteOwnedModPack(ctx, "library-a", pack); !errors.Is(err, ErrWorkspaceWriteDenied) {
		t.Fatalf("revoked pack delete: %v", err)
	}
	if list, err := db.ListUserLibraryMods(ctx, "library-a"); err != nil || len(list) != 0 {
		t.Fatalf("revoked read: %+v %v", list, err)
	}
	if err := db.DeleteOwnedModPack(ctx, "", pack); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveOwnedModPack(ctx, "", pack, pack); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("pack resurrected: %v", err)
	}
	if err := db.DeleteOwnedLibraryMod(ctx, "", item); err != nil {
		t.Fatal(err)
	}
	preset.ID = "deleted-reference-preset"
	preset.ModPackID = ""
	if err := db.CreateOwnedConfigPreset(ctx, "", &preset); !errors.Is(err, ErrInvalidModLibrary) {
		t.Fatalf("deleted mod reference accepted: %v", err)
	}
	if err := db.SaveOwnedLibraryMod(ctx, "", item, item); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("mod resurrected: %v", err)
	}

	// Pack creation and metadata deletion must never both commit successfully.
	racing := domain.ModFile{ID: "racing-library-mod", InstanceID: "unassigned", OrganizationID: "library-b", ProviderKey: domain.ProviderDST, FileName: "racing.zip"}
	if err := db.CreateOwnedLibraryMod(ctx, "library-b", &racing); err != nil {
		t.Fatal(err)
	}
	racingPack := domain.ModPack{ID: "racing-library-pack", Name: "Race", OrganizationID: "library-b", ModIDsJSON: `["racing-library-mod"]`}
	start := make(chan struct{})
	raceResults := make(chan error, 2)
	go func() { <-start; raceResults <- db.CreateOwnedModPack(ctx, "library-b", &racingPack) }()
	go func() { <-start; raceResults <- db.DeleteOwnedLibraryMod(ctx, "library-b", racing) }()
	close(start)
	successes = 0
	for i := 0; i < 2; i++ {
		err := <-raceResults
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrInvalidModLibrary) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("reference/delete accepted %d operations", successes)
	}
}
