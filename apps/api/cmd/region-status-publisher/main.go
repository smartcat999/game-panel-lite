package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/delivery"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/messaging/rabbitmq"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func main() {
	region := flag.String("region", "", "Region owning this publisher")
	queue := flag.String("queue", "", "dedicated global status queue")
	dead := flag.String("dead-letter-queue", "", "dedicated durable parking queue")
	deliveryLimit := flag.Int("delivery-limit", 20, "failed broker deliveries before parking")
	interval := flag.Duration("interval", 30*time.Second, "status observation interval")
	heartbeatFreshness := flag.Duration("heartbeat-freshness", 45*time.Second, "maximum online heartbeat age")
	lease := flag.Duration("lease", time.Minute, "outbox claim lifetime")
	publishTimeout := flag.Duration("publish-timeout", 5*time.Second, "broker confirmation timeout")
	retry := flag.Duration("retry-delay", 10*time.Second, "persistent outbox retry delay")
	maxBytes := flag.Int("max-payload-bytes", 16384, "maximum status payload size")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, *region, *queue, *dead, *deliveryLimit, *interval, *heartbeatFreshness, *lease, *publishTimeout, *retry, *maxBytes); err != nil {
		slog.Error("Region status publisher stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, region, queue, dead string, deliveryLimit int, interval, heartbeatFreshness, lease, publishTimeout, retry time.Duration, maxBytes int) error {
	if strings.TrimSpace(region) == "" || region != strings.TrimSpace(region) || interval < time.Second || interval > time.Hour {
		return errors.New("invalid Region status publisher configuration")
	}
	db, err := store.OpenRegionalPostgres(os.Getenv("GAMEPANEL_REGIONAL_DATABASE_URL"), region, 2)
	if err != nil {
		return errors.New("regional database unavailable")
	}
	defer db.Close()
	publisher, err := rabbitmq.NewPublisher(rabbitmq.Options{URL: os.Getenv("GAMEPANEL_RABBITMQ_URL"), RegionID: region, Queue: queue, DeadLetterQueue: dead, DeliveryLimit: deliveryLimit, Timeout: publishTimeout, MaxPayloadBytes: maxBytes})
	if err != nil {
		return err
	}
	defer publisher.Close()
	dispatcher := delivery.Dispatcher{Outbox: db.RegionStatusOutbox(), Publisher: publisher, RegionID: region, Batch: 10, Lease: lease, PublishTimeout: publishTimeout, RetryDelay: retry}
	if err := dispatcher.Validate(); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := db.CaptureRegionStatus(ctx, heartbeatFreshness); err != nil && ctx.Err() == nil {
			slog.Warn("Region status capture failed", "region", region)
		}
		if _, err := dispatcher.RunOnce(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("Region status publication incomplete", "region", region)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
