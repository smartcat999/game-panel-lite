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
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/preview"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regioncontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionexecution"
)

func main() {
	environment := preview.Modules()
	var region regioncontrol.RegionModule = environment.Region
	if databaseURL := os.Getenv("GAMEPANEL_REGION_DATABASE_URL"); databaseURL != "" {
		database, err := sql.Open("pgx", databaseURL)
		if err != nil {
			slog.Error("open Region database", "error", err)
			os.Exit(1)
		}
		defer database.Close()
		if err := database.Ping(); err != nil {
			slog.Error("connect to Region database", "error", err)
			os.Exit(1)
		}
		regionID := contract.RegionID(os.Getenv("GAMEPANEL_REGION_ID"))
		if regionID == "" {
			regionID = "reg_asia_east"
		}
		region = regionexecution.NewPostgres(database, regionID)
	}
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for now := range ticker.C {
			region.Reconcile(context.Background(), now.UTC())
		}
	}()
	server := bootstrap.HealthServer{Name: "region-controller", Addr: bootstrap.Address(":8081"), AppHandler: regioncontrol.NewHandler(environment.Identity, region)}
	if err := server.Run(); err != nil {
		slog.Error("region controller stopped", "error", err)
	}
}
