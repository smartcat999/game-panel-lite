package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestModInstallationIntent(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "installation.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testModInstallationIntent(t, db)
}
func testModInstallationIntent(t *testing.T, db *Store) {
	ctx := context.Background()
	org := domain.Organization{ID: "intent-space", Slug: "intent-space"}
	if err := db.CreateOrganization(ctx, &org, "intent-owner"); err != nil {
		t.Fatal(err)
	}
	before := domain.GameServer{ID: "intent-server", OrganizationID: org.ID, ProviderKey: domain.ProviderTerrariaTModLoader, Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredStopped}, Status: domain.ServerRuntimeStatus{Phase: domain.PhaseStopped}}
	if err := db.CreateGameServer(ctx, &before); err != nil {
		t.Fatal(err)
	}
	source := domain.ModFile{ID: "intent-source", InstanceID: "unassigned", OrganizationID: org.ID, ProviderKey: before.ProviderKey, FileName: "intent.tmod", Source: "upload"}
	if err := db.CreateOwnedLibraryMod(ctx, "intent-owner", &source); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetUserGameServer(ctx, "outsider", before.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unscoped server read: %v", err)
	}
	after := before
	after.Spec.ModIDs = []string{source.ID}
	after.Spec.Generation++
	if err := db.SaveModInstallationIntent(ctx, "outsider", before, after, source); !errors.Is(err, ErrWorkspaceWriteDenied) {
		t.Fatalf("foreign commit: %v", err)
	}
	revoked := domain.OrganizationMember{OrganizationID: org.ID, UserID: "intent-revoked", Role: domain.RoleMember}
	if err := db.AddOrganizationMember(ctx, &revoked); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetUserGameServer(ctx, revoked.UserID, before.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.RemoveOrganizationMember(ctx, org.ID, revoked.UserID); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveModInstallationIntent(ctx, revoked.UserID, before, after, source); !errors.Is(err, ErrWorkspaceWriteDenied) {
		t.Fatalf("revoked commit: %v", err)
	}
	changed := before
	changed.Status.Phase = domain.PhaseRunning
	if err := db.SaveGameServer(ctx, &changed); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveModInstallationIntent(ctx, "intent-owner", before, after, source); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("stale state: %v", err)
	}
	if err := db.SaveGameServer(ctx, &before); err != nil {
		t.Fatal(err)
	}
	revised := source
	revised.Title = "new metadata"
	if err := db.SaveOwnedLibraryMod(ctx, "intent-owner", source, revised); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveModInstallationIntent(ctx, "intent-owner", before, after, source); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("stale source: %v", err)
	}
	source, err := db.GetUserLibraryMod(ctx, "intent-owner", source.ID)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { <-start; results <- db.SaveModInstallationIntent(ctx, "intent-owner", before, after, source) }()
	}
	close(start)
	accepted := 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			accepted++
		} else if !errors.Is(err, ErrReconciliationSuperseded) {
			t.Fatal(err)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted concurrent requests: %d", accepted)
	}
	saved, err := db.GetGameServer(ctx, before.ID)
	if err != nil || saved.Spec.Generation != 2 || len(saved.Spec.ModIDs) != 1 || saved.Status.Phase != domain.PhaseStopped {
		t.Fatalf("saved: %+v %v", saved, err)
	}
	if err := db.DeleteOwnedLibraryMod(ctx, "intent-owner", source); !errors.Is(err, ErrInvalidModLibrary) {
		t.Fatalf("deleted requested source: %v", err)
	}
}
