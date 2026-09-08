package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/delivery"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/messaging/rabbitmq"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

func testRegionalBrokerIngress(t *testing.T, db *RegionalStore) {
	t.Helper()
	endpoint := os.Getenv("GAMEPANEL_TEST_RABBITMQ_URL")
	if endpoint == "" {
		return
	}
	queue := "gamepanel-test-" + uuid.NewString()
	options := rabbitmq.Options{URL: endpoint, RegionID: "east", Queue: queue, DeadLetterQueue: queue + ".dead", DeliveryLimit: 3, Timeout: 5 * time.Second, MaxPayloadBytes: 2048}
	p, err := rabbitmq.NewPublisher(options)
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
	event := instances.RevisionAvailable{SchemaVersion: 1, EventID: "broker-event", OperationID: "broker-operation", OrganizationID: "tenant", ServerID: "server", RevisionID: "revision", RegionID: "east", PlacementEpoch: 1, SpecGeneration: 1}
	body, _ := json.Marshal(event)
	for i := 0; i < 2; i++ {
		if err := p.Publish(context.Background(), delivery.Message{ID: event.EventID, RegionID: "east", Payload: string(body)}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	consumer := rabbitmq.Consumer{Options: options, HandlerTimeout: time.Second, RetryDelay: 20 * time.Millisecond}
	acked, _ := consumer.Run(ctx, regional.Ingress{RegionID: "east", MaxBytes: 2048, Inbox: db})
	if acked != 2 {
		t.Fatalf("real broker to inbox acknowledgements: %d", acked)
	}
	var count int64
	if err := db.db.Table("regional_inbox").Where("event_id = ?", event.EventID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("duplicate Inbox: %d %v", count, err)
	}
	if err := db.db.Table("regional_revision_tasks").Where("operation_id = ? AND status = 'awaiting_revision'", event.OperationID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("duplicate or authorized task: %d %v", count, err)
	}
	if _, ok, err := ch.Get(queue, true); err != nil || ok {
		t.Fatalf("committed notification unacknowledged: %v %v", ok, err)
	}
}
