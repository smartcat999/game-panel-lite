package eventtransport

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
)

func TestJetStreamPublishesDeduplicatesAndAcknowledgesAfterHandling(t *testing.T) {
	serverURL := os.Getenv("GAMEPANEL_NATS_TEST_URL")
	if serverURL == "" {
		t.Skip("GAMEPANEL_NATS_TEST_URL is not set")
	}
	transport, err := Connect(serverURL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	if err := transport.EnsureStreams(); err != nil {
		t.Fatal(err)
	}
	unique := fmt.Sprintf("%d", time.Now().UnixNano())
	subject := "gamepanel.global.delivery-test-" + unique
	messageID := contract.EventID("msg_" + unique)
	if err := transport.Publish(context.Background(), subject, messageID, []byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := transport.Publish(context.Background(), subject, messageID, []byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	handled := 0
	received, err := transport.ConsumeOne(ctx, "GAMEPANEL_GLOBAL", "delivery-test-"+unique, subject, func(payload []byte) error {
		handled++
		if string(payload) != `{"ok":true}` {
			t.Fatalf("payload=%s", payload)
		}
		return nil
	})
	if err != nil || !received || handled != 1 {
		t.Fatalf("received=%v handled=%d err=%v", received, handled, err)
	}
	retrySubject := subject + ".retry"
	retryID := contract.EventID("msg_retry_" + unique)
	if err := transport.Publish(context.Background(), retrySubject, retryID, []byte(`{"retry":true}`)); err != nil {
		t.Fatal(err)
	}
	retryDurable := "delivery-retry-" + unique
	ctxFailure, cancelFailure := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelFailure()
	if received, err := transport.ConsumeOne(ctxFailure, "GAMEPANEL_GLOBAL", retryDurable, retrySubject, func([]byte) error { return errors.New("database unavailable") }); !received || err == nil {
		t.Fatalf("failed handling received=%v err=%v", received, err)
	}
	ctxRetry, cancelRetry := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelRetry()
	if received, err := transport.ConsumeOne(ctxRetry, "GAMEPANEL_GLOBAL", retryDurable, retrySubject, func([]byte) error { return nil }); !received || err != nil {
		t.Fatalf("redelivery received=%v err=%v", received, err)
	}
	permanentSubject := subject + ".permanent"
	if err := transport.Publish(context.Background(), permanentSubject, contract.EventID("msg_permanent_"+unique), []byte(`bad`)); err != nil {
		t.Fatal(err)
	}
	permanentDurable := "delivery-permanent-" + unique
	ctxPermanent, cancelPermanent := context.WithTimeout(context.Background(), time.Second)
	defer cancelPermanent()
	if received, err := transport.ConsumeOne(ctxPermanent, "GAMEPANEL_GLOBAL", permanentDurable, permanentSubject, func([]byte) error { return messaging.Permanent(errors.New("invalid envelope")) }); !received || err == nil {
		t.Fatalf("permanent received=%v err=%v", received, err)
	}
	ctxGone, cancelGone := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelGone()
	if received, err := transport.ConsumeOne(ctxGone, "GAMEPANEL_GLOBAL", permanentDurable, permanentSubject, func([]byte) error { return nil }); received || err != nil {
		t.Fatalf("terminated message redelivered=%v err=%v", received, err)
	}
}
