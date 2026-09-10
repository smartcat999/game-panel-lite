package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/bootstrap"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/nodeworkload"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/productioncatalog"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionaldelivery"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionaltask"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/runtimeprovider/docker"
)

func main() {
	if databaseURL := os.Getenv("GAMEPANEL_REGION_DATABASE_URL"); databaseURL != "" {
		database, err := sql.Open("pgx", databaseURL)
		if err != nil {
			fatal("open Region database", err)
		}
		defer database.Close()
		if err := database.Ping(); err != nil {
			fatal("connect to Region database", err)
		}
		providerKey := decodeKey("GAMEPANEL_PROVIDER_SIGNING_KEY_BASE64")
		registry, providers, err := productioncatalog.Publish(context.Background(), providercontract.NewMemoryStore(), providerKey)
		if err != nil {
			fatal("load signed Provider Releases", err)
		}
		runtimeProvider, err := docker.New(os.Getenv("GAMEPANEL_DOCKER_HOST"), required("GAMEPANEL_NODE_DATA_ROOT"), required("GAMEPANEL_NODE_BACKUP_ROOT"))
		if err != nil {
			fatal("initialize Docker Runtime Provider", err)
		}
		regionID := valueOr("GAMEPANEL_REGION_ID", "reg_asia_east")
		nodeID := valueOr("GAMEPANEL_NODE_ID", "node_um773")
		delivery := regionaldelivery.NewPostgres(database, regionID, decodeKey("GAMEPANEL_DELIVERY_AUTHORITY_KEY_BASE64"))
		tasks := regionaltask.NewPostgres(database, regionID, decodeKey("GAMEPANEL_DELIVERY_AUTHORITY_KEY_BASE64"))
		agent := &nodeworkload.Agent{NodeID: nodeID, WorkerID: nodeID + "-agent", Assignments: delivery, Tasks: tasks, Registry: registry, Providers: providers, Runtime: runtimeProvider, Telemetry: delivery, Management: managementCIDRs()}
		go heartbeat(context.Background(), delivery, regionID, nodeID)
		go func() {
			if err := agent.Run(context.Background()); err != nil {
				slog.Error("node workload loop stopped", "error", err)
			}
		}()
	}
	server := bootstrap.HealthServer{Name: "node-agent", Addr: bootstrap.Address(":8082")}
	if err := server.Run(); err != nil {
		fatal("node agent stopped", err)
	}
}

func heartbeat(ctx context.Context, delivery *regionaldelivery.Postgres, regionID, nodeID string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		now := time.Now().UTC()
		node := regionaldelivery.Node{ID: nodeID, RegionID: regionID, State: "ready", CPUCapacityMilli: integerOr("GAMEPANEL_NODE_CPU_MILLI", 8000), MemoryCapacityMiB: integerOr("GAMEPANEL_NODE_MEMORY_MIB", 16384), DiskCapacityGiB: integerOr("GAMEPANEL_NODE_DISK_GIB", 200), LeaseUntil: now.Add(30 * time.Second), UpdatedAt: now}
		if err := delivery.RegisterNode(ctx, node); err != nil {
			slog.Warn("node heartbeat failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func decodeKey(name string) []byte {
	key, err := base64.StdEncoding.DecodeString(required(name))
	if err != nil || len(key) < 32 {
		fatal("decode signing key", err)
	}
	return key
}

func managementCIDRs() []string {
	value := valueOr("GAMEPANEL_MANAGEMENT_CIDRS", "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,169.254.0.0/16")
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func integerOr(name string, fallback int64) int64 {
	value, err := strconv.ParseInt(os.Getenv(name), 10, 64)
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func valueOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func required(name string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		fatal("missing required environment", nil, "name", name)
	}
	return value
}

func fatal(message string, err error, attributes ...any) {
	if err != nil {
		attributes = append(attributes, "error", err)
	}
	slog.Error(message, attributes...)
	os.Exit(1)
}
