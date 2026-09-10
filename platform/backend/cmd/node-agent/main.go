package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/bootstrap"
	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/gameprovider/terraria"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/nodeexecution"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionexecution"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/runtimeprovider/docker"
)

func main() {
	if databaseURL, rootPath := os.Getenv("GAMEPANEL_REGION_DATABASE_URL"), os.Getenv("GAMEPANEL_NODE_ROOT"); databaseURL != "" && rootPath != "" {
		database, err := sql.Open("pgx", databaseURL)
		if err != nil {
			slog.Error("open Region database", "error", err)
			os.Exit(1)
		}
		defer database.Close()
		root, err := nodeexecution.NewScopedRoot(rootPath)
		if err != nil {
			slog.Error("open scoped workload root", "error", err)
			os.Exit(1)
		}
		regionID := contract.RegionID(os.Getenv("GAMEPANEL_REGION_ID"))
		if regionID == "" {
			regionID = "reg_asia_east"
		}
		nodeID := contract.NodeID(os.Getenv("GAMEPANEL_NODE_ID"))
		if nodeID == "" {
			nodeID = "nod_local"
		}
		store := regionexecution.NewPostgres(database, regionID)
		runtimeProvider, err := docker.New(os.Getenv("GAMEPANEL_DOCKER_HOST"))
		if err != nil {
			slog.Error("initialize Docker Runtime Provider", "error", err)
			os.Exit(1)
		}
		executor := nodeexecution.Executor{Root: root, Transfer: nodeexecution.HTTPObjectTransfer{}, Results: store, WorkloadResults: store, Games: map[string]nodeexecution.GameProvider{terraria.GameKey: terraria.Provider{}}, Runtime: runtimeProvider}
		agent := nodeexecution.Agent{NodeID: nodeID, BatchSize: 16, ClaimTTL: 30 * time.Second, BackoffBase: 250 * time.Millisecond, BackoffMax: 10 * time.Second, Store: store, Reconcile: executor.Reconcile}
		go func() {
			if err := agent.Run(context.Background(), func() time.Time { return time.Now().UTC() }); err != nil {
				slog.Error("node reconciliation stopped", "error", err)
			}
		}()
	}
	server := bootstrap.HealthServer{Name: "node-agent", Addr: bootstrap.Address(":8082")}
	if err := server.Run(); err != nil {
		slog.Error("node agent stopped", "error", err)
	}
}
