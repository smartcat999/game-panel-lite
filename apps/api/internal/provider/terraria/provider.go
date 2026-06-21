package terraria

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/runtimecatalog"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

type VanillaProvider struct {
	runtime runtimecatalog.RuntimeConfig
}
type TModLoaderProvider struct {
	runtime runtimecatalog.RuntimeConfig
}

var vanillaVersions = []string{"1.4.5.6"}
var tmodloaderVersions = []string{"v2026.04.3.0", "v2026.02.3.1"}

func NewVanillaProvider(catalog ...runtimecatalog.Catalog) VanillaProvider {
	return VanillaProvider{
		runtime: runtimecatalog.FromCatalog(catalog, domain.ProviderTerrariaVanilla, vanillaRuntimeConfig()),
	}
}
func NewTModLoaderProvider(catalog ...runtimecatalog.Catalog) TModLoaderProvider {
	return TModLoaderProvider{
		runtime: runtimecatalog.FromCatalog(catalog, domain.ProviderTerrariaTModLoader, tmodLoaderRuntimeConfig()),
	}
}

func vanillaRuntimeConfig() runtimecatalog.RuntimeConfig {
	return runtimecatalog.RuntimeConfig{ImageTemplate: "smartcat99999/terraria-vanilla:{version}", Versions: vanillaVersions}
}

func tmodLoaderRuntimeConfig() runtimecatalog.RuntimeConfig {
	return runtimecatalog.RuntimeConfig{ImageTemplate: "smartcat99999/tmodloader:{version}", Versions: tmodloaderVersions}
}

func (VanillaProvider) GameKey() domain.GameKey { return domain.GameTerraria }
func (VanillaProvider) Key() domain.ProviderKey { return domain.ProviderTerrariaVanilla }
func (VanillaProvider) Name() string            { return "Terraria Vanilla" }
func (VanillaProvider) Description() string {
	return "Official Terraria dedicated server for classic worlds."
}
func (VanillaProvider) Capabilities() domain.ProviderCapabilities {
	return vanillaCapabilities()
}
func (VanillaProvider) ConfigSchema() []domain.ProviderConfigField {
	return configSchema()
}
func (VanillaProvider) SaveDisplayName() string { return "world" }
func (p VanillaProvider) Image() string         { return p.ImageFor(p.Versions()[0]) }
func (p VanillaProvider) Versions() []string {
	return p.runtime.WithFallback(vanillaRuntimeConfig()).VersionList()
}
func (p VanillaProvider) ImageFor(version string) string {
	return p.runtime.WithFallback(vanillaRuntimeConfig()).ImageFor(version)
}
func (VanillaProvider) DefaultConfig() Config {
	return Presets[0].Config
}
func (p VanillaProvider) DefaultConfigPayload() map[string]any {
	return terrariaPayloadFromConfig(p.DefaultConfig())
}
func (p VanillaProvider) NormalizeConfigPayload(payload map[string]any) (map[string]any, error) {
	config, err := terrariaConfigFromPayload(payload, p.DefaultConfig())
	if err != nil {
		return nil, err
	}
	return terrariaPayloadFromConfig(config), nil
}
func (p VanillaProvider) ValidateConfigPayload(payload map[string]any) error {
	config, err := terrariaConfigFromPayload(payload, p.DefaultConfig())
	if err != nil {
		return err
	}
	return p.ValidateConfig(config)
}
func (p VanillaProvider) ConfigSummary(payload map[string]any) (domain.ProviderConfigSummary, error) {
	config, err := terrariaConfigFromPayload(payload, p.DefaultConfig())
	if err != nil {
		return domain.ProviderConfigSummary{}, err
	}
	return summaryFromConfig(config), nil
}
func (VanillaProvider) ValidateConfig(config Config) error {
	return ValidateConfig(config)
}
func (VanillaProvider) RenderConfig(config Config) (string, error) {
	return RenderServerConfig(config)
}
func (VanillaProvider) RuntimeOptions(config Config) runtime.ContainerOptions {
	return vanillaRuntimeOptions(config)
}
func (VanillaProvider) RuntimeConfigForResource(server domain.GameServer) (domain.ProviderRuntimeConfig, error) {
	return resourceRuntimeConfig(server, Presets[0].Config, vanillaRuntimeOptions)
}
func (VanillaProvider) JoinInfo(server domain.GameServer) domain.ServerJoinInfo {
	return terrariaJoinInfo(server)
}
func (VanillaProvider) PlayerListCommand(server domain.GameServer) string {
	config, err := configFromResource(server, Presets[0].Config)
	if err != nil {
		config = Presets[0].Config
	}
	return localizedPlayerListCommand(config)
}
func (VanillaProvider) KickCommand(player string) string {
	return "kick " + sanitizePlayerName(player)
}
func (VanillaProvider) BanCommand(player string) string {
	return "ban " + sanitizePlayerName(player)
}
func (VanillaProvider) ParsePlayerListOutput(lines []string) []domain.Player {
	return parsePlayingCommandOutput(lines)
}
func (VanillaProvider) ParsePlayerLogEvent(line string) (domain.PlayerLogEvent, bool) {
	return parsePlayerLogEvent(line)
}

