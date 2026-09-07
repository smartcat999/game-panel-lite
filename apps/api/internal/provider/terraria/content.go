package terraria

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func presetsFor(key domain.ProviderKey) []domain.ProviderPreset {
	out := []domain.ProviderPreset{}
	for _, preset := range Presets {
		if preset.ProviderKey != key {
			continue
		}
		out = append(out, domain.ProviderPreset{Key: preset.Key, Label: preset.Label, Description: preset.Description, ProviderKey: key, Config: PayloadFromConfig(preset.Config)})
	}
	return out
}
func (p VanillaProvider) Presets() []domain.ProviderPreset    { return presetsFor(p.Key()) }
func (p TModLoaderProvider) Presets() []domain.ProviderPreset { return presetsFor(p.Key()) }

func previewConfig(raw json.RawMessage) (map[string]string, error) {
	var config Config
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, err
	}
	rendered, err := RenderServerConfig(config)
	if err != nil {
		return nil, err
	}
	return map[string]string{"serverconfig": rendered}, nil
}
func (VanillaProvider) PreviewConfig(raw json.RawMessage) (map[string]string, error) {
	return previewConfig(raw)
}
func (TModLoaderProvider) PreviewConfig(raw json.RawMessage) (map[string]string, error) {
	return previewConfig(raw)
}
func (VanillaProvider) ConfigRestoreFile() string    { return "serverconfig.txt" }
func (TModLoaderProvider) ConfigRestoreFile() string { return "serverconfig.txt" }

func restoreConfig(payload map[string]any, content []byte, fallback Config) (map[string]any, int, error) {
	base, err := ConfigFromPayload(payload, fallback)
	if err != nil {
		return nil, 0, err
	}
	config, err := ParseServerConfig(base, string(content))
	if err != nil {
		return nil, 0, err
	}
	config.Port = DefaultInternalPort
	config = NormalizeConfig(config)
	return PayloadFromConfig(config), config.Port, nil
}
func (p VanillaProvider) RestoreConfig(payload map[string]any, content []byte) (map[string]any, int, error) {
	return restoreConfig(payload, content, p.DefaultConfig())
}
func (p TModLoaderProvider) RestoreConfig(payload map[string]any, content []byte) (map[string]any, int, error) {
	return restoreConfig(payload, content, p.DefaultConfig())
}

func worldFiles(server domain.GameServer) []string {
	name := server.Name
	if value, ok := server.Spec.Config["worldName"].(string); ok && strings.TrimSpace(value) != "" {
		name = value
	}
	config, err := ConfigFromPayload(server.Spec.Config, Config{WorldName: name})
	if err != nil {
		config = Config{WorldName: name}
	}
	file := name + ".wld"
	return append(RuntimeWorldFiles(server.ProviderKey, config), file, filepath.Join("Worlds", file), filepath.Join("worlds", file))
}
func (VanillaProvider) WorldFiles(server domain.GameServer) []string    { return worldFiles(server) }
func (TModLoaderProvider) WorldFiles(server domain.GameServer) []string { return worldFiles(server) }
