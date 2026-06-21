package palworld

import (
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestProviderCatalogMetadata(t *testing.T) {
	provider := NewProvider()
	if provider.GameKey() != domain.GamePalworld || provider.Key() != domain.ProviderPalworld {
		t.Fatalf("unexpected provider identity: %s %s", provider.GameKey(), provider.Key())
	}
	if provider.Capabilities().ConsoleCommands {
		t.Fatal("Palworld console commands should not be exposed in the first provider slice")
	}
	if !provider.Capabilities().SaveSnapshots || !provider.Capabilities().Backups {
		t.Fatalf("expected save and backup support, got %+v", provider.Capabilities())
	}
	if len(provider.ConfigSchema()) == 0 {
		t.Fatal("expected Palworld config schema")
	}
	names := map[string]bool{}
	for _, field := range provider.ConfigSchema() {
		names[field.Name] = true
	}
	for _, expected := range []string{"serverName", "saveName", "maxPlayers", "serverPassword", "adminPassword"} {
		if !names[expected] {
			t.Fatalf("expected config schema field %q, got %+v", expected, provider.ConfigSchema())
		}
	}
}

func TestNormalizeAndValidateConfig(t *testing.T) {
	config := normalizeConfig(Config{ServerName: "Pal Friends", SaveName: "Starter Save", MaxPlayers: 10, AdminPassword: "admin-secret"})
	if config.Port != DefaultInternalPort {
		t.Fatalf("expected internal port %d, got %d", DefaultInternalPort, config.Port)
	}
	if err := validateConfig(config); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
	bad := config
	bad.MaxPlayers = 64
	if err := validateConfig(bad); err == nil {
		t.Fatal("expected invalid max players to fail")
	}
}

func TestRuntimeOptionsUsePalworldImageAndUdpPort(t *testing.T) {
	config := normalizeConfig(Config{
		ServerName:     "Pal Friends",
		SaveName:       "Starter Save",
		MaxPlayers:     10,
		ServerPassword: "join-secret",
		AdminPassword:  "admin-secret",
	})
	provider := NewProvider()
	options := runtimeOptions(config)

	if provider.ImageFor("") != "smartcat99999/palworld-server:v2.4.1" {
		t.Fatalf("unexpected Palworld image: %s", provider.ImageFor(""))
	}
	if options.PortProtocol != "udp" {
		t.Fatalf("expected UDP port protocol, got %q", options.PortProtocol)
	}
	env := strings.Join(options.Env, "\n")
	for _, expected := range []string{
		"PORT=8211",
		"PLAYERS=10",
		"SERVER_NAME=Pal Friends",
		"SERVER_PASSWORD=join-secret",
		"ADMIN_PASSWORD=admin-secret",
	} {
		if !strings.Contains(env, expected) {
			t.Fatalf("expected env to contain %q, got:\n%s", expected, env)
		}
	}
}

func TestServerRuntimeUsesSemanticConfigPayload(t *testing.T) {
	provider := NewProvider()
	runtimeConfig, err := provider.RuntimeConfigForResource(domain.GameServer{
		Spec: domain.ServerSpec{Config: map[string]any{
			"serverName":     "Payload Name",
			"saveName":       "Payload Save",
			"maxPlayers":     float64(12),
			"serverPassword": "payload-password",
			"adminPassword":  "payload-admin",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtimeConfig.ConfigText != "" {
		t.Fatalf("Palworld resource runtime should not render legacy serverconfig.txt, got %q", runtimeConfig.ConfigText)
	}
	options := runtimeConfig.Options
	env := strings.Join(options.Env, "\n")
	for _, expected := range []string{
		"PLAYERS=12",
		"SERVER_NAME=Payload Name",
		"SERVER_PASSWORD=payload-password",
		"ADMIN_PASSWORD=payload-admin",
	} {
		if !strings.Contains(env, expected) {
			t.Fatalf("expected payload env to contain %q, got:\n%s", expected, env)
		}
	}
}

func TestRenderConfigSummary(t *testing.T) {
	rendered, err := renderConfig(Config{
		ServerName:     "Pal Friends",
		SaveName:       "Starter Save",
		MaxPlayers:     10,
		ServerPassword: "join-secret",
		AdminPassword:  "admin-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"game=palworld",
		"serverName=Pal Friends",
		"saveName=Starter Save",
		"maxPlayers=10",
		"port=8211",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected rendered config to contain %q, got:\n%s", expected, rendered)
		}
	}
}
