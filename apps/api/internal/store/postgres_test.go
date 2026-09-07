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
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("GAMEPANEL_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("set GAMEPANEL_TEST_POSTGRES_URL for a PostgreSQL integration run")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Fatal("test database must be a PostgreSQL URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal("open integration database")
	}
	defer admin.Close()
	schema := "gamepanel_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	if _, err := admin.ExecContext(ctx, "CREATE TABLE "+schema+".adoption_probe (id integer)"); err != nil {
		t.Fatal(err)
	}
	if err := MigratePostgres(ctx, parsed.String()); err == nil {
		t.Fatal("unversioned populated schema silently adopted")
	}
	if _, err := admin.ExecContext(ctx, "DROP TABLE "+schema+".adoption_probe"); err != nil {
		t.Fatalf("unversioned schema modified: %v", err)
	}
	if opened, err := OpenConfigured("", parsed.String(), 2); err == nil {
		opened.Close()
		t.Fatal("API started before migration")
	}
	var tableCount int
	if err := admin.QueryRowContext(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema=$1", schema).Scan(&tableCount); err != nil || tableCount != 0 {
		t.Fatalf("API created schema objects: %d %v", tableCount, err)
	}
	baselineDB, err := connectPostgres(parsed.String(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := migratePostgres(ctx, baselineDB, postgresMigrations()[:1]); err != nil {
		t.Fatal(err)
	}
	if err := baselineDB.Exec("INSERT INTO game_servers (id, organization_id) VALUES ('legacy-world-owner','legacy-org'); INSERT INTO worlds (id, instance_id) VALUES ('legacy-world','legacy-world-owner'); INSERT INTO activity_events (id, instance_id) VALUES ('legacy-event','legacy-world-owner'); INSERT INTO mod_files (id, instance_id) VALUES ('legacy-mod-migration','unassigned'); INSERT INTO mod_packs (id) VALUES ('legacy-pack-migration')").Error; err != nil {
		t.Fatal(err)
	}
	baselinePool, _ := baselineDB.DB()
	baselinePool.Close()
	var starters sync.WaitGroup
	failures := make(chan error, 4)
	for i := 0; i < 4; i++ {
		starters.Add(1)
		go func() {
			defer starters.Done()
			err := MigratePostgres(ctx, parsed.String())
			failures <- err
		}()
	}
	starters.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatalf("concurrent migration: %v", err)
		}
	}
	db, err := OpenConfigured("", parsed.String(), 4)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var legacyEvent domain.ActivityEvent
	if err := db.db.First(&legacyEvent, "id = ?", "legacy-event").Error; err != nil || legacyEvent.OrganizationID != "legacy-org" {
		t.Fatalf("activity ownership migration: %+v %v", legacyEvent, err)
	}

	legacyWorld, err := db.GetWorld(ctx, "legacy-world")
	if err != nil || legacyWorld.OrganizationID != "legacy-org" {
		t.Fatalf("world ownership migration: %+v %v", legacyWorld, err)
	}

	legacyMod, err := db.GetMod(ctx, "legacy-mod-migration")
	if err != nil || legacyMod.OrganizationID != "" || legacyMod.Revision != 0 {
		t.Fatalf("legacy mod ownership guessed: %+v %v", legacyMod, err)
	}
	legacyPack, err := db.GetModPack(ctx, "legacy-pack-migration")
	if err != nil || legacyPack.OrganizationID != "" || legacyPack.Revision != 0 {
		t.Fatalf("legacy pack ownership guessed: %+v %v", legacyPack, err)
	}
	if err := db.DeleteMod(ctx, legacyMod.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteModPack(ctx, legacyPack.ID); err != nil {
		t.Fatal(err)
	}

	role := "gamepanel_runtime_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(ctx, "CREATE ROLE "+role+" LOGIN"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec("DROP OWNED BY " + role + "; DROP ROLE " + role); err != nil {
			t.Error(err)
		}
	}()
	if _, err := admin.ExecContext(ctx, "GRANT USAGE ON SCHEMA "+schema+" TO "+role+"; GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA "+schema+" TO "+role+"; REVOKE INSERT,UPDATE,DELETE ON "+schema+".gamepanel_schema_migrations FROM "+role); err != nil {
		t.Fatal(err)
	}
	runtimeURL := *parsed
	runtimeURL.User = url.User(role)
	runtimeDB, err := OpenConfigured("", runtimeURL.String(), 2)
	if err != nil {
		t.Fatalf("runtime role startup: %v", err)
	}
	defer runtimeDB.Close()
	if err := runtimeDB.db.Exec("CREATE TABLE forbidden_runtime_ddl (id integer)").Error; err == nil {
		t.Fatal("runtime role can create tables")
	}
	if err := runtimeDB.db.Exec("UPDATE gamepanel_schema_migrations SET checksum='forbidden'").Error; err == nil {
		t.Fatal("runtime role can edit migration history")
	}
	runtimeServer := domain.GameServer{ID: "runtime-role", Name: "runtime", Spec: domain.ServerSpec{ConfigVersion: 1}}
	if err := runtimeDB.CreateGameServer(ctx, &runtimeServer); err != nil {
		t.Fatalf("runtime role write: %v", err)
	}
	if _, err := runtimeDB.GetGameServer(ctx, runtimeServer.ID); err != nil {
		t.Fatalf("runtime role read: %v", err)
	}
	server := domain.GameServer{ID: "server", Name: "test", ProviderKey: domain.ProviderTerrariaVanilla, Spec: domain.ServerSpec{Generation: 1, ConfigVersion: 1, Config: map[string]any{"worldName": "世界"}}}
	if err := db.CreateGameServer(ctx, &server); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetGameServer(ctx, server.ID)
	if err != nil || got.Spec.ConfigVersion != 1 || got.Spec.Config["worldName"] != "世界" {
		t.Fatalf("JSON roundtrip: %+v %v", got, err)
	}
	reject := errors.New("rollback")
	err = db.Transaction(ctx, func(tx *Store) error {
		got.Name = "should-rollback"
		if err := tx.SaveGameServer(ctx, &got); err != nil {
			return err
		}
		return reject
	})
	if !errors.Is(err, reject) {
		t.Fatalf("transaction: %v", err)
	}
	got, err = db.GetGameServer(ctx, server.ID)
	if err != nil || got.Name != "test" {
		t.Fatalf("rollback not retained: %+v %v", got, err)
	}
	stamp := time.Now().UTC()
	for _, id := range []string{"a", "z"} {
		event := domain.ActivityEvent{ID: id, InstanceID: server.ID, CreatedAt: stamp}
		if err := db.CreateActivity(ctx, &event); err != nil {
			t.Fatal(err)
		}
		job := domain.GameUpdateJob{ID: id, InstanceID: server.ID, CreatedAt: stamp}
		if err := db.CreateGameUpdateJob(ctx, &job); err != nil {
			t.Fatal(err)
		}
	}
	events, err := db.ListActivityByInstance(ctx, server.ID, 10)
	if err != nil || len(events) != 2 || events[0].ID != "z" {
		t.Fatalf("activity order: %+v %v", events, err)
	}
	job, err := db.GetLatestGameUpdateJobByInstance(ctx, server.ID)
	if err != nil || job.ID != "z" {
		t.Fatalf("job query: %+v %v", job, err)
	}
	if _, err := db.ListActiveGameUpdateJobs(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetGameServer(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("not found: %v", err)
	}
	testTenantActivity(t, db)
	testReconciliationPersistence(t, db)
	testTenantAllocations(t, db)
	testTenantPresets(t, db)
	testTenantModLibrary(t, db)
	testModSources(t, db)
	testPlayerObservations(t, db)
	testServerLifecycleWrites(t, db)
	testAssignmentPublication(t, db)
	testCredentialRotation(t, db)
	testConcurrentCredentialRotation(t, db)
	testWorldOwnershipQueries(t, db)
	testTenantBackupQueries(t, db)
	testTenantServerQueries(t, db)
	testPersonalOrganizations(t, db)
	migrations := append(postgresMigrations(), sqlMigration{len(postgresMigrations()) + 1, "failure_probe", "CREATE TABLE migration_failure_probe (id integer); SELECT * FROM deliberately_missing_relation;"})
	if err := migratePostgres(ctx, db.db, migrations); err == nil {
		t.Fatal("broken migration accepted")
	}
	var exists bool
	if err := db.db.Raw("SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='migration_failure_probe')").Scan(&exists).Error; err != nil || exists {
		t.Fatalf("failed DDL not rolled back: %v %v", exists, err)
	}
	var records int64
	if err := db.db.Table("gamepanel_schema_migrations").Count(&records).Error; err != nil || records != int64(len(postgresMigrations())) {
		t.Fatalf("migration ledger: %d %v", records, err)
	}
	if err := migratePostgres(ctx, db.db, postgresMigrations()); err != nil {
		t.Fatalf("retry after failed migration: %v", err)
	}
	if err := migratePostgres(ctx, db.db, nil); err == nil {
		t.Fatal("older binary accepted newer schema")
	}
	if err := db.db.Exec("UPDATE gamepanel_schema_migrations SET checksum='tampered'").Error; err != nil {
		t.Fatal(err)
	}
	if opened, err := OpenConfigured("", parsed.String(), 2); err == nil {
		opened.Close()
		t.Fatal("checksum mismatch accepted")
	}

}

func TestPostgresConfigRejectsInvalidPoolAndRedactsDSN(t *testing.T) {
	if _, err := OpenConfigured("", "postgres://example", -1); err == nil {
		t.Fatal("negative connection pool accepted")
	}
	_, err := OpenConfigured("", "postgres://user:secret%not-a-url@example", 1)
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unsafe connection error: %v", err)
	}
}
