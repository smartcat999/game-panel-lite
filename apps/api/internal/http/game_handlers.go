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
	writeJSON(w, http.StatusOK, games[0])
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
					Image:     image,
					Status:    runtime.ImageStatusUnsupported,
					Message:   "server runtime is not supported on this Docker architecture",
					UpdatedAt: time.Now(),
				}
				continue
			}
			providerCatalog.RuntimeImage = h.runtimeInstallStatus(ctx, runtimeInstallRef{ProviderKey: providerCatalog.Key, Version: version, Image: image})
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
