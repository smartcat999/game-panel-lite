package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/config"
	apihttp "github.com/smartcat999/game-panel-lite/apps/api/internal/http"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/player"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/dst"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/minecraft"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/palworld"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
	dockerruntime "github.com/smartcat999/game-panel-lite/apps/api/internal/runtime/docker"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type App struct {
	router http.Handler
	ctx    context.Context
	cancel context.CancelFunc
}

func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	registry := provider.NewRegistry(terraria.NewVanillaProvider(), terraria.NewTModLoaderProvider(), palworld.NewProvider(), dst.NewProvider(), minecraft.NewProvider())
	adapter, err := dockerruntime.NewAdapter(cfg.DockerHost)
	var runtimeAdapter runtime.Adapter = runtime.NewMockAdapter()
	if err != nil {
		logger.Warn("falling back to mock runtime adapter", "error", err)
	} else {
		runtimeAdapter = adapter
	}
	switchableRuntime := runtime.NewSwitchableAdapter(runtimeAdapter)
	dockerMonitor := runtime.NewDockerMonitor(switchableRuntime)
	dockerMonitor.Refresh(context.Background())
	appCtx, cancel := context.WithCancel(context.Background())
	go dockerMonitor.Start(appCtx, 10*time.Second)
	go player.NewSyncer(db, registry, switchableRuntime, cfg).WithLogger(logger).Start(appCtx, 30*time.Second)

	dockerFactory := func(host string) (runtime.Adapter, error) {
		return dockerruntime.NewAdapter(host)
	}
	handler := apihttp.NewHandler(cfg, logger, db, registry, switchableRuntime, dockerMonitor, dockerFactory)

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	handler.Register(router)
	return &App{router: router, ctx: appCtx, cancel: cancel}, nil
}

func (a *App) Routes() http.Handler {
	return a.router
}

func (a *App) Context() context.Context {
	if a.ctx == nil {
		return context.Background()
	}
	return a.ctx
}

func (a *App) Close() {
	if a.cancel != nil {
		a.cancel()
	}
}
