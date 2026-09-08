package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/commerce"
)

func testCapturedPayments(t *testing.T, db *Store, original commerce.Order) {
	t.Helper()
	ctx := context.Background()
	var seed prepaidOrderRow
	if err := db.db.Table("prepaid_orders").Where("id = ?", original.ID).Take(&seed).Error; err != nil {
		t.Fatal(err)
	}
	now, err := outboxNow(db.db)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 6; i++ {
		row := seed
		row.ID = fmt.Sprintf("payment-order-%d", i)
		row.IdempotencyKey = row.ID
		row.Status = "pending"
		row.CancelReason = ""
		row.CancelledAtMS = 0
		row.CancelledBy = ""
		row.CreatedAtMS = now - 2000
		row.ExpiresAtMS = now + 3600000
		if i == 3 {
			row.ExpiresAtMS = now - 1000
		}
		if err := db.db.Table("prepaid_orders").Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	capture := commerce.CapturedPayment{Provider: "test", MerchantID: "test-merchant", TransactionID: "transaction-1", EventID: "event-1", OrderID: "payment-order-1", AmountMinor: original.Quote.AmountMinor, Currency: original.Quote.Plan.Currency, PaidAtMS: now}
	receipt, err := db.RecordCapturedPayment(ctx, capture)
	if err != nil || receipt.Disposition != "applied" {
		t.Fatalf("capture: %+v %v", receipt, err)
	}
	replay, err := db.RecordCapturedPayment(ctx, capture)
	if err != nil || replay != receipt {
		t.Fatal("event replay changed receipt")
	}
	duplicate := capture
	duplicate.EventID = "event-duplicate"
	replay, err = db.RecordCapturedPayment(ctx, duplicate)
	if err != nil || replay != receipt {
		t.Fatal("second event duplicated transaction")
	}
	bad := capture
	bad.AmountMinor++
	if _, err := db.RecordCapturedPayment(ctx, bad); !errors.Is(err, commerce.ErrPaymentConflict) {
		t.Fatal("conflicting event accepted")
	}
	bad = capture
	bad.EventID = "event-other-order"
	bad.OrderID = "payment-order-6"
	if _, err := db.RecordCapturedPayment(ctx, bad); !errors.Is(err, commerce.ErrPaymentConflict) {
		t.Fatal("transaction reused on another order")
	}
	if _, err := db.CancelPrepaidOrder(ctx, "order-user", original.OrganizationID, capture.OrderID); !errors.Is(err, commerce.ErrOrderNotCancellable) {
		t.Fatal("captured order cancelled")
	}
	for i, reason := range map[int]string{2: "amount_mismatch", 3: "order_expired"} {
		p := capture
		p.OrderID = fmt.Sprintf("payment-order-%d", i)
		p.TransactionID = fmt.Sprintf("transaction-%d", i)
		p.EventID = p.TransactionID
		if i == 2 {
			p.AmountMinor++
		}
		r, err := db.RecordCapturedPayment(ctx, p)
		if err != nil || r.Disposition != "review" || r.Reason != reason {
			t.Fatalf("review: %+v %v", r, err)
		}
	}
	// If durable delivery registration fails, neither payment nor paid state commits.
	if db.db.Dialector.Name() == "postgres" {
		if err := db.db.Exec("ALTER TABLE prepaid_fulfillment_tasks ADD CONSTRAINT payment_test_failure CHECK(order_id <> 'payment-order-4')").Error; err != nil {
			t.Fatal(err)
		}
		defer db.db.Exec("ALTER TABLE prepaid_fulfillment_tasks DROP CONSTRAINT payment_test_failure")
	} else {
		if err := db.db.Exec("CREATE TRIGGER payment_test_failure BEFORE INSERT ON prepaid_fulfillment_tasks WHEN NEW.order_id = 'payment-order-4' BEGIN SELECT RAISE(ABORT,'test failure'); END").Error; err != nil {
			t.Fatal(err)
		}
		defer db.db.Exec("DROP TRIGGER payment_test_failure")
	}
	failed := capture
	failed.OrderID = "payment-order-4"
	failed.TransactionID = "transaction-4"
	failed.EventID = "event-4"
	if _, err := db.RecordCapturedPayment(ctx, failed); err == nil {
		t.Fatal("expected injected failure")
	}
	var row prepaidOrderRow
	if err := db.db.Table("prepaid_orders").Where("id = ?", failed.OrderID).Take(&row).Error; err != nil || row.Status != "pending" {
		t.Fatal("partial paid state persisted")
	}
	var count int64
	if err := db.db.Table("prepaid_payment_captures").Where("order_id = ?", failed.OrderID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("partial capture persisted")
	}
	if db.db.Dialector.Name() == "postgres" {
		race := capture
		race.OrderID = "payment-order-5"
		race.TransactionID = "transaction-5"
		race.EventID = "event-5"
		var wg sync.WaitGroup
		wg.Add(2)
		var payment commerce.PaymentReceipt
		var paymentErr, cancelErr error
		go func() { defer wg.Done(); payment, paymentErr = db.RecordCapturedPayment(ctx, race) }()
		go func() {
			defer wg.Done()
			_, cancelErr = db.CancelPrepaidOrder(ctx, "order-user", original.OrganizationID, race.OrderID)
		}()
		wg.Wait()
		if paymentErr != nil {
			t.Fatal(paymentErr)
		}
		if payment.Disposition == "applied" {
			if !errors.Is(cancelErr, commerce.ErrOrderNotCancellable) {
				t.Fatal("payment and cancel both won")
			}
		} else if payment.Reason != "order_cancelled" || cancelErr != nil {
			t.Fatalf("race outcome: %+v %v", payment, cancelErr)
		}
	}
	for _, sql := range []string{"UPDATE prepaid_payment_captures SET amount_minor = 1 WHERE id = ?", "DELETE FROM prepaid_payment_captures WHERE id = ?"} {
		if err := db.db.Exec(sql, receipt.ID).Error; err == nil {
			t.Fatal("capture history changed")
		}
	}
	if err := db.db.Table("prepaid_fulfillment_tasks").Where("order_id = ?", capture.OrderID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("missing or duplicate fulfillment")
	}
	if err := db.db.Table("global_server_entitlements").Where("server_id = ?", original.ServerID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("capture bypassed subscription fulfillment")
	}
	testPaidSubscriptions(t, db, original)
}
