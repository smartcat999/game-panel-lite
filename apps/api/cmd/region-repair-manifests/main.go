// region-repair-manifests audits legacy fetched snapshots and optionally queues
// authorized refetch. It does not read global credentials or modify execution.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func main() {
	region := flag.String("region", "", "identity bound to the regional database")
	after := flag.String("after", "", "resume after this operation ID")
	limit := flag.Int("batch-size", 20, "rows per transaction (1-100)")
	apply := flag.Bool("apply", false, "requeue eligible snapshots; default only reports")
	timeout := flag.Duration("timeout", time.Minute, "maximum total repair duration")
	flag.Parse()
	if *limit < 1 || *limit > 100 || *timeout <= 0 {
		slog.Error("invalid repair settings")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	db, err := store.OpenRegionalPostgres(os.Getenv("GAMEPANEL_REGIONAL_DATABASE_URL"), *region, 2)
	if err != nil {
		slog.Error("cannot open regional database")
		os.Exit(1)
	}
	defer db.Close()
	encoder := json.NewEncoder(os.Stdout)
	cursor := *after
	for {
		batch, err := db.RepairMissingAssetManifests(ctx, cursor, *limit, *apply)
		if err != nil {
			slog.Error("manifest repair failed; resume after last reported cursor")
			os.Exit(1)
		}
		if err := encoder.Encode(batch); err != nil {
			slog.Error("cannot write repair report")
			os.Exit(1)
		}
		if batch.Scanned == 0 {
			return
		}
		cursor = batch.Next
	}
}
