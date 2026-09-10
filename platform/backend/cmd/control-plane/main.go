package main

import (
	"database/sql"
	"log/slog"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/bootstrap"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/controlplane"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/globalproduct"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/preview"
)

func main() {
	environment := preview.Modules()
	var product controlplane.ProductModule = environment.Product
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
	}
	server := bootstrap.HealthServer{
		Name:       "control-plane",
		Addr:       bootstrap.Address(":8080"),
		AppHandler: controlplane.NewHandler(environment.Identity, environment.Workspace, product),
	}
	if err := server.Run(); err != nil {
		slog.Error("control plane stopped", "error", err)
	}
}
