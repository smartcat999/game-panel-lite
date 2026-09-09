package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/commerce"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/entitlements"
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

// FulfillPrepaidSubscription fulfills a pending task: it activates the service_subscription,
// grants the versioned global_server_entitlements record, and marks the fulfillment task completed.
// If the logical server was stopped by the user, it remains stopped.
func (s *Store) FulfillPrepaidSubscription(ctx context.Context, orderID string) (commerce.Subscription, entitlements.Record, error) {
	if orderID == "" {
		return commerce.Subscription{}, entitlements.Record{}, commerce.ErrInvalidOrder
	}
	var sub commerce.Subscription
	var ent entitlements.Record
	err := s.Transaction(ctx, func(tx *Store) error {
		var task struct {
			OrderID        string
			PaymentID      string
			Status         string
			SubscriptionID *string
		}
		q := tx.db.Table("prepaid_fulfillment_tasks").Where("order_id = ?", orderID)
		if tx.db.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := q.Take(&task).Error; err != nil {
			return err
		}
		if task.Status == "completed" {
			var sRow subscriptionRow
			if task.SubscriptionID != nil {
				if err := tx.db.Table("service_subscriptions").Where("id = ?", *task.SubscriptionID).Take(&sRow).Error; err != nil {
					return err
				}
				var err error
				sub, err = sRow.subscription()
				if err != nil {
					return err
				}
				if err := tx.db.Table("global_server_entitlements").Where("server_id = ?", sub.ServerID).Take(&ent).Error; err != nil {
					return err
				}
				return nil
			}
			return commerce.ErrSubscriptionUnavailable
		}
		if task.SubscriptionID == nil {
			pSub, err := tx.ProvisionPaidSubscription(ctx, orderID)
			if err != nil {
				return err
			}
			task.SubscriptionID = &pSub.ID
		}
		var sRow subscriptionRow
		if err := tx.db.Table("service_subscriptions").Where("id = ?", *task.SubscriptionID).Take(&sRow).Error; err != nil {
			return err
		}
		var err error
		sub, err = sRow.subscription()
		if err != nil {
			return err
		}
		if err := tx.lockWorkspace(ctx, sub.OrganizationID); err != nil {
			return err
		}
		var server instances.Server
		sq := tx.db.Table("logical_servers").Where("id = ? AND organization_id = ?", sub.ServerID, sub.OrganizationID)
		if tx.db.Dialector.Name() == "postgres" {
			sq = sq.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := sq.Take(&server).Error; err != nil {
			return err
		}
		if server.DesiredState == "deleted" {
			return commerce.ErrSubscriptionUnavailable
		}
		now, err := outboxNow(tx.db)
		if err != nil {
			return err
		}
		durationSeconds := sub.Quote.Plan.PeriodSeconds * sub.Quote.Periods
		endsAtMS := now + (durationSeconds * 1000)

		var current entitlements.Record
		err = tx.db.Table("global_server_entitlements").Where("server_id = ?", sub.ServerID).Take(&current).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		expectedVersion := int64(0)
		if err == nil {
			expectedVersion = current.Version
			if current.Status == "active" && current.EndsAtMS > now {
				endsAtMS = current.EndsAtMS + (durationSeconds * 1000)
			}
		}
		ent = entitlements.Record{
			Policy: entitlements.Policy{
				OrganizationID: sub.OrganizationID,
				ServerID:       sub.ServerID,
				CPU:            sub.Quote.Plan.CPU,
				MemoryMB:       sub.Quote.Plan.MemoryMB,
				StartsAtMS:     now,
				EndsAtMS:       endsAtMS,
				Status:         "active",
			},
			Version:    expectedVersion + 1,
			SourceKind: "prepaid_subscription",
			SourceID:   sub.ID,
		}
		if err := tx.db.Table("global_server_entitlements").Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "server_id"}}, UpdateAll: true}).Create(&ent).Error; err != nil {
			return err
		}
		if err := tx.db.Table("service_subscriptions").Where("id = ?", sub.ID).Update("status", "active").Error; err != nil {
			return err
		}
		sub.Status = "active"
		if err := tx.db.Table("prepaid_fulfillment_tasks").Where("order_id = ?", orderID).Update("status", "completed").Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return commerce.Subscription{}, entitlements.Record{}, err
	}
	return sub, ent, nil
}

