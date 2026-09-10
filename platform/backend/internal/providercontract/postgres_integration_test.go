package providercontract

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresProviderReleaseIsImmutableAndVerified(t *testing.T) {
	database := openProviderDatabase(t)
	registry := NewRegistry(NewPostgresStore(database), []byte("provider-signing-key-012345678901"))
	published, err := registry.Publish(context.Background(), fixtureManifest(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Verified(context.Background(), published.ProviderReleaseID); err != nil {
		t.Fatal(err)
	}
	changed := fixtureManifest()
	changed.DisplayName = "Changed"
	if _, err := registry.Publish(context.Background(), changed, time.Now().UTC()); !errors.Is(err, ErrImmutableRelease) {
		t.Fatalf("immutable error=%v", err)
	}
	if _, err := database.Exec("UPDATE provider_releases SET manifest=jsonb_set(manifest,'{displayName}',to_jsonb($2::text)) WHERE id=$1", published.ProviderReleaseID, "tampered"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Verified(context.Background(), published.ProviderReleaseID); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("signature error=%v", err)
	}
}

func openProviderDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("GAMEPANEL_GLOBAL_TEST_DSN")
	if dsn == "" {
		t.Skip("GAMEPANEL_GLOBAL_TEST_DSN is not set")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("phase5_provider_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE SCHEMA \"" + schema + "\""); err != nil {
		t.Fatal(err)
	}
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
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(filename), "..", "..", "migrations", "global")
	for _, name := range []string{"0002_product_instance_messaging.sql", "0007_async_delivery.sql", "0008_provider_driven_operation.sql"} {
		source, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, statement := range strings.Split(string(source), ";") {
			if statement = strings.TrimSpace(statement); statement != "" {
				if _, err := database.Exec(statement); err != nil {
					t.Fatalf("migration %s: %v", name, err)
				}
			}
		}
	}
	t.Cleanup(func() {
		database.Close()
		_, _ = admin.Exec("DROP SCHEMA \"" + schema + "\" CASCADE")
		admin.Close()
	})
	return database
}
