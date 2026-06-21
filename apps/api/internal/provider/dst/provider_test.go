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
	if !provider.Capabilities().SaveSnapshots || !provider.Capabilities().Backups {
		t.Fatalf("expected save and backup support, got %+v", provider.Capabilities())
	}
	names := map[string]bool{}
	for _, field := range provider.ConfigSchema() {
		names[field.Name] = true
	}
	for _, expected := range []string{"identity.serverName", "identity.clusterName", "identity.description", "identity.password", "identity.clusterToken", "identity.visibility", "gameplay.maxPlayers", "gameplay.gameMode", "gameplay.pvp", "gameplay.pauseWhenEmpty", "gameplay.consoleEnabled", "world.preset", "caves.enabled"} {
		if !names[expected] {
			t.Fatalf("expected config schema field %q, got %+v", expected, provider.ConfigSchema())
		}
	}
	if names["workshopIds"] {
		t.Fatalf("workshop IDs should be managed from the mod library, not the config schema: %+v", provider.ConfigSchema())
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

	if provider.ImageFor("") != "smartcat99999/dst-server:v2026.06.21" {
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
	if _, ok := options.Files["dst/Payload Cluster/Caves/server.ini"]; !ok {
		t.Fatalf("expected caves shard files when caves are enabled, got %+v", options.Files)
	}
	if !strings.Contains(options.Files["dst/Payload Cluster/dedicated_server_mods_setup.lua"], `ServerModSetup("123456789")`) {
		t.Fatalf("expected workshop setup file, got:\n%s", options.Files["dst/Payload Cluster/dedicated_server_mods_setup.lua"])
	}
	if got := options.Files["dst/Payload Cluster/cluster_token.txt"]; got != "payload-token\n" {
		t.Fatalf("expected payload token file, got %q", got)
	}
}
