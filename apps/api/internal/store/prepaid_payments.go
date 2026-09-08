package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/commerce"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type paymentCaptureRow struct {
	ID, Provider, MerchantID, TransactionID, OrderID string
	AmountMinor                                      int64
	Currency                                         string
	PaidAtMS                                         int64
	Disposition, Reason                              string
	RecordedAtMS                                     int64
}

func (r paymentCaptureRow) receipt() commerce.PaymentReceipt {
	return commerce.PaymentReceipt{ID: r.ID, OrderID: r.OrderID, Disposition: r.Disposition, Reason: r.Reason, RecordedAtMS: r.RecordedAtMS}
}

type paymentEventRow struct{ Provider, MerchantID, EventID, RequestHash, PaymentID string }

// RecordCapturedPayment is a trusted internal adapter boundary. Never bind it
// directly to a user JSON body. Pending fulfillment is durable, not executed here.
func (s *Store) RecordCapturedPayment(ctx context.Context, p commerce.CapturedPayment) (commerce.PaymentReceipt, error) {
	if p.Validate() != nil {
		return commerce.PaymentReceipt{}, commerce.ErrInvalidPayment
	}
	encoded, _ := json.Marshal(p)
	sum := sha256.Sum256(encoded)
	hash := hex.EncodeToString(sum[:])
	var receipt commerce.PaymentReceipt
	err := s.Transaction(ctx, func(tx *Store) error {
		var order prepaidOrderRow
		if err := tx.db.Table("prepaid_orders").Where("id = ?", p.OrderID).Take(&order).Error; err != nil {
			return err
		}
		// Match cancellation's workspace -> order locking order.
		if err := tx.lockWorkspace(ctx, order.OrganizationID); err != nil {
			return err
		}
		q := tx.db.Table("prepaid_orders").Where("id = ?", p.OrderID)
		if tx.db.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := q.Take(&order).Error; err != nil {
			return err
		}
		var event paymentEventRow
		err := tx.db.Table("prepaid_payment_events").Where("provider = ? AND merchant_id = ? AND event_id = ?", p.Provider, p.MerchantID, p.EventID).Take(&event).Error
		if err == nil {
			if event.RequestHash != hash {
				return commerce.ErrPaymentConflict
			}
			var capture paymentCaptureRow
			if err := tx.db.Table("prepaid_payment_captures").Where("id = ?", event.PaymentID).Take(&capture).Error; err != nil {
				return err
			}
			receipt = capture.receipt()
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var capture paymentCaptureRow
		err = tx.db.Table("prepaid_payment_captures").Where("provider = ? AND merchant_id = ? AND transaction_id = ?", p.Provider, p.MerchantID, p.TransactionID).Take(&capture).Error
		if err == nil {
			if capture.OrderID != p.OrderID || capture.AmountMinor != p.AmountMinor || capture.Currency != p.Currency || capture.PaidAtMS != p.PaidAtMS {
				return commerce.ErrPaymentConflict
			}
		} else {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			terms, err := order.order()
			if err != nil {
				return err
			}
			now, err := outboxNow(tx.db)
			if err != nil {
				return err
			}
			reason := ""
			switch {
			case order.Status != "pending":
				reason = "order_" + order.Status
			case order.ExpiresAtMS <= now:
				if err := cancelPendingOrder(tx.db, &order, now, "expired", ""); err != nil {
					return err
				}
				reason = "order_expired"
			case p.AmountMinor != terms.Quote.AmountMinor || p.Currency != terms.Quote.Plan.Currency:
				reason = "amount_mismatch"
			case p.PaidAtMS < order.CreatedAtMS || p.PaidAtMS > now:
				reason = "capture_time_mismatch"
			}
			disposition := "applied"
			if reason != "" {
				disposition = "review"
			}
			capture = paymentCaptureRow{ID: uuid.NewString(), Provider: p.Provider, MerchantID: p.MerchantID, TransactionID: p.TransactionID, OrderID: p.OrderID, AmountMinor: p.AmountMinor, Currency: p.Currency, PaidAtMS: p.PaidAtMS, Disposition: disposition, Reason: reason, RecordedAtMS: now}
			if err := tx.db.Table("prepaid_payment_captures").Create(&capture).Error; err != nil {
				return err
			}
			if disposition == "applied" {
				changed := tx.db.Table("prepaid_orders").Where("id = ? AND status = ?", p.OrderID, "pending").Update("status", "paid")
				if changed.Error != nil {
					return changed.Error
				}
				if changed.RowsAffected != 1 {
					return commerce.ErrPaymentConflict
				}
				task := struct{ OrderID, PaymentID, Status string }{p.OrderID, capture.ID, "pending"}
				if err := tx.db.Table("prepaid_fulfillment_tasks").Create(&task).Error; err != nil {
					return err
				}
			}
		}
		event = paymentEventRow{Provider: p.Provider, MerchantID: p.MerchantID, EventID: p.EventID, RequestHash: hash, PaymentID: capture.ID}
		if err := tx.db.Table("prepaid_payment_events").Create(&event).Error; err != nil {
			return err
		}
		receipt = capture.receipt()
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = commerce.ErrOrderUnavailable
		}
		return commerce.PaymentReceipt{}, err
	}
	return receipt, nil
}

func migrateSQLitePrepaidPayments(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("gamepanel_sqlite_migrations").Where("version = 14").Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		if err := tx.Exec(prepaidPaymentsSQL).Error; err != nil {
			return err
		}
		for _, table := range []string{"prepaid_payment_captures", "prepaid_payment_events"} {
			for _, action := range []string{"UPDATE", "DELETE"} {
				if err := tx.Exec("CREATE TRIGGER " + table + "_no_" + action + " BEFORE " + action + " ON " + table + " BEGIN SELECT RAISE(ABORT, 'payment history is immutable'); END").Error; err != nil {
					return err
				}
			}
		}
		return tx.Exec("INSERT INTO gamepanel_sqlite_migrations(version) VALUES(14)").Error
	})
}
