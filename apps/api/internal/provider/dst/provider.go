package dst

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/runtimecatalog"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

const DefaultInternalPort = 10999

var versions = []string{"latest"}

type Provider struct {
	runtime runtimecatalog.RuntimeConfig
}

type Config struct {
	ServerName         string
	ClusterName        string
	MaxPlayers         int
	Port               int
	ServerPassword     string
	ClusterToken       string
	GameMode           string
	WorldPreset        string
	ClusterDescription string
	CavesEnabled       bool
	PVP                bool
	PauseWhenEmpty     bool
	OfflineServer      bool
	ConsoleEnabled     bool
	WorkshopIDs        []string
}

func NewProvider(catalog ...runtimecatalog.Catalog) Provider {
	return Provider{
		runtime: runtimecatalog.FromCatalog(catalog, domain.ProviderDST, runtimeConfig()),
	}
}

func runtimeConfig() runtimecatalog.RuntimeConfig {
	return runtimecatalog.RuntimeConfig{ImageTemplate: "smartcat99999/dst-server:{version}", Versions: versions}
}

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
func (Provider) SaveDisplayName() string { return "cluster save" }
func (Provider) ConfigSchema() []domain.ProviderConfigField {
	return configSchema()
}
func (p Provider) Image() string      { return p.ImageFor(p.Versions()[0]) }
func (p Provider) Versions() []string { return p.runtime.WithFallback(runtimeConfig()).VersionList() }
func (p Provider) ImageFor(version string) string {
	return p.runtime.WithFallback(runtimeConfig()).ImageFor(version)
}
func defaultConfig() Config {
	return normalizeConfig(Config{PauseWhenEmpty: true, ConsoleEnabled: true})
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
		WorldName:  config.ClusterName,
		MaxPlayers: config.MaxPlayers,
		Port:       config.Port,
		Password:   config.ServerPassword,
		MOTD:       config.ClusterToken,
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
	if strings.TrimSpace(config.ClusterName) == "" {
		return fmt.Errorf("cluster name is required")
	}
	if strings.Contains(config.ClusterName, "/") || strings.Contains(config.ClusterName, "\\") || strings.Contains(config.ClusterName, "..") {
		return fmt.Errorf("cluster name contains unsupported path characters")
	}
	if config.MaxPlayers < 1 || config.MaxPlayers > 64 {
		return fmt.Errorf("max players must be between 1 and 64")
	}
	if strings.TrimSpace(config.ClusterToken) == "" {
		return fmt.Errorf("Klei server token is required")
	}
	return nil
}
func renderConfig(config Config) (string, error) {
	config = normalizeConfig(config)
	if err := validateConfig(config); err != nil {
		return "", err
	}
	lines := []string{
		"game=dont-starve-together",
		"serverName=" + config.ServerName,
		"clusterName=" + config.ClusterName,
		fmt.Sprintf("maxPlayers=%d", config.MaxPlayers),
		fmt.Sprintf("port=%d", config.Port),
	}
	if config.ServerPassword != "" {
		lines = append(lines, "serverPassword="+config.ServerPassword)
	}
	return strings.Join(lines, "\n") + "\n", nil
}
func runtimeOptions(config Config) runtime.ContainerOptions {
	config = normalizeConfig(config)
	clusterDir := clusterConfigDir(config)
	return runtime.ContainerOptions{
		Env: []string{
			"DST_CLUSTER_NAME=" + config.ClusterName,
			"DST_SHARD=Master",
			fmt.Sprintf("DST_PORT=%d", config.Port),
		},
		DataMounts: []string{"/data"},
		Files: map[string]string{
			clusterDir + "/cluster.ini":         renderClusterINI(config),
			clusterDir + "/cluster_token.txt":   strings.TrimSpace(config.ClusterToken) + "\n",
			clusterDir + "/Master/server.ini":   renderShardServerINI(config.Port, true, "Master"),
			clusterDir + "/Master/worldgen.lua": renderWorldgenLua(config.WorldPreset),
		},
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
	clusterDir := clusterConfigDir(config)
	options.Files[clusterDir+"/cluster.ini"] = renderClusterINI(config)
	options.Files[clusterDir+"/Master/worldgen.lua"] = renderWorldgenLua(config.WorldPreset)
	if config.CavesEnabled {
		options.Files[clusterDir+"/Caves/server.ini"] = renderShardServerINI(config.Port+1, false, "Caves")
		options.Files[clusterDir+"/Caves/worldgen.lua"] = renderWorldgenLua("cave_default")
	}
	if len(config.WorkshopIDs) > 0 {
		options.Files[clusterDir+"/dedicated_server_mods_setup.lua"] = renderWorkshopSetup(config.WorkshopIDs)
	}
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

func normalizeConfig(config Config) Config {
	if strings.TrimSpace(config.ServerName) == "" {
		config.ServerName = "DST Friends"
	}
	if strings.TrimSpace(config.ClusterName) == "" {
		config.ClusterName = "GamePanelLite"
	}
	if config.MaxPlayers == 0 {
		config.MaxPlayers = 6
	}
	config.Port = DefaultInternalPort
	if strings.TrimSpace(config.GameMode) == "" {
		config.GameMode = "survival"
	}
	if strings.TrimSpace(config.WorldPreset) == "" {
		config.WorldPreset = "forest_default"
	}
	if strings.TrimSpace(config.ClusterDescription) == "" {
		config.ClusterDescription = "Managed by GamePanel Lite"
	}
	config.WorkshopIDs = uniqueDigits(config.WorkshopIDs)
	return config
}

func configFromPayload(payload map[string]any, fallback Config) Config {
	config := normalizeConfig(fallback)
	if value := stringPayload(payload, "serverName"); value != "" {
		config.ServerName = value
	}
	if value := stringPayload(payload, "clusterName"); value != "" {
		config.ClusterName = value
	} else if value := stringPayload(payload, "worldName"); value != "" {
		config.ClusterName = value
	}
	if value, ok := intPayload(payload, "maxPlayers"); ok {
		config.MaxPlayers = value
	}
	if value := stringPayload(payload, "serverPassword"); value != "" {
		config.ServerPassword = value
	} else if value := stringPayload(payload, "password"); value != "" {
		config.ServerPassword = value
	}
	if value := stringPayload(payload, "clusterToken"); value != "" {
		config.ClusterToken = value
	} else if value := stringPayload(payload, "motd"); value != "" {
		config.ClusterToken = value
	}
	config.GameMode = stringPayload(payload, "gameMode")
	config.WorldPreset = stringPayload(payload, "worldPreset")
	config.ClusterDescription = stringPayload(payload, "clusterDescription")
	config.CavesEnabled = boolPayload(payload, "cavesEnabled")
	config.PVP = boolPayload(payload, "pvp")
	config.PauseWhenEmpty = boolPayloadDefault(payload, "pauseWhenEmpty", config.PauseWhenEmpty)
	config.OfflineServer = boolPayload(payload, "offlineServer")
	config.ConsoleEnabled = boolPayloadDefault(payload, "consoleEnabled", config.ConsoleEnabled)
	config.WorkshopIDs = workshopIDsPayload(payload, "workshopIds")
	return normalizeConfig(config)
}

func payloadFromConfig(config Config) map[string]any {
	config = normalizeConfig(config)
	payload := map[string]any{
		"serverName":         config.ServerName,
		"clusterName":        config.ClusterName,
		"maxPlayers":         config.MaxPlayers,
		"clusterToken":       config.ClusterToken,
		"gameMode":           config.GameMode,
		"worldPreset":        config.WorldPreset,
		"clusterDescription": config.ClusterDescription,
		"cavesEnabled":       config.CavesEnabled,
		"pvp":                config.PVP,
		"pauseWhenEmpty":     config.PauseWhenEmpty,
		"offlineServer":      config.OfflineServer,
		"consoleEnabled":     config.ConsoleEnabled,
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

func renderClusterINI(config Config) string {
	config = normalizeConfig(config)
	lines := []string{
		"[GAMEPLAY]",
		"game_mode = " + config.GameMode,
		"max_players = " + fmt.Sprintf("%d", config.MaxPlayers),
		"pvp = " + boolINI(config.PVP),
		"pause_when_empty = " + boolINI(config.PauseWhenEmpty),
		"",
		"[NETWORK]",
		"cluster_name = " + config.ServerName,
		"cluster_description = " + config.ClusterDescription,
		"cluster_password = " + config.ServerPassword,
		"offline_server = " + boolINI(config.OfflineServer),
		"",
		"[MISC]",
		"console_enabled = " + boolINI(config.ConsoleEnabled),
		"",
	}
	return strings.Join(lines, "\n")
}

func clusterConfigDir(config Config) string {
	return "dst/" + config.ClusterName
}

func renderShardServerINI(port int, isMaster bool, name string) string {
	master := "false"
	if isMaster {
		master = "true"
	}
	return strings.Join([]string{
		"[NETWORK]",
		fmt.Sprintf("server_port = %d", port),
		"",
		"[SHARD]",
		"is_master = " + master,
		"name = " + name,
		"",
	}, "\n")
}

func renderWorldgenLua(preset string) string {
	preset = strings.TrimSpace(preset)
	if preset == "" {
		preset = "forest_default"
	}
	return fmt.Sprintf("return { override_enabled = true, preset = %q }\n", preset)
}

func renderWorkshopSetup(workshopIDs []string) string {
	lines := make([]string, 0, len(workshopIDs))
	for _, id := range workshopIDs {
		lines = append(lines, fmt.Sprintf("ServerModSetup(%q)", id))
	}
	return strings.Join(lines, "\n") + "\n"
}

func boolINI(value bool) string {
	if value {
		return "true"
	}
	return "false"
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

func boolPayload(payload map[string]any, key string) bool {
	value, ok := payload[key]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func boolPayloadDefault(payload map[string]any, key string, fallback bool) bool {
	if _, ok := payload[key]; !ok {
		return fallback
	}
	return boolPayload(payload, key)
}

func workshopIDsPayload(payload map[string]any, key string) []string {
	value, ok := payload[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case string:
		return strings.FieldsFunc(typed, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
		})
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func uniqueDigits(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] || !isDigits(value) {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func isDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}

func ImageForVersion(version string) string {
	if strings.TrimSpace(version) == "" {
		version = versions[0]
	}
	return "smartcat99999/dst-server:" + version
}
