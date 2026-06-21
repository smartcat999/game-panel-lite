package dst

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/runtimecatalog"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

const DefaultInternalPort = 10999

var versions = []string{"v2026.06.21"}

type Provider struct {
	runtime runtimecatalog.RuntimeConfig
}

type Config struct {
	Identity DSTIdentityConfig
	Gameplay DSTGameplayConfig
	World    DSTWorldConfig
	Caves    *DSTCaveConfig
	Mods     DSTModConfig
	Port     int
}

type DSTIdentityConfig struct {
	ServerName   string
	ClusterName  string
	Description  string
	Password     string
	ClusterToken string
	Visibility   string
}

type DSTGameplayConfig struct {
	MaxPlayers     int
	GameMode       string
	PVP            bool
	PauseWhenEmpty bool
	ConsoleEnabled bool
}

type DSTWorldConfig struct {
	Preset    string
	Customize bool
	Overrides map[string]string
}

type DSTCaveConfig struct {
	Enabled   bool
	Preset    string
	Overrides map[string]string
}

type DSTModConfig struct {
	WorkshopIDs []string
	ModPackIDs  []string
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
		Mods:            true,
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
	return normalizeConfig(Config{
		Gameplay: DSTGameplayConfig{
			PauseWhenEmpty: true,
			ConsoleEnabled: true,
		},
	})
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
		ServerName: config.Identity.ServerName,
		WorldName:  config.Identity.ClusterName,
		MaxPlayers: config.Gameplay.MaxPlayers,
		Port:       config.Port,
		Password:   config.Identity.Password,
		MOTD:       config.Identity.ClusterToken,
	}, nil
}
func validateConfig(config Config) error {
	config = normalizeConfig(config)
	if strings.TrimSpace(config.Identity.ServerName) == "" {
		return fmt.Errorf("server name is required")
	}
	if strings.Contains(config.Identity.ServerName, "/") || strings.Contains(config.Identity.ServerName, "\\") {
		return fmt.Errorf("server name contains unsupported path characters")
	}
	if strings.TrimSpace(config.Identity.ClusterName) == "" {
		return fmt.Errorf("cluster name is required")
	}
	if strings.Contains(config.Identity.ClusterName, "/") || strings.Contains(config.Identity.ClusterName, "\\") || strings.Contains(config.Identity.ClusterName, "..") {
		return fmt.Errorf("cluster name contains unsupported path characters")
	}
	if config.Gameplay.MaxPlayers < 1 || config.Gameplay.MaxPlayers > 64 {
		return fmt.Errorf("max players must be between 1 and 64")
	}
	if strings.TrimSpace(config.Identity.ClusterToken) == "" {
		return fmt.Errorf("Klei server token is required")
	}
	if !stringIn(config.Identity.Visibility, "public", "lan", "offline") {
		return fmt.Errorf("visibility must be public, lan, or offline")
	}
	if !stringIn(config.Gameplay.GameMode, "survival", "endless", "wilderness") {
		return fmt.Errorf("game mode must be survival, endless, or wilderness")
	}
	if config.World.Preset == "" {
		return fmt.Errorf("world preset is required")
	}
	for key, value := range config.World.Overrides {
		if err := validateOverride(key, value); err != nil {
			return fmt.Errorf("world override %s: %w", key, err)
		}
	}
	if config.Caves != nil {
		if config.Caves.Enabled && strings.TrimSpace(config.Caves.Preset) == "" {
			return fmt.Errorf("cave preset is required")
		}
		for key, value := range config.Caves.Overrides {
			if err := validateOverride(key, value); err != nil {
				return fmt.Errorf("cave override %s: %w", key, err)
			}
		}
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
		"serverName=" + config.Identity.ServerName,
		"clusterName=" + config.Identity.ClusterName,
		fmt.Sprintf("maxPlayers=%d", config.Gameplay.MaxPlayers),
		fmt.Sprintf("port=%d", config.Port),
	}
	if config.Identity.Password != "" {
		lines = append(lines, "serverPassword="+config.Identity.Password)
	}
	return strings.Join(lines, "\n") + "\n", nil
}
func runtimeOptions(config Config) runtime.ContainerOptions {
	config = normalizeConfig(config)
	clusterDir := clusterConfigDir(config)
	return runtime.ContainerOptions{
		Env: []string{
			"DST_CLUSTER_NAME=" + config.Identity.ClusterName,
			"DST_SHARD=Master",
			fmt.Sprintf("DST_PORT=%d", config.Port),
		},
		DataMounts: []string{"/data"},
		Files: map[string]string{
			clusterDir + "/cluster.ini":                     renderClusterINI(config),
			clusterDir + "/cluster_token.txt":               strings.TrimSpace(config.Identity.ClusterToken) + "\n",
			clusterDir + "/Master/server.ini":               renderShardServerINI(config.Port, true, "Master"),
			clusterDir + "/Master/leveldataoverride.lua":    renderLevelDataOverrideLua("forest", config.World.Preset, config.World.Overrides),
			clusterDir + "/Master/modoverrides.lua":         renderModOverrides(config.Mods.WorkshopIDs),
			clusterDir + "/dedicated_server_mods_setup.lua": renderWorkshopSetup(config.Mods.WorkshopIDs),
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
	options.Files[clusterDir+"/Master/leveldataoverride.lua"] = renderLevelDataOverrideLua("forest", config.World.Preset, config.World.Overrides)
	if config.Caves != nil && config.Caves.Enabled {
		options.Files[clusterDir+"/Caves/server.ini"] = renderShardServerINI(config.Port+1, false, "Caves")
		options.Files[clusterDir+"/Caves/leveldataoverride.lua"] = renderLevelDataOverrideLua("cave", config.Caves.Preset, config.Caves.Overrides)
		options.Files[clusterDir+"/Caves/modoverrides.lua"] = renderModOverrides(config.Mods.WorkshopIDs)
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
	password := config.Identity.Password
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
	if strings.TrimSpace(config.Identity.ServerName) == "" {
		config.Identity.ServerName = "DST Friends"
	}
	if strings.TrimSpace(config.Identity.ClusterName) == "" {
		config.Identity.ClusterName = "GamePanelLite"
	}
	if strings.TrimSpace(config.Identity.Description) == "" {
		config.Identity.Description = "Managed by GamePanel Lite"
	}
	if strings.TrimSpace(config.Identity.Visibility) == "" {
		config.Identity.Visibility = "public"
	}
	if config.Gameplay.MaxPlayers == 0 {
		config.Gameplay.MaxPlayers = 6
	}
	config.Port = DefaultInternalPort
	if strings.TrimSpace(config.Gameplay.GameMode) == "" {
		config.Gameplay.GameMode = "survival"
	}
	if !config.Gameplay.PauseWhenEmpty {
		// False is a valid explicit value; defaulting is handled in defaultConfig.
	}
	if strings.TrimSpace(config.World.Preset) == "" {
		config.World.Preset = "forest_default"
	}
	config.World.Overrides = cleanOverrides(config.World.Overrides)
	if config.Caves != nil {
		if strings.TrimSpace(config.Caves.Preset) == "" {
			config.Caves.Preset = "cave_default"
		}
		config.Caves.Overrides = cleanOverrides(config.Caves.Overrides)
		if !config.Caves.Enabled {
			config.Caves = nil
		}
	}
	config.Mods.WorkshopIDs = uniqueDigits(config.Mods.WorkshopIDs)
	config.Mods.ModPackIDs = uniqueStrings(config.Mods.ModPackIDs)
	return config
}

func configFromPayload(payload map[string]any, fallback Config) Config {
	config := normalizeConfig(fallback)
	if identity := objectPayload(payload, "identity"); identity != nil {
		config.Identity.ServerName = stringPayload(identity, "serverName")
		config.Identity.ClusterName = stringPayload(identity, "clusterName")
		config.Identity.Description = stringPayload(identity, "description")
		config.Identity.Password = stringPayload(identity, "password")
		config.Identity.ClusterToken = stringPayload(identity, "clusterToken")
		config.Identity.Visibility = stringPayload(identity, "visibility")
	}
	if gameplay := objectPayload(payload, "gameplay"); gameplay != nil {
		if value, ok := intPayload(gameplay, "maxPlayers"); ok {
			config.Gameplay.MaxPlayers = value
		}
		config.Gameplay.GameMode = stringPayload(gameplay, "gameMode")
		config.Gameplay.PVP = boolPayload(gameplay, "pvp")
		config.Gameplay.PauseWhenEmpty = boolPayloadDefault(gameplay, "pauseWhenEmpty", config.Gameplay.PauseWhenEmpty)
		config.Gameplay.ConsoleEnabled = boolPayloadDefault(gameplay, "consoleEnabled", config.Gameplay.ConsoleEnabled)
	}
	if world := objectPayload(payload, "world"); world != nil {
		config.World.Preset = stringPayload(world, "preset")
		config.World.Customize = boolPayload(world, "customize")
		config.World.Overrides = stringMapPayload(world, "overrides")
	}
	if caves := objectPayload(payload, "caves"); caves != nil {
		config.Caves = &DSTCaveConfig{
			Enabled:   boolPayload(caves, "enabled"),
			Preset:    stringPayload(caves, "preset"),
			Overrides: stringMapPayload(caves, "overrides"),
		}
	}
	if mods := objectPayload(payload, "mods"); mods != nil {
		config.Mods.WorkshopIDs = workshopIDsPayload(mods, "workshopIds")
		config.Mods.ModPackIDs = stringSlicePayload(mods, "modPackIds")
	}
	return normalizeConfig(config)
}

func payloadFromConfig(config Config) map[string]any {
	config = normalizeConfig(config)
	payload := map[string]any{
		"identity": map[string]any{
			"serverName":   config.Identity.ServerName,
			"clusterName":  config.Identity.ClusterName,
			"description":  config.Identity.Description,
			"clusterToken": config.Identity.ClusterToken,
			"visibility":   config.Identity.Visibility,
		},
		"gameplay": map[string]any{
			"maxPlayers":     config.Gameplay.MaxPlayers,
			"gameMode":       config.Gameplay.GameMode,
			"pvp":            config.Gameplay.PVP,
			"pauseWhenEmpty": config.Gameplay.PauseWhenEmpty,
			"consoleEnabled": config.Gameplay.ConsoleEnabled,
		},
		"world": map[string]any{
			"preset":    config.World.Preset,
			"customize": config.World.Customize,
			"overrides": cloneStringMap(config.World.Overrides),
		},
		"mods": map[string]any{
			"workshopIds": append([]string{}, config.Mods.WorkshopIDs...),
			"modPackIds":  append([]string{}, config.Mods.ModPackIDs...),
		},
	}
	identity := payload["identity"].(map[string]any)
	if config.Identity.Password != "" {
		identity["password"] = config.Identity.Password
	}
	if config.Caves != nil {
		payload["caves"] = map[string]any{
			"enabled":   config.Caves.Enabled,
			"preset":    config.Caves.Preset,
			"overrides": cloneStringMap(config.Caves.Overrides),
		}
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
	lanOnly := config.Identity.Visibility == "lan"
	offline := config.Identity.Visibility == "offline"
	lines := []string{
		"[GAMEPLAY]",
		"game_mode = " + config.Gameplay.GameMode,
		"max_players = " + fmt.Sprintf("%d", config.Gameplay.MaxPlayers),
		"pvp = " + boolINI(config.Gameplay.PVP),
		"pause_when_empty = " + boolINI(config.Gameplay.PauseWhenEmpty),
		"",
		"[NETWORK]",
		"cluster_name = " + config.Identity.ServerName,
		"cluster_description = " + config.Identity.Description,
		"cluster_password = " + config.Identity.Password,
		"lan_only_cluster = " + boolINI(lanOnly),
		"offline_server = " + boolINI(offline),
		"",
		"[MISC]",
		"console_enabled = " + boolINI(config.Gameplay.ConsoleEnabled),
		"",
	}
	return strings.Join(lines, "\n")
}

func clusterConfigDir(config Config) string {
	return "dst/" + config.Identity.ClusterName
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

func renderLevelDataOverrideLua(location string, preset string, overrides map[string]string) string {
	location = strings.TrimSpace(location)
	if location == "" {
		location = "forest"
	}
	preset = strings.TrimSpace(preset)
	if preset == "" {
		if location == "cave" {
			preset = "cave_default"
		} else {
			preset = "forest_default"
		}
	}
	lines := []string{
		"return {",
		"  id = \"SURVIVAL_TOGETHER\",",
		fmt.Sprintf("  location = %q,", location),
		"  version = 4,",
		"  override_enabled = true,",
		fmt.Sprintf("  preset = %q,", preset),
		"  overrides = {",
	}
	for _, key := range sortedStringKeys(overrides) {
		lines = append(lines, fmt.Sprintf("    %s = %q,", key, overrides[key]))
	}
	lines = append(lines, "  },", "}", "")
	return strings.Join(lines, "\n")
}

func renderWorkshopSetup(workshopIDs []string) string {
	if len(workshopIDs) == 0 {
		return "return nil\n"
	}
	lines := make([]string, 0, len(workshopIDs))
	for _, id := range workshopIDs {
		lines = append(lines, fmt.Sprintf("ServerModSetup(%q)", id))
	}
	return strings.Join(lines, "\n") + "\n"
}

func renderModOverrides(workshopIDs []string) string {
	lines := []string{"return {"}
	for _, id := range workshopIDs {
		lines = append(lines, fmt.Sprintf("  [\"workshop-%s\"] = { enabled = true },", id))
	}
	lines = append(lines, "}", "")
	return strings.Join(lines, "\n")
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

func objectPayload(payload map[string]any, key string) map[string]any {
	value, ok := payload[key]
	if !ok {
		return nil
	}
	object, ok := value.(map[string]any)
	if ok {
		return object
	}
	return nil
}

func stringMapPayload(payload map[string]any, key string) map[string]string {
	value, ok := payload[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case map[string]string:
		return cloneStringMap(typed)
	case map[string]any:
		out := map[string]string{}
		for key, value := range typed {
			if text, ok := value.(string); ok {
				out[key] = text
			}
		}
		return out
	default:
		return nil
	}
}

func stringSlicePayload(payload map[string]any, key string) []string {
	value, ok := payload[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
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

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func cleanOverrides(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, value := range values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func stringIn(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func validateOverride(key string, value string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("key is required")
	}
	for _, r := range key {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return fmt.Errorf("key must use lowercase letters, numbers, or underscore")
		}
	}
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("value is required")
	}
	if strings.ContainsAny(value, "\r\n\"\\") {
		return fmt.Errorf("value contains unsupported characters")
	}
	return nil
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
