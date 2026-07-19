package dst

import (
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestProviderCatalogMetadata(t *testing.T) {
	provider := NewProvider()
	if provider.GameKey() != domain.GameDST || provider.Key() != domain.ProviderDST {
		t.Fatalf("unexpected provider identity: %s %s", provider.GameKey(), provider.Key())
	}
	if provider.Capabilities().ConsoleCommands {
		t.Fatal("DST console commands should not be exposed in the first provider slice")
	}
	if !provider.Capabilities().SaveSnapshots || !provider.Capabilities().Backups || !provider.Capabilities().WorldRegeneration {
		t.Fatalf("expected save and backup support, got %+v", provider.Capabilities())
	}
	names := map[string]bool{}
	for _, field := range provider.ConfigSchema() {
		names[field.Name] = true
	}
	if got := len(provider.ConfigSchema()); got < 200 {
		t.Fatalf("expected current DST schema to expose the complete world menu, got %d fields", got)
	}
	grassGekkoFound := false
	for _, field := range provider.ConfigSchema() {
		if field.Name != "world.overrides.grassgekkos" {
			continue
		}
		grassGekkoFound = field.Label == "草壁虎转化" && len(field.Options) == 5
	}
	if !grassGekkoFound {
		t.Fatal("expected official Chinese Grass Gekko Morphing world setting")
	}
	for _, expected := range []string{"identity.serverName", "identity.clusterName", "identity.description", "identity.password", "identity.clusterToken", "identity.visibility", "gameplay.maxPlayers", "gameplay.gameMode", "gameplay.pvp", "gameplay.pauseWhenEmpty", "gameplay.consoleEnabled", "world.preset", "caves.enabled"} {
		if !names[expected] {
			t.Fatalf("expected config schema field %q, got %+v", expected, provider.ConfigSchema())
		}
	}
	for _, expected := range []string{"world.overrides.day", "world.overrides.world_size", "world.overrides.regrowth", "caves.overrides.cavelight", "caves.overrides.wormattacks"} {
		if !names[expected] {
			t.Fatalf("expected world setting field %q", expected)
		}
	}
	for _, field := range provider.ConfigSchema() {
		if strings.Contains(field.Name, ".overrides.") && (field.Group == "" || len(field.Options) == 0) {
			t.Fatalf("world setting %q must be grouped and constrained", field.Name)
		}
	}
	if names["workshopIds"] {
		t.Fatalf("workshop IDs should be managed from the mod library, not the config schema: %+v", provider.ConfigSchema())
	}
}

func TestDSTWorkshopRenderingFiltersClientOnlyModsAndRendersConfiguration(t *testing.T) {
	config := normalizeConfig(Config{
		Identity: DSTIdentityConfig{ServerName: "DST", ClusterName: "Mods", ClusterToken: "token"},
		Mods: DSTModConfig{
			WorkshopIDs: []string{"376333686", "378160973"}, // Combined Status is client-only; Global Positions is server-required.
			Configurations: map[string]map[string]any{
				"378160973": {"ENABLEPINGS": false, "SHOWPLAYERSOPTIONS": true, "position": "right"},
			},
		},
	})
	options := runtimeOptions(config)
	setup := options.Files["dst/Mods/dedicated_server_mods_setup.lua"]
	if strings.Contains(setup, "376333686") || !strings.Contains(setup, `ServerModSetup("378160973")`) {
		t.Fatalf("expected only server-required mods in setup file, got:\n%s", setup)
	}
	overrides := options.Files["dst/Mods/Master/modoverrides.lua"]
	for _, expected := range []string{`["workshop-378160973"]`, `["ENABLEPINGS"] = false`, `["SHOWPLAYERSOPTIONS"] = true`, `["position"] = "right"`} {
		if !strings.Contains(overrides, expected) {
			t.Fatalf("expected configured server mod value %q, got:\n%s", expected, overrides)
		}
	}
	if strings.Contains(overrides, "376333686") {
		t.Fatalf("client-only mod must not be activated on the dedicated server:\n%s", overrides)
	}
}

func TestWorldRegenerationPlanTargetsEnabledShardsOnly(t *testing.T) {
	provider := NewProvider()
	server := domain.GameServer{Spec: domain.ServerSpec{Config: map[string]any{
		"identity": map[string]any{"serverName": "DST", "clusterName": "Friends", "clusterToken": "token"},
		"caves":    map[string]any{"enabled": true, "preset": "cave_default"},
	}}}
	plan, err := provider.WorldRegenerationPlan(server)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"dst/Friends/Master/save", "dst/Friends/Caves/save"}
	if len(plan.SavePaths) != len(want) {
		t.Fatalf("expected both shard saves, got %+v", plan.SavePaths)
	}
	for index := range want {
		if plan.SavePaths[index] != want[index] {
			t.Fatalf("expected path %q, got %+v", want[index], plan.SavePaths)
		}
	}
}

