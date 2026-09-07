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

type sqlMigration struct {
	version   int
	name, sql string
}
type migrationRecord struct {
	Version        int
	Name, Checksum string
}

func postgresMigrations() []sqlMigration {
	return []sqlMigration{{1, "postgres_baseline", postgresBaseline}}
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
			if err := tx.Exec(migration.sql).Error; err != nil {
				return fmt.Errorf("apply migration %d (%s): %w", migration.version, migration.name, err)
			}
			if err := tx.Exec("INSERT INTO gamepanel_schema_migrations(version,name,checksum) VALUES (?,?,?)", migration.version, migration.name, checksum).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
