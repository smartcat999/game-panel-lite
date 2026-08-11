package http

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

func (h *Handler) listGames(w http.ResponseWriter, r *http.Request) {
	games := h.provider.Games()
	h.applyRuntimeGameAvailability(games)
	h.attachRuntimeImageStatuses(r.Context(), games)
	h.attachProviderVersionStatuses(r.Context(), games)
	servers, err := h.store.ListGameServers(r.Context())
	if err == nil {
		counts := map[domain.GameKey]int{}
		for _, server := range servers {
			counts[server.GameKey]++
		}
		for index := range games {
			games[index].ServerCount = counts[games[index].Key]
		}
	}
	writeJSON(w, http.StatusOK, games)
}

func (h *Handler) getGame(w http.ResponseWriter, r *http.Request) {
	game, ok := h.provider.Game(domain.GameKey(chi.URLParam(r, "gameKey")))
	if !ok {
		writeError(w, http.StatusNotFound, "game not found")
		return
	}
	games := []domain.GameCatalogEntry{game}
	h.applyRuntimeGameAvailability(games)
	h.attachRuntimeImageStatuses(r.Context(), games)
	h.attachProviderVersionStatuses(r.Context(), games)
	writeJSON(w, http.StatusOK, games[0])
}

func (h *Handler) attachProviderVersionStatuses(ctx context.Context, games []domain.GameCatalogEntry) {
	for gameIndex := range games {
		for providerIndex := range games[gameIndex].Providers {
			item := &games[gameIndex].Providers[providerIndex]
			item.GameVersion = domain.ProviderVersionStatus{Status: "unsupported"}
			if item.Key != domain.ProviderPalworld {
				continue
			}
			item.GameVersion = domain.ProviderVersionStatus{Supported: true, Status: "unknown", AutoCheckEnabled: true, AutoCheckHours: int(gameUpdateAutoCheckInterval / time.Hour)}
			if enabled, err := h.gameUpdateAutoCheckEnabled(ctx, item.Key); err == nil {
				item.GameVersion.AutoCheckEnabled = enabled
			}
			if job, err := h.store.GetLatestGameUpdateCheckByProvider(ctx, item.Key); err == nil {
				item.GameVersion.Job, item.GameVersion.LatestBuildID, item.GameVersion.CheckedAt = &job, job.LatestBuildID, job.CheckedAt
				switch job.Status {
				case domain.GameUpdateJobQueued, domain.GameUpdateJobRunning:
					item.GameVersion.Status = "checking"
				case domain.GameUpdateJobFailed:
					item.GameVersion.Status = "failed"
				default:
					item.GameVersion.Status = "ready"
				}
			}
		}
	}
}

func (h *Handler) applyRuntimeGameAvailability(games []domain.GameCatalogEntry) {
	if !h.providerRuntimeUnsupported(domain.ProviderDST) {
		return
	}
	for index := range games {
		if games[index].Key == domain.GameDST {
			games[index].Status = "unsupported"
		}
	}
}

func (h *Handler) attachRuntimeImageStatuses(ctx context.Context, games []domain.GameCatalogEntry) {
	for gameIndex := range games {
		for providerIndex := range games[gameIndex].Providers {
			providerCatalog := &games[gameIndex].Providers[providerIndex]
			gameProvider, ok := h.provider.Get(providerCatalog.Key)
			if !ok {
				continue
			}
			version := providerCatalog.RecommendedVersion
			if version == "" {
				version = normalizeStoredProviderVersion(gameProvider, "")
			}
			image := gameProvider.ImageFor(version)
			if h.providerRuntimeUnsupported(providerCatalog.Key) {
				providerCatalog.RuntimeImage = domain.RuntimeImageStatus{
					Image:         image,
					Status:        runtime.ImageStatusUnsupported,
					Message:       "server runtime is not supported on this Docker architecture",
					TargetVersion: version,
					UpdatedAt:     time.Now(),
				}
				continue
			}
			status := h.runtimeInstallStatus(ctx, runtimeInstallRef{ProviderKey: providerCatalog.Key, Version: version, Image: image})
			status.TargetVersion = version
			if status.Status == runtime.ImageStatusReady {
				status.InstalledVersion = version
			} else if status.Status == runtime.ImageStatusMissing {
				for _, installedVersion := range providerCatalog.Versions {
					if installedVersion == version {
						continue
					}
					installedImage := gameProvider.ImageFor(installedVersion)
					installedStatus := h.runtimeInstallStatus(ctx, runtimeInstallRef{
						ProviderKey: providerCatalog.Key,
						Version:     installedVersion,
						Image:       installedImage,
					})
					if installedStatus.Status != runtime.ImageStatusReady {
						continue
					}
					status.Status = runtime.ImageStatusUpdateReady
					status.Message = "a newer server runtime version is available"
					status.InstalledVersion = installedVersion
					break
				}
			}
			providerCatalog.RuntimeImage = status
		}
	}
}

func (h *Handler) gameVersions(w http.ResponseWriter, r *http.Request) {
	game, ok := h.provider.Game(domain.GameKey(chi.URLParam(r, "gameKey")))
	if !ok {
		writeError(w, http.StatusNotFound, "game not found")
		return
	}
	versions := map[domain.ProviderKey][]string{}
	for _, item := range game.Providers {
		versions[item.Key] = item.Versions
	}
	writeJSON(w, http.StatusOK, versions)
}
