// outbox-publisher drains one region's global outbox into its dedicated broker.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/delivery"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/messaging/rabbitmq"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func main() {
	region := flag.String("region", "", "region whose events to publish")
	queue := flag.String("queue", "", "dedicated durable quorum queue")
	deadLetterQueue := flag.String("dead-letter-queue", "", "dedicated durable parking queue")
	deliveryLimit := flag.Int("delivery-limit", 20, "failed deliveries before parking the message")
	batch := flag.Int("batch", 20, "maximum events per poll (1-100)")
	timeout := flag.Duration("publish-timeout", 5*time.Second, "maximum I/O and confirmation time per event")
	lease := flag.Duration("lease", 2*time.Minute, "database claim lease")
	retry := flag.Duration("retry-delay", 10*time.Second, "delay after an unconfirmed publication")
	poll := flag.Duration("poll-interval", time.Second, "interval between bounded batches")
	maxPayload := flag.Int("max-payload-bytes", 65536, "maximum event payload bytes")
	connections := flag.Int("database-connections", 4, "database pool size")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, *region, *queue, *deadLetterQueue, *deliveryLimit, *batch, *timeout, *lease, *retry, *poll, *maxPayload, *connections); err != nil {
		slog.Error("outbox publisher stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, region, queue, deadLetterQueue string, deliveryLimit, batch int, timeout, lease, retry, poll time.Duration, maxPayload, connections int) error {
	if os.Getenv("GAMEPANEL_DATABASE_URL") == "" || poll < time.Millisecond || connections < 1 {
		return fmt.Errorf("database endpoint, positive pool size and poll interval are required")
	}
	publisher, err := rabbitmq.NewPublisher(rabbitmq.Options{URL: os.Getenv("GAMEPANEL_RABBITMQ_URL"), RegionID: region, Queue: queue, DeadLetterQueue: deadLetterQueue, DeliveryLimit: deliveryLimit, Timeout: timeout, MaxPayloadBytes: maxPayload})
	if err != nil {
		return err
	}
	defer publisher.Close()
	db, err := store.OpenConfigured("", os.Getenv("GAMEPANEL_DATABASE_URL"), connections)
	if err != nil {
		return err
	}
	defer db.Close()
	dispatcher := delivery.Dispatcher{Outbox: db, Publisher: publisher, RegionID: region, Batch: batch, Lease: lease, PublishTimeout: timeout, RetryDelay: retry}
	if err := dispatcher.Validate(); err != nil {
		return err
	}
	for {
		if ctx.Err() != nil {
			return nil
		}
		published, err := dispatcher.RunOnce(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			slog.Warn("outbox batch incomplete", "region", region, "error", err)
		}
		if published > 0 {
			slog.Info("outbox messages confirmed", "region", region, "count", published)
		}
		timer := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
