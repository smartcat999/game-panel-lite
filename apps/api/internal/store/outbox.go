package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/delivery"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ delivery.Outbox = (*Store)(nil)

// Table choices are private constants; callers cannot select arbitrary SQL tables.
type sqlOutbox struct {
	db    *gorm.DB
	table string
}

func (s *Store) BackupRequestOutbox() delivery.Outbox {
	return &sqlOutbox{db: s.db, table: "backup_request_outbox"}
}

func (s *Store) ClaimOutbox(ctx context.Context, region string, limit int, lease time.Duration) ([]delivery.Message, error) {
	return (&sqlOutbox{db: s.db, table: "server_outbox"}).ClaimOutbox(ctx, region, limit, lease)
}

func (s *Store) CompleteOutbox(ctx context.Context, id, token string) error {
	return (&sqlOutbox{db: s.db, table: "server_outbox"}).CompleteOutbox(ctx, id, token)
}

func (s *Store) RetryOutbox(ctx context.Context, id, token string, delay time.Duration) error {
	return (&sqlOutbox{db: s.db, table: "server_outbox"}).RetryOutbox(ctx, id, token, delay)
}

func outboxNow(db *gorm.DB) (int64, error) {
	statement := "SELECT CAST((julianday('now') - 2440587.5) * 86400000 AS INTEGER)"
	if db.Dialector.Name() == "postgres" {
		statement = "SELECT CAST(EXTRACT(EPOCH FROM clock_timestamp()) * 1000 AS BIGINT)"
	}
	var now int64
	err := db.Raw(statement).Scan(&now).Error
	return now, err
}

func (s *sqlOutbox) ClaimOutbox(ctx context.Context, region string, limit int, lease time.Duration) ([]delivery.Message, error) {
	if region == "" || limit < 1 || limit > 100 || lease < time.Millisecond || lease > time.Hour {
		return nil, errors.New("invalid outbox claim")
	}
	var result []delivery.Message
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tx.Dialector.Name() == "sqlite" {
			// Acquire SQLite's writer reservation before reading candidates. A
			// deferred read-to-write upgrade can otherwise fail immediately when
			// concurrent dispatchers have already established read snapshots.
			if err := tx.Table(s.table).Where("1 = 0").UpdateColumn("lease_token", gorm.Expr("lease_token")).Error; err != nil {
				return err
			}
		}
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		var rows []delivery.Message
		query := tx.Table(s.table).Select("id,region_id,payload,attempts").Where("region_id = ? AND published_at_ms = 0 AND next_attempt_ms <= ? AND lease_until_ms <= ?", region, now, now).Order("next_attempt_ms,created_at,id").Limit(limit)
		if tx.Dialector.Name() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		if err := query.Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		now, err = outboxNow(tx)
		if err != nil {
			return err
		}
		token := uuid.NewString()
		ids := make([]string, len(rows))
		for i := range rows {
			ids[i] = rows[i].ID
			rows[i].Token = token
			rows[i].Attempts++
		}
		updated := tx.Table(s.table).Where("id IN ? AND published_at_ms = 0 AND lease_until_ms <= ?", ids, now).Updates(map[string]any{"lease_token": token, "lease_until_ms": now + lease.Milliseconds(), "attempts": gorm.Expr("attempts + 1")})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != int64(len(rows)) {
			return delivery.ErrClaimLost
		}
		result = rows
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *sqlOutbox) CompleteOutbox(ctx context.Context, id, token string) error {
	return s.finishOutbox(ctx, id, token, 0, true)
}

func (s *sqlOutbox) RetryOutbox(ctx context.Context, id, token string, delay time.Duration) error {
	if delay < time.Millisecond || delay > 24*time.Hour {
		return errors.New("invalid outbox retry delay")
	}
	return s.finishOutbox(ctx, id, token, delay, false)
}

func (s *sqlOutbox) finishOutbox(ctx context.Context, id, token string, delay time.Duration, complete bool) error {
	if id == "" || token == "" {
		return delivery.ErrClaimLost
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock before reading the DB clock; a blocked old worker cannot use its
		// pre-lock time to acknowledge a claim which expired while it waited.
		locked := tx.Table(s.table).Where("id = ? AND lease_token = ? AND published_at_ms = 0", id, token).UpdateColumn("lease_token", gorm.Expr("lease_token"))
		if locked.Error != nil {
			return locked.Error
		}
		if locked.RowsAffected != 1 {
			return delivery.ErrClaimLost
		}
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		values := map[string]any{"lease_token": "", "lease_until_ms": 0, "next_attempt_ms": now + delay.Milliseconds()}
		if complete {
			values["published_at_ms"] = now
		}
		updated := tx.Table(s.table).Where("id = ? AND lease_token = ? AND lease_until_ms > ?", id, token, now).Updates(values)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return delivery.ErrClaimLost
		}
		return nil
	})
}
