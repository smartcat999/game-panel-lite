package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"time"
)

var ErrChecksumMismatch = errors.New("applied migration checksum does not match embedded source")

//go:embed global/*.sql region/*.sql
var sources embed.FS

type Migration struct {
	Name     string
	SQL      string
	Checksum string
}

func List(scope string) ([]Migration, error) {
	if scope != "global" && scope != "region" {
		return nil, fmt.Errorf("invalid database scope %q", scope)
	}
	entries, err := fs.ReadDir(sources, scope)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	result := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		body, err := sources.ReadFile(scope + "/" + entry.Name())
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256(body)
		result = append(result, Migration{Name: entry.Name(), SQL: string(body), Checksum: hex.EncodeToString(digest[:])})
	}
	return result, nil
}

func Migrate(ctx context.Context, database *sql.DB, scope string) error {
	migrations, err := List(scope)
	if err != nil {
		return err
	}
	connection, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	defer connection.Close()
	lockID := int64(74532901)
	if scope == "region" {
		lockID = 74532902
	}
	if _, err := connection.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, lockID); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	defer connection.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, lockID)
	if _, err := connection.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	for _, migration := range migrations {
		var appliedChecksum string
		err := connection.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE name=$1`, migration.Name).Scan(&appliedChecksum)
		if err == nil {
			if appliedChecksum != migration.Checksum {
				return fmt.Errorf("%w: %s", ErrChecksumMismatch, migration.Name)
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		tx, err := connection.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, migration.SQL); err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations (name,checksum,applied_at) VALUES ($1,$2,$3)`, migration.Name, migration.Checksum, time.Now().UTC())
		}
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("apply %s: %w", migration.Name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit %s: %w", migration.Name, err)
		}
	}
	return nil
}
