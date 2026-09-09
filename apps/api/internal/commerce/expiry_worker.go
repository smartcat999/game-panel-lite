package commerce

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

type StoreExpiry interface {
	ClaimExpiredSubscriptions(ctx context.Context, limit int) ([]string, error)
	ExpireSubscriptionAndStopServer(ctx context.Context, serverID string) error
}

// SubscriptionExpiryWorker periodically checks for expired subscriptions,
// marks them expired, and transitions active servers to stopped.
type SubscriptionExpiryWorker struct {
	store        StoreExpiry
	batchSize    int
	pollInterval time.Duration
	logger       *slog.Logger
}

func NewSubscriptionExpiryWorker(store StoreExpiry, batchSize int, pollInterval time.Duration, logger *slog.Logger) *SubscriptionExpiryWorker {
	if batchSize <= 0 || batchSize > 200 {
		batchSize = 50
	}
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &SubscriptionExpiryWorker{
		store:        store,
		batchSize:    batchSize,
		pollInterval: pollInterval,
		logger:       logger,
	}
}

// RunOnce processes at most one batch of expired subscriptions.
func (w *SubscriptionExpiryWorker) RunOnce(ctx context.Context) (int, error) {
	if w.store == nil {
		return 0, errors.New("store required for subscription expiry worker")
	}
	expiredIDs, err := w.store.ClaimExpiredSubscriptions(ctx, w.batchSize)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, serverID := range expiredIDs {
		if ctx.Err() != nil {
			return processed, ctx.Err()
		}
		if err := w.store.ExpireSubscriptionAndStopServer(ctx, serverID); err != nil {
			w.logger.Error("failed to expire subscription and stop server", "serverID", serverID, "err", err)
			continue
		}
		w.logger.Info("expired subscription and transitioned server to stopped", "serverID", serverID)
		processed++
	}
	return processed, nil
}

// Start begins background expiry polling until context cancellation.
func (w *SubscriptionExpiryWorker) Start(ctx context.Context) {
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
