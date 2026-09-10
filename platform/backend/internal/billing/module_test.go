package billing

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"
)

var billingNow = time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC)

func testCatalog(version int64, effectiveAt time.Time) RegionCatalog {
	return RegionCatalog{
		ID: "cat_asia_" + time.Unix(version, 0).Format("05"), RegionID: "reg_asia", Version: version,
		CPU: Range{Minimum: 500, Maximum: 8000, Step: 500}, Memory: Range{Minimum: 1024, Maximum: 16384, Step: 1024}, Disk: Range{Minimum: 10, Maximum: 100, Step: 5},
		DedicatedIPAvailable: true, EndpointDeliveryModes: []string{"gateway", "dedicated-ip"}, Availability: AvailabilityAvailable, EffectiveAt: effectiveAt, CreatedAt: billingNow,
	}
}

func testPriceBook(id string, revision, cpuPrice int64, effectiveAt time.Time) PriceBook {
	return PriceBook{ID: id, RegionID: "reg_asia", Revision: revision, Currency: CurrencyCNY, EffectiveAt: effectiveAt, CreatedAt: billingNow, UnitPrices: []UnitPrice{
		{ResourceKind: ResourceCPU, PriceMinor: cpuPrice, UnitQuantity: 1000, Unit: "1000-millicpu-hour"},
		{ResourceKind: ResourceMemory, PriceMinor: 5, UnitQuantity: 1024, Unit: "gib-hour"},
		{ResourceKind: ResourceInstanceDisk, PriceMinor: 1, UnitQuantity: 1, Unit: "gib-hour"},
		{ResourceKind: ResourceBackupStorage, PriceMinor: 2, UnitQuantity: 1, Unit: "gib-hour"},
		{ResourceKind: ResourceDedicatedIP, PriceMinor: 3, UnitQuantity: 1, Unit: "address-hour"},
	}}
}

func testModule(t *testing.T) (*Module, *MemoryStore) {
	t.Helper()
	store := NewMemoryStore()
	module := New(store, []byte("funding-signature-test-key"))
	module.now = func() time.Time { return billingNow }
	if err := module.PublishCatalog(context.Background(), testCatalog(1, billingNow.Add(-time.Hour))); err != nil {
		t.Fatal(err)
	}
	if err := module.PublishPriceBook(context.Background(), testPriceBook("pb_1", 1, 10, billingNow.Add(-time.Hour))); err != nil {
		t.Fatal(err)
	}
	return module, store
}

func TestPriceBookTransitionPinsQuoteAndRejectsMutation(t *testing.T) {
	module, _ := testModule(t)
	first, err := module.CreateQuote(context.Background(), "ws_one", "reg_asia", ResourceSpec{CPUMilli: 1000, MemoryMiB: 1024, DiskGiB: 10}, false)
	if err != nil || first.PriceBookID != "pb_1" || first.EstimatedHourlyMinor != 25 {
		t.Fatalf("first quote = %#v err=%v", first, err)
	}
	secondBook := testPriceBook("pb_2", 2, 20, billingNow.Add(30*time.Minute))
	if err := module.PublishPriceBook(context.Background(), secondBook); err != nil {
		t.Fatal(err)
	}
	module.now = func() time.Time { return billingNow.Add(time.Hour) }
	second, err := module.CreateQuote(context.Background(), "ws_one", "reg_asia", ResourceSpec{CPUMilli: 1000, MemoryMiB: 1024, DiskGiB: 10}, false)
	if err != nil || second.PriceBookID != "pb_2" || second.EstimatedHourlyMinor != 35 || first.PriceBookID != "pb_1" {
		t.Fatalf("price transition first=%#v second=%#v err=%v", first, second, err)
	}
	mutated := secondBook
	mutated.UnitPrices[0].PriceMinor = 99
	if err := module.PublishPriceBook(context.Background(), mutated); !errors.Is(err, ErrImmutable) {
		t.Fatalf("price book mutation error=%v", err)
	}
}

