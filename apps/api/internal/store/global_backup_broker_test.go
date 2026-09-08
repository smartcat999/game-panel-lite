package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/backupingress"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/delivery"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/messaging/rabbitmq"
	"gorm.io/gorm"
)

func testGlobalBackupBroker(t *testing.T, db *Store, template backup.ArchiveUploaded, request backup.Requested) {
	t.Helper()
	endpoint := os.Getenv("GAMEPANEL_TEST_RABBITMQ_URL")
	if endpoint == "" {
		return
	}
	task, err := db.RequestGlobalBackup(context.Background(), "global-owner", backup.Request{OrganizationID: request.OrganizationID, ServerID: request.ServerID, Scope: request.Scope, IdempotencyKey: "broker-result"})
	if err != nil {
		t.Fatal(err)
	}
	event := template
	event.EventID = uuid.NewString()
	event.Plan.OperationID = task.Request.OperationID
	event.Plan.RequestEventID = task.Request.EventID
	event.Plan.Asset.AssetID = uuid.NewString()
	event.Receipt.Asset = event.Plan.Asset
	queue := "gamepanel-global-backup-" + uuid.NewString()
	options := rabbitmq.Options{URL: endpoint, RegionID: request.RegionID, Queue: queue, DeadLetterQueue: queue + ".dead", DeliveryLimit: 3, Timeout: 5 * time.Second, MaxPayloadBytes: 32768}
	publisher, err := rabbitmq.NewPublisher(options)
	if err != nil {
		t.Fatal(err)
	}
	defer publisher.Close()
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
	payload, _ := json.Marshal(event)
	for i := 0; i < 2; i++ {
		if err := publisher.Publish(context.Background(), delivery.Message{ID: event.EventID, RegionID: request.RegionID, Payload: string(payload)}); err != nil {
			t.Fatal(err)
		}
	}
	// Fail the first result insert inside the real transaction. Consumer must
	// retry without ACK, and rollback must leave no asset or successful task.
	attempts := 0
	callback := "test:fail_first_global_result"
	if err := db.db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "global_backup_results" {
			attempts++
			if attempts == 1 {
				tx.AddError(errors.New("injected first result failure"))
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.db.Callback().Create().Remove(callback)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	consumer := rabbitmq.Consumer{Options: options, HandlerTimeout: time.Second, RetryDelay: 20 * time.Millisecond}
	acked, _ := consumer.Run(ctx, backupingress.ResultIngress{SourceRegionID: request.RegionID, MaxBytes: 32768, Inbox: db})
	if acked != 2 || attempts != 2 {
		t.Fatalf("result retry/ACK mismatch: acked=%d attempts=%d", acked, attempts)
	}
	var count int64
	if err := db.db.Table("global_backup_results").Where("operation_id = ? AND disposition = ?", task.Request.OperationID, "published").Count(&count).Error; err != nil || count != 1 {
		t.Fatal("missing unique committed result")
	}
	var row globalBackupTaskRow
	if err := db.db.Table("global_backup_tasks").Where("id = ?", task.Request.BackupID).Take(&row).Error; err != nil || row.Status != "succeeded" {
		t.Fatal("broker result did not complete task")
	}
	if _, ok, err := ch.Get(queue, true); err != nil || ok {
		t.Fatal("committed result not acknowledged")
	}
}