// ClaimPendingFulfillment returns up to limit orderIDs of pending fulfillment tasks.
func (s *Store) ClaimPendingFulfillment(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	var ids []string
	err := s.Transaction(ctx, func(tx *Store) error {
		q := tx.db.Table("prepaid_fulfillment_tasks").Select("order_id").Where("status = ?", "pending").Order("order_id ASC").Limit(limit)
		if tx.db.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		return q.Pluck("order_id", &ids).Error
	})
	return ids, err
}

// ListOrganizationSubscriptions lists all service subscriptions for a given tenant organization.
func (s *Store) ListOrganizationSubscriptions(ctx context.Context, organizationID string) ([]commerce.Subscription, error) {
	if organizationID == "" {
		return nil, commerce.ErrSubscriptionUnavailable
	}
	var subscriptions []commerce.Subscription
	err := s.readSnapshot(ctx, func(tx *Store) error {
		var rows []subscriptionRow
		if err := tx.db.Table("service_subscriptions").Where("organization_id = ?", organizationID).Order("created_at_ms DESC").Find(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			sub, err := r.subscription()
			if err == nil {
				subscriptions = append(subscriptions, sub)
			}
		}
		return nil
	})
	return subscriptions, err
}

func migrateSQLitePrepaidSubscriptions(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("gamepanel_sqlite_migrations").Where("version = 15").Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			if err := tx.Exec(prepaidSubscriptionsSQL).Error; err != nil {
				return err
			}
			if err := tx.Exec("INSERT INTO gamepanel_sqlite_migrations(version) VALUES(15)").Error; err != nil {
				return err
			}
		}
		var count16 int64
		if err := tx.Table("gamepanel_sqlite_migrations").Where("version = 16").Count(&count16).Error; err != nil {
			return err
		}
		if count16 == 0 {
			migration16 := `
CREATE TABLE IF NOT EXISTS global_server_entitlements_v16 (
 server_id text PRIMARY KEY REFERENCES logical_servers(id),
 organization_id text NOT NULL,
 version bigint NOT NULL CHECK(version > 0),
 cpu double precision NOT NULL CHECK(cpu > 0 AND cpu <= 1.7976931348623157e308),
 memory_mb bigint NOT NULL CHECK(memory_mb > 0),
 starts_at_ms bigint NOT NULL CHECK(starts_at_ms >= 0),
 ends_at_ms bigint NOT NULL CHECK(ends_at_ms > starts_at_ms),
 status text NOT NULL CHECK(status IN ('active','suspended','revoked')),
 source_kind text NOT NULL CHECK(source_kind IN ('operator','subscription','prepaid_subscription')),
 source_id text NOT NULL UNIQUE
);
INSERT OR IGNORE INTO global_server_entitlements_v16 SELECT * FROM global_server_entitlements;
DROP TABLE global_server_entitlements;
ALTER TABLE global_server_entitlements_v16 RENAME TO global_server_entitlements;
`
			if err := tx.Exec(migration16).Error; err != nil {
				return err
			}
			if err := tx.Exec("INSERT INTO gamepanel_sqlite_migrations(version) VALUES(16)").Error; err != nil {
				return err
			}
		}
		var count17 int64
		if err := tx.Table("gamepanel_sqlite_migrations").Where("version = 17").Count(&count17).Error; err != nil {
			return err
		}
		if count17 == 0 {
			migration17 := `
CREATE TABLE IF NOT EXISTS service_subscriptions_v17 (
 id text PRIMARY KEY,
 organization_id text NOT NULL,
 server_id text NOT NULL UNIQUE REFERENCES logical_servers(id),
 order_id text NOT NULL UNIQUE REFERENCES prepaid_orders(id),
 payment_id text NOT NULL UNIQUE REFERENCES prepaid_payment_captures(id),
 revision_id text NOT NULL REFERENCES server_revisions(id),
 placement_epoch bigint NOT NULL CHECK(placement_epoch > 0),
 quote text NOT NULL,
 status text NOT NULL CHECK(status IN ('pending_activation','active','expired','cancelled')),
 created_at_ms bigint NOT NULL
);
INSERT OR IGNORE INTO service_subscriptions_v17 SELECT * FROM service_subscriptions;
DROP TABLE service_subscriptions;
ALTER TABLE service_subscriptions_v17 RENAME TO service_subscriptions;
`
			if err := tx.Exec(migration17).Error; err != nil {
				return err
			}
			if err := tx.Exec("INSERT INTO gamepanel_sqlite_migrations(version) VALUES(17)").Error; err != nil {
				return err
			}
		}
		return nil
	})
}
