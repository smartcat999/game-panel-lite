package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func main() {
	region := flag.String("region", "", "identity bound permanently to this regional database")
	timeout := flag.Duration("timeout", time.Minute, "maximum migration duration")
	flag.Parse()
	if *timeout <= 0 {
		slog.Error("migration timeout must be positive")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	if err := store.MigrateRegionalPostgres(ctx, os.Getenv("GAMEPANEL_REGIONAL_DATABASE_URL"), *region); err != nil {
		slog.Error("regional migration failed", "error", err)
		os.Exit(1)
	}
	slog.Info("regional database migrated", "region", *region)
}
