package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/config"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/metrics"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/monitoring"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type Handler struct {
	cfg            config.Config
	logger         *slog.Logger
	store          *store.Store
	provider       *provider.Registry
	runtime        *runtime.SwitchableAdapter
	dockerMonitor  *runtime.DockerMonitor
	runtimeFactory func(string) (runtime.Adapter, error)
	apiMetrics     *metrics.Registry

	runtimeImageJobsMu sync.Mutex
	runtimeImageJobs   map[string]domain.RuntimeImageStatus
}

type resourceLimitPayload struct {
	CPULimitCores float64 `json:"cpuLimitCores,omitempty"`
	MemoryLimitMB int     `json:"memoryLimitMb,omitempty"`
}

const staleLifecyclePendingAfter = 10 * time.Minute

func NewHandler(
	cfg config.Config,
	logger *slog.Logger,
	store *store.Store,
	providers *provider.Registry,
	adapter *runtime.SwitchableAdapter,
	dockerMonitor *runtime.DockerMonitor,
	runtimeFactory func(string) (runtime.Adapter, error),
	apiMetrics *metrics.Registry,
) *Handler {
	if apiMetrics == nil {
		apiMetrics = metrics.NewRegistry()
	}
	return &Handler{
		cfg:              cfg,
		logger:           logger,
		store:            store,
		provider:         providers,
		runtime:          adapter,
		dockerMonitor:    dockerMonitor,
		runtimeFactory:   runtimeFactory,
		apiMetrics:       apiMetrics,
		runtimeImageJobs: map[string]domain.RuntimeImageStatus{},
	}
}

