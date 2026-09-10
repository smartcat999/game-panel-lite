package productionseed

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/smartcat999/game-panel-lite/platform/backend/migrations"
)

func TestProductionSeedIsCompleteAndIdempotent(t *testing.T) {
	global := seedTestDatabase(t, "GAMEPANEL_GLOBAL_TEST_DSN", "global")
	region := seedTestDatabase(t, "GAMEPANEL_REGION_TEST_DSN", "region")
	ctx := context.Background()
	config := GlobalConfig{AdminUsername: "platform-admin", AdminName: "Platform Admin", AdminPassword: "temporary-pass-123", FundingKey: []byte("funding-key-32-bytes-long-for-test"), ProviderKey: []byte("provider-key-32-bytes-long-for-test")}
	firstUser, err := SeedGlobal(ctx, global, config)
	if err != nil {
		t.Fatal(err)
	}
	secondUser, err := SeedGlobal(ctx, global, config)
	if err != nil || secondUser != firstUser {
		t.Fatalf("global seed replay user=%s err=%v", secondUser, err)
	}
	regionConfig := RegionConfig{PublicAddress: "192.0.2.10", PortStart: 32000, PortEnd: 32999}
	if err := SeedRegion(ctx, region, regionConfig); err != nil {
		t.Fatal(err)
	}
	if err := SeedRegion(ctx, region, regionConfig); err != nil {
		t.Fatalf("Region seed replay: %v", err)
	}
	var bindings, providers, ledgers, memberships int
	checks := []struct {
		query  string
		args   []any
		target *int
	}{
		{`SELECT count(*) FROM authorization_role_bindings WHERE principal_id=$1`, []any{firstUser}, &bindings},
		{`SELECT count(*) FROM provider_releases`, nil, &providers},
		{`SELECT count(*) FROM ledger_entries WHERE workspace_id=$1`, []any{WorkspaceID}, &ledgers},
		{`SELECT count(*) FROM memberships WHERE workspace_id=$1 AND user_id=$2`, []any{WorkspaceID, firstUser}, &memberships},
	}
	for _, check := range checks {
		if err := global.QueryRow(check.query, check.args...).Scan(check.target); err != nil {
			t.Fatal(err)
		}
	}
	if bindings != 3 || providers != 2 || ledgers != 1 || memberships != 1 {
		t.Fatalf("bindings=%d providers=%d ledgers=%d memberships=%d", bindings, providers, ledgers, memberships)
	}
	var pools int
	if err := region.QueryRow(`SELECT count(*) FROM endpoint_pools WHERE region_id=$1 AND active=true`, RegionID).Scan(&pools); err != nil || pools != 1 {
		t.Fatalf("endpoint pools=%d err=%v", pools, err)
	}
}

func seedTestDatabase(t *testing.T, environment, scope string) *sql.DB {
	t.Helper()
	dsn := os.Getenv(environment)
	if dsn == "" {
		t.Skip(environment + " is not set")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("phase6_seed_%s_%d", scope, time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE SCHEMA "` + schema + `"`); err != nil {
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
	if err := migrations.Migrate(context.Background(), database, scope); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		database.Close()
		_, _ = admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`)
		admin.Close()
	})
	return database
}
