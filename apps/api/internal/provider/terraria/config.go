package terraria

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

type Preset struct {
	Key         string             `json:"key"`
	Label       string             `json:"label"`
	Description string             `json:"description"`
	ProviderKey domain.ProviderKey `json:"providerKey"`
	Config      Config             `json:"config"`
}

type WorldSize string

type WorldEvil string

type Difficulty string

type Config struct {
	ServerName      string     `json:"serverName"`
	WorldName       string     `json:"worldName"`
	WorldSize       WorldSize  `json:"worldSize"`
	WorldEvil       WorldEvil  `json:"worldEvil"`
	Difficulty      Difficulty `json:"difficulty"`
	MaxPlayers      int        `json:"maxPlayers"`
	Port            int        `json:"port"`
	Password        string     `json:"password,omitempty"`
	MOTD            string     `json:"motd,omitempty"`
	Seed            string     `json:"seed,omitempty"`
	SpecialSeeds    []string   `json:"specialSeeds,omitempty"`
	SecretSeeds     []string   `json:"secretSeeds,omitempty"`
	Secure          bool       `json:"secure"`
	Language        string     `json:"language"`
	AutoCreateWorld bool       `json:"autoCreateWorld"`
}

const (
	DefaultInternalPort = 7777
	DefaultLanguage     = "en-US"
)

var Presets = []Preset{
	{"friends-casual", "Friends Casual", "Relaxed co-op defaults for a small friend group.", domain.ProviderTerrariaVanilla, Config{ServerName: "Friends Server", WorldName: "Friends World", WorldSize: "medium", WorldEvil: "random", Difficulty: "classic", MaxPlayers: 8, Port: DefaultInternalPort, MOTD: "Welcome to GamePanel Lite", Secure: true, Language: DefaultLanguage, AutoCreateWorld: true}},
	{"expert-adventure", "Expert Adventure", "A tougher cooperative world for experienced players.", domain.ProviderTerrariaVanilla, Config{ServerName: "Expert Adventure", WorldName: "Expert Adventure", WorldSize: "large", WorldEvil: "random", Difficulty: "expert", MaxPlayers: 8, Port: DefaultInternalPort, MOTD: "Bring potions", Secure: true, Language: DefaultLanguage, AutoCreateWorld: true}},
	{"master-challenge", "Master Challenge", "High-intensity defaults for players who want pressure.", domain.ProviderTerrariaVanilla, Config{ServerName: "Master Challenge", WorldName: "Master Challenge", WorldSize: "large", WorldEvil: "random", Difficulty: "master", MaxPlayers: 6, Port: DefaultInternalPort, MOTD: "Good luck", Secure: true, Language: DefaultLanguage, AutoCreateWorld: true}},
	{"building-world", "Building World", "Roomy, calm defaults for builders and decorators.", domain.ProviderTerrariaVanilla, Config{ServerName: "Building World", WorldName: "Builder Base", WorldSize: "large", WorldEvil: "random", Difficulty: "classic", MaxPlayers: 12, Port: DefaultInternalPort, MOTD: "Build something sharp", Secure: true, Language: DefaultLanguage, AutoCreateWorld: true}},
	{"modded-starter", "Modded Starter", "A conservative starting point for tModLoader servers.", domain.ProviderTerrariaTModLoader, Config{ServerName: "Modded Starter", WorldName: "Modded Starter", WorldSize: "medium", WorldEvil: "random", Difficulty: "classic", MaxPlayers: 8, Port: DefaultInternalPort, MOTD: "Mods enabled", Secure: true, Language: DefaultLanguage, AutoCreateWorld: true}},
}

func NormalizeConfig(config Config) Config {
	if strings.TrimSpace(config.Language) == "" {
		config.Language = DefaultLanguage
	}
	return config
}

func ValidateConfig(config Config) error {
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
	switch config.WorldEvil {
	case "", "random", "corruption", "crimson":
	default:
		return fmt.Errorf("world evil must be random, corruption, or crimson")
	}
	switch config.Difficulty {
	case "journey", "classic", "expert", "master":
	default:
		return fmt.Errorf("difficulty must be journey, classic, expert, or master")
	}
	return nil
}

func RenderServerConfig(config Config) (string, error) {
	config = NormalizeConfig(config)
	if err := ValidateConfig(config); err != nil {
		return "", err
	}
	return renderVanillaRuntimeConfig(config), nil
}

func ParseServerConfig(base Config, input string) (Config, error) {
	next := base
	for _, rawLine := range strings.Split(input, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "world":
			if value != "" {
				world := strings.TrimSuffix(filepath.Base(value), filepath.Ext(value))
				if world != "" {
					next.WorldName = world
				}
			}
		case "worldname":
			if value != "" {
				next.WorldName = value
			}
		case "autocreate":
			switch value {
			case "1":
				next.WorldSize = "small"
			case "2":
				next.WorldSize = "medium"
			case "3":
				next.WorldSize = "large"
			}
		case "worldevil":
			switch value {
			case "0":
				next.WorldEvil = "random"
			case "1":
				next.WorldEvil = "corruption"
			case "2":
				next.WorldEvil = "crimson"
			}
		case "difficulty":
			switch value {
			case "0":
				next.Difficulty = "classic"
			case "1":
				next.Difficulty = "expert"
			case "2":
				next.Difficulty = "master"
			case "3":
				next.Difficulty = "journey"
			}
		case "maxplayers":
			if parsed, err := strconv.Atoi(value); err == nil {
				next.MaxPlayers = parsed
			}
		case "port":
			if parsed, err := strconv.Atoi(value); err == nil {
				next.Port = parsed
			}
		case "password":
			next.Password = value
		case "motd":
			next.MOTD = value
		case "seed":
			next.Seed = value
		case "secure":
			next.Secure = value == "1" || strings.EqualFold(value, "true")
		case "language":
			next.Language = value
		}
	}
	if next.ServerName == "" {
		next.ServerName = base.ServerName
	}
	next.Language = DefaultLanguage
	return next, ValidateConfig(next)
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
