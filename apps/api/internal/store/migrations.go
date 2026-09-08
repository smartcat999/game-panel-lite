package store

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"fmt"

	"gorm.io/gorm"
)

//go:embed migrations/001_postgres_baseline.sql
var postgresBaseline string

//go:embed migrations/002_world_ownership.sql
var worldOwnership string

//go:embed migrations/003_activity_ownership.sql
var activityOwnership string

//go:embed migrations/004_preset_ownership.sql
var presetOwnership string

//go:embed migrations/005_mod_library_ownership.sql
var modLibraryOwnership string

//go:embed migrations/006_artifact_references.sql
var artifactReferencesSQL string

//go:embed migrations/007_node_workload_capabilities.sql
var nodeWorkloadCapabilitiesSQL string

//go:embed migrations/008_execution_leases.sql
var executionLeasesSQL string

//go:embed migrations/009_workload_observation_artifacts.sql
var workloadObservationArtifactsSQL string

//go:embed migrations/010_credits_and_oauth.sql
var creditsAndOAuthSQL string

//go:embed migrations/011_node_scheduling.sql
var nodeSchedulingSQL string

//go:embed migrations/012_oauth_table_name.sql
var oauthTableNameSQL string

//go:embed migrations/013_node_runtime_architecture.sql
var nodeRuntimeArchitectureSQL string

//go:embed migrations/014_node_port_reservations.sql
var nodePortReservationsSQL string

//go:embed migrations/015_global_instance_intents.sql
var globalInstanceSchemaSQL string

//go:embed migrations/global_instance_revision_guards.sql
var globalInstanceRevisionGuardsSQL string

//go:embed migrations/016_outbox_publication.sql
var outboxPublicationSQL string

//go:embed migrations/017_region_directory.sql
var regionDirectorySQL string

//go:embed migrations/018_global_assets.sql
var globalAssetsSQL string

//go:embed migrations/global_asset_guards.sql
var globalAssetGuardsSQL string

//go:embed migrations/019_asset_replicas.sql
var assetReplicasSQL string

//go:embed migrations/020_global_backup_tasks.sql
var globalBackupTasksSQL string

//go:embed migrations/021_global_backup_results.sql
var globalBackupResultsSQL string

//go:embed migrations/022_server_entitlements.sql
var serverEntitlementsSQL string

//go:embed migrations/022_server_entitlement_guards.sql
var serverEntitlementGuardsSQL string

//go:embed migrations/023_prepaid_catalog.sql
var prepaidCatalogSQL string

//go:embed migrations/023_prepaid_catalog_guards.sql
var prepaidCatalogGuardsSQL string

//go:embed migrations/024_prepaid_orders.sql
var prepaidOrdersSQL string

//go:embed migrations/024_prepaid_order_guards.sql
var prepaidOrderGuardsSQL string

//go:embed migrations/025_prepaid_order_cancellation.sql
var prepaidOrderCancellationSQL string

//go:embed migrations/026_prepaid_payments.sql
var prepaidPaymentsSQL string

//go:embed migrations/026_prepaid_payment_guards.sql
var prepaidPaymentGuardsSQL string

//go:embed migrations/027_prepaid_subscriptions.sql
var prepaidSubscriptionsSQL string

type sqlMigration struct {
	version   int
	name, sql string
}
type migrationRecord struct {
	Version        int
	Name, Checksum string
}

