package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
)

const MaxDispatchBatch = 100

type PendingMessage struct {
	ID             contract.EventID
	MessageType    string
	IdempotencyKey contract.IdempotencyKey
	Payload        json.RawMessage
	CreatedAt      time.Time
	AttemptCount   int
}

type Envelope struct {
	SchemaVersion  string                  `json:"schemaVersion"`
	MessageID      contract.EventID        `json:"messageId"`
	MessageType    string                  `json:"messageType"`
	OccurredAt     time.Time               `json:"occurredAt"`
	IdempotencyKey contract.IdempotencyKey `json:"idempotencyKey"`
	Payload        json.RawMessage         `json:"payload"`
}

type Outbox interface {
	Claim(context.Context, int, time.Time, time.Duration) ([]PendingMessage, error)
	MarkPublished(context.Context, []contract.EventID, time.Time) error
	MarkFailed(context.Context, []contract.EventID, time.Time, string) error
}

type Publisher interface {
	Publish(context.Context, string, contract.EventID, []byte) error
}

type PermanentError struct{ Err error }

func (e PermanentError) Error() string { return e.Err.Error() }
func (e PermanentError) Unwrap() error { return e.Err }
func Permanent(err error) error        { return PermanentError{Err: err} }

type Dispatcher struct {
	Outbox    Outbox
	Publisher Publisher
	Lease     time.Duration
	Backoff   time.Duration
}

func (d Dispatcher) Dispatch(ctx context.Context, limit int, now time.Time) (int, error) {
	if limit < 1 || limit > MaxDispatchBatch {
		limit = MaxDispatchBatch
	}
	if d.Lease <= 0 {
		d.Lease = 30 * time.Second
	}
	if d.Backoff <= 0 {
		d.Backoff = 5 * time.Second
	}
	messages, err := d.Outbox.Claim(ctx, limit, now.UTC(), d.Lease)
	if err != nil {
		return 0, err
	}
	published := make([]contract.EventID, 0, len(messages))
	failed := make([]contract.EventID, 0, len(messages))
	var publishError error
	for _, message := range messages {
		envelope, err := json.Marshal(Envelope{SchemaVersion: "v1", MessageID: message.ID, MessageType: message.MessageType, OccurredAt: message.CreatedAt, IdempotencyKey: message.IdempotencyKey, Payload: message.Payload})
		if err == nil {
			err = d.Publisher.Publish(ctx, subjectFor(message), message.ID, envelope)
		}
		if err != nil {
			failed = append(failed, message.ID)
			publishError = errors.Join(publishError, err)
			continue
		}
		published = append(published, message.ID)
	}
	if err := d.Outbox.MarkPublished(ctx, published, now.UTC()); err != nil {
		return 0, err
	}
	if err := d.Outbox.MarkFailed(ctx, failed, now.UTC().Add(d.Backoff), errorText(publishError)); err != nil {
		return len(published), err
	}
	return len(published), publishError
}

func subjectFor(message PendingMessage) string {
	var payload struct {
		RegionID string `json:"regionId"`
	}
	_ = json.Unmarshal(message.Payload, &payload)
	if (message.MessageType == "deployment.desired.v1" || message.MessageType == "console.command.requested.v1" || message.MessageType == "backup.requested.v1") && payload.RegionID != "" {
		return fmt.Sprintf("gamepanel.region.%s.%s", payload.RegionID, message.MessageType)
	}
	return "gamepanel.global." + message.MessageType
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	value := err.Error()
	if len(value) > 500 {
		return value[:500]
	}
	return value
}
