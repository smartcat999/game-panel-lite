package terraria

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

type VanillaProvider struct{}
type TModLoaderProvider struct{}

var vanillaVersions = []string{"1.4.5.6", "1.4.4.9"}
var tmodloaderVersions = []string{"v2026.04.3.0", "v2026.02.3.1"}

func NewVanillaProvider() VanillaProvider       { return VanillaProvider{} }
func NewTModLoaderProvider() TModLoaderProvider { return TModLoaderProvider{} }

func (VanillaProvider) Key() domain.ProviderKey { return domain.ProviderTerrariaVanilla }
func (VanillaProvider) Name() string            { return "Terraria Vanilla" }
func (VanillaProvider) Image() string           { return VanillaImageForVersion(vanillaVersions[0]) }
func (VanillaProvider) Versions() []string      { return vanillaVersions }
func (VanillaProvider) ImageFor(version string) string {
	return VanillaImageForVersion(version)
}
func (VanillaProvider) DefaultConfig() domain.TerrariaConfig {
	return Presets[0].Config
}
func (VanillaProvider) ValidateConfig(config domain.TerrariaConfig) error {
	return ValidateConfig(config)
}
func (VanillaProvider) RenderConfig(config domain.TerrariaConfig) (string, error) {
	return RenderServerConfig(config)
}
func (VanillaProvider) RuntimeOptions(config domain.TerrariaConfig) runtime.ContainerOptions {
	return vanillaRuntimeOptions(config)
}
func (VanillaProvider) PlayerListCommand(config domain.TerrariaConfig) string {
	return localizedPlayerListCommand(config)
}
func (VanillaProvider) ParsePlayerListOutput(lines []string) []domain.Player {
	return parsePlayingCommandOutput(lines)
}
func (VanillaProvider) ParsePlayerLogEvent(line string) (domain.PlayerLogEvent, bool) {
	return parsePlayerLogEvent(line)
}

func (TModLoaderProvider) Key() domain.ProviderKey { return domain.ProviderTerrariaTModLoader }
func (TModLoaderProvider) Name() string            { return "Terraria tModLoader" }
func (TModLoaderProvider) Image() string           { return TModLoaderImageForVersion(tmodloaderVersions[0]) }
func (TModLoaderProvider) Versions() []string      { return tmodloaderVersions }
func (TModLoaderProvider) ImageFor(version string) string {
	return TModLoaderImageForVersion(version)
}
func (TModLoaderProvider) DefaultConfig() domain.TerrariaConfig {
	return Presets[4].Config
}
func (TModLoaderProvider) ValidateConfig(config domain.TerrariaConfig) error {
	return ValidateConfig(config)
}
func (TModLoaderProvider) RenderConfig(config domain.TerrariaConfig) (string, error) {
	return RenderServerConfig(config)
}
func (TModLoaderProvider) RuntimeOptions(config domain.TerrariaConfig) runtime.ContainerOptions {
	return tModLoaderRuntimeOptions(config)
}
func (TModLoaderProvider) PlayerListCommand(config domain.TerrariaConfig) string {
	return localizedPlayerListCommand(config)
}
func (TModLoaderProvider) ParsePlayerListOutput(lines []string) []domain.Player {
	return parsePlayingCommandOutput(lines)
}
func (TModLoaderProvider) ParsePlayerLogEvent(line string) (domain.PlayerLogEvent, bool) {
	return parsePlayerLogEvent(line)
}

func RuntimeWorldFiles(providerKey domain.ProviderKey, config domain.TerrariaConfig) []string {
	switch providerKey {
	case domain.ProviderTerrariaTModLoader:
		return []string{filepath.Join("Worlds", config.WorldName+".wld")}
	default:
		return []string{filepath.Join("Worlds", config.WorldName+".wld")}
	}
}

func RuntimeModFiles(providerKey domain.ProviderKey, fileName string) []string {
	if providerKey != domain.ProviderTerrariaTModLoader {
		return nil
	}
	return []string{filepath.Join("Mods", fileName)}
}

func vanillaRuntimeOptions(config domain.TerrariaConfig) runtime.ContainerOptions {
	worldSizes := map[domain.WorldSize]int{"small": 1, "medium": 2, "large": 3}
	return runtime.ContainerOptions{
		Env: []string{
			"HOME=/home/container",
			"WORLD_NAME=" + config.WorldName,
			fmt.Sprintf("WORLD_SIZE=%d", worldSizes[config.WorldSize]),
		},
		Cmd: []string{
			"sh",
			"-c",
			"cd /home/container && exec ./server/gamepanel-terraria-entrypoint.sh -config /home/container/serverconfig.txt -world \"/home/container/Worlds/${WORLD_NAME}.wld\" -autocreate \"${WORLD_SIZE}\" -noupnp",
		},
		DataMounts: []string{
			"Worlds:/home/container/Worlds",
			"logs:/home/container/logs",
			"serverconfig.txt:/home/container/serverconfig.txt",
		},
		Files: map[string]string{
			"serverconfig.txt": renderVanillaRuntimeConfig(config),
		},
	}
}

