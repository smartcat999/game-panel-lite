package messaging

import (
	"context"
	"errors"
	"testing"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
)

type dispatcherOutbox struct {
	messages  []PendingMessage
	published []contract.EventID
	failed    []contract.EventID
}

func (s *dispatcherOutbox) Claim(context.Context, int, time.Time, time.Duration) ([]PendingMessage, error) {
	return append([]PendingMessage(nil), s.messages...), nil
}
func (s *dispatcherOutbox) MarkPublished(_ context.Context, ids []contract.EventID, _ time.Time) error {
	s.published = append(s.published, ids...)
	return nil
}
func (s *dispatcherOutbox) MarkFailed(_ context.Context, ids []contract.EventID, _ time.Time, _ string) error {
	s.failed = append(s.failed, ids...)
	return nil
}

type failingPublisher struct{ fail bool }

func (p failingPublisher) Publish(context.Context, string, contract.EventID, []byte) error {
	if p.fail {
		return errors.New("broker unavailable")
	}
	return nil
}

func TestDispatcherRetainsFailedMessagesForRetry(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	outbox := &dispatcherOutbox{messages: []PendingMessage{{ID: "msg_one", MessageType: "deployment.desired.v1", IdempotencyKey: "delivery-one", Payload: []byte(`{"regionId":"reg_asia"}`), CreatedAt: now}}}
	dispatcher := Dispatcher{Outbox: outbox, Publisher: failingPublisher{fail: true}}
	if count, err := dispatcher.Dispatch(context.Background(), 101, now); count != 0 || err == nil || len(outbox.failed) != 1 || len(outbox.published) != 0 {
		t.Fatalf("count=%d err=%v failed=%v published=%v", count, err, outbox.failed, outbox.published)
	}
	outbox.failed = nil
	dispatcher.Publisher = failingPublisher{}
	if count, err := dispatcher.Dispatch(context.Background(), 1, now); count != 1 || err != nil || len(outbox.published) != 1 {
		t.Fatalf("retry count=%d err=%v published=%v", count, err, outbox.published)
	}
}

func TestRequestedInstanceWorkRoutesToOwningRegion(t *testing.T) {
	for _, messageType := range []string{"deployment.desired.v1", "console.command.requested.v1", "backup.requested.v1"} {
		message := PendingMessage{MessageType: messageType, Payload: []byte(`{"regionId":"reg_asia"}`)}
		if subject := subjectFor(message); subject != "gamepanel.region.reg_asia."+messageType {
			t.Fatalf("type=%s subject=%s", messageType, subject)
		}
	}
	if subject := subjectFor(PendingMessage{MessageType: "backup.observed.v1", Payload: []byte(`{"regionId":"reg_asia"}`)}); subject != "gamepanel.global.backup.observed.v1" {
		t.Fatalf("global observation subject=%s", subject)
	}
}
