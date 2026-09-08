package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/delivery"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/messaging/rabbitmq"
)

func testBackupRequestDelivery(t *testing.T, db *Store, event backup.Requested) {
	t.Helper()
	ctx := context.Background()
	outbox := db.BackupRequestOutbox()
	if messages, err := outbox.ClaimOutbox(ctx, "other", 1, time.Minute); err != nil || len(messages) != 0 {
		t.Fatal("wrong region claimed backup")
	}
	var wg sync.WaitGroup
	results := make(chan []delivery.Message, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m, e := outbox.ClaimOutbox(ctx, event.RegionID, 1, time.Minute)
			results <- m
			errs <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var claims []delivery.Message
	for messages := range results {
		claims = append(claims, messages...)
	}
	if len(claims) != 1 || claims[0].ID != event.EventID {
		t.Fatalf("unexpected claims %+v", claims)
	}
	old := claims[0]
	if err := db.db.Table("backup_request_outbox").Where("id = ?", old.ID).Update("lease_until_ms", 1).Error; err != nil {
		t.Fatal(err)
	}
	if !errors.Is(outbox.CompleteOutbox(ctx, old.ID, old.Token), delivery.ErrClaimLost) {
		t.Fatal("expired publisher completed")
	}
	dispatch := delivery.Dispatcher{Outbox: outbox, RegionID: event.RegionID, Batch: 1, Lease: time.Minute, PublishTimeout: 5 * time.Second, RetryDelay: time.Hour,
		Publisher: outboxPublisherFunc(func(context.Context, delivery.Message) error { return errors.New("unconfirmed publication") })}
	if n, err := dispatch.RunOnce(ctx); n != 0 || err == nil {
		t.Fatal("unconfirmed backup marked published")
	}
	if messages, err := outbox.ClaimOutbox(ctx, event.RegionID, 1, time.Minute); err != nil || len(messages) != 0 {
		t.Fatal("retry delay ignored")
	}
	if err := db.db.Table("backup_request_outbox").Where("id = ?", old.ID).Update("next_attempt_ms", 0).Error; err != nil {
		t.Fatal(err)
	}
	dispatch.Publisher = outboxPublisherFunc(func(_ context.Context, m delivery.Message) error {
		var got backup.Requested
		if json.Unmarshal([]byte(m.Payload), &got) != nil || got != event || m.ID != event.EventID {
			return errors.New("wrong backup publication")
		}
		return nil
	})
	var channel *amqp.Channel
	queue := ""
	if endpoint := os.Getenv("GAMEPANEL_TEST_RABBITMQ_URL"); endpoint != "" {
		queue = "gamepanel-backup-test-" + uuid.NewString()
		publisher, err := rabbitmq.NewPublisher(rabbitmq.Options{URL: endpoint, RegionID: event.RegionID, Queue: queue, DeadLetterQueue: queue + ".dead", DeliveryLimit: 3, Timeout: 5 * time.Second, MaxPayloadBytes: 4096})
		if err != nil {
			t.Fatal(err)
		}
		defer publisher.Close()
		conn, err := amqp.Dial(endpoint)
		if err != nil {
			t.Fatal("connect test broker")
		}
		defer conn.Close()
		channel, err = conn.Channel()
		if err != nil {
			t.Fatal(err)
		}
		defer channel.Close()
		defer func() {
			_, _ = channel.QueueDelete(queue, false, false, false)
			_, _ = channel.QueueDelete(queue+".dead", false, false, false)
		}()
		dispatch.Publisher = publisher
	}
	if n, err := dispatch.RunOnce(ctx); n != 1 || err != nil {
		t.Fatalf("backup publication n=%d err=%v", n, err)
	}
	if channel != nil {
		message, ok, err := channel.Get(queue, true)
		if err != nil || !ok {
			t.Fatal("confirmed backup missing from broker")
		}
		var got backup.Requested
		if json.Unmarshal(message.Body, &got) != nil || got != event || message.MessageId != event.EventID || message.ContentType != "application/json" {
			t.Fatal("broker changed backup identity")
		}
	}
	if messages, err := outbox.ClaimOutbox(ctx, event.RegionID, 1, time.Minute); err != nil || len(messages) != 0 {
		t.Fatal("confirmed backup remained pending")
	}
	var revision struct {
		PublishedAtMS int64
		LeaseToken    string
	}
	if err := db.db.Table("server_outbox").Where("region_id = ?", event.RegionID).Take(&revision).Error; err != nil || revision.PublishedAtMS != 0 || revision.LeaseToken != "" {
		t.Fatal("backup dispatch changed revision outbox")
	}
}
