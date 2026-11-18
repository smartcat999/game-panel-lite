package main

import (
	"context"
	"os"

	"github.com/smartcat999/game-panel-cloud/pkg/agent"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
)

func main() {
	cmd := config.NewCommand("node-agent", "GamePanel Lite compute host node agent", func(ctx context.Context, cfg *config.Config) error {
		drivers := runtime.NewDefaultRegistry(cfg.Agent.WorkDir)
		ag := agent.NewAgent(&cfg.Agent, drivers)
		return ag.Run(ctx)
	})

	if err := cmd.Execute(); err != nil {
		log.Error("node-agent exited with error", "error", err)
		os.Exit(1)
	}
}
