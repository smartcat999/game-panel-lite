package messaging

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
)

type OutboxScope string

const (
	GlobalOutbox OutboxScope = "global"
	RegionOutbox OutboxScope = "region"
)

type PostgresOutbox struct {
	database *sql.DB
	table    string
}

func NewPostgresOutbox(database *sql.DB, scope OutboxScope) *PostgresOutbox {
	table := "global_outbox"
	if scope == RegionOutbox {
		table = "regional_outbox"
	}
	return &PostgresOutbox{database: database, table: table}
}

func (p *PostgresOutbox) Claim(ctx context.Context, limit int, now time.Time, lease time.Duration) ([]PendingMessage, error) {
	query := fmt.Sprintf(`UPDATE %s SET claimed_until=$2, attempt_count=attempt_count+1
		WHERE id IN (SELECT id FROM %s WHERE published_at IS NULL AND next_attempt_at <= $1 AND (claimed_until IS NULL OR claimed_until <= $1) ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT $3)
		RETURNING id,message_type,idempotency_key,payload,created_at,attempt_count`, p.table, p.table)
	rows, err := p.database.QueryContext(ctx, query, now, now.Add(lease), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []PendingMessage
	for rows.Next() {
		var message PendingMessage
		if err := rows.Scan(&message.ID, &message.MessageType, &message.IdempotencyKey, &message.Payload, &message.CreatedAt, &message.AttemptCount); err != nil {
			return nil, err
		}
		result = append(result, message)
	}
	return result, rows.Err()
}

func (p *PostgresOutbox) MarkPublished(ctx context.Context, ids []contract.EventID, now time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	query := fmt.Sprintf(`UPDATE %s SET published_at=$2,claimed_until=NULL,last_error=NULL WHERE id=ANY($1) AND published_at IS NULL`, p.table)
	_, err := p.database.ExecContext(ctx, query, ids, now)
	return err
}

func (p *PostgresOutbox) MarkFailed(ctx context.Context, ids []contract.EventID, retryAt time.Time, reason string) error {
	if len(ids) == 0 {
		return nil
	}
	query := fmt.Sprintf(`UPDATE %s SET next_attempt_at=$2,claimed_until=NULL,last_error=$3 WHERE id=ANY($1) AND published_at IS NULL`, p.table)
	_, err := p.database.ExecContext(ctx, query, ids, retryAt, reason)
	return err
}
