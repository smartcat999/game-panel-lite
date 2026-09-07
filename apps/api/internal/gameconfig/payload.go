package gameconfig

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
)

// Normalize overlays top-level input fields on an isolated copy of the stored
// payload or provider defaults. Validation is separate for callers that add
// provider-owned values before persisting.
func (s *Service) Normalize(key domain.ProviderKey, version int, raw json.RawMessage, fallback map[string]any) (map[string]any, string, error) {
	gameProvider, ok := s.providers.Get(key)
	if !ok {
		return nil, "", fmt.Errorf("unknown provider: %s", key)
	}
	if err := provider.CheckConfigVersion(gameProvider, version); err != nil {
		return nil, "", err
	}
	payloadProvider, ok := gameProvider.(provider.ConfigPayloadProvider)
	if !ok {
		return nil, "", fmt.Errorf("provider %s does not support resource config payloads", gameProvider.Key())
	}
	payload, err := cloneConfigPayload(fallback)
	if err != nil {
		return nil, "", err
	}
	if len(payload) == 0 {
		payload, err = cloneConfigPayload(payloadProvider.DefaultConfigPayload())
		if err != nil {
			return nil, "", err
		}
	}
	if !isEmptyRawJSON(raw) {
		var input map[string]any
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, "", fmt.Errorf("invalid config payload")
		}
		if input == nil {
			input = map[string]any{}
		}
		for key, value := range input {
			payload[key] = value
		}
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

func (s *Service) Validate(key domain.ProviderKey, payload map[string]any) error {
	gameProvider, ok := s.providers.Get(key)
	if !ok {
		return fmt.Errorf("unknown provider: %s", key)
	}
	payloadProvider, ok := gameProvider.(provider.ConfigPayloadProvider)
	if !ok {
		return fmt.Errorf("provider %s does not support resource config payloads", gameProvider.Key())
	}
	return payloadProvider.ValidateConfigPayload(payload)
}

func (s *Service) Summary(key domain.ProviderKey, payload map[string]any) (domain.ProviderConfigSummary, error) {
	gameProvider, ok := s.providers.Get(key)
	if !ok {
		return domain.ProviderConfigSummary{}, fmt.Errorf("unknown provider: %s", key)
	}
	summaryProvider, ok := gameProvider.(provider.ConfigSummaryProvider)
	if !ok {
		return domain.ProviderConfigSummary{}, fmt.Errorf("provider %s does not support config summaries", gameProvider.Key())
	}
	return summaryProvider.ConfigSummary(payload)
}

// Clone through JSON so provider normalization cannot mutate nested caller maps.
func cloneConfigPayload(payload map[string]any) (map[string]any, error) {
	if len(payload) == 0 {
		return map[string]any{}, nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}
func isEmptyRawJSON(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed == "" || trimmed == "null"
}
