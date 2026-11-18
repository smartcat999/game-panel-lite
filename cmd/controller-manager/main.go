package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/controller"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
)

func main() {
	cmd := config.NewCommand("controller-manager", "GamePanel Lite MQ-driven declarative controller manager", func(ctx context.Context, cfg *config.Config) error {
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

		mgr := controller.NewManager(reg, &cfg.Controller, mqBus)
		if addr := cfg.Controller.Addr; addr != "" {
			healthSrv := &http.Server{Addr: addr, Handler: mgr.HealthHandler(), ReadHeaderTimeout: 5 * time.Second}
			go func() {
				if err := healthSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					log.Error("controller-manager health server failed", "addr", addr, "error", err)
				}
			}()
			defer func() {
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = healthSrv.Shutdown(shutdownCtx)
			}()
		}
		return mgr.Run(ctx)
	})

	if err := cmd.Execute(); err != nil {
		log.Error("controller-manager exited with error", "error", err)
		os.Exit(1)
	}
}
