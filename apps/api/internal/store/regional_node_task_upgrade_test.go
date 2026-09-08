package store

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"gorm.io/gorm"
)

func testRegionalNodeTaskUpgrade(t *testing.T, dsn string) {
	t.Helper()
	ctx := context.Background()
	admin, err := connectPostgres(dsn, 1)
	if err != nil {
		t.Fatal(err)
	}
	adminPool, _ := admin.DB()
	defer adminPool.Close()
	schema := "node_task_upgrade_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	defer admin.Exec("DROP SCHEMA " + schema + " CASCADE")
	endpoint, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := endpoint.Query()
	query.Set("search_path", schema)
	endpoint.RawQuery = query.Encode()
	oldDB, err := connectPostgres(endpoint.String(), 2)
	if err != nil {
		t.Fatal(err)
	}
	oldPool, _ := oldDB.DB()
	defer oldPool.Close()
	migrations := regionalMigrations()
	if err := oldDB.Transaction(func(tx *gorm.DB) error { return migratePostgres(ctx, tx, migrations[:14]) }); err != nil {
		t.Fatal(err)
	}
	region := &RegionalStore{db: oldDB, regionID: "upgrade"}
	create := func(id, state string) {
		t.Helper()
		event := instances.RevisionAvailable{SchemaVersion: 1, EventID: id, OperationID: id, OrganizationID: "tenant", ServerID: id, RegionID: "upgrade", RevisionID: id, SpecGeneration: 1, PlacementEpoch: 1}
		if err := region.RecordRevisionNotification(ctx, event); err != nil {
			t.Fatal(err)
		}
		claim, err := region.ClaimRevision(ctx, time.Minute)
		if err != nil || claim == nil {
			t.Fatal(err)
		}
		snapshot := regional.RevisionSnapshot{Event: event, CurrentSpecGeneration: 1, IntentVersion: 1, DesiredState: state, Revision: instances.Revision{ID: id, ServerID: id, SpecGeneration: 1, Specification: instances.Specification{ProviderKey: "fixture", GameVersion: "1", ConfigSchemaVersion: 1, Resources: instances.Resources{CPU: 1, MemoryMB: 128}, Configuration: instances.ProtectedConfiguration{KeyID: "key", Ciphertext: []byte("opaque")}}}}
		if err := region.SaveRevision(ctx, *claim, snapshot); err != nil {
			t.Fatal(err)
		}
		assets, err := region.ClaimAssets(ctx, time.Minute)
		if err != nil || assets == nil {
			t.Fatal(err)
		}
		if err := region.CompleteAssets(ctx, *assets); err != nil {
			t.Fatal(err)
		}
		// Simulate historical completion state. The migration must revalidate it,
		// rather than constructing an executable task from this marker alone.
		if err := oldDB.Table("regional_deployments").Where("server_id = ?", id).Update("scheduling_status", "reserved").Error; err != nil {
			t.Fatal(err)
		}
	}
	create("upgrade-running", "running")
	create("upgrade-stopped", "stopped")
	if err := MigrateRegionalPostgres(ctx, endpoint.String(), "upgrade"); err != nil {
		t.Fatal(err)
	}
	current, err := OpenRegionalPostgres(endpoint.String(), "upgrade", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	var running, stopped regionalSchedulingRow
	if err := current.db.Table("regional_deployments").Where("server_id = ?", "upgrade-running").Take(&running).Error; err != nil {
		t.Fatal(err)
	}
	if err := current.db.Table("regional_deployments").Where("server_id = ?", "upgrade-stopped").Take(&stopped).Error; err != nil {
		t.Fatal(err)
	}
	if running.SchedulingStatus != "pending" || stopped.SchedulingStatus != "reserved" {
		t.Fatal("upgrade requeued wrong desired states")
	}
	var tasks int64
	if err := current.db.Table("regional_node_tasks").Count(&tasks).Error; err != nil || tasks != 0 {
		t.Fatal("upgrade fabricated node tasks", err)
	}
	claim, err := current.ClaimScheduling(ctx, time.Minute)
	if err != nil || claim == nil || claim.Deployment.ServerID != "upgrade-running" {
		t.Fatal("upgraded deployment not recoverable", err)
	}
	t.Log("regional 014 -> 015 upgrade requeues running markers for verified recovery; no authority/task fabricated")
}