func tModLoaderRuntimeOptions(config domain.TerrariaConfig) runtime.ContainerOptions {
	worldSizes := map[domain.WorldSize]int{"small": 1, "medium": 2, "large": 3}
	return runtime.ContainerOptions{
		Env: []string{
			"HOME=/home/container",
			"TMOD_HOME=/home/container",
			"WORLD_NAME=" + config.WorldName,
			fmt.Sprintf("WORLD_SIZE=%d", worldSizes[config.WorldSize]),
		},
		Cmd: []string{
			"sh",
			"-c",
			"cd /home/container && exec ./server/start-tModLoaderServer.sh -config /home/container/serverconfig.txt -tmlsavedirectory /home/container -world \"/home/container/Worlds/${WORLD_NAME}.wld\" -autocreate \"${WORLD_SIZE}\" -noupnp",
		},
		DataMounts: []string{
			"Worlds:/home/container/Worlds",
			"Mods:/home/container/Mods",
			"logs:/home/container/logs",
			"steamapps:/home/container/steamapps",
			"serverconfig.txt:/home/container/serverconfig.txt",
		},
		Files: map[string]string{
			"serverconfig.txt": renderTModLoaderRuntimeConfig(config),
		},
	}
}

func TModLoaderImageForVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		version = tmodloaderVersions[0]
	}
	return "smartcat99999/tmodloader:" + version
}

func VanillaImageForVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		version = vanillaVersions[0]
	}
	return "smartcat99999/terraria-vanilla:" + version
}

func localizedPlayerListCommand(config domain.TerrariaConfig) string {
	switch strings.ToLower(strings.TrimSpace(config.Language)) {
	case "zh-hans", "zh-cn", "zh":
		return "游戏中"
	default:
		return "playing"
	}
}

func parsePlayerLogEvent(line string) (domain.PlayerLogEvent, bool) {
	line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), ":"))
	line = strings.TrimSpace(regexp.MustCompile(`^\[[^\]]+\]\s*`).ReplaceAllString(line, ""))
	if line == "" {
		return "", false
	}
	lower := strings.ToLower(line)
	switch {
	case strings.HasSuffix(line, "已加入。") || strings.HasSuffix(line, "已加入.") || strings.HasSuffix(lower, " has joined.") || strings.HasSuffix(lower, " joined."):
		return domain.PlayerJoined, true
	case strings.HasSuffix(line, "已离开。") || strings.HasSuffix(line, "已离开.") || strings.HasSuffix(lower, " has left.") || strings.HasSuffix(lower, " left."):
		return domain.PlayerLeft, true
	default:
		return "", false
	}
}

func parsePlayingCommandOutput(lines []string) []domain.Player {
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "no players") || strings.Contains(line, "无玩家连接") {
			return []domain.Player{}
		}
		if players, ok := parseNamedPlayerLine(line); ok {
			return players
		}
		if count, ok := parsePlayerCountLine(line); ok {
			if players := parsePlayerRowsBefore(lines[:i], count); len(players) > 0 {
				return players
			}
			return unnamedPlayers(count)
		}
	}
	return nil
}

func parseNamedPlayerLine(line string) ([]domain.Player, bool) {
	index := strings.LastIndex(line, ":")
	if index < 0 {
		return nil, false
	}
	label := strings.ToLower(strings.TrimSpace(line[:index]))
	if !strings.Contains(label, "player") {
		return nil, false
	}
	namesText := strings.TrimSpace(line[index+1:])
	if namesText == "" {
		return []domain.Player{}, true
	}
	names := strings.FieldsFunc(namesText, func(r rune) bool {
		return r == ',' || r == ';'
	})
	players := make([]domain.Player, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		players = append(players, domain.Player{Name: name})
	}
	return players, true
}

func parsePlayerRowsBefore(lines []string, count int) []domain.Player {
	if count <= 0 {
		return []domain.Player{}
	}
	players := make([]domain.Player, 0, count)
	for i := len(lines) - 1; i >= 0; i-- {
		name, ok := parsePlayerRow(lines[i])
		if !ok {
			if len(players) > 0 {
				break
			}
			continue
		}
		players = append([]domain.Player{{Name: name}}, players...)
		if len(players) == count {
			break
		}
	}
	return players
}

