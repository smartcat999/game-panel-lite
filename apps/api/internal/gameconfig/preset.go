package gameconfig

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

// PublicConfig removes declared password fields at literal and nested paths.
// Legacy top-level credential keys remain redacted for previously saved presets.
// It never normalizes or reintroduces provider defaults after redaction.
func (s *Service) PublicConfig(key domain.ProviderKey, payload map[string]any) (map[string]any, string, error) {
	item, ok := s.providers.Get(key)
	if !ok {
		return nil, "", fmt.Errorf("unknown provider: %s", key)
	}
	clean, err := cloneConfigPayload(payload)
	if err != nil {
		return nil, "", err
	}
	for _, key := range []string{"password", "serverPassword", "adminPassword", "clusterToken"} {
		delete(clean, key)
	}
	for _, field := range item.ConfigSchema() {
		if field.Type != "password" {
			continue
		}
		deleteConfigPath(clean, strings.Split(field.Name, "."))
	}
	data, err := json.Marshal(clean)
	if err != nil {
		return nil, "", err
	}
	return clean, string(data), nil
}

func deleteConfigPath(payload map[string]any, path []string) {
	// A payload may use either nested maps or literal dotted keys at any level.
	delete(payload, strings.Join(path, "."))
	if len(path) > 1 {
		if child, ok := payload[path[0]].(map[string]any); ok {
			deleteConfigPath(child, path[1:])
		}
	}
}

// PublicPreset also covers legacy persisted representations independently;
// clients may read config, configPayload or configPayloadJson.
func (s *Service) PublicPreset(preset domain.ConfigPreset) (domain.ConfigPreset, error) {
	var err error
	preset.Config, _, err = s.PublicConfig(preset.ProviderKey, preset.Config)
	if err != nil {
		return domain.ConfigPreset{}, err
	}
	preset.ConfigPayload, _, err = s.PublicConfig(preset.ProviderKey, preset.ConfigPayload)
	if err != nil {
		return domain.ConfigPreset{}, err
	}
	if strings.TrimSpace(preset.ConfigPayloadJSON) != "" {
		var payload map[string]any
		if err := json.Unmarshal([]byte(preset.ConfigPayloadJSON), &payload); err != nil {
			return domain.ConfigPreset{}, fmt.Errorf("invalid preset config: %w", err)
		}
		_, preset.ConfigPayloadJSON, err = s.PublicConfig(preset.ProviderKey, payload)
		if err != nil {
			return domain.ConfigPreset{}, err
		}
	}
	return preset, nil
}
