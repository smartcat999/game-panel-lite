package commerce

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/entitlements"
)

type StoreFulfillment interface {
	ClaimPendingFulfillment(ctx context.Context, limit int) ([]string, error)
	FulfillPrepaidSubscription(ctx context.Context, orderID string) (Subscription, entitlements.Record, error)
}

// FulfillmentWorker processes pending fulfillment tasks in bounded batches.
type FulfillmentWorker struct {
	store        StoreFulfillment
	batchSize    int
	pollInterval time.Duration
	logger       *slog.Logger
}

func NewFulfillmentWorker(store StoreFulfillment, batchSize int, pollInterval time.Duration, logger *slog.Logger) *FulfillmentWorker {
	if batchSize <= 0 || batchSize > 200 {
		batchSize = 50
	}
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &FulfillmentWorker{
		store:        store,
		batchSize:    batchSize,
		pollInterval: pollInterval,
		logger:       logger,
	}
}

// RunOnce processes at most one batch of pending fulfillment tasks.
func (w *FulfillmentWorker) RunOnce(ctx context.Context) (int, error) {
	if w.store == nil {
		return 0, errors.New("store required for fulfillment worker")
	}
	pending, err := w.store.ClaimPendingFulfillment(ctx, w.batchSize)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, orderID := range pending {
		if ctx.Err() != nil {
			return processed, ctx.Err()
		}
		sub, ent, err := w.store.FulfillPrepaidSubscription(ctx, orderID)
		if err != nil {
			w.logger.Error("failed to fulfill subscription", "orderID", orderID, "err", err)
			continue
		}
		w.logger.Info("fulfilled subscription", "orderID", orderID, "subscriptionID", sub.ID, "entitlementVersion", ent.Version)
		processed++
	}
	return processed, nil
}

// Start begins background fulfillment processing until the context is cancelled.
func (w *FulfillmentWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = w.RunOnce(ctx)
		}
	}
}
