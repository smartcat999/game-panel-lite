package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func main() {
	region := flag.String("region", "", "region bound to this database")
	list := flag.Bool("list", false, "list one page of node configuration")
	after := flag.String("after", "", "exclusive node ID cursor for listing")
	limit := flag.Int("limit", 50, "maximum nodes per page (1-200)")
	version := flag.Int64("expected-version", 0, "0 creates; updates require the currently observed version")
	var config regional.NodeConfiguration
	flag.StringVar(&config.ID, "id", "", "stable regional node ID")
	flag.StringVar(&config.Name, "name", "", "operator-assigned node name")
	flag.StringVar(&config.Architecture, "architecture", "", "canonical runtime architecture")
	flag.Float64Var(&config.CPU, "cpu", 0, "operator-approved CPU capacity")
	flag.Int64Var(&config.MemoryMB, "memory-mb", 0, "operator-approved memory capacity")
	flag.BoolVar(&config.Schedulable, "schedulable", false, "allow consideration for scheduling; does not imply online or free capacity")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := run(ctx, *region, os.Getenv("GAMEPANEL_REGIONAL_DATABASE_URL"), *list, *after, *limit, config, *version); err != nil {
		slog.Error("regional node configuration failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, region, dsn string, list bool, after string, limit int, config regional.NodeConfiguration, version int64) error {
	db, err := store.OpenRegionalPostgres(dsn, region, 2)
	if err != nil {
		return fmt.Errorf("open regional database: %w", err)
	}
	defer db.Close()
	if list {
		nodes, err := db.ListRegionalNodes(ctx, after, limit)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(nodes)
	}
	node, err := db.ConfigureRegionalNode(ctx, config, version)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(node)
}
