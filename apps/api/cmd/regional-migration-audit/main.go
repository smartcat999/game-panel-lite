package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func main() {
	timeout := flag.Duration("timeout", time.Minute, "maximum read-only ownership audit duration")
	flag.Parse()
	if *timeout <= 0 {
		fmt.Fprintln(os.Stderr, "audit timeout must be positive")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	report, err := store.AuditPostgresRegionalMigration(ctx, os.Getenv("GAMEPANEL_DATABASE_URL"))
	if err != nil {
		// Do not echo database errors that could carry connection information.
		fmt.Fprintln(os.Stderr, "ownership audit failed; check database access and matching schema")
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, "could not write audit report")
		os.Exit(1)
	}
	if len(report.Issues) > 0 {
		os.Exit(2)
	}
}
