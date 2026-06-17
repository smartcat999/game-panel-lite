package dst

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

const DefaultInternalPort = 10999

var versions = []string{"latest"}

type Provider struct{}

func NewProvider() Provider { return Provider{} }

func (Provider) GameKey() domain.GameKey { return domain.GameDST }
func (Provider) Key() domain.ProviderKey { return domain.ProviderDST }
func (Provider) Name() string            { return "Don't Starve Together" }
func (Provider) Description() string {
	return "Don't Starve Together dedicated server for small private groups."
}
func (Provider) Capabilities() domain.ProviderCapabilities {
	return domain.ProviderCapabilities{
		ConsoleCommands: false,
		PlayerList:      false,
		KickPlayer:      false,
		BanPlayer:       false,
		SaveSnapshots:   true,
		Backups:         true,
		Mods:            false,
		Versions:        true,
	}
}
func (Provider) ConfigSchema() []domain.ProviderConfigField {
	return []domain.ProviderConfigField{
		{Name: "serverName", Label: "服务器名称", Type: "text", Required: true, Default: "DST Friends"},
		{Name: "clusterName", Label: "存档名称", Type: "text", Required: true, Default: "GamePanelLite"},
		{Name: "maxPlayers", Label: "最大玩家数", Type: "number", Required: true, Default: 6},
		{Name: "serverPassword", Label: "服务器密码", Type: "password", Required: false},
		{Name: "clusterToken", Label: "Klei 服务器令牌", Type: "password", Required: true, Help: "在 Klei 账号页面创建专用服务器令牌后填入。"},
		{
			Name:     "gameMode",
			Label:    "游戏模式",
			Type:     "select",
			Required: true,
			Default:  "survival",
			Options: []domain.ProviderConfigFieldOption{
				{Value: "survival", Label: "生存"},
				{Value: "endless", Label: "无尽"},
				{Value: "wilderness", Label: "荒野"},
			},
		},
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
		return fmt.Errorf("cluster name is required")
	}
	if strings.Contains(config.WorldName, "/") || strings.Contains(config.WorldName, "\\") || strings.Contains(config.WorldName, "..") {
		return fmt.Errorf("cluster name contains unsupported path characters")
	}
	if config.MaxPlayers < 1 || config.MaxPlayers > 64 {
		return fmt.Errorf("max players must be between 1 and 64")
	}
	if strings.TrimSpace(config.MOTD) == "" {
		return fmt.Errorf("Klei server token is required")
	}
	return nil
}
func (Provider) RenderConfig(config domain.TerrariaConfig) (string, error) {
	config = NormalizeConfig(config)
	if err := (Provider{}).ValidateConfig(config); err != nil {
		return "", err
	}
	lines := []string{
		"game=dont-starve-together",
		"serverName=" + config.ServerName,
		"clusterName=" + config.WorldName,
		fmt.Sprintf("maxPlayers=%d", config.MaxPlayers),
		fmt.Sprintf("port=%d", config.Port),
	}
	if config.Password != "" {
		lines = append(lines, "serverPassword="+config.Password)
	}
	return strings.Join(lines, "\n") + "\n", nil
}
func (Provider) RuntimeOptions(config domain.TerrariaConfig) runtime.ContainerOptions {
	config = NormalizeConfig(config)
	return runtime.ContainerOptions{
		Env: []string{
			"DST_CLUSTER_NAME=" + config.WorldName,
			"DST_SHARD=Master",
			fmt.Sprintf("DST_PORT=%d", config.Port),
		},
		DataMounts: []string{"/data"},
		Files: map[string]string{
			"dst/cluster.ini":         renderClusterINI(config, "survival"),
			"dst/server_token.txt":    strings.TrimSpace(config.MOTD) + "\n",
			"dst/Master/server.ini":   renderMasterServerINI(config),
			"dst/Master/worldgen.lua": "return {}\n",
		},
		PortProtocol: "udp",
	}
}