func TestQuoteValidationAndFundingHold(t *testing.T) {
	module, _ := testModule(t)
	if _, err := module.CreateQuote(context.Background(), "ws_one", "reg_asia", ResourceSpec{CPUMilli: 750, MemoryMiB: 1024, DiskGiB: 10}, false); !errors.Is(err, ErrInvalidResourceSpec) {
		t.Fatalf("invalid resource step accepted: %v", err)
	}
	quote, err := module.CreateQuote(context.Background(), "ws_one", "reg_asia", ResourceSpec{CPUMilli: 1000, MemoryMiB: 1024, DiskGiB: 10}, true)
	if err != nil || quote.EstimatedHourlyMinor != 28 {
		t.Fatalf("quote=%#v err=%v", quote, err)
	}
	if _, err := module.AuthorizeCreate(context.Background(), "ws_other", quote.ID); !errors.Is(err, ErrQuoteScope) {
		t.Fatalf("cross-workspace quote accepted: %v", err)
	}
	if _, err := module.AuthorizeCreate(context.Background(), "ws_one", quote.ID); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("underfunded quote accepted: %v", err)
	}
	if _, err := module.GrantPromotionalCredit(context.Background(), "ws_one", "grant_beta", 1000, "beta credit"); err != nil {
		t.Fatal(err)
	}
	authorized, err := module.AuthorizeCreate(context.Background(), "ws_one", quote.ID)
	if err != nil || authorized.Hold == nil || authorized.Funding == nil || authorized.Funding.MaximumDebitMinor != 672 || len(authorized.Funding.Signature) != 64 {
		t.Fatalf("funding authorization=%#v err=%v", authorized, err)
	}
	repeated, err := module.AuthorizeCreate(context.Background(), "ws_one", quote.ID)
	if err != nil || repeated.Hold.ID != authorized.Hold.ID {
		t.Fatalf("hold retry not idempotent: %#v %v", repeated, err)
	}
}

func TestStoppedAllocationChargesOnlyRetainedResources(t *testing.T) {
	module, _ := testModule(t)
	if _, err := module.GrantPromotionalCredit(context.Background(), "ws_one", "grant_usage", 1000, "usage credit"); err != nil {
		t.Fatal(err)
	}
	allocation := Allocation{WorkspaceID: "ws_one", LogicalInstanceID: "lin_one", RegionID: "reg_asia", PriceBookID: "pb_1", CPUMilli: 1000, MemoryMiB: 1024, DiskGiB: 10, BackupGiB: 2, DedicatedIP: true, ComputeAllocated: false}
	result, err := module.RecordUsage(context.Background(), allocation, billingNow.Add(-time.Hour), billingNow)
	if err != nil {
		t.Fatal(err)
	}
	if result.RequestedMinor != 17 {
		t.Fatalf("stopped charge=%d want 17", result.RequestedMinor)
	}
	for _, entry := range result.Entries {
		if entry.AmountMinor >= 0 {
			t.Fatalf("usage debit is not negative: %#v", entry)
		}
	}
	repeated, err := module.RecordUsage(context.Background(), allocation, billingNow.Add(-time.Hour), billingNow)
	if err != nil || repeated.CollectedMinor != 0 {
		t.Fatalf("usage replay charged again: %#v %v", repeated, err)
	}
	wallet, _ := module.Wallet(context.Background(), "ws_one")
	if wallet.AvailableMinor != 983 {
		t.Fatalf("wallet=%#v", wallet)
	}
}

func TestUsageCannotApplyAnotherRegionsPriceBook(t *testing.T) {
	module, _ := testModule(t)
	allocation := Allocation{WorkspaceID: "ws_one", LogicalInstanceID: "lin_one", RegionID: "reg_other", PriceBookID: "pb_1", DiskGiB: 10}
	if _, err := module.RecordUsage(context.Background(), allocation, billingNow.Add(-time.Hour), billingNow); !errors.Is(err, ErrInvalidPriceBook) {
		t.Fatalf("cross-region price book accepted: %v", err)
	}
}

