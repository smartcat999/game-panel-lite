package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestTenantBackupQueries(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testTenantBackupQueries(t, db)
}

func testTenantBackupQueries(t *testing.T, db *Store) {
	t.Helper()
	ctx := context.Background()
	for _, id := range []string{"backup-scope-a", "backup-scope-b"} {
		org := domain.Organization{ID: id, Slug: id}
		if err := db.CreateOrganization(ctx, &org, id); err != nil {
			t.Fatal(err)
		}
		server := domain.GameServer{ID: id, OrganizationID: id}
		if err := db.CreateGameServer(ctx, &server); err != nil {
			t.Fatal(err)
		}
		backup := domain.Backup{ID: id, InstanceID: id, FileName: "snapshot.zip"}
		if err := db.CreateBackup(ctx, &backup); err != nil {
			t.Fatal(err)
		}
	}
	orphan := domain.Backup{ID: "backup-scope-orphan", InstanceID: "missing-server"}
	if err := db.CreateBackup(ctx, &orphan); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"backup-scope-a", "backup-scope-b"} {
		items, err := db.ListUserBackups(ctx, id)
		if err != nil || len(items) != 1 || items[0].ID != id {
			t.Fatalf("scoped backups: %+v %v", items, err)
		}
		if _, err := db.GetUserBackup(ctx, id, id); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"backup-scope-b", "backup-scope-orphan", "missing-backup"} {
		if _, err := db.GetUserBackup(ctx, "backup-scope-a", id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("foreign or orphan %s: %v", id, err)
		}
	}
	if items, err := db.ListUserBackups(ctx, ""); err != nil || len(items) != 0 {
		t.Fatalf("empty principal: %+v %v", items, err)
	}
	if err := db.RemoveOrganizationMember(ctx, "backup-scope-a", "backup-scope-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetUserBackup(ctx, "backup-scope-a", "backup-scope-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revocation: %v", err)
	}
}
