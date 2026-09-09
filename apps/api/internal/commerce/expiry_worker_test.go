package commerce

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

type mockExpiryStore struct {
	claimed     []string
	expired     map[string]bool
	failServerID string
}

func (m *mockExpiryStore) ClaimExpiredSubscriptions(ctx context.Context, limit int) ([]string, error) {
	var result []string
	for _, id := range m.claimed {
		if !m.expired[id] {
			result = append(result, id)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *mockExpiryStore) ExpireSubscriptionAndStopServer(ctx context.Context, serverID string) error {
	if serverID == m.failServerID {
		return errors.New("simulated error")
	}
	m.expired[serverID] = true
	return nil
}

func TestExpiryWorkerRunOnce(t *testing.T) {
	store := &mockExpiryStore{
		claimed:      []string{"srv-1", "srv-2", "srv-err"},
		expired:      make(map[string]bool),
		failServerID: "srv-err",
	}
	worker := NewSubscriptionExpiryWorker(store, 10, time.Second, slog.Default())

	processed, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if processed != 2 {
		t.Fatalf("expected 2 processed, got %d", processed)
	}
	if !store.expired["srv-1"] || !store.expired["srv-2"] {
		t.Fatalf("expected srv-1 and srv-2 expired: %+v", store.expired)
	}
	if store.expired["srv-err"] {
		t.Fatalf("srv-err should not be marked expired")
	}
}
