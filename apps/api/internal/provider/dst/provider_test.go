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
	for _, expected := range []string{"serverName", "clusterName", "maxPlayers", "serverPassword", "clusterToken", "gameMode", "worldPreset", "cavesEnabled", "workshopIds"} {
		if !names[expected] {
			t.Fatalf("expected config schema field %q, got %+v", expected, provider.ConfigSchema())
		}
	}
}

func TestNormalizeAndValidateConfig(t *testing.T) {
	config := NormalizeConfig(domain.TerrariaConfig{ServerName: "DST Friends", WorldName: "Cluster", MaxPlayers: 6, MOTD: "klei-token"})
	if config.Port != DefaultInternalPort {
		t.Fatalf("expected internal port %d, got %d", DefaultInternalPort, config.Port)
	}
	if err := NewProvider().ValidateConfig(config); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
	bad := config
	bad.MOTD = ""
	if err := NewProvider().ValidateConfig(bad); err == nil {
		t.Fatal("expected missing Klei token to fail")
	}
}

func TestRuntimeOptionsRenderDSTFiles(t *testing.T) {
	config := NormalizeConfig(domain.TerrariaConfig{
		ServerName: "DST Friends",
		WorldName:  "Cluster",
		MaxPlayers: 5,
		Password:   "join-secret",
		MOTD:       "klei-token",
	})
	provider := NewProvider()
	options := provider.RuntimeOptions(config)

	if provider.ImageFor("latest") != "smartcat99999/dst-server:latest" {
		t.Fatalf("unexpected DST image: %s", provider.ImageFor("latest"))
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
	server := domain.GameServerInstance{
		Config: NormalizeConfig(domain.TerrariaConfig{
			ServerName: "Old Name",
			WorldName:  "Old Cluster",
			MaxPlayers: 4,
			Password:   "old-password",
			MOTD:       "old-token",
		}),
		ConfigPayload: map[string]any{
			"serverName":     "Payload Name",
			"clusterName":    "Payload Cluster",
			"maxPlayers":     float64(12),
			"serverPassword": "payload-password",
			"clusterToken":   "payload-token",
			"gameMode":       "endless",
			"worldPreset":    "forest_classic",
			"cavesEnabled":   true,
			"workshopIds":    "123456789, 987654321",
		},
	}
	rendered, err := provider.RenderServerConfig(server)
	if err != nil {
		t.Fatal(err)
	}
	options, err := provider.RuntimeOptionsForServer(server)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"serverName=Payload Name",
		"clusterName=Payload Cluster",
		"maxPlayers=12",
		"serverPassword=payload-password",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected rendered payload config to contain %q, got:\n%s", expected, rendered)
		}
	}
	if !strings.Contains(options.Files["dst/Payload Cluster/cluster.ini"], "game_mode = endless") {
		t.Fatalf("expected payload game mode in cluster.ini, got:\n%s", options.Files["dst/Payload Cluster/cluster.ini"])
	}
	if !strings.Contains(options.Files["dst/Payload Cluster/Master/worldgen.lua"], `preset = "forest_classic"`) {
		t.Fatalf("expected payload world preset in Master worldgen, got:\n%s", options.Files["dst/Payload Cluster/Master/worldgen.lua"])
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
