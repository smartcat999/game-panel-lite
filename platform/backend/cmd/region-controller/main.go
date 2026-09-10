package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"log/slog"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/bootstrap"
	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliveryworker"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/eventtransport"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/preview"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionaldelivery"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionaltask"
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
		if natsURL := os.Getenv("GAMEPANEL_NATS_URL"); natsURL != "" {
			authorityKey := decodeAuthorityKey()
			transport, err := eventtransport.Connect(natsURL, 5*time.Second)
			if err != nil {
				slog.Error("connect JetStream", "error", err)
				os.Exit(1)
			}
			defer transport.Close()
			if err := transport.EnsureStreams(); err != nil {
				slog.Error("configure JetStream", "error", err)
				os.Exit(1)
			}
			delivery := regionaldelivery.NewPostgres(database, string(regionID), authorityKey)
			worker := deliveryworker.Region{RegionID: string(regionID), Dispatch: messaging.Dispatcher{Outbox: messaging.NewPostgresOutbox(database, messaging.RegionOutbox), Publisher: transport}, Consume: transport, Control: delivery, Tasks: regionaltask.NewPostgres(database, string(regionID), authorityKey)}
			go runDeliveryWorker(worker)
		}
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

func decodeAuthorityKey() []byte {
	key, err := base64.StdEncoding.DecodeString(os.Getenv("GAMEPANEL_DELIVERY_AUTHORITY_KEY_BASE64"))
	if err != nil || len(key) < 32 {
		slog.Error("configure delivery authority", "error", "key must contain at least 32 bytes encoded as base64")
		os.Exit(1)
	}
	return key
}

func runDeliveryWorker(worker deliveryworker.Region) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for now := range ticker.C {
		if err := worker.Tick(context.Background(), now.UTC()); err != nil {
			slog.Warn("regional delivery tick failed", "error", err)
		}
	}
}
