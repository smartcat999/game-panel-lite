package main

import (
	"log/slog"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/bootstrap"
)

func main() {
	server := bootstrap.HealthServer{Name: "region-controller", Addr: bootstrap.Address(":8081")}
	if err := server.Run(); err != nil {
		slog.Error("region controller stopped", "error", err)
	}
}
