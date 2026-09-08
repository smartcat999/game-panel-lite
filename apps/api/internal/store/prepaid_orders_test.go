package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/commerce"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

func TestPrepaidOrders(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testPrepaidOrders(t, db)
}
func testPrepaidOrders(t *testing.T, db *Store) {
	ctx := context.Background()
	org := domain.Organization{ID: "order-org", Slug: "order-org"}
	if err := db.CreateOrganization(ctx, &org, "order-user"); err != nil {
		t.Fatal(err)
	}
	admin := domain.AdminAccount{ID: "order-admin", Username: "order-admin", Role: domain.RoleAdmin, PasswordHash: "test"}
	if err := db.db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	plan := commerce.PlanVersion{PlanID: "order-plan", Version: 1, ProviderKey: "test", RegionID: "east", CPU: 1, MemoryMB: 128, StorageBytes: 1024, Currency: "CNY", UnitAmountMinor: 100, PeriodSeconds: 86400}
	if err := db.PublishPrepaidPlan(ctx, admin.ID, plan); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPrepaidPlanSale(ctx, admin.ID, plan.PlanID, 1, 1, true); err != nil {
		t.Fatal(err)
	}
	created, err := db.CreateGlobalServer(ctx, "order-user", instances.CreateRequest{OrganizationID: org.ID, Name: "order-instance", RegionID: "east", IdempotencyKey: "order-instance", Specification: instances.Specification{ProviderKey: "test", GameVersion: "1", ConfigSchemaVersion: 1, Configuration: instances.ProtectedConfiguration{KeyID: "test", Ciphertext: []byte("opaque")}, Resources: instances.Resources{CPU: 1, MemoryMB: 128}}})
	if err != nil {
		t.Fatal(err)
	}
	request := commerce.OrderRequest{OrganizationID: org.ID, ServerID: created.Server.ID, PlanID: plan.PlanID, PlanVersion: 1, Periods: 2, IdempotencyKey: "purchase"}
	if _, err := db.CreatePrepaidOrder(ctx, "foreign", request, time.Minute); err == nil {
		t.Fatal("foreign actor purchased")
	}
	order, err := db.CreatePrepaidOrder(ctx, "order-user", request, time.Minute)
	if err != nil || order.Status != "pending" || order.Quote.AmountMinor != 200 || order.RevisionID != created.Revision.ID || order.ExpiresAtMS-order.CreatedAtMS != 60000 {
		t.Fatalf("order: %+v %v", order, err)
	}
	replay, err := db.CreatePrepaidOrder(ctx, "order-user", request, time.Hour)
	if err != nil || replay != order {
		t.Fatalf("replay changed deadline: %+v %v", replay, err)
	}
	bad := request
	bad.Periods = 3
	if _, err := db.CreatePrepaidOrder(ctx, "order-user", bad, time.Minute); !errors.Is(err, commerce.ErrOrderConflict) {
		t.Fatal("request collision")
	}
	if err := db.SetPrepaidPlanSale(ctx, admin.ID, plan.PlanID, 1, 2, false); err != nil {
		t.Fatal(err)
	}
	replay, err = db.CreatePrepaidOrder(ctx, "order-user", request, time.Minute)
	if err != nil || replay != order {
		t.Fatal("delisting hid order receipt")
	}
	bad = request
	bad.IdempotencyKey = "new-after-delist"
	if _, err := db.CreatePrepaidOrder(ctx, "order-user", bad, time.Minute); !errors.Is(err, commerce.ErrOrderUnavailable) {
		t.Fatal("delisted plan purchased")
	}
	for _, sql := range []string{"UPDATE prepaid_orders SET quote = '{}' WHERE id = ?", "DELETE FROM prepaid_orders WHERE id = ?"} {
		if err := db.db.Exec(sql, order.ID).Error; err == nil {
			t.Fatal("order history changed")
		}
	}
	if err := db.SetPrepaidPlanSale(ctx, admin.ID, plan.PlanID, 1, 3, true); err != nil {
		t.Fatal(err)
	}
	if db.db.Dialector.Name() == "postgres" {
		var wg sync.WaitGroup
		results := make(chan commerce.Order, 2)
		errs := make(chan error, 2)
		concurrent := request
		concurrent.IdempotencyKey = "concurrent"
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				o, e := db.CreatePrepaidOrder(ctx, "order-user", concurrent, time.Minute)
				results <- o
				errs <- e
			}()
		}
		wg.Wait()
		close(results)
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		var id string
		for o := range results {
			if id != "" && id != o.ID {
				t.Fatal("duplicate orders")
			}
			id = o.ID
		}
	}
	plan.Version = 2
	plan.CPU = 2
	if err := db.PublishPrepaidPlan(ctx, admin.ID, plan); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPrepaidPlanSale(ctx, admin.ID, plan.PlanID, 2, 1, true); err != nil {
		t.Fatal(err)
	}
	bad = request
	bad.PlanVersion = 2
	bad.IdempotencyKey = "wrong-spec"
	if _, err := db.CreatePrepaidOrder(ctx, "order-user", bad, time.Minute); !errors.Is(err, commerce.ErrOrderUnavailable) {
		t.Fatal("mismatched spec purchased")
	}
	var rights int64
	if err := db.db.Table("global_server_entitlements").Where("server_id = ?", created.Server.ID).Count(&rights).Error; err != nil || rights != 0 {
		t.Fatal("unpaid order granted service")
	}
}
