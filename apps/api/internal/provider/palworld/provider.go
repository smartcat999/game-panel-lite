package palworld

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/runtimecatalog"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

const DefaultInternalPort = 8211

var versions = []string{"v2.4.1", "v2.4.0", "v2.3.2"}

type Provider struct {
	runtime runtimecatalog.RuntimeConfig
}

type Config struct {
	ServerName     string
	SaveName       string
	MaxPlayers     int
	Port           int
	ServerPassword string
	AdminPassword  string
}

func NewProvider(catalog ...runtimecatalog.Catalog) Provider {
	return Provider{
		runtime: runtimecatalog.FromCatalog(catalog, domain.ProviderPalworld, runtimeConfig()),
	}
}

func runtimeConfig() runtimecatalog.RuntimeConfig {
	return runtimecatalog.RuntimeConfig{ImageTemplate: "smartcat99999/palworld-server:{version}", Versions: versions}
}

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
		Mods:            true,
		Versions:        true,
	}
}
func (Provider) SaveDisplayName() string { return "save" }
func (Provider) ConfigSchema() []domain.ProviderConfigField {
	return configSchema()
}
func (p Provider) Image() string      { return p.ImageFor(p.Versions()[0]) }
func (p Provider) Versions() []string { return p.runtime.WithFallback(runtimeConfig()).VersionList() }
func (p Provider) ImageFor(version string) string {
	return p.runtime.WithFallback(runtimeConfig()).ImageFor(version)
}
func defaultConfig() Config {
	return normalizeConfig(Config{})
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
		WorldName:  config.SaveName,
		MaxPlayers: config.MaxPlayers,
		Port:       config.Port,
		Password:   config.ServerPassword,
		MOTD:       config.AdminPassword,
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
	if strings.TrimSpace(config.SaveName) == "" {
		return fmt.Errorf("save name is required")
	}
	if strings.Contains(config.SaveName, "/") || strings.Contains(config.SaveName, "\\") || strings.Contains(config.SaveName, "..") {
		return fmt.Errorf("save name contains unsupported path characters")
	}
	if config.MaxPlayers < 1 || config.MaxPlayers > 32 {
		return fmt.Errorf("max players must be between 1 and 32")
	}
	if strings.TrimSpace(config.AdminPassword) == "" {
		return fmt.Errorf("admin password is required")
	}
	return nil
}
func renderConfig(config Config) (string, error) {
	config = normalizeConfig(config)
	if err := validateConfig(config); err != nil {
		return "", err
	}
	lines := []string{
		"game=palworld",
		"serverName=" + config.ServerName,
		"saveName=" + config.SaveName,
		fmt.Sprintf("maxPlayers=%d", config.MaxPlayers),
		fmt.Sprintf("port=%d", config.Port),
	}
	if config.ServerPassword != "" {
		lines = append(lines, "serverPassword="+config.ServerPassword)
	}
	if config.AdminPassword != "" {
		lines = append(lines, "adminPassword="+config.AdminPassword)
	}
	return strings.Join(lines, "\n") + "\n", nil
}
func runtimeOptions(config Config) runtime.ContainerOptions {
	config = normalizeConfig(config)
	return runtime.ContainerOptions{
		Env: []string{
			"PUID=1000",
			"PGID=1000",
			"PORT=" + fmt.Sprintf("%d", config.Port),
			"PLAYERS=" + fmt.Sprintf("%d", config.MaxPlayers),
			"SERVER_NAME=" + config.ServerName,
			"SERVER_PASSWORD=" + config.ServerPassword,
			"ADMIN_PASSWORD=" + config.AdminPassword,
			"MULTITHREADING=true",
			"COMMUNITY=false",
			"UPDATE_ON_BOOT=true",
		},
		DataMounts:   []string{"/palworld"},
		PortProtocol: "udp",
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
	options := runtimeOptions(config)
	return domain.ProviderRuntimeConfig{
		Port:     config.Port,
		Protocol: options.PortProtocol,
		Options:  workloadOptions(options),
	}, nil
}

func (p Provider) JoinInfo(server domain.GameServer) domain.ServerJoinInfo {
	address := "127.0.0.1"
	port := server.Spec.Network.HostPort
	if port == 0 {
		port = server.Spec.Network.Port
	}
	config := configFromPayload(server.Spec.Config, defaultConfig())
	password := config.ServerPassword
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

func normalizeConfig(config Config) Config {
	if strings.TrimSpace(config.ServerName) == "" {
		config.ServerName = "Palworld Server"
	}
	if strings.TrimSpace(config.SaveName) == "" {
		config.SaveName = "Palworld Save"
	}
	if config.MaxPlayers == 0 {
		config.MaxPlayers = 8
	}
	config.Port = DefaultInternalPort
	return config
}

func configFromPayload(payload map[string]any, fallback Config) Config {
	config := normalizeConfig(fallback)
	if len(payload) == 0 {
		return config
	}
	if value := stringPayload(payload, "serverName"); value != "" {
		config.ServerName = value
	}
	if value := stringPayload(payload, "saveName"); value != "" {
		config.SaveName = value
	} else if value := stringPayload(payload, "worldName"); value != "" {
		config.SaveName = value
	}
	if value, ok := intPayload(payload, "maxPlayers"); ok {
		config.MaxPlayers = value
	}
	if value := stringPayload(payload, "serverPassword"); value != "" {
		config.ServerPassword = value
	} else if value := stringPayload(payload, "password"); value != "" {
		config.ServerPassword = value
	}
	if value := stringPayload(payload, "adminPassword"); value != "" {
		config.AdminPassword = value
	} else if value := stringPayload(payload, "motd"); value != "" {
		config.AdminPassword = value
	}
	return normalizeConfig(config)
}

func payloadFromConfig(config Config) map[string]any {
	config = normalizeConfig(config)
	payload := map[string]any{
		"serverName":    config.ServerName,
		"saveName":      config.SaveName,
		"maxPlayers":    config.MaxPlayers,
		"adminPassword": config.AdminPassword,
	}
	if config.ServerPassword != "" {
		payload["serverPassword"] = config.ServerPassword
	}
	return payload
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
