package http

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func (h *Handler) configSummaryForServer(server domain.GameServer) (domain.ProviderConfigSummary, error) {
	return h.gameConfig.Summary(server.ProviderKey, server.Spec.Config)
}

func stringPayload(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func serverDataDir(server domain.GameServer) (string, error) {
	dataDir := strings.TrimSpace(server.Spec.Runtime.DataDir)
	if dataDir == "" {
		return "", fmt.Errorf("server data directory is not ready")
	}
	return dataDir, nil
}

func serverWorldName(server domain.GameServer) string {
	for _, key := range []string{"worldName", "saveName", "clusterName"} {
		if value := stringPayload(server.Spec.Config, key); value != "" {
			return value
		}
	}
	return server.Name
}

func hydratePresetConfigPayload(preset *domain.ConfigPreset) {
	if preset == nil {
		return
	}
	if strings.TrimSpace(preset.ConfigPayloadJSON) != "" {
		var payload map[string]any
		if err := json.Unmarshal([]byte(preset.ConfigPayloadJSON), &payload); err == nil {
			preset.ConfigPayload = payload
		}
	}
	preset.ModIDs = []string{}
	if strings.TrimSpace(preset.ModIDsJSON) != "" {
		_ = json.Unmarshal([]byte(preset.ModIDsJSON), &preset.ModIDs)
	}
}
