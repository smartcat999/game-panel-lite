package main

import (
	"database/sql"
	"encoding/base64"
	"log/slog"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/accessapi"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/billing"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/billingapi"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/bootstrap"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/controlplane"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/globalproduct"
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
