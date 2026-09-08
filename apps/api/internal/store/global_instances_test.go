package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"gorm.io/gorm"
)

func TestGlobalInstanceIntents(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "global.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testGlobalInstanceIntents(t, db)
}

func testGlobalInstanceIntents(t *testing.T, db *Store) {
	t.Helper()
	ctx := context.Background()
	org := domain.Organization{ID: "global-intents", Slug: "global-intents"}
	if err := db.CreateOrganization(ctx, &org, "global-owner"); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateTenantQuota(ctx, domain.TenantQuota{OrganizationID: org.ID, MaxServers: 2, MaxCPUCores: 4, MaxMemoryMB: 4096, MaxStorageGB: 10}); err != nil {
		t.Fatal(err)
	}
	request := instances.CreateRequest{OrganizationID: org.ID, Name: "Global logical server", RegionID: "east", IdempotencyKey: "purchase-1", Specification: instances.Specification{
		ProviderKey: "example-provider", GameVersion: "1.2.3", ConfigSchemaVersion: 1,
		Configuration: instances.ProtectedConfiguration{KeyID: "test-key", Ciphertext: []byte("opaque-protected-config")},
		Resources:     instances.Resources{CPU: 1, MemoryMB: 512},
	}}
	for _, actor := range []string{"", "another-user"} {
		if _, err := db.CreateGlobalServer(ctx, actor, request); !errors.Is(err, ErrWorkspaceWriteDenied) {
			t.Fatalf("unauthorized creation: %v", err)
		}
	}
	var wg sync.WaitGroup
	results := make(chan instances.IntentResult, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := db.CreateGlobalServer(ctx, "global-owner", request)
			results <- result
			failures <- err
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	var created instances.IntentResult
	for result := range results {
		if created.Server.ID != "" && (created.Server.ID != result.Server.ID || created.Operation.ID != result.Operation.ID || created.Revision.ID != result.Revision.ID) {
			t.Fatal("retry created another logical server, operation or revision")
		}
		created = result
	}
	if created.Placement.RegionID != "east" || created.Placement.PlacementEpoch != 1 || created.Server.SpecGeneration != 1 || created.Operation.Status != "pending" {
		t.Fatalf("incorrect initial identities: %+v", created)
	}
	changed := request
	changed.RegionID = "west"
	if _, err := db.CreateGlobalServer(ctx, "global-owner", changed); !errors.Is(err, instances.ErrIdempotencyConflict) {
		t.Fatalf("same key changed Region: %v", err)
	}
	var payloads []string
	if err := db.db.Table("server_outbox").Select("payload").Pluck("payload", &payloads).Error; err != nil {
		t.Fatal(err)
	}
	if len(payloads) != 1 || strings.Contains(payloads[0], "configuration") || strings.Contains(payloads[0], "opaque-protected") {
		t.Fatalf("outbox contains config or duplicate events: %v", payloads)
	}
	var event instances.RevisionAvailable
	if err := json.Unmarshal([]byte(payloads[0]), &event); err != nil || event.OperationID != created.Operation.ID || event.RevisionID != created.Revision.ID || event.PlacementEpoch != 1 {
		t.Fatalf("outbox identity mismatch: %v", err)
	}
	for _, query := range []string{
		"UPDATE server_revisions SET specification = 'changed' WHERE id = ?",
		"DELETE FROM server_revisions WHERE id = ?",
	} {
		if err := db.db.Exec(query, created.Revision.ID).Error; err == nil {
			t.Fatal("immutable revision mutated")
		}
	}
	if db.db.Dialector.Name() == "sqlite" {
		if err := db.db.Exec("INSERT OR REPLACE INTO server_revisions SELECT id,server_id,spec_generation,'changed',cpu,memory_mb,created_at FROM server_revisions WHERE id = ?", created.Revision.ID).Error; err == nil {
			t.Fatal("SQLite REPLACE bypassed revision immutability")
		}
	}
	// Simulate failure at the last transactional write; no partial reservation,
	// revision, operation or placement may remain after an outbox write fails.
	callbackName := "test:global-outbox-failure"
	if err := db.db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "server_outbox" {
			tx.AddError(errors.New("injected outbox write failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	failed := request
	failed.IdempotencyKey = "failed-create"
	_, failure := db.CreateGlobalServer(ctx, "global-owner", failed)
	if err := db.db.Callback().Create().Remove(callbackName); err != nil {
		t.Fatal(err)
	}
	if failure == nil {
		t.Fatal("outbox failure was hidden")
	}
	for _, table := range []string{"logical_servers", "server_revisions", "server_placements", "server_operations", "server_outbox"} {
		var count int64
		if err := db.db.Table(table).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("%s partial create: %d %v", table, count, err)
		}
	}
	// Two independent edits from the same revision must not silently overwrite
	// each other; the winning revision leaves the original immutable snapshot.
	revise := instances.ReviseRequest{OrganizationID: org.ID, ServerID: created.Server.ID, ExpectedGeneration: 1, IdempotencyKey: "edit-1", Specification: request.Specification}
	revise.Specification.Resources.MemoryMB = 1024
	edited, err := db.ReviseGlobalServer(ctx, "global-owner", revise)
	if err != nil {
		t.Fatal(err)
	}
	if edited.Server.SpecGeneration != 2 || edited.Placement != created.Placement || edited.Server.IntentVersion != 1 || edited.Server.DesiredState != "running" {
		t.Fatalf("configuration edit changed another owner's state: %+v", edited)
	}
	if replay, err := db.ReviseGlobalServer(ctx, "global-owner", revise); err != nil || replay.Operation.ID != edited.Operation.ID {
		t.Fatalf("revision retry failed: %v", err)
	}
	stale := revise
	stale.IdempotencyKey = "edit-2"
	if _, err := db.ReviseGlobalServer(ctx, "global-owner", stale); !errors.Is(err, instances.ErrVersionConflict) {
		t.Fatalf("stale edit accepted: %v", err)
	}
	if replay, err := db.CreateGlobalServer(ctx, "global-owner", request); err != nil || replay.Revision.ID != created.Revision.ID || replay.Server.CurrentRevisionID != edited.Revision.ID {
		t.Fatalf("create replay rolled back current revision: %+v %v", replay, err)
	}
	// A failed delivery record must also roll back an appended revision and
	// its pointer update, not just first creation.
	if err := db.db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "server_outbox" {
			tx.AddError(errors.New("injected revision outbox failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	failedRevision := revise
	failedRevision.ExpectedGeneration = 2
	failedRevision.IdempotencyKey = "failed-revision"
	_, failure = db.ReviseGlobalServer(ctx, "global-owner", failedRevision)
	if err := db.db.Callback().Create().Remove(callbackName); err != nil {
		t.Fatal(err)
	}
	if failure == nil {
		t.Fatal("revision outbox failure was hidden")
	}
	var revisionCount int64
	if err := db.db.Table("server_revisions").Where("server_id = ?", created.Server.ID).Count(&revisionCount).Error; err != nil || revisionCount != 2 {
		t.Fatalf("failed revision remained: %d %v", revisionCount, err)
	}
	concurrent := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			change := failedRevision
			change.IdempotencyKey = fmt.Sprintf("concurrent-revision-%d", i)
			_, err := db.ReviseGlobalServer(ctx, "global-owner", change)
			concurrent <- err
		}(i)
	}
	wg.Wait()
	close(concurrent)
	winners := 0
	for err := range concurrent {
		if err == nil {
			winners++
		} else if !errors.Is(err, instances.ErrVersionConflict) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("concurrent revision winners: %d", winners)
	}
	legacy := domain.GameServer{ID: "global-legacy", OrganizationID: org.ID, Spec: domain.ServerSpec{Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 512}}}
	if err := db.CreateAllocatedGameServer(ctx, "global-owner", &legacy); err != nil {
		t.Fatal(err)
	}
	failed.IdempotencyKey = "over-quota"
	if _, err := db.CreateGlobalServer(ctx, "global-owner", failed); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("global create bypasses legacy reservations: %v", err)
	}
	legacy.ID = "global-legacy-extra"
	if err := db.CreateAllocatedGameServer(ctx, "global-owner", &legacy); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("legacy create bypasses global reservations: %v", err)
	}
	usage, err := db.GetTenantUsage(ctx, org.ID)
	if err != nil || usage.TotalServers != 2 || usage.UsedMemoryMB != 1536 || usage.RunningServers != 0 {
		t.Fatalf("mixed usage: %+v %v", usage, err)
	}
	var old globalRevisionRow
	if err := db.db.Table("server_revisions").Where("id = ?", created.Revision.ID).Take(&old).Error; err != nil || old.MemoryMB != 512 {
		t.Fatalf("original revision overwritten: %+v %v", old, err)
	}
	// The database migration must be repeatable without touching revisions.
	if db.db.Dialector.Name() == "sqlite" {
		if err := migrateSQLiteGlobalInstances(db.db); err != nil {
			t.Fatal(err)
		}
	}
	other := domain.Organization{ID: "global-other", Slug: "global-other"}
	if err := db.CreateOrganization(ctx, &other, "other-owner"); err != nil {
		t.Fatal(err)
	}
	foreign := revise
	foreign.OrganizationID = other.ID
	foreign.ExpectedGeneration = 3
	if _, err := db.ReviseGlobalServer(ctx, "other-owner", foreign); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another tenant edited server: %v", err)
	}
	if err := db.UpdateTenantQuota(ctx, domain.TenantQuota{OrganizationID: other.ID, MaxServers: 1, MaxCPUCores: 1, MaxMemoryMB: 512, MaxStorageGB: 10}); err != nil {
		t.Fatal(err)
	}
	otherRequest := request
	otherRequest.OrganizationID = other.ID
	otherResult, err := db.CreateGlobalServer(ctx, "other-owner", otherRequest)
	if err != nil || otherResult.Server.ID == created.Server.ID || otherResult.Operation.ID == created.Operation.ID {
		t.Fatalf("idempotency escaped tenant scope: %v", err)
	}
	testEncryptedGlobalCreate(t, db)
	testRegionDirectory(t, db)
}

func TestSQLiteGlobalInstanceMigrationRollback(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "migration.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range []string{"server_outbox", "server_operations", "server_placements", "server_revisions", "logical_servers"} {
		if err := db.db.Exec("DROP TABLE " + table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.db.Exec("DELETE FROM gamepanel_sqlite_migrations WHERE version = 3").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.db.Exec("CREATE TABLE server_outbox (sentinel text)").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateSQLiteGlobalInstances(db.db); err == nil {
		t.Fatal("conflicting preexisting table silently adopted")
	}
	if db.db.Migrator().HasTable("logical_servers") || db.db.Migrator().HasTable("server_revisions") {
		t.Fatal("failed migration left partial model tables")
	}
	var count int64
	if err := db.db.Table("gamepanel_sqlite_migrations").Where("version = 3").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("failed migration marked applied: %d %v", count, err)
	}
	if err := db.db.Exec("DROP TABLE server_outbox").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateSQLiteGlobalInstances(db.db); err != nil {
		t.Fatal(err)
	}
}