func (TModLoaderProvider) GameKey() domain.GameKey { return domain.GameTerraria }
func (TModLoaderProvider) Key() domain.ProviderKey { return domain.ProviderTerrariaTModLoader }
func (TModLoaderProvider) Name() string            { return "Terraria tModLoader" }
func (TModLoaderProvider) Description() string {
	return "Terraria server with tModLoader mod support."
}
func (TModLoaderProvider) Capabilities() domain.ProviderCapabilities {
	capabilities := vanillaCapabilities()
	capabilities.Mods = true
	return capabilities
}
func (TModLoaderProvider) ConfigSchema() []domain.ProviderConfigField {
	return configSchema()
}
func (TModLoaderProvider) SaveDisplayName() string { return "world" }
func (p TModLoaderProvider) Image() string         { return p.ImageFor(p.Versions()[0]) }
func (p TModLoaderProvider) Versions() []string {
	return p.runtime.WithFallback(tmodLoaderRuntimeConfig()).VersionList()
}
func (p TModLoaderProvider) ImageFor(version string) string {
	return p.runtime.WithFallback(tmodLoaderRuntimeConfig()).ImageFor(version)
}
func (TModLoaderProvider) DefaultConfig() Config {
	return Presets[4].Config
}
func (p TModLoaderProvider) DefaultConfigPayload() map[string]any {
	return terrariaPayloadFromConfig(p.DefaultConfig())
}
func (p TModLoaderProvider) NormalizeConfigPayload(payload map[string]any) (map[string]any, error) {
	config, err := terrariaConfigFromPayload(payload, p.DefaultConfig())
	if err != nil {
		return nil, err
	}
	return terrariaPayloadFromConfig(config), nil
}
func (p TModLoaderProvider) ValidateConfigPayload(payload map[string]any) error {
	config, err := terrariaConfigFromPayload(payload, p.DefaultConfig())
	if err != nil {
		return err
	}
	return p.ValidateConfig(config)
}
func (p TModLoaderProvider) ConfigSummary(payload map[string]any) (domain.ProviderConfigSummary, error) {
	config, err := terrariaConfigFromPayload(payload, p.DefaultConfig())
	if err != nil {
		return domain.ProviderConfigSummary{}, err
	}
	return summaryFromConfig(config), nil
}
func (TModLoaderProvider) ValidateConfig(config Config) error {
	return ValidateConfig(config)
}
func (TModLoaderProvider) RenderConfig(config Config) (string, error) {
	return RenderServerConfig(config)
}
func (TModLoaderProvider) RuntimeOptions(config Config) runtime.ContainerOptions {
	return tModLoaderRuntimeOptions(config)
}
func (TModLoaderProvider) RuntimeConfigForResource(server domain.GameServer) (domain.ProviderRuntimeConfig, error) {
	return resourceRuntimeConfig(server, Presets[4].Config, tModLoaderRuntimeOptions)
}
func (TModLoaderProvider) JoinInfo(server domain.GameServer) domain.ServerJoinInfo {
	return terrariaJoinInfo(server)
}
func (TModLoaderProvider) PlayerListCommand(server domain.GameServer) string {
	config, err := configFromResource(server, Presets[4].Config)
	if err != nil {
		config = Presets[4].Config
	}
	return localizedPlayerListCommand(config)
}
func (TModLoaderProvider) KickCommand(player string) string {
	return "kick " + sanitizePlayerName(player)
}
func (TModLoaderProvider) BanCommand(player string) string {
	return "ban " + sanitizePlayerName(player)
}
func (TModLoaderProvider) ParsePlayerListOutput(lines []string) []domain.Player {
	return parsePlayingCommandOutput(lines)
}
func (TModLoaderProvider) ParsePlayerLogEvent(line string) (domain.PlayerLogEvent, bool) {
	return parsePlayerLogEvent(line)
}

func terrariaConfigFromPayload(payload map[string]any, fallback Config) (Config, error) {
	config := NormalizeConfig(fallback)
	if len(payload) == 0 {
		return config, nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Config{}, err
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return Config{}, fmt.Errorf("invalid config payload")
	}
	return NormalizeConfig(config), nil
}

func ConfigFromPayload(payload map[string]any, fallback Config) (Config, error) {
	return terrariaConfigFromPayload(payload, fallback)
}

