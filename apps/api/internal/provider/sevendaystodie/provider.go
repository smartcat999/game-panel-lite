package sevendaystodie

import (
	"crypto/rand"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/runtimecatalog"
)

const DefaultInternalPort = 26900

type Provider struct{ runtime runtimecatalog.RuntimeConfig }

func runtimeConfig() runtimecatalog.RuntimeConfig {
	return runtimecatalog.RuntimeConfig{ImageTemplate: "smartcat99999/seven-days-to-die:{version}", Versions: []string{"v3.3.0"}}
}
func NewProvider(catalog ...runtimecatalog.Catalog) Provider {
	return Provider{runtime: runtimecatalog.FromCatalog(catalog, domain.ProviderSevenDaysToDie, runtimeConfig())}
}
func (Provider) GameKey() domain.GameKey { return domain.GameSevenDaysToDie }
func (Provider) Key() domain.ProviderKey { return domain.ProviderSevenDaysToDie }
func (Provider) Name() string            { return "7 Days to Die" }
func (Provider) Description() string     { return "Vanilla zombie survival and base defense server." }
func (Provider) Capabilities() domain.ProviderCapabilities {
	return domain.ProviderCapabilities{Backups: true, BackupRequiresStop: true, Versions: true}
}
func (Provider) SaveDisplayName() string                 { return "world" }
func (Provider) BackupSubtree(domain.GameServer) string  { return "userdata" }
func (Provider) RestoreSubtree(domain.GameServer) string { return "userdata" }
func (Provider) SteamUpdateConfig() domain.SteamUpdateConfig {
	return domain.SteamUpdateConfig{AppID: "294420", InstallDir: "/data", Executable: "7DaysToDieServer.x86_64", BackupSubtree: "userdata", MinimumFreeDiskBytes: 25 * 1024 * 1024 * 1024}
}
func (p Provider) Versions() []string { return p.runtime.WithFallback(runtimeConfig()).VersionList() }
func (p Provider) Image() string      { return p.ImageFor(p.Versions()[0]) }
func (p Provider) ImageFor(version string) string {
	return p.runtime.WithFallback(runtimeConfig()).ImageFor(version)
}

type Config struct {
	ServerName        string `json:"serverName"`
	WorldName         string `json:"worldName"`
	ServerPassword    string `json:"serverPassword"`
	MaxPlayers        int    `json:"maxPlayers"`
	Port              int    `json:"port"`
	GameWorld         string `json:"gameWorld"`
	WorldGenSeed      string `json:"worldGenSeed"`
	WorldGenSize      int    `json:"worldGenSize"`
	ServerVisibility  string `json:"serverVisibility"`
	PlayerKillingMode string `json:"playerKillingMode"`
	EACEnabled        bool   `json:"eacEnabled"`
	SandboxCode       string `json:"sandboxCode"`
}

func defaultConfig() Config {
	return Config{ServerName: "7 Days to Die Server", WorldName: "Friends", MaxPlayers: 8, Port: DefaultInternalPort, GameWorld: "Navezgane", WorldGenSeed: "GamePanelLite", WorldGenSize: 6144, ServerVisibility: "2", PlayerKillingMode: "0", EACEnabled: true, SandboxCode: "AAAJABJACJADJARFBNC"}
}
func parseConfig(payload map[string]any) (Config, error) {
	config := defaultConfig()
	data, err := json.Marshal(payload)
	if err != nil {
		return config, err
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("invalid 7 Days to Die config: %w", err)
	}
	config.ServerName = strings.TrimSpace(config.ServerName)
	config.WorldName = strings.TrimSpace(config.WorldName)
	config.SandboxCode = strings.TrimSpace(config.SandboxCode)
	return config, validateConfig(config)
}
func payload(config Config) map[string]any {
	data, _ := json.Marshal(config)
	out := map[string]any{}
	_ = json.Unmarshal(data, &out)
	return out
}
func (Provider) DefaultConfigPayload() map[string]any {
	config := defaultConfig()
	// Keep the legacy parse fallback for existing payloads without a seed.
	// Generate defaults only for creation; normalization never changes an existing seed.
	config.WorldGenSeed = rand.Text()[:16]
	return payload(config)
}
func (Provider) NormalizeConfigPayload(input map[string]any) (map[string]any, error) {
	config, err := parseConfig(input)
	if err != nil {
		return nil, err
	}
	return payload(config), nil
}
func (Provider) ValidateConfigPayload(input map[string]any) error {
	_, err := parseConfig(input)
	return err
}

func (Provider) ValidateConfigUpdate(current, next map[string]any) error {
	oldConfig, err := parseConfig(current)
	if err != nil {
		return err
	}
	newConfig, err := parseConfig(next)
	if err != nil {
		return err
	}
	if oldConfig.WorldName != newConfig.WorldName || oldConfig.GameWorld != newConfig.GameWorld || oldConfig.WorldGenSeed != newConfig.WorldGenSeed || oldConfig.WorldGenSize != newConfig.WorldGenSize {
		return fmt.Errorf("world identity cannot be changed on an existing server; create another server for a new world")
	}
	return nil
}

var sandboxPattern = regexp.MustCompile(`^[A-Z]+$`)

