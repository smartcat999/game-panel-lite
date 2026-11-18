package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/client"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker"
)

func main() {
	cmd := config.NewCommand("registry-worker", "GamePanel Lite stateless Harbor-style artifact registry worker", func(ctx context.Context, cfg *config.Config) error {
		cli, err := client.NewRegistryWorkerFromConfig(cfg)
		if err != nil {
			return err
		}
		worker := registryworker.New(cfg, cli)
		if addr := cfg.Registry.WorkerAddr; addr != "" {
			healthSrv := &http.Server{
				Addr:              addr,
				Handler:           worker.HealthHandler(),
				ReadHeaderTimeout: 5 * time.Second,
			}
			go func() {
				if err := healthSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					log.Error("registry-worker health server failed", "addr", addr, "error", err)
				}
			}()
			defer func() {
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = healthSrv.Shutdown(shutdownCtx)
			}()
		}
		return worker.Run(ctx)
	})

	if err := cmd.Execute(); err != nil {
		log.Error("registry-worker exited with error", "error", err)
		os.Exit(1)
	}
}
