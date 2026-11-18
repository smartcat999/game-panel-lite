package main

import (
	"context"
	"os"

	"github.com/smartcat999/game-panel-cloud/pkg/apiserver"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/controller"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
)

func main() {
	cmd := config.NewCommand("apiserver", "GamePanel Lite declarative HTTP API Server", func(ctx context.Context, cfg *config.Config) error {
		reg, closeDB, err := repo.Open(ctx, &cfg.Database)
		if err != nil {
			return err
		}
		defer closeDB()

		cacheStore, err := cache.Open(ctx, &cfg.Cache)
		if err != nil {
			return err
		}
		defer func() { _ = cacheStore.Close() }()

		mqBus, err := mq.Open(ctx, &cfg.MQ)
		if err != nil {
			return err
		}
		defer func() { _ = mqBus.Close() }()

		if err := apiserver.Bootstrap(ctx, reg, cfg.Bootstrap.AdminUser, cfg.Bootstrap.AdminEmail, cfg.Bootstrap.AdminPassword); err != nil {
			return err
		}

		if cfg.Server.EnableController {
			mgr := controller.NewManager(reg, &cfg.Controller, mqBus)
			mgrDone := make(chan struct{})
			go func() {
				defer close(mgrDone)
				if cErr := mgr.Run(ctx); cErr != nil {
					log.Error("embedded controller manager exited", "error", cErr)
				}
			}()
			defer func() { <-mgrDone }()
		}

		drivers := runtime.NewDefaultRegistry(cfg.Agent.WorkDir)
		srv := apiserver.NewServer(&cfg.Server, &cfg.Auth, reg, cacheStore, drivers, mqBus)
		return srv.Run(ctx)
	})

	if err := cmd.Execute(); err != nil {
		log.Error("apiserver exited with error", "error", err)
		os.Exit(1)
	}
}
