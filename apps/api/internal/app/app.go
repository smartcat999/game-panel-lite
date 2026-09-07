package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/config"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/gameconfig"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/gateway"
	apihttp "github.com/smartcat999/game-panel-lite/apps/api/internal/http"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/metrics"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modruntime"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/player"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/dst"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/minecraft"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/palworld"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/runtimecatalog"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
	dockerruntime "github.com/smartcat999/game-panel-lite/apps/api/internal/runtime/docker"
	serverctrl "github.com/smartcat999/game-panel-lite/apps/api/internal/server"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type App struct {
	database  *store.Store
	router    http.Handler
	ctx       context.Context
	cancel    context.CancelFunc
	handler   *apihttp.Handler
	logger    *slog.Logger
	closeOnce sync.Once
}

func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	db, err := store.OpenConfigured(cfg.DBPath, cfg.DatabaseURL, cfg.DBMaxConnections)
	if err != nil {
		return nil, err
	}
	initialized := false
	defer func() {
		if !initialized {
			_ = db.Close()
		}
	}()
	providerCatalog, err := runtimecatalog.Load(cfg.ProviderCatalogPath)
	if err != nil {
		logger.Warn("using built-in provider runtime catalog", "error", err)
	}
	imageRegion := cfg.ImageRegion
	if savedImageRegion, getErr := db.GetSetting(context.Background(), "imageRegion"); getErr == nil && strings.TrimSpace(savedImageRegion) != "" {
		imageRegion = savedImageRegion
	}
	providerCatalog = providerCatalog.WithActiveRegistry(imageRegion)
	registry, err := provider.NewRegistry(
		terraria.NewVanillaProvider(providerCatalog),
		terraria.NewTModLoaderProvider(providerCatalog),
		palworld.NewProvider(providerCatalog),
		dst.NewProvider(providerCatalog),
		minecraft.NewProvider(providerCatalog),
	)
	if err != nil {
		return nil, fmt.Errorf("initialize providers: %w", err)
	}
	adapter, err := dockerruntime.NewAdapter(cfg.DockerHost)
	var runtimeAdapter runtime.Adapter = runtime.NewMockAdapter()
	if err != nil {
		logger.Warn("runtime adapter unavailable", "error", err)
		runtimeAdapter = runtime.NewUnavailableAdapter(err)
	} else {
		runtimeAdapter = adapter
	}
	switchableRuntime := runtime.NewSwitchableAdapter(runtimeAdapter)
	dockerMonitor := runtime.NewDockerMonitor(switchableRuntime)
	dockerMonitor.Refresh(context.Background())
	appCtx, cancel := context.WithCancel(context.Background())
	go dockerMonitor.Start(appCtx, 10*time.Second)
	go player.NewSyncer(db, registry, switchableRuntime, cfg).WithLogger(logger).Start(appCtx, 30*time.Second)
	streamGateway := gateway.NewStreamGateway(logger)
	go func() {
		<-appCtx.Done()
		streamGateway.Close()
	}()

	go serverctrl.NewController(
		db,
		serverctrl.NewRuntimeReconciler(
			serverctrl.NewProviderWorkloadBuilder(registry).WithModPlanner(serverctrl.NewRuntimeModPlanner(cfg.DataDir, db, registry)),
			serverctrl.NewRuntimeAdapterClient(switchableRuntime),
		).WithImageLoader(serverctrl.NewRuntimeImageLoader(cfg.DataDir, switchableRuntime)),
		logger,
	).WithGateway(streamGateway).Start(appCtx)

	dockerFactory := func(host string) (runtime.Adapter, error) {
		return dockerruntime.NewAdapter(host)
	}
	apiMetrics := metrics.NewRegistry()
	handler := apihttp.NewHandler(cfg, logger, db, registry, switchableRuntime, dockerMonitor, dockerFactory, apiMetrics, streamGateway, gameconfig.NewService(registry, db, domain.ProviderTerrariaVanilla), modruntime.NewService(registry, db))
	handler.Start(appCtx)

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	handler.Register(router)
	initialized = true
	return &App{database: db, router: router, ctx: appCtx, cancel: cancel, handler: handler, logger: logger}, nil
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
	a.closeOnce.Do(func() {
		if a.cancel != nil {
			a.cancel()
		}
		if a.handler != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			if err := a.handler.WaitForGameUpdates(ctx); err != nil && a.logger != nil {
				a.logger.Warn("timed out waiting for game update workers", "error", err)
			}
		}
		if a.database != nil {
			_ = a.database.Close()
		}
	})
}
