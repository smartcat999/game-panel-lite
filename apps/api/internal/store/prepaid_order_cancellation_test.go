package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/commerce"
)

func testOrderCancellation(t *testing.T, db *Store, order commerce.Order) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.CancelPrepaidOrder(ctx, "foreign", order.OrganizationID, order.ID); err == nil {
		t.Fatal("foreign cancellation")
	}
	cancelled, err := db.CancelPrepaidOrder(ctx, "order-user", order.OrganizationID, order.ID)
	if err != nil || cancelled.Status != "cancelled" || cancelled.CancelReason != "user" || cancelled.CancelledBy != "order-user" || cancelled.CancelledAtMS < order.CreatedAtMS || cancelled.Quote != order.Quote {
		t.Fatalf("cancel: %+v %v", cancelled, err)
	}
	replay, err := db.CancelPrepaidOrder(ctx, "order-user", order.OrganizationID, order.ID)
	if err != nil || replay != cancelled {
		t.Fatal("cancellation replay changed receipt")
	}
	if _, err := db.CancelPrepaidOrder(ctx, "order-user", "another-org", order.ID); err == nil {
		t.Fatal("cross tenant cancellation")
	}
	var original prepaidOrderRow
	if err := db.db.Table("prepaid_orders").Where("id = ?", order.ID).Take(&original).Error; err != nil {
		t.Fatal(err)
	}
	now, err := outboxNow(db.db)
	if err != nil {
		t.Fatal(err)
	}
	// Seed independent historical fixtures; no payment provider or real charge.
	for i := 0; i < 5; i++ {
		row := original
		row.ID = fmt.Sprintf("expiry-fixture-%d", i)
		row.IdempotencyKey = row.ID
		row.Status = "pending"
		row.CancelReason = ""
		row.CancelledAtMS = 0
		row.CancelledBy = ""
		row.CreatedAtMS = now - 2000
		row.ExpiresAtMS = now - 1000
		if i == 4 {
			row.Status = "paid"
		}
		if err := db.db.Table("prepaid_orders").Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.CancelPrepaidOrder(ctx, "order-user", order.OrganizationID, "expiry-fixture-4"); !errors.Is(err, commerce.ErrOrderNotCancellable) {
		t.Fatal("paid order cancelled")
	}
	expired, err := db.CancelPrepaidOrder(ctx, "order-user", order.OrganizationID, "expiry-fixture-0")
	if err != nil || expired.CancelReason != "expired" || expired.CancelledBy != "" {
		t.Fatal("expired request reported user cancellation")
	}
	if count, err := db.ExpirePrepaidOrders(ctx, 1); err != nil || count != 1 {
		t.Fatalf("batch bound: %d %v", count, err)
	}
	if db.db.Dialector.Name() == "postgres" {
		var wg sync.WaitGroup
		results := make(chan int, 2)
		errs := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); n, e := db.ExpirePrepaidOrders(ctx, 1); results <- n; errs <- e }()
		}
		wg.Wait()
		close(results)
		close(errs)
		total := 0
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		for n := range results {
			total += n
		}
		if total != 2 {
			t.Fatalf("concurrent expiry count: %d", total)
		}
	} else {
		if count, err := db.ExpirePrepaidOrders(ctx, 2); err != nil || count != 2 {
			t.Fatalf("expiry: %d %v", count, err)
		}
	}
	if count, err := db.ExpirePrepaidOrders(ctx, 200); err != nil || count != 0 {
		t.Fatalf("expired future/paid order: %d %v", count, err)
	}
	for _, limit := range []int{0, 201} {
		if _, err := db.ExpirePrepaidOrders(ctx, limit); !errors.Is(err, commerce.ErrInvalidOrder) {
			t.Fatal("unbounded expiry accepted")
		}
	}
	var row prepaidOrderRow
	if err := db.db.Table("prepaid_orders").Where("id = ?", "expiry-fixture-4").Take(&row).Error; err != nil || row.Status != "paid" || row.CancelledAtMS != 0 {
		t.Fatal("maintenance changed paid order")
	}
}