func (h *Handler) Register(r chi.Router) {
	r.Use(h.cors)
	r.Use(h.apiMetrics.Middleware)
	r.Get("/healthz", h.health)
	r.With(h.optionalAuth).Get("/api/auth/bootstrap", h.authBootstrap)
	r.Post("/api/auth/setup", h.setupAdmin)
	r.Post("/api/auth/login", h.login)
	r.Post("/api/auth/logout", h.logout)
	r.Get("/metrics", h.prometheusMetrics)
	r.Get("/api/public/servers/{token}", h.getPublicServerShare)
	r.Group(func(r chi.Router) {
		r.Use(h.requireAuth)
		r.Get("/api/auth/me", h.currentAccount)
		r.Post("/api/auth/password", h.changePassword)
		r.Get("/api/version", h.version)
		r.Get("/api/runtime/docker", h.dockerStatus)
		r.Get("/api/runtime/stats", h.runtimeStats)
		r.Get("/api/observability/metrics", h.observabilityMetrics)
		r.Get("/api/observability/prometheus", h.prometheusMetrics)
		monitoring.NewHandler(monitoring.NewService(
			h.store,
			monitoring.NewPrometheusClient(h.cfg.PrometheusURL, h.cfg.PrometheusQueryTimeout, h.apiMetrics),
		)).Register(r)
		r.Post("/api/runtime/images/prepare", h.prepareRuntimeImage)
		r.Get("/api/settings", h.getSettings)
		r.Put("/api/settings/public-host", h.updatePublicHost)
		r.Put("/api/settings/locale", h.updateLocale)
		r.Get("/api/activity", h.listActivity)
		r.Get("/api/games", h.listGames)
		r.Get("/api/games/{gameKey}", h.getGame)
		r.Get("/api/games/{gameKey}/versions", h.gameVersions)
		r.Get("/api/config-presets", h.listConfigPresets)
		r.Post("/api/config-presets", h.createConfigPreset)
		r.Get("/api/config-presets/{id}", h.getConfigPreset)
		r.Put("/api/config-presets/{id}", h.updateConfigPreset)
		r.Delete("/api/config-presets/{id}", h.deleteConfigPreset)
		r.Get("/api/servers", h.listServers)
		r.Post("/api/servers", h.createServer)
		r.Get("/api/servers/{id}", h.getServer)
		r.Put("/api/servers/{id}/config", h.updateServerConfig)
		r.Post("/api/servers/{id}/start", h.startServer)
		r.Post("/api/servers/{id}/stop", h.stopServer)
		r.Post("/api/servers/{id}/restart", h.restartServer)
		r.Post("/api/servers/{id}/command", h.sendServerCommand)
		r.Delete("/api/servers/{id}", h.deleteServer)
		r.Get("/api/servers/{id}/logs", h.serverLogs)
		r.Get("/api/servers/{id}/logs/snapshot", h.serverLogSnapshot)
		r.Get("/api/servers/{id}/stats", h.serverStats)
		r.Get("/api/worlds", h.listWorlds)
		r.Post("/api/worlds/import", h.importWorld)
		r.Post("/api/worlds/{id}/assign", h.assignWorld)
		r.Get("/api/worlds/{id}/download", h.downloadWorld)
		r.Delete("/api/worlds/{id}", h.deleteWorld)
		r.Get("/api/backups", h.listBackups)
		r.Post("/api/servers/{id}/world-snapshots", h.createWorldSnapshot)
		r.Post("/api/servers/{id}/backups", h.createBackup)
		r.Get("/api/backups/{id}/download", h.downloadBackup)
		r.Post("/api/backups/{id}/restore", h.restoreBackup)
		r.Delete("/api/backups/{id}", h.deleteBackup)
		r.Get("/api/servers/{id}/saves", h.listServerSaves)
		r.Post("/api/servers/{id}/saves/snapshot", h.createServerSaveSnapshot)
		r.Get("/api/servers/{id}/saves/{saveId}/download", h.downloadServerSave)
		r.Post("/api/servers/{id}/saves/{saveId}/restore", h.restoreServerSave)
		r.Get("/api/servers/{id}/players", h.listServerPlayers)
		r.Post("/api/servers/{id}/players/{player}/kick", h.kickServerPlayer)
		r.Post("/api/servers/{id}/players/{player}/ban", h.banServerPlayer)
		r.Get("/api/servers/{id}/whitelist", h.getServerWhitelist)
		r.Post("/api/servers/{id}/whitelist/{player}", h.addServerWhitelistPlayer)
		r.Delete("/api/servers/{id}/whitelist/{player}", h.removeServerWhitelistPlayer)
		r.Get("/api/servers/{id}/join-info", h.getServerJoinInfo)
		r.Get("/api/servers/{id}/share", h.getServerShare)
		r.Post("/api/servers/{id}/share", h.enableServerShare)
		r.Delete("/api/servers/{id}/share", h.disableServerShare)
		r.Get("/api/servers/{id}/mods", h.listMods)
		r.Post("/api/servers/{id}/mods/upload", h.uploadMod)
		r.Post("/api/servers/{id}/mods/workshop", h.importWorkshopMods)
		r.Patch("/api/servers/{id}/mods/{modId}", h.updateMod)
		r.Delete("/api/servers/{id}/mods/{modId}", h.deleteMod)
		r.Get("/api/mods", h.listGlobalMods)
		r.Get("/api/mods/recommended", h.listRecommendedMods)
		r.Post("/api/mods/upload", h.uploadGlobalMod)
		r.Post("/api/mods/workshop", h.importGlobalWorkshopMods)
		r.Post("/api/mods/{id}/assign", h.assignMod)
		r.Delete("/api/mods/{id}", h.deleteGlobalMod)
		r.Get("/api/mod-packs", h.listModPacks)
		r.Post("/api/mod-packs", h.createModPack)
		r.Patch("/api/mod-packs/{id}", h.updateModPack)
		r.Delete("/api/mod-packs/{id}", h.deleteModPack)
		r.Get("/api/terraria/presets", h.presets)
		r.Get("/api/terraria/versions", h.versions)
		r.Post("/api/terraria/config/preview", h.configPreview)
	})
}

func (h *Handler) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isGameServerLockedForMutation(server domain.GameServer) bool {
	switch server.Status.Phase {
	case domain.PhaseRunning, domain.PhasePending, domain.PhaseReconciling, domain.PhaseDeleting:
		return true
	default:
		return false
	}
}

