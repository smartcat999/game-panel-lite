package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestTenantPresets(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "presets.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testTenantPresets(t, db)
}
func testTenantPresets(t *testing.T, db *Store) {
	ctx := context.Background()
	for _, id := range []string{"preset-a", "preset-b"} {
		org := domain.Organization{ID: id, Slug: id}
		if err := db.CreateOrganization(ctx, &org, id); err != nil {
			t.Fatal(err)
		}
	}
	preset := domain.ConfigPreset{ID: "owned-preset", OrganizationID: "preset-a", Name: "Original", ConfigPayloadJSON: `{"name":"original"}`, UpdatedAt: time.Now()}
	if err := db.CreateOwnedConfigPreset(ctx, "preset-b", &preset); !errors.Is(err, ErrWorkspaceWriteDenied) {
		t.Fatalf("foreign create: %v", err)
	}
	if err := db.CreateOwnedConfigPreset(ctx, "preset-a", &preset); err != nil {
		t.Fatal(err)
	}
	legacy := domain.ConfigPreset{ID: "unowned-preset"}
	if err := db.CreateConfigPreset(ctx, &legacy); err != nil {
		t.Fatal(err)
	}
	list, err := db.ListUserConfigPresets(ctx, "preset-a")
	if err != nil || len(list) != 1 || list[0].ID != preset.ID {
		t.Fatalf("scoped list: %v %v", list, err)
	}
	if _, err := db.GetUserConfigPreset(ctx, "preset-b", preset.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign get: %v", err)
	}
	if _, err := db.GetUserConfigPreset(ctx, "preset-a", legacy.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unowned get: %v", err)
	}
	before, err := db.GetConfigPreset(ctx, preset.ID)
	if err != nil {
		t.Fatal(err)
	}
	after := before
	after.Name = "Updated" // Same timestamp must still invalidate the old revision.
	if err := db.SaveOwnedConfigPreset(ctx, "preset-b", before, after); !errors.Is(err, ErrWorkspaceWriteDenied) {
		t.Fatalf("foreign edit: %v", err)
	}
	if err := db.SaveOwnedConfigPreset(ctx, "preset-a", before, after); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveOwnedConfigPreset(ctx, "preset-a", before, after); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("stale edit: %v", err)
	}
	if err := db.DeleteOwnedConfigPreset(ctx, "preset-a", before); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("stale delete: %v", err)
	}
	before, err = db.GetConfigPreset(ctx, preset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Revision != 1 || before.Name != "Updated" {
		t.Fatalf("edit not committed: %+v", before)
	}

	var writers sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		writers.Add(1)
		go func() { defer writers.Done(); results <- db.SaveOwnedConfigPreset(ctx, "preset-a", before, before) }()
	}
	writers.Wait()
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
		t.Fatalf("accepted %d concurrent writers", successes)
	}
	before, err = db.GetConfigPreset(ctx, preset.ID)
	if err != nil {
		t.Fatal(err)
	}
	member := domain.OrganizationMember{ID: "preset-viewer-member", OrganizationID: "preset-a", UserID: "preset-viewer", Role: domain.RoleViewer}
	if err := db.AddOrganizationMember(ctx, &member); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetUserConfigPreset(ctx, "preset-viewer", preset.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteOwnedConfigPreset(ctx, "preset-viewer", before); !errors.Is(err, ErrWorkspaceWriteDenied) {
		t.Fatalf("viewer delete: %v", err)
	}
	after = before
	after.OrganizationID = "preset-b"
	if err := db.SaveOwnedConfigPreset(ctx, "preset-a", before, after); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("ownership move: %v", err)
	}
	if err := db.RemoveOrganizationMember(ctx, "preset-a", "preset-a"); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveOwnedConfigPreset(ctx, "preset-a", before, before); !errors.Is(err, ErrWorkspaceWriteDenied) {
		t.Fatalf("revoked edit: %v", err)
	}
	if err := db.DeleteOwnedConfigPreset(ctx, "preset-a", before); !errors.Is(err, ErrWorkspaceWriteDenied) {
		t.Fatalf("revoked delete: %v", err)
	}
	list, err = db.ListUserConfigPresets(ctx, "preset-a")
	if err != nil || len(list) != 0 {
		t.Fatalf("revoked list: %v %v", list, err)
	}
	if err := db.DeleteOwnedConfigPreset(ctx, "", before); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveOwnedConfigPreset(ctx, "", before, before); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("deleted preset resurrected: %v", err)
	}
}
