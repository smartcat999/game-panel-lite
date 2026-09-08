package store

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/commerce"
)

func testPaidSubscriptions(t *testing.T, db *Store, original commerce.Order) {
	t.Helper()
	ctx := context.Background()
	orderID := "payment-order-1"
	if _, err := db.ProvisionPaidSubscription(ctx, "payment-order-6"); !errors.Is(err, commerce.ErrSubscriptionUnavailable) {
		t.Fatal("unpaid order created subscription")
	}
	// The user stopped this logical instance before commercial provisioning.
	if err := db.db.Table("logical_servers").Where("id = ?", original.ServerID).Update("desired_state", "stopped").Error; err != nil {
		t.Fatal(err)
	}
	if db.db.Dialector.Name() == "postgres" {
		if err := db.db.Exec("ALTER TABLE prepaid_fulfillment_tasks ADD CONSTRAINT subscription_test_failure CHECK(subscription_id IS NULL)").Error; err != nil {
			t.Fatal(err)
		}
	} else {
		if err := db.db.Exec("CREATE TRIGGER subscription_test_failure BEFORE UPDATE OF subscription_id ON prepaid_fulfillment_tasks BEGIN SELECT RAISE(ABORT,'test failure'); END").Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ProvisionPaidSubscription(ctx, orderID); err == nil {
		t.Fatal("expected task-link failure")
	}
	var count int64
	if err := db.db.Table("service_subscriptions").Where("server_id = ?", original.ServerID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("subscription partially committed")
	}
	if db.db.Dialector.Name() == "postgres" {
		if err := db.db.Exec("ALTER TABLE prepaid_fulfillment_tasks DROP CONSTRAINT subscription_test_failure").Error; err != nil {
			t.Fatal(err)
		}
	} else {
		if err := db.db.Exec("DROP TRIGGER subscription_test_failure").Error; err != nil {
			t.Fatal(err)
		}
	}
	var first commerce.Subscription
	if db.db.Dialector.Name() == "postgres" {
		var wg sync.WaitGroup
		results := make(chan commerce.Subscription, 2)
		errs := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); s, e := db.ProvisionPaidSubscription(ctx, orderID); results <- s; errs <- e }()
		}
		wg.Wait()
		close(results)
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		for s := range results {
			if first.ID != "" && first != s {
				t.Fatal("concurrent provisioning duplicated service")
			}
			first = s
		}
	} else {
		var err error
		first, err = db.ProvisionPaidSubscription(ctx, orderID)
		if err != nil {
			t.Fatal(err)
		}
	}
	replay, err := db.ProvisionPaidSubscription(ctx, orderID)
	if err != nil || replay != first || first.Status != "pending_activation" || first.Quote != original.Quote || first.RevisionID != original.RevisionID {
		t.Fatalf("subscription: %+v %v", replay, err)
	}
	var task struct {
		Status         string
		SubscriptionID *string
	}
	if err := db.db.Table("prepaid_fulfillment_tasks").Where("order_id = ?", orderID).Take(&task).Error; err != nil || task.Status != "pending" || task.SubscriptionID == nil || *task.SubscriptionID != first.ID {
		t.Fatal("delivery prematurely completed or unlinked")
	}
	var server struct{ DesiredState string }
	if err := db.db.Table("logical_servers").Where("id = ?", original.ServerID).Take(&server).Error; err != nil || server.DesiredState != "stopped" {
		t.Fatal("subscription overrode user stop")
	}
	if err := db.db.Table("global_server_entitlements").Where("server_id = ?", original.ServerID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("pending service granted runtime rights")
	}
	now, err := outboxNow(db.db)
	if err != nil {
		t.Fatal(err)
	}
	p := commerce.CapturedPayment{Provider: "test", MerchantID: "test-merchant", TransactionID: "subscription-conflict", EventID: "subscription-conflict", OrderID: "payment-order-6", AmountMinor: original.Quote.AmountMinor, Currency: original.Quote.Plan.Currency, PaidAtMS: now}
	if r, err := db.RecordCapturedPayment(ctx, p); err != nil || r.Disposition != "applied" {
		t.Fatalf("second fixture payment: %+v %v", r, err)
	}
	if _, err := db.ProvisionPaidSubscription(ctx, p.OrderID); !errors.Is(err, commerce.ErrSubscriptionConflict) {
		t.Fatal("second purchase replaced existing subscription")
	}
}
