package http

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
)

func normalizeTerrariaRuntimeConfig(config terraria.Config) terraria.Config {
	config.Port = terraria.DefaultInternalPort
	return terraria.NormalizeConfig(config)
}

func decodeProviderConfigPayload(gameProvider provider.GameProvider, raw json.RawMessage, fallback map[string]any) (map[string]any, string, error) {
	payloadProvider, ok := gameProvider.(provider.ConfigPayloadProvider)
	if !ok {
		return nil, "", fmt.Errorf("provider %s does not support resource config payloads", gameProvider.Key())
	}
	payload := cloneConfigPayload(fallback)
	if len(payload) == 0 {
		payload = payloadProvider.DefaultConfigPayload()
	}
	if !isEmptyRawJSON(raw) {
		var input map[string]any
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, "", fmt.Errorf("invalid config payload")
		}
		if input == nil {
			input = map[string]any{}
		}
		payload = mergeConfigPayload(payload, input)
	}
	normalized, err := payloadProvider.NormalizeConfigPayload(payload)
	if err != nil {
		return nil, "", err
	}
	if normalized == nil {
		normalized = map[string]any{}
	}
	buf, err := json.Marshal(normalized)
	if err != nil {
		return nil, "", err
	}
	return normalized, string(buf), nil
}

func validateProviderConfigPayload(gameProvider provider.GameProvider, payload map[string]any) error {
	payloadProvider, ok := gameProvider.(provider.ConfigPayloadProvider)
	if !ok {
		return fmt.Errorf("provider %s does not support resource config payloads", gameProvider.Key())
	}
	return payloadProvider.ValidateConfigPayload(payload)
}

func providerConfigSummary(gameProvider provider.GameProvider, payload map[string]any) (domain.ProviderConfigSummary, error) {
	summaryProvider, ok := gameProvider.(provider.ConfigSummaryProvider)
	if !ok {
		return domain.ProviderConfigSummary{}, fmt.Errorf("provider %s does not support config summaries", gameProvider.Key())
	}
	return summaryProvider.ConfigSummary(payload)
}

func (h *Handler) configSummaryForServer(server domain.GameServer) (domain.ProviderConfigSummary, error) {
	gameProvider, ok := h.provider.Get(server.ProviderKey)
	if !ok {
		return domain.ProviderConfigSummary{}, fmt.Errorf("unknown provider")
	}
	return providerConfigSummary(gameProvider, server.Spec.Config)
}

func mergeConfigPayload(base map[string]any, overlay map[string]any) map[string]any {
	out := cloneConfigPayload(base)
	for key, value := range overlay {
		out[key] = value
	}
	return out
}

func cloneConfigPayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		out[key] = value
	}
	return out
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

func configPayloadMap(payloadJSON string) (map[string]any, error) {
	var payload map[string]any
	if strings.TrimSpace(payloadJSON) == "" {
		return map[string]any{}, nil
	}
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return nil, fmt.Errorf("invalid config payload")
	}
	if payload == nil {
		payload = map[string]any{}
	}
	return payload, nil
}

func sanitizePresetConfigPayload(gameProvider provider.GameProvider, configPayloadJSON string) (string, error) {
	if strings.TrimSpace(configPayloadJSON) == "" {
		return "", nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(configPayloadJSON), &payload); err != nil {
		return "", err
	}
	for _, key := range []string{"password", "serverPassword", "adminPassword", "clusterToken"} {
		delete(payload, key)
	}
	for _, field := range gameProvider.ConfigSchema() {
		if field.Type == "password" {
			delete(payload, field.Name)
		}
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

func isEmptyRawJSON(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed == "" || trimmed == "null"
}

func hydratePresetConfigPayload(preset *domain.ConfigPreset) {
	if preset == nil || strings.TrimSpace(preset.ConfigPayloadJSON) == "" {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(preset.ConfigPayloadJSON), &payload); err == nil {
		preset.ConfigPayload = payload
	}
}
