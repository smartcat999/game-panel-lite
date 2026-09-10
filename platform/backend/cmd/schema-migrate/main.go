package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/smartcat999/game-panel-lite/platform/backend/migrations"
)

func main() {
	databaseURL := os.Getenv("GAMEPANEL_DATABASE_URL")
	scope := os.Getenv("GAMEPANEL_DATABASE_SCOPE")
	if databaseURL == "" || scope == "" {
		slog.Error("GAMEPANEL_DATABASE_URL and GAMEPANEL_DATABASE_SCOPE are required")
		os.Exit(2)
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		slog.Error("connect database", "error", err)
		os.Exit(1)
	}
	if err := migrations.Migrate(ctx, database, scope); err != nil {
		slog.Error("apply migrations", "scope", scope, "error", err)
		os.Exit(1)
	}
	slog.Info("migrations complete", "scope", scope)
}
