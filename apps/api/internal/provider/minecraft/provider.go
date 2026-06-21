package minecraft

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/runtimecatalog"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

const (
	DefaultInternalPort = 25565
	runtimeImage        = "itzg/minecraft-server:latest"
)

var versions = []string{"latest", "1.21.4", "1.21", "1.20.6", "1.20.4", "1.20.1", "1.19.4", "1.19.2"}

type Provider struct {
	runtime runtimecatalog.RuntimeConfig
}

type Config struct {
	ServerName       string
	WorldName        string
	MaxPlayers       int
	Port             int
	EULAAccepted     bool
	GameMode         string
	Difficulty       string
	OnlineMode       bool
	WhitelistEnabled bool
}

func NewProvider(catalog ...runtimecatalog.Catalog) Provider {
	return Provider{
		runtime: runtimecatalog.FromCatalog(catalog, domain.ProviderMinecraft, runtimeConfig()),
	}
}

func runtimeConfig() runtimecatalog.RuntimeConfig {
	return runtimecatalog.RuntimeConfig{Image: runtimeImage, Versions: versions}
}

func (Provider) GameKey() domain.GameKey { return domain.GameMinecraft }
func (Provider) Key() domain.ProviderKey { return domain.ProviderMinecraft }
func (Provider) Name() string            { return "Minecraft Java" }
func (Provider) Description() string {
	return "Vanilla Minecraft Java Edition dedicated server for friend groups."
}
func (Provider) Capabilities() domain.ProviderCapabilities {
	return domain.ProviderCapabilities{
		ConsoleCommands: true,
		PlayerList:      true,
		KickPlayer:      true,
		BanPlayer:       true,
		Whitelist:       true,
		SaveSnapshots:   true,
		Backups:         true,
		Mods:            false,
		Versions:        true,
	}
}
func (Provider) SaveDisplayName() string { return "world" }
func (Provider) KickCommand(player string) string {
	return "kick " + sanitizePlayerName(player)
}
func (Provider) BanCommand(player string) string {
	return "ban " + sanitizePlayerName(player)
}
func (Provider) WhitelistAddCommand(player string) string {
	return "whitelist add " + sanitizePlayerName(player)
}
func (Provider) WhitelistRemoveCommand(player string) string {
	return "whitelist remove " + sanitizePlayerName(player)
}
func (Provider) WhitelistListCommand() string {
	return "whitelist list"
}
func (Provider) ConfigSchema() []domain.ProviderConfigField {
	return configSchema()
}
func (p Provider) Image() string      { return p.ImageFor(p.Versions()[0]) }
func (p Provider) Versions() []string { return p.runtime.WithFallback(runtimeConfig()).VersionList() }
func (p Provider) ImageFor(version string) string {
	return p.runtime.WithFallback(runtimeConfig()).ImageFor(version)
}
func defaultConfig() Config {
	return normalizeConfig(Config{OnlineMode: true})
}
func (p Provider) DefaultConfigPayload() map[string]any {
	return payloadFromConfig(defaultConfig())
}
func (p Provider) NormalizeConfigPayload(payload map[string]any) (map[string]any, error) {
	config := configFromPayload(payload, defaultConfig())
	return payloadFromConfig(config), nil
}
func (p Provider) ValidateConfigPayload(payload map[string]any) error {
	config := configFromPayload(payload, defaultConfig())
	return validateConfig(config)
}
func (p Provider) ConfigSummary(payload map[string]any) (domain.ProviderConfigSummary, error) {
	config := configFromPayload(payload, defaultConfig())
	return domain.ProviderConfigSummary{
		ServerName: config.ServerName,
		WorldName:  config.WorldName,
		MaxPlayers: config.MaxPlayers,
		Port:       config.Port,
		Secure:     config.EULAAccepted,
	}, nil
}
func validateConfig(config Config) error {
	config = normalizeConfig(config)
	if strings.TrimSpace(config.ServerName) == "" {
		return fmt.Errorf("server name is required")
	}
	if strings.Contains(config.ServerName, "/") || strings.Contains(config.ServerName, "\\") {
		return fmt.Errorf("server name contains unsupported path characters")
	}
	if strings.TrimSpace(config.WorldName) == "" {
		return fmt.Errorf("world name is required")
	}
	if strings.Contains(config.WorldName, "/") || strings.Contains(config.WorldName, "\\") || strings.Contains(config.WorldName, "..") {
		return fmt.Errorf("world name contains unsupported path characters")
	}
	if config.MaxPlayers < 1 || config.MaxPlayers > 100 {
		return fmt.Errorf("max players must be between 1 and 100")
	}
	if !config.EULAAccepted {
		return fmt.Errorf("EULA must be accepted before creating a Minecraft server")
	}
	return nil
}
func renderConfig(config Config) (string, error) {
	config = normalizeConfig(config)
	if err := validateConfig(config); err != nil {
		return "", err
	}
	return renderServerProperties(config), nil
}
func runtimeOptions(config Config) runtime.ContainerOptions {
	config = normalizeConfig(config)
	return runtime.ContainerOptions{
		Env: []string{
			"EULA=TRUE",
			"TYPE=VANILLA",
			versionEnv(NewProvider().Versions()[0]),
			fmt.Sprintf("SERVER_PORT=%d", config.Port),
			fmt.Sprintf("MAX_PLAYERS=%d", config.MaxPlayers),
			"MOTD=" + config.ServerName,
			"LEVEL_NAME=" + config.WorldName,
			"MODE=" + config.GameMode,
			"DIFFICULTY=" + config.Difficulty,
			fmt.Sprintf("ONLINE_MODE=%t", config.OnlineMode),
			fmt.Sprintf("WHITE_LIST=%t", config.WhitelistEnabled),
		},
		DataMounts:   []string{"/data"},
		PortProtocol: "tcp",
		Files: map[string]string{
			"data/server.properties": renderServerProperties(config),
			"data/eula.txt":          "eula=true\n",
		},
	}
}

