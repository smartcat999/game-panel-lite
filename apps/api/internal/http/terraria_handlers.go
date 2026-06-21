package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
)

func (h *Handler) presets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, terraria.Presets)
}

func (h *Handler) versions(w http.ResponseWriter, r *http.Request) {
	out := map[string][]string{}
	for _, provider := range h.provider.List() {
		out[string(provider.Key())] = provider.Versions()
	}
	writeJSON(w, http.StatusOK, out)
}

func providerSupportsVersion(gameProvider provider.GameProvider, version string) bool {
	for _, supported := range gameProvider.Versions() {
		if supported == version {
			return true
		}
	}
	return false
}

func normalizeStoredProviderVersion(gameProvider provider.GameProvider, version string) string {
	version = strings.TrimSpace(version)
	if providerSupportsVersion(gameProvider, version) {
		return version
	}
	versions := gameProvider.Versions()
	if len(versions) == 0 {
		return version
	}
	return versions[0]
}

func (h *Handler) configPreview(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Config terrariaPreviewConfig `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	rendered, err := terraria.RenderServerConfig(payload.Config.ToDomain())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"serverconfig": rendered})
}
