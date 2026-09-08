package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/commerce"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type subscriptionRow struct {
	ID, OrganizationID, ServerID, OrderID, PaymentID, RevisionID string
	PlacementEpoch                                               int64
	Quote, Status                                                string
	CreatedAtMS                                                  int64
}

func (r subscriptionRow) subscription() (commerce.Subscription, error) {
	var quote commerce.Quote
	if json.Unmarshal([]byte(r.Quote), &quote) != nil {
		return commerce.Subscription{}, commerce.ErrSubscriptionUnavailable
	}
	expected, err := commerce.QuotePrepaid(quote.Plan, quote.Periods)
	if err != nil || expected != quote {
		return commerce.Subscription{}, commerce.ErrSubscriptionUnavailable
	}
	return commerce.Subscription{ID: r.ID, OrganizationID: r.OrganizationID, ServerID: r.ServerID, OrderID: r.OrderID, PaymentID: r.PaymentID, RevisionID: r.RevisionID, PlacementEpoch: r.PlacementEpoch, Quote: quote, Status: r.Status, CreatedAtMS: r.CreatedAtMS}, nil
}

// ProvisionPaidSubscription is an internal fulfillment operation. It creates a
// pending service from an applied capture; neither the billing clock nor runtime
// starts here. Existing receipts remain readable after desired-state changes.
func (s *Store) ProvisionPaidSubscription(ctx context.Context, orderID string) (commerce.Subscription, error) {
	if orderID == "" {
		return commerce.Subscription{}, commerce.ErrInvalidOrder
	}
	var result commerce.Subscription
	err := s.Transaction(ctx, func(tx *Store) error {
		var order prepaidOrderRow
		if err := tx.db.Table("prepaid_orders").Where("id = ?", orderID).Take(&order).Error; err != nil {
			return err
		}
		if err := tx.lockWorkspace(ctx, order.OrganizationID); err != nil {
			return err
		}
		var server instances.Server
		q := tx.db.Table("logical_servers").Where("id = ? AND organization_id = ?", order.ServerID, order.OrganizationID)
		if tx.db.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := q.Take(&server).Error; err != nil {
			return err
		}
		q = tx.db.Table("prepaid_orders").Where("id = ?", orderID)
		if tx.db.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := q.Take(&order).Error; err != nil {
			return err
		}
		var task struct {
			OrderID, PaymentID, Status string
			SubscriptionID             *string
		}
		q = tx.db.Table("prepaid_fulfillment_tasks").Where("order_id = ?", orderID)
		if tx.db.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := q.Take(&task).Error; err != nil {
			return err
		}
		if task.SubscriptionID != nil {
			var row subscriptionRow
			if err := tx.db.Table("service_subscriptions").Where("id = ? AND order_id = ? AND payment_id = ?", *task.SubscriptionID, orderID, task.PaymentID).Take(&row).Error; err != nil {
				return err
			}
			var err error
			result, err = row.subscription()
			return err
		}
		if order.Status != "paid" || task.Status != "pending" || server.DesiredState == "deleted" || server.CurrentRevisionID != order.RevisionID {
			return commerce.ErrSubscriptionUnavailable
		}
		var capture paymentCaptureRow
		if err := tx.db.Table("prepaid_payment_captures").Where("id = ? AND order_id = ? AND disposition = ?", task.PaymentID, orderID, "applied").Take(&capture).Error; err != nil {
			return err
		}
		terms, err := order.order()
		if err != nil {
			return err
		}
		if capture.AmountMinor != terms.Quote.AmountMinor || capture.Currency != terms.Quote.Plan.Currency {
			return commerce.ErrSubscriptionUnavailable
		}
		var placement instances.Placement
		if err := tx.db.Table("server_placements").Where("server_id = ?", server.ID).Take(&placement).Error; err != nil {
			return err
		}
		if placement.PlacementEpoch != order.PlacementEpoch || placement.RegionID != terms.Quote.Plan.RegionID {
			return commerce.ErrSubscriptionUnavailable
		}
		var existing subscriptionRow
		err = tx.db.Table("service_subscriptions").Where("server_id = ?", server.ID).Take(&existing).Error
		if err == nil {
			return commerce.ErrSubscriptionConflict
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		now, err := outboxNow(tx.db)
		if err != nil {
			return err
		}
		row := subscriptionRow{ID: uuid.NewString(), OrganizationID: order.OrganizationID, ServerID: server.ID, OrderID: orderID, PaymentID: capture.ID, RevisionID: order.RevisionID, PlacementEpoch: order.PlacementEpoch, Quote: order.Quote, Status: "pending_activation", CreatedAtMS: now}
		if err := tx.db.Table("service_subscriptions").Create(&row).Error; err != nil {
			return err
		}
		changed := tx.db.Table("prepaid_fulfillment_tasks").Where("order_id = ? AND subscription_id IS NULL AND status = ?", orderID, "pending").Update("subscription_id", row.ID)
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return commerce.ErrSubscriptionConflict
		}
		result, err = row.subscription()
		return err
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = commerce.ErrSubscriptionUnavailable
		}
		return commerce.Subscription{}, err
	}
	return result, nil
}

func migrateSQLitePrepaidSubscriptions(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("gamepanel_sqlite_migrations").Where("version = 15").Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		if err := tx.Exec(prepaidSubscriptionsSQL).Error; err != nil {
			return err
		}
		return tx.Exec("INSERT INTO gamepanel_sqlite_migrations(version) VALUES(15)").Error
	})
}
