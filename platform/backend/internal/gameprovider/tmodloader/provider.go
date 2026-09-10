package tmodloader

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instanceobservability"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/nodeworkload"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
)

const (
	GameKey                 = "tmodloader"
	GameVersion             = "v2026.07.3.0"
	Image                   = "gamepanel/tmodloader:v2026.07.3.0"
	DefaultInternalPort     = 7777
	CalamityWorkshopID      = "2824688072"
	CalamityMusicWorkshopID = "2824688266"
	CalamityVersion         = "2.2.4"
	CalamityMusicVersion    = "2.1"
	CalamityDigest          = "sha256:d50fea9025f88ee2073c00e3482976ca1e6ea54d0b8770eb7810bb7b280397d6"
	CalamityMusicDigest     = "sha256:ca202bc18ae690d5ea7267b42c9ec8162241a19aafc9efdf40eff0a59eada9e5"
)

var runtimeNames = map[string]string{CalamityWorkshopID: "CalamityMod", CalamityMusicWorkshopID: "CalamityModMusic"}

var runtimeArtifacts = map[string]string{
	CalamityWorkshopID:      "2026.6/CalamityMod.tmod " + strings.TrimPrefix(CalamityDigest, "sha256:"),
	CalamityMusicWorkshopID: "2025.12/CalamityModMusic.tmod " + strings.TrimPrefix(CalamityMusicDigest, "sha256:"),
}

type Provider struct{}

type Config struct {
	ServerName string `json:"serverName"`
	WorldName  string `json:"worldName"`
	WorldSize  string `json:"worldSize"`
	Difficulty string `json:"difficulty"`
	MaxPlayers int    `json:"maxPlayers"`
	Password   string `json:"password,omitempty"`
	MOTD       string `json:"motd,omitempty"`
}

func (Provider) Materialize(_ context.Context, intent nodeworkload.Intent) (nodeworkload.Specification, error) {
	if intent.ProviderReleaseID == "" || intent.GameVersion != GameVersion || intent.LogicalInstanceID == "" || intent.DesiredState != "running" && intent.DesiredState != "stopped" || intent.DataScope != "instances/"+intent.LogicalInstanceID {
		return nodeworkload.Specification{}, errors.New("invalid tModLoader workload intent")
	}
	config, err := decodeConfig(intent.Configuration)
	if err != nil {
		return nodeworkload.Specification{}, err
	}
	listeners, err := listeners(intent)
	if err != nil {
		return nodeworkload.Specification{}, err
	}
	enabled, installs, artifacts, err := renderMods(intent.ModLock)
	if err != nil {
		return nodeworkload.Specification{}, err
	}
	worldSizes := map[string]int{"small": 1, "medium": 2, "large": 3}
	difficulties := map[string]int{"classic": 0, "expert": 1, "master": 2, "journey": 3}
	configText := strings.Join([]string{
		"world=/data/Worlds/" + config.WorldName + ".wld",
		fmt.Sprintf("autocreate=%d", worldSizes[config.WorldSize]),
		"worldname=" + config.WorldName,
		fmt.Sprintf("difficulty=%d", difficulties[config.Difficulty]),
		fmt.Sprintf("maxplayers=%d", config.MaxPlayers),
		fmt.Sprintf("port=%d", DefaultInternalPort),
		"password=" + config.Password,
		"motd=" + config.MOTD,
		"worldpath=/data/Worlds",
		"upnp=0",
	}, "\n")
	return nodeworkload.Specification{
		LogicalInstanceID: intent.LogicalInstanceID,
		DesiredState:      intent.DesiredState,
		Artifact:          Image,
		Args:              []string{"/opt/gamepanel/start-tmodloader.sh", "-nosteam", "-config", "/data/serverconfig.txt", "-steamworkshopfolder", "/data/steamapps/workshop", "-tmlsavedirectory", "/home/tml/.local/share/Terraria/tModLoader"},
		Env:               map[string]string{"TMOD_VERSION": GameVersion},
		Files:             map[string]string{"serverconfig.txt": configText, "tml-data/Mods/enabled.json": enabled, "tml-data/Mods/install.txt": installs, "tml-data/Mods/artifacts.lock": artifacts},
		Mounts:            map[string]string{"Worlds": "/data/Worlds", "calamity-home": "/opt/tmodloader/~", "tml-data": "/home/tml/.local/share/Terraria/tModLoader", "logs": "/opt/tmodloader/tModLoader-Logs", "steamapps": "/data/steamapps", "steamcmd": "/data/steamcmd", "serverconfig.txt": "/data/serverconfig.txt"},
		Listeners:         listeners,
		CPUMilli:          intent.CPUMilli,
		MemoryMiB:         intent.MemoryMiB,
		RunAsUID:          10001,
		RunAsGID:          10001,
		ExecutableTemp:    true,
		ReadyLogMarker:    "Server started",
		DataScope:         intent.DataScope,
		FencingToken:      intent.FencingToken,
	}, nil
}

