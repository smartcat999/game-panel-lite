package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestEmbeddedMigrationsAreOrderedAndChecksummed(t *testing.T) {
	for _, scope := range []string{"global", "region"} {
		items, err := List(scope)
		if err != nil || len(items) < 6 {
			t.Fatalf("scope=%s migrations=%d err=%v", scope, len(items), err)
		}
		for index, item := range items {
			if item.Name == "" || item.SQL == "" || len(item.Checksum) != 64 {
				t.Fatalf("invalid migration %#v", item)
			}
			if index > 0 && strings.Compare(items[index-1].Name, item.Name) >= 0 {
				t.Fatalf("migrations are not strictly ordered: %s then %s", items[index-1].Name, item.Name)
			}
		}
	}
	if _, err := List("unknown"); err == nil {
		t.Fatal("invalid database scope accepted")
	}
}

func TestMigrateFreshSchemaAndReplay(t *testing.T) {
	for _, item := range []struct {
		scope string
		env   string
	}{{"global", "GAMEPANEL_GLOBAL_TEST_DSN"}, {"region", "GAMEPANEL_REGION_TEST_DSN"}} {
		t.Run(item.scope, func(t *testing.T) {
			dsn := os.Getenv(item.env)
			if dsn == "" {
				t.Skip(item.env + " is not set")
			}
			admin, err := sql.Open("pgx", dsn)
			if err != nil {
				t.Fatal(err)
			}
			schema := fmt.Sprintf("phase6_migrate_%s_%d", item.scope, time.Now().UnixNano())
			if _, err := admin.Exec(`CREATE SCHEMA "` + schema + `"`); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_, _ = admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`)
				admin.Close()
			})
			parsed, err := url.Parse(dsn)
			if err != nil {
				t.Fatal(err)
			}
			query := parsed.Query()
			query.Set("search_path", schema)
			parsed.RawQuery = query.Encode()
			database, err := sql.Open("pgx", parsed.String())
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			if err := Migrate(context.Background(), database, item.scope); err != nil {
				t.Fatal(err)
			}
			if item.scope == "global" {
				for _, table := range []string{"plans", "orders", "payments", "entitlements", "logical_instances", "backup_requests", "platform_operators"} {
					var present *string
					if err := database.QueryRow(`SELECT to_regclass($1)`, table).Scan(&present); err != nil || present != nil {
						t.Fatalf("superseded table %s remains present=%v err=%v", table, present, err)
					}
				}
			}
			if err := Migrate(context.Background(), database, item.scope); err != nil {
				t.Fatalf("idempotent replay: %v", err)
			}
			expected, _ := List(item.scope)
			var count int
			if err := database.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&count); err != nil || count != len(expected) {
				t.Fatalf("ledger count=%d expected=%d err=%v", count, len(expected), err)
			}
			if _, err := database.Exec(`UPDATE schema_migrations SET checksum='tampered' WHERE name=$1`, expected[0].Name); err != nil {
				t.Fatal(err)
			}
			if err := Migrate(context.Background(), database, item.scope); !errors.Is(err, ErrChecksumMismatch) {
				t.Fatalf("checksum drift was not rejected: %v", err)
			}
		})
	}
}
