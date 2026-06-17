package minecraft

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

const DefaultInternalPort = 25565

var versions = []string{"latest", "1.21.4", "1.21", "1.20.6", "1.20.4", "1.20.1", "1.19.4", "1.19.2"}

type Provider struct{}

func NewProvider() Provider { return Provider{} }

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
func (Provider) ConfigSchema() []domain.ProviderConfigField {
	return []domain.ProviderConfigField{
		{Name: "serverName", Label: "服务器名称 / MOTD", Type: "text", Required: true, Default: "Friends Server"},
		{Name: "worldName", Label: "世界名称", Type: "text", Required: true, Default: "world"},
		{Name: "maxPlayers", Label: "最大玩家数", Type: "number", Required: true, Default: 20},
		{
			Name:     "gameMode",
			Label:    "游戏模式",
			Type:     "select",
			Required: true,
			Default:  "survival",
			Options: []domain.ProviderConfigFieldOption{
				{Value: "survival", Label: "生存"},
				{Value: "creative", Label: "创造"},
				{Value: "adventure", Label: "冒险"},
				{Value: "spectator", Label: "旁观"},
			},
		},
		{
			Name:     "difficulty",
			Label:    "难度",
			Type:     "select",
			Required: true,
			Default:  "normal",
			Options: []domain.ProviderConfigFieldOption{
				{Value: "peaceful", Label: "和平"},
				{Value: "easy", Label: "简单"},
				{Value: "normal", Label: "普通"},
				{Value: "hard", Label: "困难"},
			},
		},
		{Name: "onlineMode", Label: "正版验证 (online-mode)", Type: "boolean", Required: false, Default: true, Help: "关闭后允许非正版账号加入，建议仅用于私人好友服。"},
		{Name: "whitelistEnabled", Label: "启用白名单", Type: "boolean", Required: false, Default: false, Help: "开启后仅白名单内玩家可加入，需要后续手动添加玩家。"},
		{Name: "eulaAccepted", Label: "我已阅读并接受 Minecraft EULA", Type: "boolean", Required: true, Default: false, Help: "运行 Minecraft 服务器必须接受最终用户许可协议。"},
	}
}
func (Provider) Image() string      { return ImageForVersion(versions[0]) }
func (Provider) Versions() []string { return append([]string{}, versions...) }
func (Provider) ImageFor(version string) string {
	return ImageForVersion(version)
}
func (Provider) DefaultConfig() domain.TerrariaConfig {
	return NormalizeConfig(domain.TerrariaConfig{})
}
func (Provider) ValidateConfig(config domain.TerrariaConfig) error {
	config = NormalizeConfig(config)
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
	if !config.Secure {
		return fmt.Errorf("EULA must be accepted before creating a Minecraft server")
	}
	return nil
}
func (Provider) RenderConfig(config domain.TerrariaConfig) (string, error) {
	config = NormalizeConfig(config)
	if err := (Provider{}).ValidateConfig(config); err != nil {
		return "", err
	}
	return renderServerProperties(config, settingsFromPayload(map[string]any{})), nil
}
func (Provider) RuntimeOptions(config domain.TerrariaConfig) runtime.ContainerOptions {
	config = NormalizeConfig(config)
	settings := settingsFromPayload(map[string]any{})
	return runtime.ContainerOptions{
		Env: []string{
			"EULA=TRUE",
			"TYPE=VANILLA",
			fmt.Sprintf("SERVER_PORT=%d", config.Port),
			fmt.Sprintf("MAX_PLAYERS=%d", config.MaxPlayers),
			"MOTD=" + config.ServerName,
			"LEVEL_NAME=" + config.WorldName,
			"MODE=" + settings.GameMode,
			"DIFFICULTY=" + settings.Difficulty,
			fmt.Sprintf("ONLINE_MODE=%t", settings.OnlineMode),
			fmt.Sprintf("WHITE_LIST=%t", settings.WhitelistEnabled),
		},
		DataMounts:   []string{"/data"},
		PortProtocol: "tcp",
		Files: map[string]string{
			"data/server.properties": renderServerProperties(config, settings),
			"data/eula.txt":          "eula=true\n",
		},
	}
}