func (Provider) ConsoleCommand(command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" || len(command) > 4096 || strings.ContainsAny(command, "\r\n\x00") {
		return "", errors.New("invalid tModLoader console command")
	}
	return command, nil
}

func (Provider) CollectMetrics(context.Context, nodeworkload.RuntimeHandle, []providercontract.Metric, time.Time) ([]instanceobservability.MetricSample, error) {
	return nil, nil
}

func decodeConfig(input map[string]any) (Config, error) {
	config := Config{ServerName: "GamePanel tModLoader", WorldName: "GamePanel World", WorldSize: "medium", Difficulty: "classic", MaxPlayers: 8, MOTD: "Welcome to GamePanel"}
	raw, err := json.Marshal(input)
	if err != nil {
		return Config{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, errors.New("invalid tModLoader configuration")
	}
	if strings.TrimSpace(config.WorldName) == "" || filepath.Base(config.WorldName) != config.WorldName || strings.ContainsAny(config.WorldName, "/\\\\\r\n\x00") || config.MaxPlayers < 1 || config.MaxPlayers > 255 {
		return Config{}, fmt.Errorf("invalid tModLoader world identity: name=%q maxPlayers=%d", config.WorldName, config.MaxPlayers)
	}
	if config.WorldSize != "small" && config.WorldSize != "medium" && config.WorldSize != "large" || config.Difficulty != "journey" && config.Difficulty != "classic" && config.Difficulty != "expert" && config.Difficulty != "master" {
		return Config{}, fmt.Errorf("invalid tModLoader world mode: size=%q difficulty=%q", config.WorldSize, config.Difficulty)
	}
	for _, value := range []string{config.ServerName, config.Password, config.MOTD} {
		if strings.ContainsAny(value, "\r\n\x00") {
			return Config{}, errors.New("multiline tModLoader configuration is unsupported")
		}
	}
	return config, nil
}

func renderMods(lock []providercontract.ModLockEntry) (string, string, string, error) {
	workshopIDs := make([]string, 0, len(lock))
	names := make([]string, 0, len(lock))
	artifacts := make([]string, 0, len(lock))
	for _, item := range lock {
		name, ok := runtimeNames[item.ModID]
		artifact, artifactOK := runtimeArtifacts[item.ModID]
		if !ok || !artifactOK || item.Version == "" || !strings.HasPrefix(item.Digest, "sha256:") {
			return "", "", "", errors.New("unsupported tModLoader mod lock")
		}
		workshopIDs = append(workshopIDs, item.ModID)
		names = append(names, name)
		artifacts = append(artifacts, item.ModID+" "+artifact)
	}
	sort.Strings(workshopIDs)
	sort.Strings(names)
	sort.Strings(artifacts)
	enabled, err := json.Marshal(names)
	if err != nil {
		return "", "", "", err
	}
	installs := strings.Join(workshopIDs, "\n")
	if installs != "" {
		installs += "\n"
	}
	artifactLock := strings.Join(artifacts, "\n")
	if artifactLock != "" {
		artifactLock += "\n"
	}
	return string(enabled), installs, artifactLock, nil
}

func listeners(intent nodeworkload.Intent) ([]nodeworkload.Listener, error) {
	result := make([]nodeworkload.Listener, 0, len(intent.Endpoints))
	for _, endpoint := range intent.Endpoints {
		if endpoint.Port == nil {
			return nil, errors.New("tModLoader requires an allocated port")
		}
		for _, transport := range endpoint.Transports {
			if transport != "tcp" {
				return nil, errors.New("tModLoader supports TCP endpoints only")
			}
			result = append(result, nodeworkload.Listener{InternalPort: DefaultInternalPort, HostPort: *endpoint.Port, Protocol: transport})
		}
	}
	if len(result) == 0 {
		return nil, errors.New("tModLoader endpoint is missing")
	}
	return result, nil
}

var _ nodeworkload.GameProvider = Provider{}
