package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/messaging/rabbitmq"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regionstatusingress"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func main() {
	region := flag.String("region", "", "trusted source Region for this status queue")
	queue := flag.String("queue", "", "dedicated Region status quorum queue")
	dead := flag.String("dead-letter-queue", "", "dedicated durable parking queue")
	deliveryLimit := flag.Int("delivery-limit", 20, "failed broker deliveries before parking")
	ioTimeout := flag.Duration("io-timeout", 5*time.Second, "connection and acknowledgement timeout")
	workTimeout := flag.Duration("transaction-timeout", 5*time.Second, "maximum projection transaction time")
	retry := flag.Duration("retry-delay", time.Second, "delay between retries")
	maxBytes := flag.Int("max-payload-bytes", 16384, "maximum status payload size")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	options := rabbitmq.Options{URL: os.Getenv("GAMEPANEL_RABBITMQ_URL"), RegionID: *region, Queue: *queue, DeadLetterQueue: *dead, DeliveryLimit: *deliveryLimit, Timeout: *ioTimeout, MaxPayloadBytes: *maxBytes}
	if options.Validate() != nil || *retry < time.Millisecond || *workTimeout <= 0 || os.Getenv("GAMEPANEL_DATABASE_URL") == "" {
		slog.Error("invalid Region status receiver configuration")
		os.Exit(1)
	}
	db, err := store.OpenConfigured("", os.Getenv("GAMEPANEL_DATABASE_URL"), 2)
	if err != nil {
		slog.Error("global database unavailable")
		os.Exit(1)
	}
	defer db.Close()
	ingress := regionstatusingress.Ingress{SourceRegionID: *region, MaxBytes: *maxBytes, Projection: db}
	consumer := rabbitmq.Consumer{Options: options, HandlerTimeout: *workTimeout, RetryDelay: *retry}
	for ctx.Err() == nil {
		acked, err := consumer.Run(ctx, ingress)
		if ctx.Err() != nil {
			return
		}
		slog.Warn("Region status receiver session ended", "region", *region, "acknowledgements_sent", acked, "error", err)
		timer := time.NewTimer(*retry)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