func postgresMigrations() []sqlMigration {
	return []sqlMigration{
		{1, "postgres_baseline", postgresBaseline},
		{2, "world_ownership", worldOwnership},
		{3, "activity_ownership", activityOwnership},
		{4, "preset_ownership", presetOwnership},
		{5, "mod_library_ownership", modLibraryOwnership},
		{6, "artifact_references", artifactReferencesSQL},
		{7, "node_workload_capabilities", nodeWorkloadCapabilitiesSQL},
		{8, "execution_leases", executionLeasesSQL},
		{9, "workload_observation_artifacts", workloadObservationArtifactsSQL},
		{10, "credits_and_oauth", creditsAndOAuthSQL},
		{11, "node_scheduling", nodeSchedulingSQL},
		{12, "oauth_table_name", oauthTableNameSQL},
		{13, "node_runtime_architecture", nodeRuntimeArchitectureSQL},
		{14, "node_port_reservations", nodePortReservationsSQL},
		{15, "global_instance_intents", globalInstanceSchemaSQL + "\n" + globalInstanceRevisionGuardsSQL},
		{16, "outbox_publication", outboxPublicationSQL},
		{17, "region_directory", regionDirectorySQL},
		{18, "global_assets", globalAssetsSQL + "\n" + globalAssetGuardsSQL},
		{19, "asset_replicas", assetReplicasSQL},
		{20, "global_backup_tasks", globalBackupTasksSQL},
		{21, "global_backup_results", globalBackupResultsSQL},
		{22, "server_entitlements", serverEntitlementsSQL + "\n" + serverEntitlementGuardsSQL},
		{23, "prepaid_catalog", prepaidCatalogSQL + "\n" + prepaidCatalogGuardsSQL},
		{24, "prepaid_orders", prepaidOrdersSQL + "\n" + prepaidOrderGuardsSQL},
		{25, "prepaid_order_cancellation", prepaidOrderCancellationSQL},
		{26, "prepaid_payments", prepaidPaymentsSQL + "\n" + prepaidPaymentGuardsSQL},
		{27, "prepaid_subscriptions", prepaidSubscriptionsSQL},
	}
}

// migratePostgres serializes cooperating initializers per schema and commits
// DDL plus its checksum ledger together. Existing scripts must never be edited.
func migratePostgres(ctx context.Context, db *gorm.DB, migrations []sqlMigration) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Two-key advisory locks keep this application's migration lock namespaced.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(current_schema()), 1735421191)").Error; err != nil {
			return err
		}
		var ledgerExists bool
		if err := tx.Raw("SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'gamepanel_schema_migrations')").Scan(&ledgerExists).Error; err != nil {
			return err
		}
		if !ledgerExists {
			var count int64
			if err := tx.Raw("SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema()").Scan(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return fmt.Errorf("unversioned PostgreSQL schema requires explicit adoption; refusing automatic schema changes")
			}
		}
		if err := tx.Exec("CREATE TABLE IF NOT EXISTS gamepanel_schema_migrations (version integer PRIMARY KEY, name text NOT NULL, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())").Error; err != nil {
			return err
		}
		var applied []migrationRecord
		if err := tx.Table("gamepanel_schema_migrations").Order("version").Find(&applied).Error; err != nil {
			return err
		}
		if len(applied) > len(migrations) {
			return fmt.Errorf("database schema is newer than this binary")
		}
		for index, migration := range migrations {
			if migration.version != index+1 {
				return fmt.Errorf("migration versions must be contiguous from 1")
			}
			checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(migration.sql)))
			if index < len(applied) {
				record := applied[index]
				if record.Version != migration.version || record.Name != migration.name || record.Checksum != checksum {
					return fmt.Errorf("migration %d history/checksum mismatch", migration.version)
				}
				continue
			}
			if err := executeMigration(tx, migration); err != nil {
				return fmt.Errorf("apply migration %d (%s): %w", migration.version, migration.name, err)
			}
			if err := tx.Exec("INSERT INTO gamepanel_schema_migrations(version,name,checksum) VALUES (?,?,?)", migration.version, migration.name, checksum).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// checkPostgresSchema performs SELECTs only, so runtime roles do not need DDL
// or writes to the migration ledger. Deployment must migrate before startup.
func checkPostgresSchema(ctx context.Context, db *gorm.DB, migrations []sqlMigration) error {
	var applied []migrationRecord
	if err := db.WithContext(ctx).Table("gamepanel_schema_migrations").Order("version").Find(&applied).Error; err != nil {
		return fmt.Errorf("read schema version; run database migration before starting API: %w", err)
	}
	if len(applied) != len(migrations) {
		return fmt.Errorf("database migration version does not match this binary; run the matching migration job")
	}
	for index, migration := range migrations {
		checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(migration.sql)))
		record := applied[index]
		if record.Version != migration.version || record.Name != migration.name || record.Checksum != checksum {
			return fmt.Errorf("migration %d history/checksum mismatch", migration.version)
		}
	}
	return nil
}
