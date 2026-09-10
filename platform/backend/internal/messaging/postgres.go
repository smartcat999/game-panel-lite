package messaging

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/persistence"
)

type Postgres struct{ db *sql.DB }

func NewPostgres(db *sql.DB) *Postgres { return &Postgres{db: db} }

func (p *Postgres) InsertOutbox(ctx context.Context, query persistence.DBTX, message OutboxMessage) error {
	payload, err := json.Marshal(message.Payload)
	if err != nil {
		return err
	}
	_, err = query.ExecContext(ctx, `INSERT INTO global_outbox (id, message_type, schema_version, idempotency_key, payload, created_at) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (message_type, idempotency_key) DO NOTHING`, message.ID, message.MessageType, message.SchemaVersion, message.IdempotencyKey, payload, message.CreatedAt)
	return err
}

func (p *Postgres) HandleOnce(ctx context.Context, messageID contract.EventID, messageType string, receivedAt time.Time, handler func(persistence.DBTX) error) (bool, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO global_inbox (message_id, message_type, received_at) VALUES ($1, $2, $3) ON CONFLICT (message_id) DO NOTHING`, messageID, messageType, receivedAt)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows == 0 {
		return false, nil
	}
	if err := handler(tx); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE global_inbox SET handled_at = $1 WHERE message_id = $2`, time.Now().UTC(), messageID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