func (Provider) JoinInfo(server domain.GameServerInstance) domain.ServerJoinInfo {
	address := "127.0.0.1"
	port := server.HostPort
	if port == 0 {
		port = server.Port
	}
	password := server.Password
	invite := fmt.Sprintf("Join %s in Don't Starve Together at %s:%d", server.Name, address, port)
	if password != "" {
		invite += " password: " + password
	}
	return domain.ServerJoinInfo{
		Address:    address,
		Port:       port,
		Password:   password,
		InviteText: invite,
		Instructions: []string{
			"Open Don't Starve Together.",
			"Browse games or connect using the host and port shown here.",
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
	options := provider.RuntimeOptions(config)
	payload := payloadFromServer(server)
	gameMode := stringPayload(payload, "gameMode")
	if gameMode == "" {
		gameMode = "survival"
	}
	options.Files["dst/cluster.ini"] = renderClusterINI(config, gameMode)
	return options, nil
}

func NormalizeConfig(config domain.TerrariaConfig) domain.TerrariaConfig {
	if strings.TrimSpace(config.ServerName) == "" {
		config.ServerName = "DST Friends"
	}
	if strings.TrimSpace(config.WorldName) == "" {
		config.WorldName = "GamePanelLite"
	}
	if config.MaxPlayers == 0 {
		config.MaxPlayers = 6
	}
	config.Port = DefaultInternalPort
	config.WorldSize = ""
	config.WorldEvil = ""
	config.Difficulty = ""
	config.Secure = false
	config.AutoCreateWorld = false
	config.Language = "en-US"
	return config
}

func ConfigFromPayload(payload map[string]any, fallback domain.TerrariaConfig) domain.TerrariaConfig {
	config := NormalizeConfig(fallback)
	if value := stringPayload(payload, "serverName"); value != "" {
		config.ServerName = value
	}
	if value := stringPayload(payload, "clusterName"); value != "" {
		config.WorldName = value
	} else if value := stringPayload(payload, "worldName"); value != "" {
		config.WorldName = value
	}
	if value, ok := intPayload(payload, "maxPlayers"); ok {
		config.MaxPlayers = value
	}
	if value := stringPayload(payload, "serverPassword"); value != "" {
		config.Password = value
	} else if value := stringPayload(payload, "password"); value != "" {
		config.Password = value
	}
	if value := stringPayload(payload, "clusterToken"); value != "" {
		config.MOTD = value
	} else if value := stringPayload(payload, "motd"); value != "" {
		config.MOTD = value
	}
	return NormalizeConfig(config)
}

func PayloadFromConfig(config domain.TerrariaConfig, gameMode string) map[string]any {
	config = NormalizeConfig(config)
	if strings.TrimSpace(gameMode) == "" {
		gameMode = "survival"
	}
	payload := map[string]any{
		"serverName":   config.ServerName,
		"clusterName":  config.WorldName,
		"maxPlayers":   config.MaxPlayers,
		"clusterToken": config.MOTD,
		"gameMode":     gameMode,
	}
	if config.Password != "" {
		payload["serverPassword"] = config.Password
	}
	return payload
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

func renderClusterINI(config domain.TerrariaConfig, gameMode string) string {
	if strings.TrimSpace(gameMode) == "" {
		gameMode = "survival"
	}
	lines := []string{
		"[GAMEPLAY]",
		"game_mode = " + gameMode,
		"max_players = " + fmt.Sprintf("%d", config.MaxPlayers),
		"pvp = false",
		"pause_when_empty = true",
		"",
		"[NETWORK]",
		"cluster_name = " + config.ServerName,
		"cluster_description = Managed by GamePanel Lite",
		"cluster_password = " + config.Password,
		"offline_server = false",
		"",
		"[MISC]",
		"console_enabled = true",
		"",
	}
	return strings.Join(lines, "\n")
}

func renderMasterServerINI(config domain.TerrariaConfig) string {
	return strings.Join([]string{
		"[NETWORK]",
		fmt.Sprintf("server_port = %d", config.Port),
		"",
		"[SHARD]",
		"is_master = true",
		"name = Master",
		"",
	}, "\n")
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

func ImageForVersion(version string) string {
	if strings.TrimSpace(version) == "" {
		version = versions[0]
	}
	return "smartcat99999/dst-server:" + version
}