func TestWorldOverridesFromPayloadRenderIntoBothShards(t *testing.T) {
	config := configFromPayload(map[string]any{
		"identity": map[string]any{"serverName": "DST", "clusterName": "Cluster", "clusterToken": "token"},
		"world":    map[string]any{"overrides": map[string]any{"day": "longday", "world_size": "huge"}},
		"caves":    map[string]any{"enabled": true, "overrides": map[string]any{"cavelight": "slow", "wormattacks": "rare"}},
	}, defaultConfig())
	forest := renderLevelDataOverrideLua("forest", config.World.Preset, config.World.Overrides)
	if config.Caves == nil {
		t.Fatal("expected caves config to remain enabled")
	}
	caves := renderLevelDataOverrideLua("cave", config.Caves.Preset, config.Caves.Overrides)
	for _, expected := range []string{"day = \"longday\"", "world_size = \"huge\""} {
		if !strings.Contains(forest, expected) {
			t.Fatalf("forest override missing %q:\n%s", expected, forest)
		}
	}
	for _, expected := range []string{"cavelight = \"slow\"", "wormattacks = \"rare\""} {
		if !strings.Contains(caves, expected) {
			t.Fatalf("cave override missing %q:\n%s", expected, caves)
		}
	}
}

func TestKnownWorldOverrideRejectsUnsupportedValue(t *testing.T) {
	config := defaultConfig()
	config.Identity.ClusterToken = "token"
	config.World.Overrides = map[string]string{"day": "sometimes"}
	if err := validateConfig(config); err == nil || !strings.Contains(err.Error(), "unsupported value") {
		t.Fatalf("expected constrained override validation, got %v", err)
	}
}

func TestCaveSchemaUsesCaveSpecificWorldGenerationDefaults(t *testing.T) {
	defaults := map[string]any{}
	for _, field := range configSchema() {
		if field.Name == "caves.overrides.task_set" || field.Name == "caves.overrides.start_location" {
			defaults[field.Name] = field.Default
		}
	}
	if defaults["caves.overrides.task_set"] != "cave_default" {
		t.Fatalf("expected cave task set default, got %v", defaults["caves.overrides.task_set"])
	}
	if defaults["caves.overrides.start_location"] != "caves" {
		t.Fatalf("expected cave start location default, got %v", defaults["caves.overrides.start_location"])
	}
}

func TestLegacyCaveWorldGenerationDefaultsAreNormalized(t *testing.T) {
	config := configFromPayload(map[string]any{
		"identity": map[string]any{"serverName": "DST", "clusterName": "Cluster", "clusterToken": "token"},
		"caves": map[string]any{
			"enabled": true,
			"overrides": map[string]any{
				"task_set":       "default",
				"start_location": "default",
			},
		},
	}, defaultConfig())
	if got := config.Caves.Overrides["task_set"]; got != "cave_default" {
		t.Fatalf("expected legacy cave task set to normalize, got %q", got)
	}
	if got := config.Caves.Overrides["start_location"]; got != "caves" {
		t.Fatalf("expected legacy cave start location to normalize, got %q", got)
	}
	if err := validateConfig(config); err != nil {
		t.Fatalf("expected normalized cave config to validate: %v", err)
	}
}

func TestNormalizeAndValidateConfig(t *testing.T) {
	config := normalizeConfig(Config{
		Identity: DSTIdentityConfig{ServerName: "DST Friends", ClusterName: "Cluster", ClusterToken: "klei-token"},
		Gameplay: DSTGameplayConfig{MaxPlayers: 6},
	})
	if config.Port != DefaultInternalPort {
		t.Fatalf("expected internal port %d, got %d", DefaultInternalPort, config.Port)
	}
	if err := validateConfig(config); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
	bad := config
	bad.Identity.ClusterToken = ""
	if err := validateConfig(bad); err == nil {
		t.Fatal("expected missing Klei token to fail")
	}
}

func TestRuntimeOptionsRenderDSTFiles(t *testing.T) {
	config := normalizeConfig(Config{
		Identity: DSTIdentityConfig{
			ServerName:   "DST Friends",
			ClusterName:  "Cluster",
			Password:     "join-secret",
			ClusterToken: "klei-token",
		},
		Gameplay: DSTGameplayConfig{
			MaxPlayers:     5,
			PauseWhenEmpty: true,
			ConsoleEnabled: true,
		},
	})
	provider := NewProvider()
	options := runtimeOptions(config)

	if provider.ImageFor("") != "smartcat99999/dst-server:v2026.07.17" {
		t.Fatalf("unexpected DST image: %s", provider.ImageFor(""))
	}
	if options.PortProtocol != "udp" {
		t.Fatalf("expected UDP port protocol, got %q", options.PortProtocol)
	}
	cluster := options.Files["dst/Cluster/cluster.ini"]
	for _, expected := range []string{
		"cluster_name = DST Friends",
		"cluster_password = join-secret",
		"max_players = 5",
		"game_mode = survival",
	} {
		if !strings.Contains(cluster, expected) {
			t.Fatalf("expected cluster.ini to contain %q, got:\n%s", expected, cluster)
		}
	}
	if got := options.Files["dst/Cluster/cluster_token.txt"]; got != "klei-token\n" {
		t.Fatalf("expected server token file, got %q", got)
	}
	if !strings.Contains(options.Files["dst/Cluster/Master/server.ini"], "server_port = 10999") {
		t.Fatalf("expected Master server.ini to contain port, got:\n%s", options.Files["dst/Cluster/Master/server.ini"])
	}
}

