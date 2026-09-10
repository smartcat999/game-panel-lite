package terraria

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/nodeexecution"
)

const (
	GameKey             = "terraria"
	GameVersion         = "1.4.5.6"
	Image               = "smartcat99999/terraria-vanilla:1.4.5.6"
	DefaultInternalPort = 7777
)

type Provider struct{}

type Config struct {
	ServerName string `json:"serverName"`
	WorldName  string `json:"worldName"`
	WorldSize  string `json:"worldSize"`
	WorldEvil  string `json:"worldEvil"`
	Difficulty string `json:"difficulty"`
	MaxPlayers int    `json:"maxPlayers"`
	Password   string `json:"password,omitempty"`
	MOTD       string `json:"motd,omitempty"`
	Seed       string `json:"seed,omitempty"`
	Secure     bool   `json:"secure"`
	Language   string `json:"language"`
}

func (Provider) Key() string { return GameKey }

func (Provider) Build(intent nodeexecution.WorkloadIntent) (nodeexecution.WorkloadSpec, error) {
	if intent.GameKey != GameKey || (intent.GameVersion != "" && intent.GameVersion != GameVersion) || intent.LogicalInstanceID == "" || (intent.DesiredState != "running" && intent.DesiredState != "stopped") || intent.DataDir == "" {
		return nodeexecution.WorkloadSpec{}, errors.New("invalid Terraria workload intent")
	}
	config, err := decodeConfig(intent.Configuration)
	if err != nil {
		return nodeexecution.WorkloadSpec{}, err
	}
	rendered, err := renderConfig(config)
	if err != nil {
		return nodeexecution.WorkloadSpec{}, err
	}
	worldSizes := map[string]int{"small": 1, "medium": 2, "large": 3}
	return nodeexecution.WorkloadSpec{
		LogicalInstanceID: intent.LogicalInstanceID,
		DesiredState:      intent.DesiredState,
		Image:             Image,
		DataDir:           intent.DataDir,
		Port:              DefaultInternalPort,
		Protocol:          "tcp",
		CPUUnits:          intent.CPUUnits,
		MemoryMegabytes:   intent.MemoryMegabytes,
		Environment:       []string{"HOME=/home/container", "WORLD_NAME=" + config.WorldName, fmt.Sprintf("WORLD_SIZE=%d", worldSizes[config.WorldSize])},
		Command:           []string{"sh", "-c", "cd /home/container && exec ./server/gamepanel-terraria-entrypoint.sh -config /home/container/serverconfig.txt -world \"/home/container/Worlds/${WORLD_NAME}.wld\" -autocreate \"${WORLD_SIZE}\" -noupnp"},
		Files:             map[string]string{"serverconfig.txt": rendered},
		DataMounts:        map[string]string{"Worlds": "/home/container/Worlds", "logs": "/home/container/logs", "serverconfig.txt": "/home/container/serverconfig.txt"},
		FencingToken:      intent.FencingToken,
	}, nil
}

func decodeConfig(input map[string]any) (Config, error) {
	config := Config{ServerName: "GamePanel Server", WorldName: "GamePanel World", WorldSize: "medium", WorldEvil: "random", Difficulty: "classic", MaxPlayers: 8, MOTD: "Welcome to GamePanel", Secure: true, Language: "en-US"}
	if len(input) > 0 {
		raw, err := json.Marshal(input)
		if err != nil {
			return Config{}, err
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&config); err != nil {
			return Config{}, errors.New("invalid Terraria configuration")
		}
	}
	return config, validateConfig(config)
}

func validateConfig(config Config) error {
	if strings.TrimSpace(config.WorldName) == "" || strings.Contains(config.WorldName, "..") || strings.ContainsAny(config.WorldName, `/\\`) || filepath.Base(config.WorldName) != config.WorldName {
		return errors.New("world name cannot contain path traversal characters")
	}
	if config.MaxPlayers < 1 || config.MaxPlayers > 255 {
		return errors.New("max players must be between 1 and 255")
	}
	if config.WorldSize != "small" && config.WorldSize != "medium" && config.WorldSize != "large" {
		return errors.New("invalid world size")
	}
	if config.WorldEvil != "random" && config.WorldEvil != "corruption" && config.WorldEvil != "crimson" {
		return errors.New("invalid world evil")
	}
	if config.Difficulty != "journey" && config.Difficulty != "classic" && config.Difficulty != "expert" && config.Difficulty != "master" {
		return errors.New("invalid difficulty")
	}
	for _, value := range []string{config.ServerName, config.WorldName, config.Password, config.MOTD, config.Seed, config.Language} {
		if strings.ContainsAny(value, "\r\n\x00") {
			return errors.New("multiline Terraria configuration is unsupported")
		}
	}
	return nil
}

func renderConfig(config Config) (string, error) {
	if err := validateConfig(config); err != nil {
		return "", err
	}
	worldSizes := map[string]int{"small": 1, "medium": 2, "large": 3}
	worldEvils := map[string]int{"random": 0, "corruption": 1, "crimson": 2}
	difficulties := map[string]int{"classic": 0, "expert": 1, "master": 2, "journey": 3}
	secure := 0
	if config.Secure {
		secure = 1
	}
	return strings.Join([]string{
		fmt.Sprintf("world=/home/container/Worlds/%s.wld", config.WorldName),
		fmt.Sprintf("autocreate=%d", worldSizes[config.WorldSize]),
		"worldname=" + config.WorldName,
		fmt.Sprintf("worldevil=%d", worldEvils[config.WorldEvil]),
		fmt.Sprintf("difficulty=%d", difficulties[config.Difficulty]),
		fmt.Sprintf("maxplayers=%d", config.MaxPlayers),
		fmt.Sprintf("port=%d", DefaultInternalPort),
		"password=" + config.Password,
		"motd=" + config.MOTD,
		"seed=" + config.Seed,
		"worldpath=/home/container/Worlds",
		fmt.Sprintf("secure=%d", secure),
		"language=" + config.Language,
		"upnp=0",
		"priority=1",
	}, "\n"), nil
}
