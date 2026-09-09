package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/entitlements"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

func TestServerEntitlements(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "entitlements.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testServerEntitlements(t, db)
}

func testServerEntitlements(t *testing.T, db *Store) {
	t.Helper()
	ctx := context.Background()
	org := domain.Organization{ID: "ent-owner", Slug: "ent-owner"}
	if err := db.CreateOrganization(ctx, &org, "ent-user"); err != nil {
		t.Fatal(err)
	}
	admin := domain.AdminAccount{ID: "ent-admin", Username: "ent-admin", Role: domain.RoleAdmin, PasswordHash: "test"}
	if err := db.CreateAdminAccount(ctx, &admin); err != nil {
		t.Fatal(err)
	}
	created, err := db.CreateGlobalServer(ctx, "ent-user", instances.CreateRequest{OrganizationID: org.ID, Name: "ent-server", RegionID: "ent-east", IdempotencyKey: "ent-create", Specification: instances.Specification{ProviderKey: "test", GameVersion: "1", ConfigSchemaVersion: 1, Configuration: instances.ProtectedConfiguration{KeyID: "test", Ciphertext: []byte("opaque")}, Resources: instances.Resources{CPU: 1, MemoryMB: 128}}})
	if err != nil {
		t.Fatal(err)
	}
	var row struct{ Payload string }
	if err := db.db.Table("server_outbox").Where("operation_id = ?", created.Operation.ID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	var event instances.RevisionAvailable
	if err := json.Unmarshal([]byte(row.Payload), &event); err != nil {
		t.Fatal(err)
	}
	denied := func(region string, intent int64) {
		t.Helper()
		got, err := db.RegionalRunEntitlement(ctx, region, event, intent)
		if !errors.Is(err, entitlements.ErrUnavailable) || got.Version != 0 {
			t.Fatalf("unauthorized entitlement: %+v %v", got, err)
		}
	}
	denied("ent-east", 1)
	now := time.Now().UnixMilli()
	change := entitlements.OperatorChange{Policy: entitlements.Policy{OrganizationID: org.ID, ServerID: created.Server.ID, CPU: 1, MemoryMB: 128, StartsAtMS: now - 1000, EndsAtMS: now + 3600000, Status: "active"}, RequestID: "grant", Reason: "operator test"}
	if _, err := db.ChangeOperatorEntitlement(ctx, "ent-user", change); !errors.Is(err, entitlements.ErrOperatorRequired) {
		t.Fatalf("tenant issued entitlement: %v", err)
	}
	first, err := db.ChangeOperatorEntitlement(ctx, admin.ID, change)
	if err != nil || first.Version != 1 || first.SourceKind != "operator" || first.SourceID == "" {
		t.Fatalf("grant: %+v %v", first, err)
	}
	replay, err := db.ChangeOperatorEntitlement(ctx, admin.ID, change)
	if err != nil || replay != first {
		t.Fatalf("replay: %+v %v", replay, err)
	}
	conflict := change
	conflict.Reason = "different"
	if _, err := db.ChangeOperatorEntitlement(ctx, admin.ID, conflict); !errors.Is(err, entitlements.ErrRequestConflict) {
		t.Fatalf("request conflict: %v", err)
	}
	got, err := db.RegionalRunEntitlement(ctx, "ent-east", event, 1)
	if err != nil || got != first {
		t.Fatalf("read: %+v %v", got, err)
	}
	denied("ent-west", 1)
	denied("ent-east", 2)
	next := change
	next.RequestID = "suspend"
	next.Status = "suspended"
	if _, err := db.ChangeOperatorEntitlement(ctx, admin.ID, next); !errors.Is(err, entitlements.ErrVersionConflict) {
		t.Fatalf("stale write: %v", err)
	}
	next.ExpectedVersion = 1
	second, err := db.ChangeOperatorEntitlement(ctx, admin.ID, next)
	if err != nil || second.Version != 2 {
		t.Fatalf("suspend: %+v %v", second, err)
	}
	denied("ent-east", 1)
	replay, err = db.ChangeOperatorEntitlement(ctx, admin.ID, change)
	if err != nil || replay != first {
		t.Fatalf("historical replay: %+v %v", replay, err)
	}
	var current entitlements.Record
	if err := db.db.Table("global_server_entitlements").Where("server_id = ?", created.Server.ID).Take(&current).Error; err != nil || current != second {
		t.Fatalf("replay changed current: %+v %v", current, err)
	}
	for _, statement := range []string{"UPDATE global_entitlement_changes SET reason = 'rewrite' WHERE source_id = ?", "DELETE FROM global_entitlement_changes WHERE source_id = ?"} {
		if err := db.db.Exec(statement, first.SourceID).Error; err == nil {
			t.Fatal("audit mutation permitted")
		}
	}
	// Force the second write to fail: the current-policy update must roll back too.
	if db.db.Dialector.Name() == "postgres" {
		if err := db.db.Exec("ALTER TABLE global_entitlement_changes ADD CONSTRAINT test_entitlement_failure CHECK(reason <> 'fail-commit')").Error; err != nil {
			t.Fatal(err)
		}
		defer db.db.Exec("ALTER TABLE global_entitlement_changes DROP CONSTRAINT test_entitlement_failure")
	} else {
		if err := db.db.Exec("CREATE TRIGGER test_entitlement_failure BEFORE INSERT ON global_entitlement_changes WHEN NEW.reason = 'fail-commit' BEGIN SELECT RAISE(ABORT, 'test failure'); END").Error; err != nil {
			t.Fatal(err)
		}
		defer db.db.Exec("DROP TRIGGER test_entitlement_failure")
	}
	next.ExpectedVersion = 2
	next.RequestID = "failed"
	next.Reason = "fail-commit"
	next.Status = "active"
	if _, err := db.ChangeOperatorEntitlement(ctx, admin.ID, next); err == nil {
		t.Fatal("expected audit failure")
	}
	if err := db.db.Table("global_server_entitlements").Where("server_id = ?", created.Server.ID).Take(&current).Error; err != nil || current != second {
		t.Fatalf("partial commit: %+v %v", current, err)
	}
	// Two writers based on the same version cannot both grant a new revision.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		write := func(i int) {
			defer wg.Done()
			concurrent := change
			concurrent.ExpectedVersion = 2
			concurrent.RequestID = fmt.Sprintf("concurrent-%d", i)
			_, err := db.ChangeOperatorEntitlement(ctx, admin.ID, concurrent)
			results <- err
		}
		if db.db.Dialector.Name() == "postgres" {
			go write(i)
		} else {
			write(i)
		}
	}
	wg.Wait()
	close(results)
	success, stale := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, entitlements.ErrVersionConflict) {
			stale++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || stale != 1 {
		t.Fatalf("concurrent outcomes: %d success, %d stale", success, stale)
	}
	version := int64(3)
	for _, tc := range []struct {
		name   string
		mutate func(*entitlements.OperatorChange)
	}{
		{"cpu", func(c *entitlements.OperatorChange) { c.CPU = 0.5 }},
		{"memory", func(c *entitlements.OperatorChange) { c.MemoryMB = 64 }},
		{"future", func(c *entitlements.OperatorChange) { c.StartsAtMS = now + 1800000 }},
		{"revoked", func(c *entitlements.OperatorChange) { c.Status = "revoked" }},
	} {
		c := change
		c.RequestID = tc.name
		c.ExpectedVersion = version
		tc.mutate(&c)
		if _, err := db.ChangeOperatorEntitlement(ctx, admin.ID, c); err != nil {
			t.Fatal(err)
		}
		version++
		denied("ent-east", 1)
	}
	expired := change
	expired.RequestID = "expired"
	expired.ExpectedVersion = version
	expired.StartsAtMS = now - 2000
	expired.EndsAtMS = now - 1000
	if _, err := db.ChangeOperatorEntitlement(ctx, admin.ID, expired); !errors.Is(err, entitlements.ErrInvalid) {
		t.Fatalf("expired grant: %v", err)
	}
	// Restoring rights must not overwrite an independently recorded user stop.
	if err := db.db.Table("logical_servers").Where("id = ?", created.Server.ID).Updates(map[string]any{"desired_state": "stopped", "intent_version": 2}).Error; err != nil {
		t.Fatal(err)
	}
	restored := change
	restored.RequestID = "restore"
	restored.ExpectedVersion = version
	if _, err := db.ChangeOperatorEntitlement(ctx, admin.ID, restored); err != nil {
		t.Fatal(err)
	}
	denied("ent-east", 2)

}
