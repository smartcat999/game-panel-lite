package main

import (
	"database/sql"
	"log/slog"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/accessapi"
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
			access, err := accessapi.PostgresRoutes(database, accessapi.Config{
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
			application = accessapi.WithFallback(access, application)
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
