package store

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPostgresRegionalMigrationAuditReadOnly(t *testing.T) {
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
		t.Fatal("open test database")
	}
	defer admin.Close()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	schema, role := "audit_"+suffix, "audit_reader_"+suffix
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec("DROP SCHEMA " + schema + " CASCADE")
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	if err := MigratePostgres(ctx, parsed.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(ctx, "CREATE ROLE "+role+" LOGIN"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec("DROP OWNED BY " + role + "; DROP ROLE " + role); err != nil {
			t.Error(err)
		}
	}()
	for _, grant := range []string{
		"GRANT USAGE ON SCHEMA " + schema + " TO " + role,
		"GRANT SELECT ON " + schema + ".gamepanel_schema_migrations TO " + role,
		"GRANT SELECT(id) ON " + schema + ".organizations TO " + role,
		"GRANT SELECT(id,region) ON " + schema + ".compute_nodes TO " + role,
		"GRANT SELECT(id,organization_id,node_id) ON " + schema + ".game_servers TO " + role,
		"GRANT SELECT(id,server_id,node_id) ON " + schema + ".workload_assignments TO " + role,
	} {
		if _, err := admin.ExecContext(ctx, grant); err != nil {
			t.Fatal(err)
		}
	}
	parsed.User = url.User(role)
	report, err := AuditPostgresRegionalMigration(ctx, parsed.String())
	if err != nil || len(report.Issues) != 0 {
		t.Fatalf("column-limited read-only audit failed: %v", err)
	}
	reader, err := sql.Open("pgx", parsed.String())
	if err != nil {
		t.Fatal("open audit reader")
	}
	defer reader.Close()
	if _, err := reader.ExecContext(ctx, "UPDATE compute_nodes SET region='forbidden'"); err == nil {
		t.Fatal("audit reader can write")
	}
	if _, err := reader.ExecContext(ctx, "SELECT token FROM compute_nodes"); err == nil {
		t.Fatal("audit reader can read node credentials")
	}
}
