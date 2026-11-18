// Package testpg provides an embedded/ephemeral PostgreSQL server and isolated
// per-test schemas for repository and model tests.
package testpg

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
)

var (
	schemaPattern = regexp.MustCompile(`^gpc_rc_[a-f0-9]{16}$`)

	serverOnce sync.Once
	serverDSN  string
	serverErr  error
	stopServer func()
)

type Fixture struct {
	Pool     *pgxpool.Pool
	Registry *repo.Registry
	Schema   string
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func findBinary(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/lib/postgresql/16/bin", "/usr/lib/postgresql/15/bin"} {
		candidate := filepath.Join(dir, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func ensureTestPostgresDSN() (string, error) {
	if dsn := os.Getenv("GP_TEST_POSTGRES_DSN"); dsn != "" {
		return dsn, nil
	}
	serverOnce.Do(func() {
		port, err := freePort()
		if err != nil {
			serverErr = err
			return
		}
		// Prefer fast local initdb + postgres -F when installed on host; otherwise use embedded-postgres.
		if initdbBin, pgCtlBin := findBinary("initdb"), findBinary("pg_ctl"); initdbBin != "" && pgCtlBin != "" {
			tmpDir, err := os.MkdirTemp("", "gpc-ephemeral-pg-*")
			if err == nil {
				dataDir := filepath.Join(tmpDir, "data")
				sockDir := filepath.Join(tmpDir, "sock")
				_ = os.MkdirAll(sockDir, 0o700)
				initCmd := exec.Command(initdbBin, "-D", dataDir, "-U", "postgres", "-A", "trust", "--no-sync", "--encoding=UTF8", "--locale=C")
				if out, err := initCmd.CombinedOutput(); err == nil {
					opts := fmt.Sprintf("-F -p %d -h 127.0.0.1 -k %s -c fsync=off -c synchronous_commit=off -c full_page_writes=off -c max_connections=100", port, sockDir)
					startCmd := exec.Command(pgCtlBin, "-D", dataDir, "-l", filepath.Join(tmpDir, "postgres.log"), "-o", opts, "-w", "start")
					if _, err := startCmd.CombinedOutput(); err == nil {
						serverDSN = fmt.Sprintf("postgres://postgres@127.0.0.1:%d/postgres?sslmode=disable", port)
						_ = os.Setenv("GP_TEST_POSTGRES_DSN", serverDSN)
						stopServer = func() {
							_ = exec.Command(pgCtlBin, "-D", dataDir, "-m", "immediate", "stop").Run()
							_ = os.RemoveAll(tmpDir)
						}
						return
					}
				} else {
					_ = out
				}
			}
		}

		epg := embeddedpostgres.NewDatabase(
			embeddedpostgres.DefaultConfig().
				Port(uint32(port)).
				Username("postgres").
				Password("postgres").
				Database("postgres"),
		)
		if err := epg.Start(); err != nil {
			serverErr = err
			return
		}
		serverDSN = fmt.Sprintf("postgres://postgres:postgres@127.0.0.1:%d/postgres?sslmode=disable", port)
		_ = os.Setenv("GP_TEST_POSTGRES_DSN", serverDSN)
		stopServer = func() { _ = epg.Stop() }
	})
	return serverDSN, serverErr
}

// Stop shuts down the ephemeral PostgreSQL server if one was started by this process.
func Stop() {
	if stopServer != nil {
		stopServer()
	}
}

// New creates an isolated disposable PostgreSQL schema with all platform migrations applied.
func New(t *testing.T, tracer pgx.QueryTracer) Fixture {
	t.Helper()
	dsn, err := ensureTestPostgresDSN()
	if err != nil || dsn == "" {
		t.Skipf("PostgreSQL test server unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)

	schema, quoted := createIsolatedTestSchema(t, ctx, admin)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",pg_catalog"
	cfg.ConnConfig.Tracer = tracer
	cfg.MaxConns = 16
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = admin.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE")
	})
	if err := db.MigrateSchema(ctx, pool); err != nil {
		t.Fatalf("MigrateSchema: %v", err)
	}
	return Fixture{Pool: pool, Registry: repo.NewPostgresRegistry(pool), Schema: schema}
}

func createIsolatedTestSchema(t *testing.T, ctx context.Context, admin *pgxpool.Pool) (string, string) {
	t.Helper()
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	schema := "gpc_rc_" + hex.EncodeToString(nonce[:])
	if !schemaPattern.MatchString(schema) {
		t.Fatal("invalid owned test schema")
	}
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	return schema, quoted
}

// NewRegistry returns a PostgreSQL-backed *repo.Registry in an isolated per-test schema.
func NewRegistry(t *testing.T) *repo.Registry {
	t.Helper()
	return New(t, nil).Registry
}

// ConnectWorker opens a second process's pool in its parent's owned schema.
func ConnectWorker(ctx context.Context, schema string) (*pgxpool.Pool, error) {
	if !schemaPattern.MatchString(schema) {
		return nil, &schemaError{}
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("GP_TEST_POSTGRES_DSN"))
	if err != nil {
		return nil, err
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",pg_catalog"
	cfg.MaxConns = 4
	return pgxpool.NewWithConfig(ctx, cfg)
}

type schemaError struct{}

func (*schemaError) Error() string { return "worker refused non-test schema" }
