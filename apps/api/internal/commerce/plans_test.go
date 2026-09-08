package commerce

import (
	"errors"
	"math"
	"testing"
)

func testPlan() PlanVersion {
	return PlanVersion{PlanID: "standard", Version: 1, ProviderKey: "test-provider", RegionID: "east", CPU: 1, MemoryMB: 1024, StorageBytes: 1024 * 1024, BackupRetentionCount: 3, Currency: "CNY", UnitAmountMinor: 1990, PeriodSeconds: 86400}
}

func TestQuoteCapturesTerms(t *testing.T) {
	plan := testPlan()
	quote, err := QuotePrepaid(plan, 3)
	if err != nil || quote.AmountMinor != 5970 || quote.DurationMS != 259200000 || quote.Plan != plan {
		t.Fatalf("quote: %+v %v", quote, err)
	}
	plan.Version++
	plan.UnitAmountMinor = 2990
	plan.StorageBytes *= 2
	if quote.Plan.Version != 1 || quote.Plan.UnitAmountMinor != 1990 || quote.Plan.StorageBytes == plan.StorageBytes {
		t.Fatal("catalog change mutated captured order terms")
	}
	updated, err := QuotePrepaid(plan, 3)
	if err != nil || updated.AmountMinor != 8970 {
		t.Fatalf("updated price: %+v %v", updated, err)
	}
	// Free plans are explicit catalog data, never an absent-price fallback.
	plan.UnitAmountMinor = 0
	if free, err := QuotePrepaid(plan, 1); err != nil || free.AmountMinor != 0 {
		t.Fatal("explicit free plan rejected")
	}
}

func TestQuoteRejectsInvalidTermsAndOverflow(t *testing.T) {
	for _, mutate := range []func(*PlanVersion){
		func(p *PlanVersion) { p.Version = 0 }, func(p *PlanVersion) { p.PlanID = " " },
		func(p *PlanVersion) { p.Currency = "cny" }, func(p *PlanVersion) { p.Currency = "" },
		func(p *PlanVersion) { p.UnitAmountMinor = -1 }, func(p *PlanVersion) { p.CPU = math.NaN() },
		func(p *PlanVersion) { p.CPU = math.Inf(1) }, func(p *PlanVersion) { p.MemoryMB = 0 },
		func(p *PlanVersion) { p.StorageBytes = 0 }, func(p *PlanVersion) { p.BackupRetentionCount = -1 },
		func(p *PlanVersion) { p.PeriodSeconds = 0 }, func(p *PlanVersion) { p.PeriodSeconds = math.MaxInt64 },
	} {
		plan := testPlan()
		mutate(&plan)
		got, err := QuotePrepaid(plan, 1)
		if !errors.Is(err, ErrInvalidPlan) || got.Periods != 0 {
			t.Fatalf("invalid plan produced quote: %+v %v", got, err)
		}
	}
	for _, periods := range []int64{0, -1, math.MaxInt64} {
		got, err := QuotePrepaid(testPlan(), periods)
		if !errors.Is(err, ErrInvalidQuote) || got.Periods != 0 {
			t.Fatalf("invalid periods: %+v %v", got, err)
		}
	}
	plan := testPlan()
	plan.UnitAmountMinor = math.MaxInt64
	if _, err := QuotePrepaid(plan, 2); !errors.Is(err, ErrInvalidQuote) {
		t.Fatal("money overflow")
	}
	plan.UnitAmountMinor = 0
	plan.PeriodSeconds = math.MaxInt64 / 1000
	if _, err := QuotePrepaid(plan, 2); !errors.Is(err, ErrInvalidQuote) {
		t.Fatal("duration overflow")
	}
}
