package rabbitmq

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/delivery"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

type handlerFunc func(context.Context, regional.Notification) error

func (f handlerFunc) Handle(ctx context.Context, n regional.Notification) error { return f(ctx, n) }

func TestRabbitMQConsumerAcknowledgements(t *testing.T) {
	endpoint := os.Getenv("GAMEPANEL_TEST_RABBITMQ_URL")
	if endpoint == "" {
		t.Skip("set GAMEPANEL_TEST_RABBITMQ_URL for a real broker")
	}
	queue := "gamepanel-test-" + uuid.NewString()
	options := Options{URL: endpoint, RegionID: "east", Queue: queue, DeadLetterQueue: queue + ".dead", DeliveryLimit: 3, Timeout: 5 * time.Second, MaxPayloadBytes: 1024}
	p, err := NewPublisher(options)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	admin, err := amqp.Dial(endpoint)
	if err != nil {
		t.Fatal("connect test broker")
	}
	defer admin.Close()
	ch, err := admin.Channel()
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	defer func() {
		_, _ = ch.QueueDelete(queue, false, false, false)
		_, _ = ch.QueueDelete(options.DeadLetterQueue, false, false, false)
	}()
	c := Consumer{Options: options, HandlerTimeout: time.Second, RetryDelay: 20 * time.Millisecond}
	publish := func(id string) {
		t.Helper()
		if err := p.Publish(context.Background(), delivery.Message{ID: id, RegionID: "east", Type: "deployment.status.observed", Payload: "{}"}); err != nil {
			t.Fatal(err)
		}
	}
	publish("retry")
	calls := 0
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	n, _ := c.Run(ctx, handlerFunc(func(_ context.Context, message regional.Notification) error {
		calls++
		if message.ID != "retry" || message.Type != "deployment.status.observed" {
			t.Error("wrong event")
		}
		if calls == 1 {
			return errors.New("database unavailable")
		}
		return nil
	}))
	cancel()
	if n != 1 || calls != 2 {
		t.Fatalf("transient failure ACK handling: acknowledgements=%d attempts=%d", n, calls)
	}
	if _, ok, err := ch.Get(queue, true); err != nil || ok {
		t.Fatalf("acknowledged message retained: %v %v", ok, err)
	}
	publish("invalid")
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	n, _ = c.Run(ctx, handlerFunc(func(context.Context, regional.Notification) error { return regional.ErrInvalidNotification }))
	cancel()
	if n != 0 {
		t.Fatal("invalid message ACKed as accepted")
	}
	parked, ok, err := ch.Get(options.DeadLetterQueue, false)
	if err != nil || !ok || parked.MessageId != "invalid" {
		t.Fatalf("invalid message not parked: %v %v", ok, err)
	}
	if err := parked.Ack(false); err != nil {
		t.Fatal(err)
	}
	publish("commit-before-cancel")
	ctx, cancel = context.WithCancel(context.Background())
	n, _ = c.Run(ctx, handlerFunc(func(context.Context, regional.Notification) error { cancel(); return nil }))
	cancel()
	if n != 0 {
		t.Fatal("cancelled processing sent ACK")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		message, ok, err := ch.Get(queue, false)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			if message.MessageId != "commit-before-cancel" || !message.Redelivered {
				t.Fatal("unconfirmed notification did not redeliver")
			}
			if err := message.Ack(false); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("unacknowledged notification lost on cancellation")
}
