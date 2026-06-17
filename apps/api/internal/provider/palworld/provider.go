package palworld

import (
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
		{Name: "worldName", Label: "存档名称", Type: "text", Required: true, Default: "Palworld Save"},
		{Name: "maxPlayers", Label: "最大玩家数", Type: "number", Required: true, Default: 8},
		{Name: "password", Label: "服务器密码", Type: "password", Required: false},
		{Name: "motd", Label: "管理员密码", Type: "password", Required: true, Help: "用于 Palworld 管理员操作。"},
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

func ImageForVersion(version string) string {
	if strings.TrimSpace(version) == "" {
		version = versions[0]
	}
	return "thijsvanloef/palworld-server-docker:" + version
}
