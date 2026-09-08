package commerce

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/entitlements"
)

type mockFulfillmentStore struct {
	claimed     []string
	fulfilled   map[string]bool
	failOrderID string
}

func (m *mockFulfillmentStore) ClaimPendingFulfillment(ctx context.Context, limit int) ([]string, error) {
	var result []string
	for _, id := range m.claimed {
		if !m.fulfilled[id] {
			result = append(result, id)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *mockFulfillmentStore) FulfillPrepaidSubscription(ctx context.Context, orderID string) (Subscription, entitlements.Record, error) {
	if orderID == m.failOrderID {
		return Subscription{}, entitlements.Record{}, errors.New("simulated error")
	}
	m.fulfilled[orderID] = true
	return Subscription{ID: "sub-" + orderID, Status: "active"}, entitlements.Record{Version: 1}, nil
}

func TestFulfillmentWorkerRunOnce(t *testing.T) {
	store := &mockFulfillmentStore{
		claimed:     []string{"ord-1", "ord-2", "ord-err"},
		fulfilled:   make(map[string]bool),
		failOrderID: "ord-err",
	}
	worker := NewFulfillmentWorker(store, 10, time.Second, slog.Default())

	processed, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if processed != 2 {
		t.Fatalf("expected 2 processed, got %d", processed)
	}
	if !store.fulfilled["ord-1"] || !store.fulfilled["ord-2"] {
		t.Fatalf("expected ord-1 and ord-2 fulfilled: %+v", store.fulfilled)
	}
	if store.fulfilled["ord-err"] {
		t.Fatalf("ord-err should not be fulfilled")
	}
}