func parsePlayerRow(line string) (string, bool) {
	line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), ":"))
	if line == "" {
		return "", false
	}
	index := strings.LastIndex(line, "(")
	if index <= 0 || !strings.HasSuffix(line, ")") {
		return "", false
	}
	name := strings.TrimSpace(line[:index])
	if name == "" {
		return "", false
	}
	return name, true
}

func parsePlayerCountLine(line string) (int, bool) {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "player") && strings.Contains(lower, "connected") {
		match := regexp.MustCompile(`(?i)(\d+)\s+players?\s+connected`).FindStringSubmatch(line)
		if len(match) != 2 {
			return 0, false
		}
		count, err := strconv.Atoi(match[1])
		if err != nil {
			return 0, false
		}
		return count, true
	}
	match := regexp.MustCompile(`(\d+)\s*个玩家已连接`).FindStringSubmatch(line)
	if len(match) != 2 {
		return 0, false
	}
	count, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, false
	}
	return count, true
}

func unnamedPlayers(count int) []domain.Player {
	if count <= 0 {
		return []domain.Player{}
	}
	players := make([]domain.Player, count)
	return players
}

func renderVanillaRuntimeConfig(config domain.TerrariaConfig) string {
	config = NormalizeConfig(config)
	worldSizes := map[domain.WorldSize]int{"small": 1, "medium": 2, "large": 3}
	worldEvils := map[domain.WorldEvil]int{"": 0, "random": 0, "corruption": 1, "crimson": 2}
	difficulties := map[domain.Difficulty]int{"journey": 0, "classic": 1, "expert": 2, "master": 3}
	lines := []string{
		fmt.Sprintf("world=/home/container/Worlds/%s.wld", config.WorldName),
		fmt.Sprintf("autocreate=%d", worldSizes[config.WorldSize]),
		fmt.Sprintf("worldname=%s", config.WorldName),
		fmt.Sprintf("worldevil=%d", worldEvils[config.WorldEvil]),
		fmt.Sprintf("difficulty=%d", difficulties[config.Difficulty]),
		fmt.Sprintf("maxplayers=%d", config.MaxPlayers),
		fmt.Sprintf("port=%d", config.Port),
		fmt.Sprintf("password=%s", config.Password),
		fmt.Sprintf("motd=%s", value(config.MOTD, "Welcome to GamePanel Lite")),
		fmt.Sprintf("seed=%s", config.Seed),
		fmt.Sprintf("worldpath=%s", "/home/container/Worlds"),
		fmt.Sprintf("secure=%d", boolInt(config.Secure)),
		fmt.Sprintf("language=%s", value(config.Language, DefaultLanguage)),
		"upnp=0",
		"priority=1",
	}
	return strings.Join(lines, "\n")
}

func renderTModLoaderRuntimeConfig(config domain.TerrariaConfig) string {
	config = NormalizeConfig(config)
	worldSizes := map[domain.WorldSize]int{"small": 1, "medium": 2, "large": 3}
	worldEvils := map[domain.WorldEvil]int{"": 0, "random": 0, "corruption": 1, "crimson": 2}
	difficulties := map[domain.Difficulty]int{"journey": 0, "classic": 1, "expert": 2, "master": 3}
	lines := []string{
		fmt.Sprintf("world=/home/container/Worlds/%s.wld", config.WorldName),
		fmt.Sprintf("autocreate=%d", worldSizes[config.WorldSize]),
		fmt.Sprintf("worldname=%s", config.WorldName),
		fmt.Sprintf("worldevil=%d", worldEvils[config.WorldEvil]),
		fmt.Sprintf("difficulty=%d", difficulties[config.Difficulty]),
		fmt.Sprintf("maxplayers=%d", config.MaxPlayers),
		fmt.Sprintf("port=%d", config.Port),
		fmt.Sprintf("password=%s", config.Password),
		fmt.Sprintf("motd=%s", value(config.MOTD, "Welcome to GamePanel Lite")),
		fmt.Sprintf("seed=%s", config.Seed),
		fmt.Sprintf("worldpath=%s", "/home/container/Worlds"),
		fmt.Sprintf("secure=%d", boolInt(config.Secure)),
		fmt.Sprintf("language=%s", value(config.Language, DefaultLanguage)),
		"upnp=0",
		"priority=1",
	}
	return strings.Join(lines, "\n")
}
