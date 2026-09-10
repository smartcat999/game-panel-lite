package messaging

import (
	"errors"
	"testing"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
)

func TestInboxRunsSuccessfulHandlerOnce(t *testing.T) {
	module := New()
	called := 0
	handler := func() error { called++; return nil }
	if handled, err := module.HandleOnce(contract.EventID("evt_once"), time.Now(), handler); err != nil || !handled {
		t.Fatalf("first delivery: handled=%v error=%v", handled, err)
	}
	if handled, err := module.HandleOnce(contract.EventID("evt_once"), time.Now(), handler); err != nil || handled {
		t.Fatalf("redelivery: handled=%v error=%v", handled, err)
	}
	if called != 1 {
		t.Fatalf("handler called %d times, want 1", called)
	}
}

func TestInboxRetriesFailedHandler(t *testing.T) {
	module := New()
	failed := errors.New("temporary")
	if handled, err := module.HandleOnce(contract.EventID("evt_retry"), time.Now(), func() error { return failed }); handled || !errors.Is(err, failed) {
		t.Fatalf("failed delivery: handled=%v error=%v", handled, err)
	}
	if handled, err := module.HandleOnce(contract.EventID("evt_retry"), time.Now(), func() error { return nil }); err != nil || !handled {
		t.Fatalf("retry: handled=%v error=%v", handled, err)
	}
}