func (p Provider) RuntimeConfigForResource(server domain.GameServer) (domain.ProviderRuntimeConfig, error) {
	config := configFromPayload(server.Spec.Config, defaultConfig())
	if server.Spec.Network.Port != 0 {
		config.Port = server.Spec.Network.Port
	}
	if err := validateConfig(config); err != nil {
		return domain.ProviderRuntimeConfig{}, err
	}
	options := runtime.ContainerOptions{
		Env: []string{
			"EULA=TRUE",
			"TYPE=VANILLA",
			versionEnv(server.Spec.Version),
			fmt.Sprintf("SERVER_PORT=%d", config.Port),
			fmt.Sprintf("MAX_PLAYERS=%d", config.MaxPlayers),
			"MOTD=" + config.ServerName,
			"LEVEL_NAME=" + config.WorldName,
			"MODE=" + config.GameMode,
			"DIFFICULTY=" + config.Difficulty,
			fmt.Sprintf("ONLINE_MODE=%t", config.OnlineMode),
			fmt.Sprintf("WHITE_LIST=%t", config.WhitelistEnabled),
		},
		DataMounts:   []string{"/data"},
		PortProtocol: "tcp",
		Files: map[string]string{
			"data/server.properties": renderServerProperties(config),
			"data/eula.txt":          "eula=true\n",
		},
	}
	return domain.ProviderRuntimeConfig{
		Port:     config.Port,
		Protocol: options.PortProtocol,
		Options:  workloadOptions(options),
	}, nil
}

func (Provider) JoinInfo(server domain.GameServer) domain.ServerJoinInfo {
	address := "127.0.0.1"
	port := server.Spec.Network.HostPort
	if port == 0 {
		port = server.Spec.Network.Port
	}
	return domain.ServerJoinInfo{
		Address:    address,
		Port:       port,
		InviteText: fmt.Sprintf("Join %s on Minecraft Java at %s:%d", server.Name, address, port),
		Instructions: []string{
			"Open Minecraft Java Edition.",
			"Add a server or use Direct Connect with the address shown here.",
		},
	}
}

func normalizeConfig(config Config) Config {
	if strings.TrimSpace(config.ServerName) == "" {
		config.ServerName = "Friends Server"
	}
	if strings.TrimSpace(config.WorldName) == "" {
		config.WorldName = "world"
	}
	if config.MaxPlayers == 0 {
		config.MaxPlayers = 20
	}
	config.Port = DefaultInternalPort
	if strings.TrimSpace(config.GameMode) == "" {
		config.GameMode = "survival"
	}
	if strings.TrimSpace(config.Difficulty) == "" {
		config.Difficulty = "normal"
	}
	return config
}

func configFromPayload(payload map[string]any, fallback Config) Config {
	config := normalizeConfig(fallback)
	if value := stringPayload(payload, "serverName"); value != "" {
		config.ServerName = value
	}
	if value := stringPayload(payload, "worldName"); value != "" {
		config.WorldName = value
	}
	if value, ok := intPayload(payload, "maxPlayers"); ok {
		config.MaxPlayers = value
	}
	if value, ok := payload["eulaAccepted"]; ok {
		config.EULAAccepted = boolValue(value)
	}
	if value := stringPayload(payload, "gameMode"); value != "" {
		config.GameMode = value
	}
	if value := stringPayload(payload, "difficulty"); value != "" {
		config.Difficulty = value
	}
	if value, ok := payload["onlineMode"]; ok {
		config.OnlineMode = boolValue(value)
	}
	if value, ok := payload["whitelistEnabled"]; ok {
		config.WhitelistEnabled = boolValue(value)
	}
	return normalizeConfig(config)
}

func payloadFromConfig(config Config) map[string]any {
	config = normalizeConfig(config)
	out := map[string]any{
		"serverName":       config.ServerName,
		"worldName":        config.WorldName,
		"maxPlayers":       config.MaxPlayers,
		"gameMode":         config.GameMode,
		"difficulty":       config.Difficulty,
		"onlineMode":       config.OnlineMode,
		"whitelistEnabled": config.WhitelistEnabled,
		"eulaAccepted":     config.EULAAccepted,
	}
	return out
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

func renderServerProperties(config Config) string {
	lines := []string{
		"# Minecraft server properties - managed by GamePanel Lite",
		"server-port=" + fmt.Sprintf("%d", config.Port),
		"max-players=" + fmt.Sprintf("%d", config.MaxPlayers),
		"motd=" + config.ServerName,
		"level-name=" + config.WorldName,
		"gamemode=" + config.GameMode,
		"difficulty=" + config.Difficulty,
		fmt.Sprintf("online-mode=%t", config.OnlineMode),
		fmt.Sprintf("white-list=%t", config.WhitelistEnabled),
		"enforce-whitelist=true",
		"allow-flight=false",
		"pvp=true",
		"enable-command-block=true",
		"spawn-protection=16",
		"view-distance=10",
		"simulation-distance=10",
	}
	sort.Strings(lines[1:])
	return strings.Join(lines, "\n") + "\n"
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

func intPayload(payload map[string]any, key string) (int, bool) {
	value, ok := payload[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		got, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(got), true
	default:
		return 0, false
	}
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	case float64:
		return typed != 0
	case int:
		return typed != 0
	default:
		return false
	}
}

func ImageForVersion(version string) string {
	return runtimeImage
}

func versionEnv(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || version == "latest" {
		return "VERSION=LATEST"
	}
	return "VERSION=" + version
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