func (Provider) JoinInfo(server domain.GameServerInstance) domain.ServerJoinInfo {
	address := "127.0.0.1"
	port := server.HostPort
	if port == 0 {
		port = server.Port
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

func (provider Provider) RenderServerConfig(server domain.GameServerInstance) (string, error) {
	return provider.RenderConfig(configFromServer(server))
}

func (provider Provider) RuntimeOptionsForServer(server domain.GameServerInstance) (runtime.ContainerOptions, error) {
	config := configFromServer(server)
	if err := provider.ValidateConfig(config); err != nil {
		return runtime.ContainerOptions{}, err
	}
	settings := settingsFromPayload(payloadFromServer(server))
	return runtime.ContainerOptions{
		Env: []string{
			"EULA=TRUE",
			"TYPE=VANILLA",
			fmt.Sprintf("SERVER_PORT=%d", config.Port),
			fmt.Sprintf("MAX_PLAYERS=%d", config.MaxPlayers),
			"MOTD=" + config.ServerName,
			"LEVEL_NAME=" + config.WorldName,
			"MODE=" + settings.GameMode,
			"DIFFICULTY=" + settings.Difficulty,
			fmt.Sprintf("ONLINE_MODE=%t", settings.OnlineMode),
			fmt.Sprintf("WHITE_LIST=%t", settings.WhitelistEnabled),
		},
		DataMounts:   []string{"/data"},
		PortProtocol: "tcp",
		Files: map[string]string{
			"data/server.properties": renderServerProperties(config, settings),
			"data/eula.txt":          "eula=true\n",
		},
	}, nil
}

func NormalizeConfig(config domain.TerrariaConfig) domain.TerrariaConfig {
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
	config.WorldSize = ""
	config.WorldEvil = ""
	config.Difficulty = ""
	config.Password = ""
	config.MOTD = ""
	config.Seed = ""
	config.AutoCreateWorld = false
	config.Language = "en-US"
	return config
}

type minecraftSettings struct {
	GameMode         string
	Difficulty       string
	OnlineMode       bool
	WhitelistEnabled bool
}

func settingsFromPayload(payload map[string]any) minecraftSettings {
	settings := minecraftSettings{
		GameMode:   stringPayload(payload, "gameMode"),
		Difficulty: stringPayload(payload, "difficulty"),
		OnlineMode: true,
	}
	if settings.GameMode == "" {
		settings.GameMode = "survival"
	}
	if settings.Difficulty == "" {
		settings.Difficulty = "normal"
	}
	if value, ok := payload["onlineMode"]; ok {
		settings.OnlineMode = boolValue(value)
	}
	settings.WhitelistEnabled = boolValue(payload["whitelistEnabled"])
	return settings
}

func ConfigFromPayload(payload map[string]any, fallback domain.TerrariaConfig) domain.TerrariaConfig {
	config := NormalizeConfig(fallback)
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
		config.Secure = boolValue(value)
	}
	return NormalizeConfig(config)
}

func PayloadFromConfig(config domain.TerrariaConfig, payload map[string]any) map[string]any {
	settings := settingsFromPayload(payload)
	out := map[string]any{
		"serverName":       config.ServerName,
		"worldName":        config.WorldName,
		"maxPlayers":       config.MaxPlayers,
		"gameMode":         settings.GameMode,
		"difficulty":       settings.Difficulty,
		"onlineMode":       settings.OnlineMode,
		"whitelistEnabled": settings.WhitelistEnabled,
		"eulaAccepted":     config.Secure,
	}
	return out
}

func EnrichPayloadFromConfig(config domain.TerrariaConfig, payload map[string]any) map[string]any {
	next := PayloadFromConfig(config, payload)
	return next
}

func configFromServer(server domain.GameServerInstance) domain.TerrariaConfig {
	payload := payloadFromServer(server)
	if len(payload) == 0 {
		return NormalizeConfig(server.Config)
	}
	return ConfigFromPayload(payload, server.Config)
}

func payloadFromServer(server domain.GameServerInstance) map[string]any {
	payload := server.ConfigPayload
	if len(payload) == 0 && strings.TrimSpace(server.ConfigPayloadJSON) != "" {
		_ = json.Unmarshal([]byte(server.ConfigPayloadJSON), &payload)
	}
	return payload
}

func renderServerProperties(config domain.TerrariaConfig, settings minecraftSettings) string {
	lines := []string{
		"# Minecraft server properties - managed by GamePanel Lite",
		"server-port=" + fmt.Sprintf("%d", config.Port),
		"max-players=" + fmt.Sprintf("%d", config.MaxPlayers),
		"motd=" + config.ServerName,
		"level-name=" + config.WorldName,
		"gamemode=" + settings.GameMode,
		"difficulty=" + settings.Difficulty,
		fmt.Sprintf("online-mode=%t", settings.OnlineMode),
		fmt.Sprintf("white-list=%t", settings.WhitelistEnabled),
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
	if strings.TrimSpace(version) == "" {
		version = versions[0]
	}
	return "itzg/minecraft-server:" + version
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
