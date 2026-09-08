package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func main() {
	region := flag.String("region", "", "region bound to the database")
	organization := flag.String("organization", "", "global tenant ID whose node policy is replaced")
	nodes := flag.String("nodes", "", "comma-separated allowed node IDs; empty denies all nodes")
	enabled := flag.Bool("enabled", false, "allow new admissions within the explicit node set")
	version := flag.Int64("expected-version", 0, "0 creates; updates require the current version")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	db, err := store.OpenRegionalPostgres(os.Getenv("GAMEPANEL_REGIONAL_DATABASE_URL"), *region, 2)
	if err != nil {
		slog.Error("open regional database failed")
		os.Exit(1)
	}
	defer db.Close()
	ids := []string{}
	if *nodes != "" {
		ids = strings.Split(*nodes, ",")
	}
	policy, err := db.ConfigureRegionalNodeAccess(ctx, regional.NodeAccessPolicy{OrganizationID: *organization, NodeIDs: ids, Enabled: *enabled}, *version)
	if err != nil {
		slog.Error("configure regional node access failed", "error", err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(policy); err != nil {
		slog.Error("write node policy receipt failed")
		os.Exit(1)
	}
}
