package store

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"gorm.io/gorm"
)

func TestPostgresRegionalInbox(t *testing.T) {
	dsn := os.Getenv("GAMEPANEL_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("set GAMEPANEL_TEST_POSTGRES_URL for a PostgreSQL integration run")
	}
	endpoint, err := url.Parse(dsn)
	if err != nil || (endpoint.Scheme != "postgres" && endpoint.Scheme != "postgresql") {
		t.Fatal("PostgreSQL test URL required")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal("open test PostgreSQL")
	}
	t.Cleanup(func() { _ = admin.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	openRegion := func(region string) (*RegionalStore, string) {
		t.Helper()
		schema := "region_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
				t.Error(err)
			}
		})
		regionalURL := *endpoint
		query := regionalURL.Query()
		query.Set("search_path", schema)
		regionalURL.RawQuery = query.Encode()
		for i := 0; i < 2; i++ {
			if err := MigrateRegionalPostgres(ctx, regionalURL.String(), region); err != nil {
				t.Fatal(err)
			}
		}
		db, err := OpenRegionalPostgres(regionalURL.String(), region, 8)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		return db, regionalURL.String()
	}
	east, eastURL := openRegion("east")
	west, _ := openRegion("west")
	if db, err := OpenRegionalPostgres(eastURL, "west", 1); err == nil {
		db.Close()
		t.Fatal("opened region database under another identity")
	}
	if err := MigrateRegionalPostgres(ctx, eastURL, "west"); !errors.Is(err, ErrRegionMismatch) {
		t.Fatalf("region rebinding: %v", err)
	}
	if db, err := OpenConfigured("", eastURL, 1); err == nil {
		db.Close()
		t.Fatal("global store opened regional schema")
	}
	if err := MigratePostgres(ctx, eastURL); err == nil {
		t.Fatal("global migration adopted regional schema")
	}
	if east.db.Migrator().HasTable("organizations") || east.db.Migrator().HasTable("logical_servers") {
		t.Fatal("global business tables created in region")
	}
	event := instances.RevisionAvailable{SchemaVersion: 1, EventID: "event-1", OperationID: "operation-1", OrganizationID: "tenant", ServerID: "server", RevisionID: "revision-1", RegionID: "east", PlacementEpoch: 1, SpecGeneration: 1}
	role := "region_runtime_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	password := uuid.NewString()
	if _, err := admin.ExecContext(ctx, "CREATE ROLE "+role+" LOGIN PASSWORD '"+password+"'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP OWNED BY " + role); err != nil {
			t.Error(err)
		}
		if _, err := admin.Exec("DROP ROLE " + role); err != nil {
			t.Error(err)
		}
	})
	runtimeURL, _ := url.Parse(eastURL)
	schema := runtimeURL.Query().Get("search_path")
	for _, statement := range []string{
		"GRANT USAGE ON SCHEMA " + schema + " TO " + role,
		"GRANT SELECT ON " + schema + ".gamepanel_schema_migrations," + schema + ".regional_identity TO " + role,
		"GRANT SELECT,INSERT ON " + schema + ".regional_inbox," + schema + ".regional_revision_tasks TO " + role,
	} {
		if _, err := admin.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	runtimeURL.User = url.UserPassword(role, password)
	runtimeStore, err := OpenRegionalPostgres(runtimeURL.String(), "east", 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtimeStore.Close() })
	if err := runtimeStore.RecordRevisionNotification(ctx, event); err != nil {
		t.Fatal(err)
	}
	if err := runtimeStore.db.Exec("UPDATE regional_identity SET region_id = 'west'").Error; err == nil {
		t.Fatal("runtime role changed regional identity")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := east.RecordRevisionNotification(ctx, event); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	count := func(db *RegionalStore, table string, want int64) {
		t.Helper()
		var n int64
		if err := db.db.Table(table).Count(&n).Error; err != nil || n != want {
			t.Fatalf("%s count %d want %d: %v", table, n, want, err)
		}
	}
	count(east, "regional_inbox", 1)
	count(east, "regional_revision_tasks", 1)
	if err := west.RecordRevisionNotification(ctx, event); !errors.Is(err, ErrRegionMismatch) {
		t.Fatalf("cross-region event: %v", err)
	}
	count(west, "regional_inbox", 0)
	changed := event
	changed.RevisionID = "different"
	if err := east.RecordRevisionNotification(ctx, changed); !errors.Is(err, ErrNotificationConflict) {
		t.Fatalf("changed event identity: %v", err)
	}
	changed.EventID = "event-conflict"
	if err := east.RecordRevisionNotification(ctx, changed); !errors.Is(err, ErrNotificationConflict) {
		t.Fatalf("changed operation identity: %v", err)
	}
	count(east, "regional_inbox", 1)
	replay := event
	replay.EventID = "event-repair"
	if err := east.RecordRevisionNotification(ctx, replay); err != nil {
		t.Fatal(err)
	}
	count(east, "regional_inbox", 2)
	count(east, "regional_revision_tasks", 1)
	failed := event
	failed.EventID = "event-rollback"
	failed.OperationID = "operation-rollback"
	const callback = "test:regional-task-failure"
	if err := east.db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "regional_revision_tasks" {
			tx.AddError(errors.New("injected regional task failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	err = east.RecordRevisionNotification(ctx, failed)
	_ = east.db.Callback().Create().Remove(callback)
	if err == nil {
		t.Fatal("injected failure ignored")
	}
	count(east, "regional_inbox", 2)
	count(east, "regional_revision_tasks", 1)
	failed.SchemaVersion = 2
	if err := east.RecordRevisionNotification(ctx, failed); !errors.Is(err, instances.ErrInvalidIntent) {
		t.Fatalf("unknown schema accepted: %v", err)
	}
	var task struct{ Status string }
	if err := east.db.Table("regional_revision_tasks").Where("operation_id = ?", event.OperationID).Take(&task).Error; err != nil || task.Status != "awaiting_revision" {
		t.Fatalf("notification granted execution: %+v %v", task, err)
	}
	testRegionalBrokerIngress(t, east)
	fetchDB, fetchURL := openRegion("fetch-test")
	testRegionalRevisionTasks(t, fetchDB, fetchURL)
	assetDB, assetURL := openRegion("asset-test")
	testRegionalAssetTasks(t, assetDB, assetURL)
	uploadDB, uploadURL := openRegion("upload-test")
	testRegionalArchiveUploads(t, uploadDB, uploadURL)
	preparationDB, preparationURL := openRegion("preparation-test")
	testRegionalBackupPreparation(t, preparationDB, preparationURL)
	backupDB, _ := openRegion("backup-test")
	testRegionalBackupIngress(t, backupDB)
}
