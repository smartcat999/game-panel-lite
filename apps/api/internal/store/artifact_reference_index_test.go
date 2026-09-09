package store

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func TestSQLiteArtifactReferenceUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	item := domain.ModFile{ID: "upgrade-source", OrganizationID: "upgrade-space", InstanceID: "unassigned", ProviderKey: domain.ProviderTerrariaTModLoader}
	if err := db.CreateMod(ctx, &item); err != nil {
		t.Fatal(err)
	}
	assignment := domain.WorkloadAssignment{ID: "upgrade-assignment", UID: "upgrade-uid", ServerID: "deleted-instance", Spec: workload.Spec{Options: workload.Options{Artifacts: []workload.Artifact{{ID: item.ID, Path: "one", SizeBytes: 1, SHA256: strings.Repeat("a", 64)}, {ID: item.ID, Path: "two", SizeBytes: 1, SHA256: strings.Repeat("a", 64)}}}}}
	if err := db.db.Create(&assignment).Error; err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"prepaid_fulfillment_tasks", "service_subscriptions", "prepaid_payment_events", "prepaid_payment_captures", "prepaid_orders", "prepaid_plan_sales", "prepaid_plan_versions", "global_entitlement_changes", "global_server_entitlements", "global_backup_results", "backup_request_outbox", "global_backup_tasks", "global_asset_replicas", "global_asset_versions", "global_assets", "global_region_statuses", "global_regions", "server_outbox", "server_operations", "server_placements", "server_revisions", "logical_servers", "workload_artifact_references", "node_port_reservations", "node_port_pools", "gamepanel_sqlite_migrations"} {
		if err := db.db.Exec("DROP TABLE " + table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int64
	if err := db.db.Model(&artifactReference{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("backfill did not deduplicate: %d %v", count, err)
	}
	if err := db.artifactReferences(ctx, item.OrganizationID, item.ID); !errors.Is(err, ErrInvalidModLibrary) {
		t.Fatalf("lost orphaned instance reference: %v", err)
	}
	// EXPLAIN verifies the production lookup can use the composite index.
	var rows []struct{ Detail string }
	if err := db.db.Raw("EXPLAIN QUERY PLAN SELECT count(*) FROM workload_artifact_references WHERE organization_id = ? AND artifact_id = ?", item.OrganizationID, item.ID).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	indexed := false
	for _, row := range rows {
		indexed = indexed || strings.Contains(row.Detail, "idx_workload_artifact_references_owner_source")
	}
	if !indexed {
		t.Fatalf("lookup scans: %+v", rows)
	}
	if err := migrateSQLiteArtifactReferences(db.db); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
	if err := db.DeleteWorkloadAssignment(ctx, assignment.ServerID); err != nil {
		t.Fatal(err)
	}
	if err := db.artifactReferences(ctx, item.OrganizationID, item.ID); err != nil {
		t.Fatalf("assignment deletion retained reference: %v", err)
	}
}

func TestSQLiteArtifactReferenceMigrationRollsBack(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "rollback.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range []string{"server_outbox", "server_operations", "server_placements", "server_revisions", "logical_servers", "workload_artifact_references", "node_port_reservations", "node_port_pools", "gamepanel_sqlite_migrations"} {
		if err := db.db.Exec("DROP TABLE " + table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.db.Exec("INSERT INTO workload_assignments(id,spec) VALUES (?,?)", "invalid", "broken-json").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateSQLiteArtifactReferences(db.db); err == nil {
		t.Fatal("invalid historical manifest accepted")
	}
	if db.db.Migrator().HasTable("workload_artifact_references") || db.db.Migrator().HasTable("gamepanel_sqlite_migrations") {
		t.Fatal("failed migration left partial schema")
	}
}
