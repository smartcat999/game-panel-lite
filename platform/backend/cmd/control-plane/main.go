package main

import (
	"log/slog"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/bootstrap"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/controlplane"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/preview"
)

func main() {
	identityModule, workspaceModule := preview.Modules()
	server := bootstrap.HealthServer{
		Name:       "control-plane",
		Addr:       bootstrap.Address(":8080"),
		AppHandler: controlplane.NewHandler(identityModule, workspaceModule),
	}
	if err := server.Run(); err != nil {
		slog.Error("control plane stopped", "error", err)
	}
}
