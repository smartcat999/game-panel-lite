package store

import (
	"context"
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/commerce"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CancelPrepaidOrder does not cancel a running instance or an existing service.
// Repeated cancellation returns the original result, including its reason/time.
func (s *Store) CancelPrepaidOrder(ctx context.Context, actor, organizationID, orderID string) (commerce.Order, error) {
	if actor == "" || organizationID == "" || orderID == "" {
		return commerce.Order{}, commerce.ErrInvalidOrder
	}
	var result commerce.Order
	err := s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockWorkspaceWriter(ctx, organizationID, actor); err != nil {
			return err
		}
		var row prepaidOrderRow
		q := tx.db.Table("prepaid_orders").Where("id = ? AND organization_id = ?", orderID, organizationID)
		if tx.db.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := q.Take(&row).Error; err != nil {
			return err
		}
		if row.Status == "cancelled" {
			var err error
			result, err = row.order()
			return err
		}
		if row.Status != "pending" {
			return commerce.ErrOrderNotCancellable
		}
		now, err := outboxNow(tx.db)
		if err != nil {
			return err
		}
		reason, by := "user", actor
		if row.ExpiresAtMS <= now {
			reason, by = "expired", ""
		}
		if err := cancelPendingOrder(tx.db, &row, now, reason, by); err != nil {
			return err
		}
		result, err = row.order()
		return err
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = commerce.ErrOrderUnavailable
		}
		return commerce.Order{}, err
	}
	return result, nil
}

func cancelPendingOrder(tx *gorm.DB, row *prepaidOrderRow, now int64, reason, actor string) error {
	result := tx.Table("prepaid_orders").Where("id = ? AND status = ?", row.ID, "pending").Updates(map[string]any{"status": "cancelled", "cancel_reason": reason, "cancelled_at_ms": now, "cancelled_by": actor})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return commerce.ErrOrderNotCancellable
	}
	row.Status = "cancelled"
	row.CancelReason = reason
	row.CancelledAtMS = now
	row.CancelledBy = actor
	return nil
}

// ExpirePrepaidOrders uses one bounded transaction and database time. PostgreSQL
// workers skip rows owned by checkout/payment/cancellation rather than waiting.
// This only closes orders; no resources or service rights are released here.
func (s *Store) ExpirePrepaidOrders(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 200 {
		return 0, commerce.ErrInvalidOrder
	}
	count := 0
	err := s.Transaction(ctx, func(tx *Store) error {
		now, err := outboxNow(tx.db)
		if err != nil {
			return err
		}
		var rows []prepaidOrderRow
		q := tx.db.Table("prepaid_orders").Where("status = ? AND expires_at_ms <= ?", "pending", now).Order("expires_at_ms,id").Limit(limit)
		if tx.db.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		if err := q.Find(&rows).Error; err != nil {
			return err
		}
		for i := range rows {
			if err := cancelPendingOrder(tx.db, &rows[i], now, "expired", ""); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

func migrateSQLiteOrderCancellation(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("gamepanel_sqlite_migrations").Where("version = 13").Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		if err := tx.Exec(prepaidOrderCancellationSQL).Error; err != nil {
			return err
		}
		return tx.Exec("INSERT INTO gamepanel_sqlite_migrations(version) VALUES(13)").Error
	})
}