func validateConfig(config Config) error {
	if config.ServerName == "" || len(config.ServerName) > 256 {
		return fmt.Errorf("server name must contain 1-256 bytes")
	}
	if config.WorldName == "" || len(config.WorldName) > 128 || strings.ContainsAny(config.WorldName, "/\\\x00") || config.WorldName == "." || strings.Contains(config.WorldName, "..") {
		return fmt.Errorf("invalid world name")
	}
	if config.MaxPlayers < 1 || config.MaxPlayers > 64 {
		return fmt.Errorf("max players must be between 1 and 64")
	}
	if config.Port < 1024 || config.Port > 65533 {
		return fmt.Errorf("game port must be between 1024 and 65533")
	}
	switch config.GameWorld {
	case "Navezgane", "RWG", "Pregen06k01", "Pregen06k02", "Pregen08k01", "Pregen08k02":
	default:
		return fmt.Errorf("unsupported world type")
	}
	// Retain 4K parsing for previously created saves; new UI choices follow the 3.3 template.
	if config.WorldGenSize != 4096 && config.WorldGenSize != 6144 && config.WorldGenSize != 8192 && config.WorldGenSize != 10240 {
		return fmt.Errorf("unsupported world size")
	}
	if config.ServerVisibility != "0" && config.ServerVisibility != "1" && config.ServerVisibility != "2" {
		return fmt.Errorf("invalid server visibility")
	}
	if config.PlayerKillingMode != "0" && config.PlayerKillingMode != "1" && config.PlayerKillingMode != "2" && config.PlayerKillingMode != "3" {
		return fmt.Errorf("invalid player killing mode")
	}
	if len(config.SandboxCode) > 4096 || !sandboxPattern.MatchString(config.SandboxCode) {
		return fmt.Errorf("sandbox code must contain 1-4096 uppercase letters; copy it from the game's Sandbox Options menu")
	}
	return nil
}
func (Provider) ConfigSummary(input map[string]any) (domain.ProviderConfigSummary, error) {
	c, err := parseConfig(input)
	return domain.ProviderConfigSummary{ServerName: c.ServerName, WorldName: c.WorldName, MaxPlayers: c.MaxPlayers, Port: c.Port, Password: c.ServerPassword}, err
}

// Only properties supported by 3.x are emitted. Gameplay rules belong to
// SandboxCode; obsolete individual XML properties are silently ignored by 3.x.
func renderConfig(c Config) (string, error) {
	type property struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value,attr"`
	}
	properties := []property{
		{"ServerName", c.ServerName}, {"ServerPassword", c.ServerPassword}, {"ServerDescription", "Managed by GamePanel Lite"},
		{"ServerPort", strconv.Itoa(c.Port)}, {"ServerMaxPlayerCount", strconv.Itoa(c.MaxPlayers)},
		{"ServerVisibility", c.ServerVisibility}, {"ServerDisabledNetworkProtocols", "SteamNetworking"},
		{"GameWorld", c.GameWorld}, {"GameName", c.WorldName}, {"WorldGenSeed", c.WorldGenSeed}, {"WorldGenSize", strconv.Itoa(c.WorldGenSize)},
		{"UserDataFolder", "/data/userdata"},
		{"EACEnabled", strconv.FormatBool(c.EACEnabled)}, {"PlayerKillingMode", c.PlayerKillingMode}, {"SandboxCode", c.SandboxCode},
		{"TelnetEnabled", "false"}, {"WebDashboardEnabled", "false"}, {"ServerAllowCrossplay", "false"},
	}
	document := struct {
		XMLName    xml.Name   `xml:"ServerSettings"`
		Properties []property `xml:"property"`
	}{Properties: properties}
	data, err := xml.MarshalIndent(document, "", "  ")
	return xml.Header + string(data) + "\n", err
}
func (Provider) RuntimeConfigForResource(server domain.GameServer) (domain.ProviderRuntimeConfig, error) {
	c, err := parseConfig(server.Spec.Config)
	if err != nil {
		return domain.ProviderRuntimeConfig{}, err
	}
	if server.Spec.Network.Port != 0 {
		c.Port = server.Spec.Network.Port
	}
	if err := validateConfig(c); err != nil {
		return domain.ProviderRuntimeConfig{}, err
	}
	text, err := renderConfig(c)
	return domain.ProviderRuntimeConfig{Port: c.Port, Protocol: "tcp", PortBindings: []domain.WorkloadPort{
		{Port: c.Port, Protocol: "udp"}, {Port: c.Port + 1, Protocol: "udp"}, {Port: c.Port + 2, Protocol: "udp"},
	}, Options: domain.WorkloadOptions{Files: map[string]string{"gamepanel-serverconfig.xml": text}, DataMounts: []string{"/data"}}}, err
}
func (Provider) JoinInfo(server domain.GameServer) domain.ServerJoinInfo {
	c, _ := parseConfig(server.Spec.Config)
	port := server.Spec.Network.HostPort
	if port == 0 {
		port = c.Port
	}
	invite := fmt.Sprintf("Join %s in 7 Days to Die at 127.0.0.1:%d", server.Name, port)
	if c.ServerPassword != "" {
		invite += " password: " + c.ServerPassword
	}
	return domain.ServerJoinInfo{Address: "127.0.0.1", Port: port, Password: c.ServerPassword, InviteText: invite, Instructions: []string{"Open Join a Game, then Connect to IP and enter the address and port."}}
}