func terrariaPayloadFromConfig(config Config) map[string]any {
	config = NormalizeConfig(config)
	payload := map[string]any{
		"serverName":      config.ServerName,
		"worldName":       config.WorldName,
		"worldSize":       config.WorldSize,
		"worldEvil":       config.WorldEvil,
		"difficulty":      config.Difficulty,
		"maxPlayers":      config.MaxPlayers,
		"port":            config.Port,
		"motd":            config.MOTD,
		"secure":          config.Secure,
		"language":        config.Language,
		"autoCreateWorld": config.AutoCreateWorld,
	}
	if config.Password != "" {
		payload["password"] = config.Password
	}
	if config.Seed != "" {
		payload["seed"] = config.Seed
	}
	return payload
}

func PayloadFromConfig(config Config) map[string]any {
	return terrariaPayloadFromConfig(config)
}

func summaryFromConfig(config Config) domain.ProviderConfigSummary {
	config = NormalizeConfig(config)
	return domain.ProviderConfigSummary{
		ServerName: config.ServerName,
		WorldName:  config.WorldName,
		MaxPlayers: config.MaxPlayers,
		Port:       config.Port,
		Password:   config.Password,
		MOTD:       config.MOTD,
		Secure:     config.Secure,
	}
}

func vanillaCapabilities() domain.ProviderCapabilities {
	return domain.ProviderCapabilities{
		ConsoleCommands: true,
		PlayerList:      true,
		KickPlayer:      true,
		BanPlayer:       true,
		SaveSnapshots:   true,
		Backups:         true,
		Versions:        true,
	}
}

func terrariaJoinInfo(server domain.GameServer) domain.ServerJoinInfo {
	address := defaultJoinAddress
	port := joinPort(server)
	config, err := configFromResource(server, Config{})
	password := ""
	if err == nil {
		password = config.Password
	}
	invite := fmt.Sprintf("Join %s in Terraria at %s:%d", server.Name, address, port)
	if password != "" {
		invite += " password: " + password
	}
	return domain.ServerJoinInfo{
		Address:    address,
		Port:       port,
		Password:   password,
		InviteText: invite,
		Instructions: []string{
			"Open Terraria multiplayer.",
			"Choose Join via IP and enter the address and port.",
		},
	}
}

const defaultJoinAddress = "127.0.0.1"

func joinPort(server domain.GameServer) int {
	if server.Spec.Network.HostPort > 0 {
		return server.Spec.Network.HostPort
	}
	return server.Spec.Network.Port
}

func configSchema() []domain.ProviderConfigField {
	return []domain.ProviderConfigField{
		{Name: "serverName", Label: "服务器名称", Type: "text", Required: true, Default: "Friends Server"},
		{Name: "worldName", Label: "世界名称", Type: "text", Required: true, Default: "Friends World"},
		{
			Name: "worldSize", Label: "世界大小", Type: "select", Required: true, Default: "medium",
			Options: []domain.ProviderConfigFieldOption{
				{Value: "small", Label: "小型世界"},
				{Value: "medium", Label: "中型世界"},
				{Value: "large", Label: "大型世界"},
			},
		},
		{
			Name: "difficulty", Label: "难度", Type: "select", Required: true, Default: "classic",
			Options: []domain.ProviderConfigFieldOption{
				{Value: "classic", Label: "经典"},
				{Value: "expert", Label: "专家"},
				{Value: "master", Label: "大师"},
				{Value: "journey", Label: "旅途"},
			},
		},
		{
			Name: "worldEvil", Label: "世界邪恶地形", Type: "select", Required: true, Default: "random",
			Options: []domain.ProviderConfigFieldOption{
				{Value: "random", Label: "随机"},
				{Value: "corruption", Label: "腐化之地"},
				{Value: "crimson", Label: "猩红之地"},
			},
		},
		{Name: "maxPlayers", Label: "最大玩家数", Type: "number", Required: true, Default: 8},
		{Name: "password", Label: "服务器密码", Type: "password", Required: false},
		{Name: "motd", Label: "服务器公告", Type: "text", Required: false, Default: "Welcome to GamePanel Lite"},
		{Name: "seed", Label: "世界种子", Type: "text", Required: false},
	}
}

