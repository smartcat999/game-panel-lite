// Package delivery owns durable publication orchestration, not broker clients.
package delivery

import (
	"context"
	"errors"
	"time"
)

var ErrClaimLost = errors.New("outbox claim is no longer current")

type Message struct {
	ID, RegionID, Token, Type string
	Payload                   string
	Attempts                  int64
}

type Outbox interface {
	ClaimOutbox(context.Context, string, int, time.Duration) ([]Message, error)
	CompleteOutbox(context.Context, string, string) error
	RetryOutbox(context.Context, string, string, time.Duration) error
}

// Publish must return nil only after durable broker confirmation, including
// routing acceptance. A timeout is uncertain and must be retried with the same ID.
type Publisher interface {
	Publish(context.Context, Message) error
}

type Dispatcher struct {
	Outbox                            Outbox
	Publisher                         Publisher
	RegionID                          string
	Batch                             int
	Lease, PublishTimeout, RetryDelay time.Duration
}

// Validate rejects settings that cannot fit the publication timeout budget.
func (d Dispatcher) Validate() error {
	if d.Outbox == nil || d.Publisher == nil || d.RegionID == "" || d.Batch < 1 || d.Batch > 100 ||
		d.PublishTimeout <= 0 || d.Lease <= 0 || d.Lease > time.Hour || d.PublishTimeout >= d.Lease/time.Duration(d.Batch) ||
		d.RetryDelay < time.Millisecond || d.RetryDelay > 24*time.Hour {
		return errors.New("invalid outbox dispatcher settings")
	}
	return nil
}

// RunOnce bounds work to one regional batch. Broker I/O never holds a database
// transaction. The caller controls poll cadence, concurrency and shutdown.
func (d Dispatcher) RunOnce(ctx context.Context) (int, error) {
	if err := d.Validate(); err != nil {
		return 0, err
	}
	messages, err := d.Outbox.ClaimOutbox(ctx, d.RegionID, d.Batch, d.Lease)
	if err != nil {
		return 0, err
	}
	completed := 0
	var failures []error
	for _, message := range messages {
		if err := ctx.Err(); err != nil {
			return completed, errors.Join(append(failures, err)...)
		}
		publishCtx, cancel := context.WithTimeout(ctx, d.PublishTimeout)
		err := d.Publisher.Publish(publishCtx, message)
		cancel()
		if err != nil {
			// Do not persist broker errors: connection errors may contain secrets.
			if retryErr := d.Outbox.RetryOutbox(ctx, message.ID, message.Token, d.RetryDelay); retryErr != nil {
				failures = append(failures, retryErr)
			}
			failures = append(failures, errors.New("outbox publication unconfirmed"))
			continue
		}
		if err := d.Outbox.CompleteOutbox(ctx, message.ID, message.Token); err != nil {
			failures = append(failures, err)
			continue
		}
		completed++
	}
	return completed, errors.Join(failures...)
}
