package main

import (
	"log/slog"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/bootstrap"
)

func main() {
	server := bootstrap.HealthServer{Name: "node-agent", Addr: bootstrap.Address(":8082")}
	if err := server.Run(); err != nil {
		slog.Error("node agent stopped", "error", err)
	}
}
