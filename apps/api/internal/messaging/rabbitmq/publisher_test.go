package rabbitmq

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/delivery"
)

func TestPublishCanceledHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
		close(accepted)
	}()
	p, err := NewPublisher(Options{URL: "amqp://guest:guest@" + listener.Addr().String() + "/", RegionID: "east", Queue: "test", Timeout: 100 * time.Millisecond, MaxPayloadBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	started := time.Now()
	err = p.Publish(context.Background(), delivery.Message{ID: "event", RegionID: "east", Payload: "{}"})
	if err == nil || time.Since(started) > 2*time.Second {
		t.Fatalf("stalled AMQP handshake did not terminate: %v", err)
	}
	_ = listener.Close()
	if conn := <-accepted; conn != nil {
		_ = conn.Close()
	}
	if p.connection != nil || p.raw != nil {
		t.Fatal("failed connection retained")
	}
}

func TestRabbitMQConfirmedPublication(t *testing.T) {
	endpoint := os.Getenv("GAMEPANEL_TEST_RABBITMQ_URL")
	if endpoint == "" {
		t.Skip("set GAMEPANEL_TEST_RABBITMQ_URL for a real broker")
	}
	queue := "gamepanel-test-" + uuid.NewString()
	p, err := NewPublisher(Options{URL: endpoint, RegionID: "east", Queue: queue, Timeout: 5 * time.Second, MaxPayloadBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	admin, err := amqp.Dial(endpoint)
	if err != nil {
		t.Fatal("connect test RabbitMQ")
	}
	defer admin.Close()
	channel, err := admin.Channel()
	if err != nil {
		t.Fatal(err)
	}
	defer channel.Close()
	defer func() { _, _ = channel.QueueDelete(queue, false, false, false) }()
	message := delivery.Message{ID: "stable-event", RegionID: "east", Payload: `{"schemaVersion":1}`}
	ctx := context.Background()
	if err := p.Publish(ctx, message); err != nil {
		t.Fatal(err)
	}
	firstConnection := p.connection
	item, ok, err := channel.Get(queue, false)
	if err != nil || !ok || item.MessageId != message.ID || string(item.Body) != message.Payload || item.DeliveryMode != amqp.Persistent {
		t.Fatalf("confirmed delivery mismatch: %+v %v %v", item, ok, err)
	}
	if err := item.Ack(false); err != nil {
		t.Fatal(err)
	}
	message.RegionID = "west"
	if err := p.Publish(ctx, message); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("cross-region routing accepted: %v", err)
	}
	message.RegionID = "east"
	if err := p.Publish(ctx, message); err != nil {
		t.Fatal(err)
	}
	if p.connection != firstConnection {
		t.Fatal("healthy connection not reused")
	}
	item, ok, err = channel.Get(queue, false)
	if err != nil || !ok {
		t.Fatalf("second delivery: %v %v", ok, err)
	}
	if err := item.Ack(false); err != nil {
		t.Fatal(err)
	}
	// Delete the destination after setup. RabbitMQ ACKs unroutable mandatory
	// publishes too; basic.return must take precedence over that confirmation.
	if _, err := channel.QueueDelete(queue, false, false, false); err != nil {
		t.Fatal(err)
	}
	if err := p.Publish(ctx, message); !errors.Is(err, ErrUnroutable) {
		t.Fatalf("unroutable ACK treated as delivered: %v", err)
	}
	if p.connection != nil {
		t.Fatal("uncertain channel retained")
	}
	if err := p.Publish(ctx, message); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	item, ok, err = channel.Get(queue, false)
	if err != nil || !ok || item.MessageId != message.ID {
		t.Fatalf("reconnected delivery: %v %v", ok, err)
	}
	if err := item.Ack(false); err != nil {
		t.Fatal(err)
	}
	_ = p.raw.Close()
	// Whether closure has reached the client reader is nondeterministic. The
	// first attempt may fail; the next must establish a usable connection.
	if err := p.Publish(ctx, message); err != nil {
		if err := p.Publish(ctx, message); err != nil {
			t.Fatalf("closed connection recovery: %v", err)
		}
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.Publish(ctx, message); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("closed publisher reused: %v", err)
	}
}
