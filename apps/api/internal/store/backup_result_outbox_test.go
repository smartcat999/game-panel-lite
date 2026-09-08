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

func testBackupResultDelivery(t *testing.T, db *RegionalStore, event backup.ArchiveUploaded) {
	t.Helper()
	ctx := context.Background()
	outbox := db.BackupResultOutbox()
	if _, err := outbox.ClaimOutbox(ctx, "other", 1, time.Minute); !errors.Is(err, ErrRegionMismatch) {
		t.Fatal("regional publisher adopted another identity")
	}
	var wg sync.WaitGroup
	results := make(chan []delivery.Message, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m, e := outbox.ClaimOutbox(ctx, db.regionID, 1, time.Minute)
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
	for m := range results {
		claims = append(claims, m...)
	}
	if len(claims) != 1 || claims[0].ID != event.EventID || claims[0].RegionID != db.regionID {
		t.Fatalf("result claims %+v", claims)
	}
	old := claims[0]
	if err := db.db.Table("regional_backup_result_outbox").Where("id = ?", old.ID).Update("lease_until_ms", 1).Error; err != nil {
		t.Fatal(err)
	}
	if !errors.Is(outbox.CompleteOutbox(ctx, old.ID, old.Token), delivery.ErrClaimLost) {
		t.Fatal("expired result publisher completed")
	}
	d := delivery.Dispatcher{Outbox: outbox, RegionID: db.regionID, Batch: 1, Lease: time.Minute, PublishTimeout: 5 * time.Second, RetryDelay: time.Hour,
		Publisher: outboxPublisherFunc(func(context.Context, delivery.Message) error { return errors.New("unconfirmed result") })}
	if n, err := d.RunOnce(ctx); n != 0 || err == nil {
		t.Fatal("unconfirmed result marked sent")
	}
	if m, err := outbox.ClaimOutbox(ctx, db.regionID, 1, time.Minute); len(m) != 0 || err != nil {
		t.Fatal("result retry delay ignored")
	}
	if err := db.db.Table("regional_backup_result_outbox").Where("id = ?", old.ID).Update("next_attempt_ms", 0).Error; err != nil {
		t.Fatal(err)
	}
	d.Publisher = outboxPublisherFunc(func(_ context.Context, m delivery.Message) error {
		var got backup.ArchiveUploaded
		if json.Unmarshal([]byte(m.Payload), &got) != nil || got != event || m.RegionID != db.regionID {
			return errors.New("wrong result identity")
		}
		return nil
	})
	var ch *amqp.Channel
	queue := ""
	if endpoint := os.Getenv("GAMEPANEL_TEST_RABBITMQ_URL"); endpoint != "" {
		queue = "gamepanel-backup-results-" + uuid.NewString()
		p, err := rabbitmq.NewPublisher(rabbitmq.Options{URL: endpoint, RegionID: db.regionID, Queue: queue, DeadLetterQueue: queue + ".dead", DeliveryLimit: 3, Timeout: 5 * time.Second, MaxPayloadBytes: 32768})
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()
		conn, err := amqp.Dial(endpoint)
		if err != nil {
			t.Fatal("connect test broker")
		}
		defer conn.Close()
		ch, err = conn.Channel()
		if err != nil {
			t.Fatal(err)
		}
		defer ch.Close()
		defer func() {
			_, _ = ch.QueueDelete(queue, false, false, false)
			_, _ = ch.QueueDelete(queue+".dead", false, false, false)
		}()
		d.Publisher = p
	}
	if n, err := d.RunOnce(ctx); n != 1 || err != nil {
		t.Fatalf("result publication %d %v", n, err)
	}
	if ch != nil {
		message, ok, err := ch.Get(queue, true)
		if err != nil || !ok {
			t.Fatal("confirmed result missing")
		}
		var got backup.ArchiveUploaded
		if json.Unmarshal(message.Body, &got) != nil || got != event || message.MessageId != event.EventID || message.ContentType != "application/json" {
			t.Fatal("broker result differs")
		}
	}
	var row struct {
		PublishedAtMS, Attempts int64
		LeaseToken              string
	}
	if err := db.db.Table("regional_backup_result_outbox").Where("id = ?", event.EventID).Take(&row).Error; err != nil || row.PublishedAtMS <= 0 || row.Attempts != 3 || row.LeaseToken != "" {
		t.Fatalf("result publication state %+v %v", row, err)
	}
	if m, err := outbox.ClaimOutbox(ctx, db.regionID, 1, time.Minute); len(m) != 0 || err != nil {
		t.Fatal("confirmed result reclaimed")
	}
}