func RuntimeWorldFiles(providerKey domain.ProviderKey, config Config) []string {
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

func resourceRuntimeConfig(server domain.GameServer, fallback Config, optionsFor func(Config) runtime.ContainerOptions) (domain.ProviderRuntimeConfig, error) {
	config, err := configFromResource(server, fallback)
	if err != nil {
		return domain.ProviderRuntimeConfig{}, err
	}
	config = NormalizeConfig(config)
	if err := ValidateConfig(config); err != nil {
		return domain.ProviderRuntimeConfig{}, err
	}
	configText, err := RenderServerConfig(config)
	if err != nil {
		return domain.ProviderRuntimeConfig{}, err
	}
	options := optionsFor(config)
	return domain.ProviderRuntimeConfig{
		Port:       config.Port,
		Protocol:   options.PortProtocol,
		ConfigText: configText,
		Options:    workloadOptions(options),
	}, nil
}

func configFromResource(server domain.GameServer, fallback Config) (Config, error) {
	config := NormalizeConfig(fallback)
	if server.Spec.Config != nil {
		buf, err := json.Marshal(server.Spec.Config)
		if err != nil {
			return Config{}, err
		}
		if err := json.Unmarshal(buf, &config); err != nil {
			return Config{}, fmt.Errorf("invalid Terraria config payload")
		}
	}
	if server.Spec.Network.Port != 0 {
		config.Port = server.Spec.Network.Port
	}
	return NormalizeConfig(config), nil
}

func workloadOptions(options runtime.ContainerOptions) domain.WorkloadOptions {
	return domain.WorkloadOptions{
		Env:        append([]string{}, options.Env...),
		Cmd:        append([]string{}, options.Cmd...),
		Files:      cloneFiles(options.Files),
		DataMounts: append([]string{}, options.DataMounts...),
	}
}

func cloneFiles(files map[string]string) map[string]string {
	if files == nil {
		return nil
	}
	clone := make(map[string]string, len(files))
	for key, value := range files {
		clone[key] = value
	}
	return clone
}

func vanillaRuntimeOptions(config Config) runtime.ContainerOptions {
	worldSizes := map[WorldSize]int{"small": 1, "medium": 2, "large": 3}
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

func tModLoaderRuntimeOptions(config Config) runtime.ContainerOptions {
	worldSizes := map[WorldSize]int{"small": 1, "medium": 2, "large": 3}
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
			"cd /home/container && exec ./gamepanel-tmodloader-entrypoint.sh -nosteam -config /home/container/serverconfig.txt -tmlsavedirectory /home/container -world \"/home/container/Worlds/${WORLD_NAME}.wld\" -autocreate \"${WORLD_SIZE}\" -noupnp",
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

func localizedPlayerListCommand(config Config) string {
	switch strings.ToLower(strings.TrimSpace(config.Language)) {
	case "zh-hans", "zh-cn", "zh":
		return "游戏中"
	default:
		return "playing"
	}
}

func sanitizePlayerName(player string) string {
	player = strings.TrimSpace(player)
	player = strings.Trim(player, "\"'")
	player = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ';' {
			return -1
		}
		return r
	}, player)
	return player
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

func renderVanillaRuntimeConfig(config Config) string {
	config = NormalizeConfig(config)
	worldSizes := map[WorldSize]int{"small": 1, "medium": 2, "large": 3}
	worldEvils := map[WorldEvil]int{"": 0, "random": 0, "corruption": 1, "crimson": 2}
	difficulties := map[Difficulty]int{"classic": 0, "expert": 1, "master": 2, "journey": 3}
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
		fmt.Sprintf("seed=%s", renderSeedValue(config)),
		fmt.Sprintf("worldpath=%s", "/home/container/Worlds"),
		fmt.Sprintf("secure=%d", boolInt(config.Secure)),
		fmt.Sprintf("language=%s", value(config.Language, DefaultLanguage)),
		"upnp=0",
		"priority=1",
	}
	return strings.Join(lines, "\n")
}

func renderTModLoaderRuntimeConfig(config Config) string {
	config = NormalizeConfig(config)
	worldSizes := map[WorldSize]int{"small": 1, "medium": 2, "large": 3}
	worldEvils := map[WorldEvil]int{"": 0, "random": 0, "corruption": 1, "crimson": 2}
	difficulties := map[Difficulty]int{"classic": 0, "expert": 1, "master": 2, "journey": 3}
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
		fmt.Sprintf("seed=%s", renderSeedValue(config)),
		fmt.Sprintf("worldpath=%s", "/home/container/Worlds"),
		fmt.Sprintf("secure=%d", boolInt(config.Secure)),
		fmt.Sprintf("language=%s", value(config.Language, DefaultLanguage)),
		"upnp=0",
		"priority=1",
	}
	return strings.Join(lines, "\n")
}

func renderSeedValue(config Config) string {
	seed := strings.TrimSpace(config.Seed)
	modes := append([]string{}, normalizedSeedList(config.SpecialSeeds)...)
	modes = append(modes, normalizedSeedList(config.SecretSeeds)...)
	if len(modes) == 0 {
		return seed
	}
	if seed == "" {
		seed = "0"
	}
	return fmt.Sprintf("1.1.1.%s.%s|", seed, strings.Join(modes, "|"))
}

func normalizedSeedList(values []string) []string {
	next := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		item := strings.TrimSpace(value)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if seen[key] {
			continue
		}
		seen[key] = true
		next = append(next, item)
	}
	return next
}
