package main

import (
	"log/slog"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/bootstrap"
)

func main() {
	server := bootstrap.HealthServer{Name: "control-plane", Addr: bootstrap.Address(":8080")}
	if err := server.Run(); err != nil {
		slog.Error("control plane stopped", "error", err)
	}
}
