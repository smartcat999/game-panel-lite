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
	"github.com/smartcat999/game-panel-lite/apps/api/internal/backupingress"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/delivery"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/messaging/rabbitmq"
)

func testRegionalBackupIngress(t *testing.T, db *RegionalStore) {
	t.Helper()
	ctx := context.Background()
	e := backup.Requested{SchemaVersion: 1, EventID: "event", OperationID: "operation", BackupID: "backup", OrganizationID: "tenant", ServerID: "server", RegionID: "backup-test", RevisionID: "revision", SpecGeneration: 1, IntentVersion: 1, PlacementEpoch: 1, Scope: "world"}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- db.RecordBackupRequest(ctx, e) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	duplicate := e
	duplicate.EventID = "redelivery"
	if err := db.RecordBackupRequest(ctx, duplicate); err != nil {
		t.Fatal(err)
	}
	for _, changed := range []backup.Requested{
		func() backup.Requested { c := e; c.Scope = "instance"; return c }(),
		func() backup.Requested { c := e; c.EventID = "conflict"; c.OrganizationID = "other"; return c }(),
		func() backup.Requested {
			c := e
			c.EventID = "other-operation"
			c.OperationID = "other-operation"
			return c
		}(),
	} {
		if !errors.Is(db.RecordBackupRequest(ctx, changed), ErrNotificationConflict) {
			t.Fatal("conflicting request accepted")
		}
	}
	wrong := e
	wrong.RegionID = "other"
	if !errors.Is(db.RecordBackupRequest(ctx, wrong), ErrRegionMismatch) {
		t.Fatal("wrong region accepted")
	}
	var count int64
	if err := db.db.Table("regional_backup_inbox").Count(&count).Error; err != nil || count != 2 {
		t.Fatal("conflict left inbox rows")
	}
	var row struct{ Payload, Status, EventID string }
	if err := db.db.Table("regional_backup_requests").Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	var saved backup.Requested
	if json.Unmarshal([]byte(row.Payload), &saved) != nil || saved != e || row.Status != "awaiting_authority" || row.EventID != e.EventID {
		t.Fatal("duplicate changed pending request")
	}
	if err := db.db.Exec("ALTER TABLE regional_backup_requests ADD CONSTRAINT test_reject_backup CHECK(false) NOT VALID").Error; err != nil {
		t.Fatal(err)
	}
	failed := e
	failed.EventID = "failed"
	failed.OperationID = "failed"
	failed.BackupID = "failed"
	if err := db.RecordBackupRequest(ctx, failed); err == nil {
		t.Fatal("request persistence failure ignored")
	}
	if err := db.db.Table("regional_backup_inbox").Where("event_id = ?", failed.EventID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("inbox committed without work")
	}
	if err := db.db.Exec("ALTER TABLE regional_backup_requests DROP CONSTRAINT test_reject_backup").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.RecordBackupRequest(ctx, failed); err != nil {
		t.Fatal(err)
	}
	testBackupBrokerIngress(t, db, e)
}

func testBackupBrokerIngress(t *testing.T, db *RegionalStore, e backup.Requested) {
	t.Helper()
	endpoint := os.Getenv("GAMEPANEL_TEST_RABBITMQ_URL")
	if endpoint == "" {
		return
	}
	queue := "gamepanel-backup-ingress-" + uuid.NewString()
	options := rabbitmq.Options{URL: endpoint, RegionID: e.RegionID, Queue: queue, DeadLetterQueue: queue + ".dead", DeliveryLimit: 3, Timeout: 5 * time.Second, MaxPayloadBytes: 4096}
	p, err := rabbitmq.NewPublisher(options)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	conn, err := amqp.Dial(endpoint)
	if err != nil {
		t.Fatal("connect test broker")
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	defer func() {
		_, _ = ch.QueueDelete(queue, false, false, false)
		_, _ = ch.QueueDelete(options.DeadLetterQueue, false, false, false)
	}()
	e.EventID = "broker-event"
	e.OperationID = "broker-operation"
	e.BackupID = "broker-backup"
	payload, _ := json.Marshal(e)
	for i := 0; i < 2; i++ {
		if err := p.Publish(context.Background(), delivery.Message{ID: e.EventID, RegionID: e.RegionID, Payload: string(payload)}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	consumer := rabbitmq.Consumer{Options: options, HandlerTimeout: time.Second, RetryDelay: 20 * time.Millisecond}
	acked, _ := consumer.Run(ctx, backupingress.Ingress{RegionID: e.RegionID, MaxBytes: 4096, Inbox: db})
	if acked != 2 {
		t.Fatalf("backup acknowledgements %d", acked)
	}
	var count int64
	if err := db.db.Table("regional_backup_requests").Where("operation_id = ? AND status = ?", e.OperationID, "awaiting_authority").Count(&count).Error; err != nil || count != 1 {
		t.Fatal("broker did not persist one pending backup")
	}
	if _, ok, err := ch.Get(queue, true); err != nil || ok {
		t.Fatal("committed request not acknowledged")
	}
}