func isGameServerBusyForModMutation(server domain.GameServer) bool {
	switch server.Status.Phase {
	case domain.PhasePending, domain.PhaseReconciling, domain.PhaseDeleting:
		return true
	default:
		return false
	}
}

func (h *Handler) recordActivity(ctx context.Context, instanceID, eventType, message string, payload ...map[string]any) {
	event := domain.ActivityEvent{
		ID:         uuid.NewString(),
		InstanceID: instanceID,
		Type:       eventType,
		Message:    message,
		CreatedAt:  time.Now(),
	}
	if len(payload) > 0 {
		event.Payload = payload[0]
	}
	if err := h.store.CreateActivity(ctx, &event); err != nil {
		h.logger.Warn("failed to record activity", "error", err, "type", eventType)
	}
}

func activityServerPayload(server domain.GameServer) map[string]any {
	return map[string]any{
		"serverId":      server.ID,
		"serverName":    server.Name,
		"gameKey":       server.GameKey,
		"providerKey":   server.ProviderKey,
		"desiredState":  server.Spec.DesiredState,
		"generation":    server.Spec.Generation,
		"runtimePhase":  server.Status.Phase,
		"runtimeId":     server.Status.RuntimeID,
		"runtimeStatus": server.Status.ActualState,
	}
}

func activityWorldPayload(world domain.World, server *domain.GameServer) map[string]any {
	payload := map[string]any{
		"worldId":     world.ID,
		"worldName":   world.Name,
		"fileName":    world.FileName,
		"sizeBytes":   world.SizeBytes,
		"gameKey":     world.GameKey,
		"providerKey": world.ProviderKey,
	}
	if world.ActiveInstanceID != "" {
		payload["activeServerId"] = world.ActiveInstanceID
	}
	if server != nil {
		payload["serverId"] = server.ID
		payload["serverName"] = server.Name
		payload["gameKey"] = server.GameKey
		payload["providerKey"] = server.ProviderKey
	}
	return payload
}

func activityBackupPayload(backup domain.Backup, server *domain.GameServer) map[string]any {
	payload := map[string]any{
		"backupId":    backup.ID,
		"fileName":    backup.FileName,
		"worldName":   backup.WorldName,
		"sizeBytes":   backup.SizeBytes,
		"backupType":  backup.Type,
		"gameKey":     backup.GameKey,
		"providerKey": backup.ProviderKey,
	}
	if backup.InstanceID != "" {
		payload["serverId"] = backup.InstanceID
	}
	if server != nil {
		payload["serverId"] = server.ID
		payload["serverName"] = server.Name
		payload["gameKey"] = server.GameKey
		payload["providerKey"] = server.ProviderKey
	}
	return payload
}

func activitySavePayload(backup domain.Backup, server domain.GameServer, saveName string) map[string]any {
	payload := activityBackupPayload(backup, &server)
	payload["saveName"] = saveName
	return payload
}

func activityModPayload(mod domain.ModFile, server *domain.GameServer) map[string]any {
	payload := map[string]any{
		"modId":       mod.ID,
		"fileName":    mod.FileName,
		"modName":     mod.ModName,
		"title":       mod.Title,
		"source":      mod.Source,
		"workshopId":  mod.WorkshopID,
		"enabled":     mod.Enabled,
		"sizeBytes":   mod.SizeBytes,
		"gameKey":     mod.GameKey,
		"providerKey": mod.ProviderKey,
	}
	if mod.InstanceID != "" {
		payload["serverId"] = mod.InstanceID
	}
	if server != nil {
		payload["serverId"] = server.ID
		payload["serverName"] = server.Name
		payload["gameKey"] = server.GameKey
		payload["providerKey"] = server.ProviderKey
	}
	return payload
}

func activityPlayerPayload(server domain.GameServer, player string) map[string]any {
	payload := activityServerPayload(server)
	payload["playerName"] = player
	return payload
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
