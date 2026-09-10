package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"log/slog"
	"os"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/productionseed"
)

func main() {
	global := open("GAMEPANEL_GLOBAL_DATABASE_URL")
	defer global.Close()
	region := open("GAMEPANEL_REGION_DATABASE_URL")
	defer region.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	userID, err := productionseed.SeedGlobal(ctx, global, productionseed.GlobalConfig{AdminUsername: required("GAMEPANEL_BOOTSTRAP_USERNAME"), AdminName: required("GAMEPANEL_BOOTSTRAP_DISPLAY_NAME"), AdminPassword: required("GAMEPANEL_BOOTSTRAP_PASSWORD"), FundingKey: key("GAMEPANEL_FUNDING_SIGNING_KEY_BASE64"), ProviderKey: key("GAMEPANEL_PROVIDER_SIGNING_KEY_BASE64")})
	if err != nil {
		slog.Error("seed global database", "error", err)
		os.Exit(1)
	}
	start, err := strconv.Atoi(defaultValue("GAMEPANEL_ENDPOINT_PORT_START", "32000"))
	if err != nil {
		slog.Error("parse endpoint port start", "error", err)
		os.Exit(2)
	}
	end, err := strconv.Atoi(defaultValue("GAMEPANEL_ENDPOINT_PORT_END", "32999"))
	if err != nil {
		slog.Error("parse endpoint port end", "error", err)
		os.Exit(2)
	}
	if err := productionseed.SeedRegion(ctx, region, productionseed.RegionConfig{PublicAddress: required("GAMEPANEL_PUBLIC_ADDRESS"), PortStart: start, PortEnd: end}); err != nil {
		slog.Error("seed Region database", "error", err)
		os.Exit(1)
	}
	slog.Info("production seed complete", "summary", productionseed.Summary(userID), "temporaryCredential", "expires after 24 hours and must be changed")
}

func open(name string) *sql.DB {
	database, err := sql.Open("pgx", required(name))
	if err != nil {
		slog.Error("open database", "name", name, "error", err)
		os.Exit(1)
	}
	if err := database.Ping(); err != nil {
		slog.Error("connect database", "name", name, "error", err)
		os.Exit(1)
	}
	return database
}

func key(name string) []byte {
	decoded, err := base64.StdEncoding.DecodeString(required(name))
	if err != nil || len(decoded) < 32 {
		slog.Error("decode signing key", "name", name)
		os.Exit(2)
	}
	return decoded
}

func required(name string) string {
	value := os.Getenv(name)
	if value == "" {
		slog.Error("required environment variable is empty", "name", name)
		os.Exit(2)
	}
	return value
}

func defaultValue(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