func TestPromotionalFundsAreConsumedBeforeCashAndCorrectionIsImmutable(t *testing.T) {
	module, store := testModule(t)
	if _, err := module.GrantPromotionalCredit(context.Background(), "ws_one", "promo", 20, "beta"); err != nil {
		t.Fatal(err)
	}
	if _, err := module.Correct(context.Background(), "ws_one", "cash_seed", BucketCash, 100, "operator correction"); err != nil {
		t.Fatal(err)
	}
	allocation := Allocation{WorkspaceID: "ws_one", LogicalInstanceID: "lin_one", RegionID: "reg_asia", PriceBookID: "pb_1", CPUMilli: 1000, MemoryMiB: 1024, DiskGiB: 10, ComputeAllocated: true}
	result, err := module.RecordUsage(context.Background(), allocation, billingNow.Add(-time.Hour), billingNow)
	if err != nil || len(result.Entries) != 2 || result.Wallet.PromotionalMinor != 0 || result.Wallet.CashMinor != 95 {
		t.Fatalf("bucket debit result=%#v err=%v", result, err)
	}
	if _, err := module.Correct(context.Background(), "ws_one", "cash_seed", BucketCash, 101, "operator correction"); !errors.Is(err, ErrImmutable) {
		t.Fatalf("correction mutation error=%v", err)
	}
	if len(store.entries["ws_one"]) != 4 {
		t.Fatalf("ledger entry count=%d", len(store.entries["ws_one"]))
	}
}

func TestWalletAndUsageAmountsRejectIntegerOverflow(t *testing.T) {
	module, store := testModule(t)
	if _, err := module.GrantPromotionalCredit(context.Background(), "ws_overflow", "first", math.MaxInt64, "boundary"); err != nil {
		t.Fatal(err)
	}
	if _, err := module.GrantPromotionalCredit(context.Background(), "ws_overflow", "second", 1, "overflow"); !errors.Is(err, ErrNegativeBalance) {
		t.Fatalf("wallet overflow error=%v", err)
	}
	records := []UsageRecord{
		{ID: "use_one", WorkspaceID: "ws_usage_overflow", ChargeMinor: math.MaxInt64},
		{ID: "use_two", WorkspaceID: "ws_usage_overflow", ChargeMinor: 1},
	}
	if _, err := store.PostUsage(context.Background(), records, billingNow); !errors.Is(err, ErrInvalidPriceBook) {
		t.Fatalf("usage overflow error=%v", err)
	}
}

func TestConcurrentUsageDebitsNeverProduceNegativeBalance(t *testing.T) {
	module, _ := testModule(t)
	if _, err := module.GrantPromotionalCredit(context.Background(), "ws_one", "promo", 100, "beta"); err != nil {
		t.Fatal(err)
	}
	allocations := []Allocation{
		{WorkspaceID: "ws_one", LogicalInstanceID: "lin_a", RegionID: "reg_asia", PriceBookID: "pb_1", CPUMilli: 8000, ComputeAllocated: true},
		{WorkspaceID: "ws_one", LogicalInstanceID: "lin_b", RegionID: "reg_asia", PriceBookID: "pb_1", CPUMilli: 8000, ComputeAllocated: true},
	}
	var wait sync.WaitGroup
	results := make(chan DebitResult, len(allocations))
	errorsChannel := make(chan error, len(allocations))
	for _, allocation := range allocations {
		wait.Add(1)
		go func(value Allocation) {
			defer wait.Done()
			result, err := module.RecordUsage(context.Background(), value, billingNow.Add(-time.Hour), billingNow)
			results <- result
			errorsChannel <- err
		}(allocation)
	}
	wait.Wait()
	close(results)
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}
	var collected int64
	for result := range results {
		collected += result.CollectedMinor
	}
	wallet, _ := module.Wallet(context.Background(), "ws_one")
	if collected != 100 || wallet.AvailableMinor != 0 || wallet.State != WalletExhausted {
		t.Fatalf("collected=%d wallet=%#v", collected, wallet)
	}
	policy := DefaultExhaustionPolicy()
	if !policy.StopRunningInstances || policy.FullDataRetention != 7*24*time.Hour || policy.BackupOnlyRetention != 30*24*time.Hour {
		t.Fatalf("exhaustion policy=%#v", policy)
	}
}
