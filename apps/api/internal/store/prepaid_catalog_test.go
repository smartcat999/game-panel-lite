package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/commerce"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestPrepaidCatalog(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testPrepaidCatalog(t, db)
}
func testPrepaidCatalog(t *testing.T, db *Store) {
	ctx := context.Background()
	admin := domain.AdminAccount{ID: "catalog-admin", Username: "catalog-admin", Role: domain.RoleAdmin, PasswordHash: "test"}
	if err := db.db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	plan := commerce.PlanVersion{PlanID: "standard", Version: 1, ProviderKey: "test", RegionID: "east", CPU: 1, MemoryMB: 128, StorageBytes: 1024, BackupRetentionCount: 1, Currency: "CNY", UnitAmountMinor: 100, PeriodSeconds: 86400}
	if err := db.PublishPrepaidPlan(ctx, "unknown", plan); !errors.Is(err, commerce.ErrOperatorRequired) {
		t.Fatalf("unauthorized publish: %v", err)
	}
	if err := db.PublishPrepaidPlan(ctx, admin.ID, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := db.QuoteListedPrepaidPlan(ctx, plan.PlanID, 1, 1); !errors.Is(err, commerce.ErrPlanUnavailable) {
		t.Fatal("publication enabled sale")
	}
	if err := db.SetPrepaidPlanSale(ctx, admin.ID, plan.PlanID, 1, 1, true); err != nil {
		t.Fatal(err)
	}
	quote, err := db.QuoteListedPrepaidPlan(ctx, plan.PlanID, 1, 2)
	if err != nil || quote.Plan != plan || quote.AmountMinor != 200 {
		t.Fatalf("catalog quote: %+v %v", quote, err)
	}
	conflicting := plan
	conflicting.UnitAmountMinor++
	if err := db.PublishPrepaidPlan(ctx, admin.ID, conflicting); !errors.Is(err, commerce.ErrPlanConflict) {
		t.Fatal("changed published price")
	}
	for _, statement := range []string{"UPDATE prepaid_plan_versions SET terms = '{}' WHERE plan_id = ?", "DELETE FROM prepaid_plan_versions WHERE plan_id = ?"} {
		if err := db.db.Exec(statement, plan.PlanID).Error; err == nil {
			t.Fatal("immutable catalog changed")
		}
	}
	if err := db.SetPrepaidPlanSale(ctx, admin.ID, plan.PlanID, 1, 2, false); err != nil {
		t.Fatal(err)
	}
	if err := db.PublishPrepaidPlan(ctx, admin.ID, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := db.QuoteListedPrepaidPlan(ctx, plan.PlanID, 1, 1); !errors.Is(err, commerce.ErrPlanUnavailable) {
		t.Fatal("replay revived sale")
	}
	if err := db.SetPrepaidPlanSale(ctx, admin.ID, plan.PlanID, 1, 2, true); !errors.Is(err, commerce.ErrCatalogVersionConflict) {
		t.Fatal("stale sale update")
	}
	plan.Version = 2
	plan.UnitAmountMinor = 150
	if err := db.PublishPrepaidPlan(ctx, admin.ID, plan); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPrepaidPlanSale(ctx, admin.ID, plan.PlanID, 2, 1, true); err != nil {
		t.Fatal(err)
	}
	updated, err := db.QuoteListedPrepaidPlan(ctx, plan.PlanID, 2, 2)
	if err != nil || updated.AmountMinor != 300 || quote.AmountMinor != 200 {
		t.Fatal("versioned price not preserved")
	}
	if db.db.Dialector.Name() == "postgres" {
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); results <- db.SetPrepaidPlanSale(ctx, admin.ID, plan.PlanID, 2, 2, false) }()
		}
		wg.Wait()
		close(results)
		success, conflict := 0, 0
		for err := range results {
			if err == nil {
				success++
			} else if errors.Is(err, commerce.ErrCatalogVersionConflict) {
				conflict++
			} else {
				t.Fatal(err)
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatal("concurrent catalog update lost")
		}
	}
	// An interrupted publication must not retain terms without their sale record.
	if db.db.Dialector.Name() == "postgres" {
		if err := db.db.Exec("ALTER TABLE prepaid_plan_sales ADD CONSTRAINT catalog_test_failure CHECK(plan_version <> 3)").Error; err != nil {
			t.Fatal(err)
		}
		defer db.db.Exec("ALTER TABLE prepaid_plan_sales DROP CONSTRAINT catalog_test_failure")
	} else {
		if err := db.db.Exec("CREATE TRIGGER catalog_test_failure BEFORE INSERT ON prepaid_plan_sales WHEN NEW.plan_version = 3 BEGIN SELECT RAISE(ABORT,'test failure'); END").Error; err != nil {
			t.Fatal(err)
		}
		defer db.db.Exec("DROP TRIGGER catalog_test_failure")
	}
	plan.Version = 3
	if err := db.PublishPrepaidPlan(ctx, admin.ID, plan); err == nil {
		t.Fatal("expected injected failure")
	}
	var count int64
	if err := db.db.Table("prepaid_plan_versions").Where("plan_id = ? AND version = ?", plan.PlanID, 3).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("partial publication persisted")
	}
}
