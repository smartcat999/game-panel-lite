package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/backupingress"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/messaging/rabbitmq"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func main() {
	region := flag.String("region", "", "region owning this receiver")
	stream := flag.String("stream", "revisions", "event stream: revisions or backup-requests; use separate queues")
	queue := flag.String("queue", "", "regional notification quorum queue")
	dead := flag.String("dead-letter-queue", "", "dedicated durable parking queue")
	limit := flag.Int("delivery-limit", 20, "failed broker deliveries before parking")
	timeout := flag.Duration("io-timeout", 5*time.Second, "connection setup and acknowledgement timeout")
	workTimeout := flag.Duration("transaction-timeout", 5*time.Second, "maximum notification transaction time")
	retry := flag.Duration("retry-delay", time.Second, "delay between handler or connection retries")
	maxBytes := flag.Int("max-payload-bytes", 65536, "maximum accepted notification size")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	options := rabbitmq.Options{URL: os.Getenv("GAMEPANEL_RABBITMQ_URL"), RegionID: *region, Queue: *queue, DeadLetterQueue: *dead, DeliveryLimit: *limit, Timeout: *timeout, MaxPayloadBytes: *maxBytes}
	if err := options.Validate(); err != nil || *retry < time.Millisecond || *workTimeout <= 0 || (*stream != "revisions" && *stream != "backup-requests") {
		slog.Error("invalid receiver configuration")
		os.Exit(1)
	}
	db, err := store.OpenRegionalPostgres(os.Getenv("GAMEPANEL_REGIONAL_DATABASE_URL"), *region, 2)
	if err != nil {
		slog.Error("regional database unavailable", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	var ingress rabbitmq.Handler = regional.Ingress{RegionID: *region, MaxBytes: *maxBytes, Inbox: db}
	if *stream == "backup-requests" {
		ingress = backupingress.Ingress{RegionID: *region, MaxBytes: *maxBytes, Inbox: db}
	}
	consumer := rabbitmq.Consumer{Options: options, HandlerTimeout: *workTimeout, RetryDelay: *retry}
	for ctx.Err() == nil {
		acked, err := consumer.Run(ctx, ingress)
		if ctx.Err() != nil {
			return
		}
		slog.Warn("regional receiver session ended", "region", *region, "acknowledgements_sent", acked, "error", err)
		timer := time.NewTimer(*retry)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
