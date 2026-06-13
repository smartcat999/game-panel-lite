package terraria

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

type Preset struct {
	Key         string                `json:"key"`
	Label       string                `json:"label"`
	Description string                `json:"description"`
	ProviderKey domain.ProviderKey    `json:"providerKey"`
	Config      domain.TerrariaConfig `json:"config"`
}

var Presets = []Preset{
	{"friends-casual", "Friends Casual", "Relaxed co-op defaults for a small friend group.", domain.ProviderTerrariaVanilla, domain.TerrariaConfig{ServerName: "Journey Friends", WorldName: "Moon Garden", WorldSize: "medium", Difficulty: "classic", MaxPlayers: 8, Port: 7777, MOTD: "Welcome to GamePanel Lite", Secure: true, Language: "en-US", AutoCreateWorld: true}},
	{"expert-adventure", "Expert Adventure", "A tougher cooperative world for experienced players.", domain.ProviderTerrariaVanilla, domain.TerrariaConfig{ServerName: "Expert Adventure", WorldName: "Adventure", WorldSize: "large", Difficulty: "expert", MaxPlayers: 8, Port: 7778, MOTD: "Bring potions", Secure: true, Language: "en-US", AutoCreateWorld: true}},
	{"master-challenge", "Master Challenge", "High-intensity defaults for veteran players.", domain.ProviderTerrariaVanilla, domain.TerrariaConfig{ServerName: "Master Challenge", WorldName: "Master Challenge", WorldSize: "large", Difficulty: "master", MaxPlayers: 6, Port: 7779, MOTD: "Good luck", Secure: true, Language: "en-US", AutoCreateWorld: true}},
	{"building-world", "Building World", "Roomy, calm defaults for builders and decorators.", domain.ProviderTerrariaVanilla, domain.TerrariaConfig{ServerName: "Building Server", WorldName: "Builder's Heaven", WorldSize: "large", Difficulty: "classic", MaxPlayers: 12, Port: 7780, MOTD: "Build something sharp", Secure: true, Language: "en-US", AutoCreateWorld: true}},
	{"modded-starter", "Modded Starter", "A conservative starting point for tModLoader servers.", domain.ProviderTerrariaTModLoader, domain.TerrariaConfig{ServerName: "Modded Adventure", WorldName: "Adventure", WorldSize: "medium", Difficulty: "classic", MaxPlayers: 10, Port: 7781, MOTD: "Mods enabled", Secure: true, Language: "en-US", AutoCreateWorld: true}},
}

func ValidateConfig(config domain.TerrariaConfig) error {
	if strings.TrimSpace(config.WorldName) == "" {
		return fmt.Errorf("world name is required")
	}
	if strings.Contains(config.WorldName, "..") || strings.ContainsAny(config.WorldName, `/\`) || filepath.Base(config.WorldName) != config.WorldName {
		return fmt.Errorf("world name cannot contain path traversal characters")
	}
	if config.Port < 1024 || config.Port > 65535 {
		return fmt.Errorf("port must be between 1024 and 65535")
	}
	if config.MaxPlayers < 1 || config.MaxPlayers > 255 {
		return fmt.Errorf("max players must be between 1 and 255")
	}
	switch config.WorldSize {
	case "small", "medium", "large":
	default:
		return fmt.Errorf("world size must be small, medium, or large")
	}
	switch config.Difficulty {
	case "journey", "classic", "expert", "master":
	default:
		return fmt.Errorf("difficulty must be journey, classic, expert, or master")
	}
	return nil
}

func RenderServerConfig(config domain.TerrariaConfig) (string, error) {
	if err := ValidateConfig(config); err != nil {
		return "", err
	}
	worldSizes := map[domain.WorldSize]int{"small": 1, "medium": 2, "large": 3}
	difficulties := map[domain.Difficulty]int{"journey": 0, "classic": 1, "expert": 2, "master": 3}
	lines := []string{
		fmt.Sprintf("world=worlds/%s.wld", config.WorldName),
		fmt.Sprintf("autocreate=%d", worldSizes[config.WorldSize]),
		fmt.Sprintf("difficulty=%d", difficulties[config.Difficulty]),
		fmt.Sprintf("maxplayers=%d", config.MaxPlayers),
		fmt.Sprintf("port=%d", config.Port),
		fmt.Sprintf("password=%s", config.Password),
		fmt.Sprintf("motd=%s", config.MOTD),
		fmt.Sprintf("seed=%s", config.Seed),
		fmt.Sprintf("secure=%d", boolInt(config.Secure)),
		fmt.Sprintf("language=%s", value(config.Language, "en-US")),
	}
	return strings.Join(lines, "\n"), nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func value(got string, fallback string) string {
	if got != "" {
		return got
	}
	return fallback
}
