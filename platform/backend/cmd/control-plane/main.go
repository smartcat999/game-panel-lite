package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"log/slog"
	"net/http"
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
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instanceaction"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instanceconfiguration"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instanceobservability"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instanceprovisioning"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/preview"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/productioncatalog"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/workspaceapi"
)

func main() {
	var application http.Handler
	databaseURL := os.Getenv("GAMEPANEL_GLOBAL_DATABASE_URL")
	if databaseURL == "" {
		environment := preview.Modules()
		application = controlplane.NewHandler(environment.Identity, environment.Workspace, environment.Product)
	} else {
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
		if os.Getenv("GAMEPANEL_TOTP_KEY_BASE64") == "" {
			slog.Error("configure production access", "error", "GAMEPANEL_TOTP_KEY_BASE64 is required")
			os.Exit(1)
		}
		application = http.NotFoundHandler()
		fundingKey := decodeKey("GAMEPANEL_FUNDING_SIGNING_KEY_BASE64")
		authorityKey := decodeKey("GAMEPANEL_DELIVERY_AUTHORITY_KEY_BASE64")
		providerRegistry, _, err := productioncatalog.Publish(context.Background(), providercontract.NewPostgresStore(database), decodeKey("GAMEPANEL_PROVIDER_SIGNING_KEY_BASE64"))
		if err != nil {
			slog.Error("publish Provider Releases", "error", err)
			os.Exit(1)
		}
		delivery := deliverycontrol.NewPostgres(database, fundingKey, authorityKey)
		actions := instanceaction.NewPostgres(database, delivery, providerRegistry, authorityKey)
		observability := instanceobservability.NewPostgres(database, providerRegistry)
		if natsURL := os.Getenv("GAMEPANEL_NATS_URL"); natsURL != "" {
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
			worker := deliveryworker.Global{Dispatch: messaging.Dispatcher{Outbox: messaging.NewPostgresOutbox(database, messaging.GlobalOutbox), Publisher: transport}, Consume: transport, Control: delivery, Actions: actions, Telemetry: observability}
			go runDeliveryWorker("global delivery", func(now time.Time) error { return worker.Tick(context.Background(), now) })
		}
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
		application = accessapi.WithFallback(accessapi.Routes(accessServices), application)
		billingModule := billing.New(billing.NewPostgresStore(database), fundingKey)
		billingRoutes := billingapi.Routes(billingapi.Services{Billing: billingModule, Sessions: accessServices.Sessions, Authorizer: accessServices.Authorizer, Resolver: accessServices.Resolver})
		application = billingapi.WithFallback(billingRoutes, application)
		application = deliveryapi.WithFallback(deliveryapi.Routes(deliveryapi.Services{Control: delivery, Sessions: accessServices.Sessions, Authorizer: accessServices.Authorizer, Resolver: accessServices.Resolver}), application)
		configuration := instanceconfiguration.New(providerRegistry, delivery, instanceconfiguration.NewPostgresStore(database))
		workspaceRoutes := workspaceapi.Routes(workspaceapi.Services{Database: database, Sessions: accessServices.Sessions, Authorizer: accessServices.Authorizer, Resolver: accessServices.Resolver, Providers: providerRegistry, Billing: billingModule, Delivery: delivery, Provisioning: instanceprovisioning.New(providerRegistry, delivery), Configuration: configuration, Actions: actions, Observability: observability})
		application = workspaceapi.WithFallback(workspaceRoutes, application)
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
