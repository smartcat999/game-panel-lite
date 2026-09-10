package billing

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
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresPricingWalletAndConcurrentDebit(t *testing.T) {
	database := openBillingDatabase(t)
	ctx := context.Background()
	store := NewPostgresStore(database)
	module := New(store, []byte("postgres-funding-signature-key"))
	module.now = func() time.Time { return billingNow }
	if err := module.PublishCatalog(ctx, testCatalog(1, billingNow.Add(-time.Hour))); err != nil {
		t.Fatal(err)
	}
	if err := module.PublishPriceBook(ctx, testPriceBook("pb_pg_1", 1, 10, billingNow.Add(-time.Hour))); err != nil {
		t.Fatal(err)
	}
	if err := module.PublishPriceBook(ctx, testPriceBook("pb_pg_1", 1, 10, billingNow.Add(-time.Hour))); err != nil {
		t.Fatalf("identical price publication retry: %v", err)
	}
	if _, err := module.GrantPromotionalCredit(ctx, "ws_pg", "promo_pg", 20, "beta"); err != nil {
		t.Fatal(err)
	}
	if _, err := module.Correct(ctx, "ws_pg", "cash_pg", BucketCash, 80, "cash fixture"); err != nil {
		t.Fatal(err)
	}
	quote, err := module.CreateQuote(ctx, "ws_pg", "reg_asia", ResourceSpec{CPUMilli: 1000, MemoryMiB: 1024, DiskGiB: 10}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := module.AuthorizeCreate(ctx, "ws_pg", quote.ID); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("24-hour funding check error=%v", err)
	}
	if _, err := module.GrantPromotionalCredit(ctx, "ws_pg", "promo_hold", 500, "hold coverage"); err != nil {
		t.Fatal(err)
	}
	authorized, err := module.AuthorizeCreate(ctx, "ws_pg", quote.ID)
	if err != nil || authorized.Hold == nil || authorized.Funding == nil {
		t.Fatalf("authorize create=%#v err=%v", authorized, err)
	}

	allocations := []Allocation{
		{WorkspaceID: "ws_pg", LogicalInstanceID: "lin_pg_a", RegionID: "reg_asia", PriceBookID: "pb_pg_1", CPUMilli: 8000, ComputeAllocated: true},
		{WorkspaceID: "ws_pg", LogicalInstanceID: "lin_pg_b", RegionID: "reg_asia", PriceBookID: "pb_pg_1", CPUMilli: 8000, ComputeAllocated: true},
	}
	var wait sync.WaitGroup
	errorsChannel := make(chan error, 2)
	for _, allocation := range allocations {
		wait.Add(1)
		go func(value Allocation) {
			defer wait.Done()
			_, err := module.RecordUsage(ctx, value, billingNow.Add(-time.Hour), billingNow)
			errorsChannel <- err
		}(allocation)
	}
	wait.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}
	wallet, err := module.Wallet(ctx, "ws_pg")
	if err != nil || wallet.AvailableMinor != 440 || wallet.PromotionalMinor != 360 || wallet.CashMinor != 80 {
		t.Fatalf("wallet after concurrent debit=%#v err=%v", wallet, err)
	}
	var usageCount int
	if err := database.QueryRow(`SELECT count(*) FROM usage_records`).Scan(&usageCount); err != nil || usageCount != 2 {
		t.Fatalf("usage count=%d err=%v", usageCount, err)
	}
	if _, err := module.RecordUsage(ctx, allocations[0], billingNow.Add(-time.Hour), billingNow); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT count(*) FROM usage_records`).Scan(&usageCount); err != nil || usageCount != 2 {
		t.Fatalf("usage replay count=%d err=%v", usageCount, err)
	}

	if _, err := database.Exec(`UPDATE ledger_entries SET reason = 'mutated' WHERE source_id = 'cash_pg'`); err == nil {
		t.Fatal("immutable correction row was updated")
	}
	secondBook := testPriceBook("pb_pg_2", 2, 20, billingNow.Add(time.Hour))
	if err := module.PublishPriceBook(ctx, secondBook); err != nil {
		t.Fatal(err)
	}
	module.now = func() time.Time { return billingNow.Add(2 * time.Hour) }
	transitioned, err := module.CreateQuote(ctx, "ws_pg", "reg_asia", ResourceSpec{CPUMilli: 1000, MemoryMiB: 1024, DiskGiB: 10}, false)
	if err != nil || transitioned.PriceBookID != "pb_pg_2" || transitioned.EstimatedHourlyMinor != 35 {
		t.Fatalf("PostgreSQL price transition quote=%#v err=%v", transitioned, err)
	}
	if _, err := database.Exec(`UPDATE price_books SET currency = 'USD' WHERE id = 'pb_pg_2'`); err == nil {
		t.Fatal("immutable price book was updated")
	}
}

func TestPostgresDebitAtBalanceBoundaryNeverGoesNegative(t *testing.T) {
	database := openBillingDatabase(t)
	ctx := context.Background()
	module := New(NewPostgresStore(database), []byte("postgres-funding-signature-key"))
	module.now = func() time.Time { return billingNow }
	if err := module.PublishPriceBook(ctx, testPriceBook("pb_boundary", 1, 10, billingNow.Add(-time.Hour))); err != nil {
		t.Fatal(err)
	}
	if _, err := module.GrantPromotionalCredit(ctx, "ws_boundary", "promo", 100, "beta"); err != nil {
		t.Fatal(err)
	}
	allocations := []Allocation{
		{WorkspaceID: "ws_boundary", LogicalInstanceID: "lin_boundary_a", RegionID: "reg_asia", PriceBookID: "pb_boundary", CPUMilli: 8000, ComputeAllocated: true},
		{WorkspaceID: "ws_boundary", LogicalInstanceID: "lin_boundary_b", RegionID: "reg_asia", PriceBookID: "pb_boundary", CPUMilli: 8000, ComputeAllocated: true},
	}
	var wait sync.WaitGroup
	for _, allocation := range allocations {
		wait.Add(1)
		go func(value Allocation) {
			defer wait.Done()
			if _, err := module.RecordUsage(ctx, value, billingNow.Add(-time.Hour), billingNow); err != nil {
				t.Error(err)
			}
		}(allocation)
	}
	wait.Wait()
	wallet, err := module.Wallet(ctx, "ws_boundary")
	if err != nil || wallet.AvailableMinor != 0 || wallet.PromotionalMinor != 0 || wallet.CashMinor != 0 || wallet.State != WalletExhausted {
		t.Fatalf("boundary wallet=%#v err=%v", wallet, err)
	}
	var total int64
	if err := database.QueryRow(`SELECT COALESCE(SUM(amount_minor), 0) FROM ledger_entries WHERE workspace_id = 'ws_boundary'`).Scan(&total); err != nil || total != 0 {
		t.Fatalf("ledger total=%d err=%v", total, err)
	}
}

func openBillingDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("GAMEPANEL_GLOBAL_TEST_DSN")
	if dsn == "" {
		t.Skip("GAMEPANEL_GLOBAL_TEST_DSN is not set")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("phase3_billing_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE SCHEMA "` + schema + `"`); err != nil {
		admin.Close()
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
		admin.Close()
		t.Fatal(err)
	}
	database.SetMaxOpenConns(10)
	_, filename, _, _ := runtime.Caller(0)
	migration, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "..", "..", "migrations", "global", "0006_resource_pricing_wallet.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range splitMigration(string(migration)) {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		database.Close()
		_, _ = admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`)
		admin.Close()
	})
	return database
}

func splitMigration(source string) []string {
	marker := "CREATE FUNCTION reject_immutable_financial_row()"
	index := strings.Index(source, marker)
	if index < 0 {
		return strings.Split(source, ";")
	}
	prefix := source[:index]
	remainder := source[index:]
	functionEnd := strings.Index(remainder, "$$;")
	if functionEnd < 0 {
		return strings.Split(source, ";")
	}
	functionEnd += len("$$;")
	statements := strings.Split(prefix, ";")
	statements = append(statements, remainder[:functionEnd])
	statements = append(statements, strings.Split(remainder[functionEnd:], ";")...)
	var result []string
	for _, statement := range statements {
		if strings.TrimSpace(statement) != "" {
			result = append(result, statement)
		}
	}
	return result
}
