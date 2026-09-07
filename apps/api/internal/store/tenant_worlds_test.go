package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestWorldOwnershipQueries(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testWorldOwnershipQueries(t, db)
}
func testWorldOwnershipQueries(t *testing.T, db *Store) {
	t.Helper()
	ctx := context.Background()
	for _, id := range []string{"world-scope-a", "world-scope-b"} {
		org := domain.Organization{ID: id, Slug: id}
		if err := db.CreateOrganization(ctx, &org, id); err != nil {
			t.Fatal(err)
		}
		item := domain.World{ID: id, OrganizationID: id, InstanceID: "unassigned", FileName: "same.wld"}
		if err := db.CreateWorld(ctx, &item); err != nil {
			t.Fatal(err)
		}
		got, err := db.GetWorldByOrganizationInstanceAndFile(ctx, id, "unassigned", "same.wld")
		if err != nil || got.ID != id {
			t.Fatalf("scoped upsert lookup: %+v %v", got, err)
		}
	}
	items, err := db.ListUserWorlds(ctx, "world-scope-a")
	if err != nil || len(items) != 1 || items[0].ID != "world-scope-a" {
		t.Fatalf("list: %+v %v", items, err)
	}
	if _, err := db.GetUserWorld(ctx, "world-scope-a", "world-scope-b"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign: %v", err)
	}
	if _, err := db.GetUserWorld(ctx, "", "world-scope-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty principal: %v", err)
	}
	if err := db.RemoveOrganizationMember(ctx, "world-scope-a", "world-scope-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetUserWorld(ctx, "world-scope-a", "world-scope-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked: %v", err)
	}
}

func TestSQLiteBackfillsWorldOwnerWithoutReassigningExistingOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	server := domain.GameServer{ID: "source", OrganizationID: "original-org"}
	if err := db.CreateGameServer(ctx, &server); err != nil {
		t.Fatal(err)
	}
	world := domain.World{ID: "legacy", InstanceID: server.ID}
	if err := db.CreateWorld(ctx, &world); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := db.GetWorld(ctx, world.ID)
	if err != nil || got.OrganizationID != "original-org" {
		t.Fatalf("backfill: %+v %v", got, err)
	}
	server.OrganizationID = "new-org"
	if err := db.db.WithContext(ctx).Save(&server).Error; err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err = db.GetWorld(ctx, world.ID)
	if err != nil || got.OrganizationID != "original-org" {
		t.Fatalf("ownership changed: %+v %v", got, err)
	}
}
