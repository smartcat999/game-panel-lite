package palworld

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

const DefaultInternalPort = 8211

var versions = []string{"latest"}

type Provider struct{}

func NewProvider() Provider { return Provider{} }

func (Provider) GameKey() domain.GameKey { return domain.GamePalworld }
func (Provider) Key() domain.ProviderKey { return domain.ProviderPalworld }
func (Provider) Name() string            { return "Palworld" }
func (Provider) Description() string {
	return "Palworld dedicated server for private friend groups."
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
		{Name: "serverName", Label: "服务器名称", Type: "text", Required: true, Default: "Palworld Server"},
		{Name: "saveName", Label: "存档名称", Type: "text", Required: true, Default: "Palworld Save"},
		{Name: "maxPlayers", Label: "最大玩家数", Type: "number", Required: true, Default: 8},
		{Name: "serverPassword", Label: "服务器密码", Type: "password", Required: false},
		{Name: "adminPassword", Label: "管理员密码", Type: "password", Required: true, Help: "用于 Palworld 管理员操作。"},
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
		return fmt.Errorf("save name is required")
	}
	if strings.Contains(config.WorldName, "/") || strings.Contains(config.WorldName, "\\") || strings.Contains(config.WorldName, "..") {
		return fmt.Errorf("save name contains unsupported path characters")
	}
	if config.MaxPlayers < 1 || config.MaxPlayers > 32 {
		return fmt.Errorf("max players must be between 1 and 32")
	}
	if strings.TrimSpace(config.MOTD) == "" {
		return fmt.Errorf("admin password is required")
	}
	return nil
}
func (Provider) RenderConfig(config domain.TerrariaConfig) (string, error) {
	config = NormalizeConfig(config)
	if err := (Provider{}).ValidateConfig(config); err != nil {
		return "", err
	}
	lines := []string{
		"game=palworld",
		"serverName=" + config.ServerName,
		"saveName=" + config.WorldName,
		fmt.Sprintf("maxPlayers=%d", config.MaxPlayers),
		fmt.Sprintf("port=%d", config.Port),
	}
	if config.Password != "" {
		lines = append(lines, "serverPassword="+config.Password)
	}
	if config.MOTD != "" {
		lines = append(lines, "adminPassword="+config.MOTD)
	}
	return strings.Join(lines, "\n") + "\n", nil
}
func (Provider) RuntimeOptions(config domain.TerrariaConfig) runtime.ContainerOptions {
	config = NormalizeConfig(config)
	return runtime.ContainerOptions{
		Env: []string{
			"PUID=1000",
			"PGID=1000",
			"PORT=" + fmt.Sprintf("%d", config.Port),
			"PLAYERS=" + fmt.Sprintf("%d", config.MaxPlayers),
			"SERVER_NAME=" + config.ServerName,
			"SERVER_PASSWORD=" + config.Password,
			"ADMIN_PASSWORD=" + config.MOTD,
			"MULTITHREADING=true",
			"COMMUNITY=false",
			"UPDATE_ON_BOOT=true",
		},
		DataMounts:   []string{"/palworld"},
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
	invite := fmt.Sprintf("Join %s in Palworld at %s:%d", server.Name, address, port)
	if password != "" {
		invite += " password: " + password
	}
	return domain.ServerJoinInfo{
		Address:    address,
		Port:       port,
		Password:   password,
		InviteText: invite,
		Instructions: []string{
			"Open Palworld multiplayer.",
			"Join by address using the host and port shown here.",
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
	return provider.RuntimeOptions(config), nil
}

func NormalizeConfig(config domain.TerrariaConfig) domain.TerrariaConfig {
	if strings.TrimSpace(config.ServerName) == "" {
		config.ServerName = "Palworld Server"
	}
	if strings.TrimSpace(config.WorldName) == "" {
		config.WorldName = "Palworld Save"
	}
	if config.MaxPlayers == 0 {
		config.MaxPlayers = 8
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

func configFromServer(server domain.GameServerInstance) domain.TerrariaConfig {
	config := NormalizeConfig(server.Config)
	payload := server.ConfigPayload
	if len(payload) == 0 && strings.TrimSpace(server.ConfigPayloadJSON) != "" {
		_ = json.Unmarshal([]byte(server.ConfigPayloadJSON), &payload)
	}
	if len(payload) == 0 {
		return config
	}
	if value := stringPayload(payload, "serverName"); value != "" {
		config.ServerName = value
	}
	if value := stringPayload(payload, "saveName"); value != "" {
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
	if value := stringPayload(payload, "adminPassword"); value != "" {
		config.MOTD = value
	} else if value := stringPayload(payload, "motd"); value != "" {
		config.MOTD = value
	}
	return NormalizeConfig(config)
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
	return "thijsvanloef/palworld-server-docker:" + version
}