func TestServerRuntimeUsesSemanticConfigPayload(t *testing.T) {
	provider := NewProvider()
	runtimeConfig, err := provider.RuntimeConfigForResource(domain.GameServer{
		Spec: domain.ServerSpec{Config: map[string]any{
			"identity": map[string]any{
				"serverName":   "Payload Name",
				"clusterName":  "Payload Cluster",
				"description":  "Friends only",
				"password":     "payload-password",
				"clusterToken": "payload-token",
			},
			"gameplay": map[string]any{
				"maxPlayers":     float64(12),
				"gameMode":       "endless",
				"pauseWhenEmpty": false,
			},
			"world": map[string]any{
				"preset": "forest_classic",
			},
			"caves": map[string]any{
				"enabled": true,
			},
			"mods": map[string]any{
				"workshopIds": []any{"123456789", "987654321"},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtimeConfig.ConfigText != "" {
		t.Fatalf("DST resource runtime should not render legacy serverconfig.txt, got %q", runtimeConfig.ConfigText)
	}
	if len(runtimeConfig.AdditionalPorts) != 1 || runtimeConfig.AdditionalPorts[0] != 11000 {
		t.Fatalf("expected Caves UDP port 11000, got %v", runtimeConfig.AdditionalPorts)
	}
	options := runtimeConfig.Options
	if !strings.Contains(options.Files["dst/Payload Cluster/cluster.ini"], "game_mode = endless") {
		t.Fatalf("expected payload game mode in cluster.ini, got:\n%s", options.Files["dst/Payload Cluster/cluster.ini"])
	}
	if !strings.Contains(options.Files["dst/Payload Cluster/cluster.ini"], "cluster_description = Friends only") || !strings.Contains(options.Files["dst/Payload Cluster/cluster.ini"], "pause_when_empty = false") {
		t.Fatalf("expected payload cluster settings in cluster.ini, got:\n%s", options.Files["dst/Payload Cluster/cluster.ini"])
	}
	if !strings.Contains(options.Files["dst/Payload Cluster/Master/leveldataoverride.lua"], `preset = "forest_classic"`) {
		t.Fatalf("expected payload world preset in Master leveldataoverride, got:\n%s", options.Files["dst/Payload Cluster/Master/leveldataoverride.lua"])
	}
	for _, expected := range []string{`id = "SURVIVAL_TOGETHER"`, `name = "Forest"`, `desc = ""`, `layout_mode = "LinkNodesByKeys"`, `task_set = "default"`, `has_ocean = true`, `required_setpieces = { "Sculptures_1", "Maxwell5" }`, `numrandom_set_pieces = 4`} {
		if !strings.Contains(options.Files["dst/Payload Cluster/Master/leveldataoverride.lua"], expected) {
			t.Fatalf("expected current DST level data field %q, got:\n%s", expected, options.Files["dst/Payload Cluster/Master/leveldataoverride.lua"])
		}
	}
	for _, expected := range []string{"[SHARD]", "shard_enabled = true", "master_port = 10888", "cluster_key = gamepanel-lite"} {
		if !strings.Contains(options.Files["dst/Payload Cluster/cluster.ini"], expected) {
			t.Fatalf("expected caves shard setting %q, got:\n%s", expected, options.Files["dst/Payload Cluster/cluster.ini"])
		}
	}
	if _, ok := options.Files["dst/Payload Cluster/Caves/server.ini"]; !ok {
		t.Fatalf("expected caves shard files when caves are enabled, got %+v", options.Files)
	}
	for _, expected := range []string{`id = "DST_CAVE"`, `layout_mode = "RestrictNodesByKey"`, `start_location = "caves"`, `task_set = "cave_default"`, `background_node_range = { 0, 1 }`} {
		if !strings.Contains(options.Files["dst/Payload Cluster/Caves/leveldataoverride.lua"], expected) {
			t.Fatalf("expected cave world generation default %q, got:\n%s", expected, options.Files["dst/Payload Cluster/Caves/leveldataoverride.lua"])
		}
	}
	if !strings.Contains(options.Files["dst/Payload Cluster/dedicated_server_mods_setup.lua"], `ServerModSetup("123456789")`) {
		t.Fatalf("expected workshop setup file, got:\n%s", options.Files["dst/Payload Cluster/dedicated_server_mods_setup.lua"])
	}
	if got := options.Files["dst/Payload Cluster/cluster_token.txt"]; got != "payload-token\n" {
		t.Fatalf("expected payload token file, got %q", got)
	}
}
