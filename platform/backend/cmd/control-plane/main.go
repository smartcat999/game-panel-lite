package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"log/slog"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/accessapi"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/billing"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/billingapi"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/bootstrap"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/controlplane"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliveryapi"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliverycontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliveryworker"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/eventtransport"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/globalproduct"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/preview"
)

func main() {
	environment := preview.Modules()
	var product controlplane.ProductModule = environment.Product
	application := controlplane.NewHandler(environment.Identity, environment.Workspace, product)
	if databaseURL := os.Getenv("GAMEPANEL_GLOBAL_DATABASE_URL"); databaseURL != "" {
		database, err := sql.Open("pgx", databaseURL)
		if err != nil {
			slog.Error("open global database", "error", err)
			os.Exit(1)
		}
		defer database.Close()
		if err := database.Ping(); err != nil {
			slog.Error("connect to global database", "error", err)
			os.Exit(1)
		}
		product = globalproduct.NewPostgres(database)
		application = controlplane.NewHandler(environment.Identity, environment.Workspace, product)
		if natsURL := os.Getenv("GAMEPANEL_NATS_URL"); natsURL != "" {
			fundingKey := decodeKey("GAMEPANEL_FUNDING_SIGNING_KEY_BASE64")
			authorityKey := decodeKey("GAMEPANEL_DELIVERY_AUTHORITY_KEY_BASE64")
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
			worker := deliveryworker.Global{Dispatch: messaging.Dispatcher{Outbox: messaging.NewPostgresOutbox(database, messaging.GlobalOutbox), Publisher: transport}, Consume: transport, Control: deliverycontrol.NewPostgres(database, fundingKey, authorityKey)}
			go runDeliveryWorker("global delivery", func(now time.Time) error { return worker.Tick(context.Background(), now) })
		}
		if os.Getenv("GAMEPANEL_GITHUB_CLIENT_ID") != "" {
			accessServices, err := accessapi.PostgresServices(database, accessapi.Config{
				GitHubClientID:     os.Getenv("GAMEPANEL_GITHUB_CLIENT_ID"),
				GitHubClientSecret: os.Getenv("GAMEPANEL_GITHUB_CLIENT_SECRET"),
				GitHubRedirectURL:  os.Getenv("GAMEPANEL_GITHUB_REDIRECT_URL"),
				TOTPKeyBase64:      os.Getenv("GAMEPANEL_TOTP_KEY_BASE64"),
				SecureCookies:      !strings.EqualFold(os.Getenv("GAMEPANEL_INSECURE_COOKIES"), "true"),
			})
			if err != nil {
				slog.Error("configure access API", "error", err)
				os.Exit(1)
			}
			fundingKey, err := base64.StdEncoding.DecodeString(os.Getenv("GAMEPANEL_FUNDING_SIGNING_KEY_BASE64"))
			if err != nil || len(fundingKey) < 32 {
				slog.Error("configure billing API", "error", "funding signing key must contain at least 32 bytes encoded as base64")
				os.Exit(1)
			}
			accessRoutes := accessapi.Routes(accessServices)
			application = accessapi.WithFallback(accessRoutes, application)
			billingRoutes := billingapi.Routes(billingapi.Services{
				Billing:    billing.New(billing.NewPostgresStore(database), fundingKey),
				Sessions:   accessServices.Sessions,
				Authorizer: accessServices.Authorizer,
				Resolver:   accessServices.Resolver,
			})
			application = billingapi.WithFallback(billingRoutes, application)
			deliveryRoutes := deliveryapi.Routes(deliveryapi.Services{Control: deliverycontrol.NewPostgres(database, fundingKey, nil), Sessions: accessServices.Sessions, Authorizer: accessServices.Authorizer, Resolver: accessServices.Resolver})
			application = deliveryapi.WithFallback(deliveryRoutes, application)
		}
	}
	server := bootstrap.HealthServer{
		Name:       "control-plane",
		Addr:       bootstrap.Address(":8080"),
		AppHandler: application,
	}
	if err := server.Run(); err != nil {
		slog.Error("control plane stopped", "error", err)
	}
}

func decodeKey(name string) []byte {
	key, err := base64.StdEncoding.DecodeString(os.Getenv(name))
	if err != nil || len(key) < 32 {
		slog.Error("configure signing key", "name", name, "error", "key must contain at least 32 bytes encoded as base64")
		os.Exit(1)
	}
	return key
}

func runDeliveryWorker(name string, tick func(time.Time) error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for now := range ticker.C {
		if err := tick(now.UTC()); err != nil {
			slog.Warn(name+" tick failed", "error", err)
		}
	}
}
