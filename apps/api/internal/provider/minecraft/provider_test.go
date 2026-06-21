package minecraft

import (
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestProviderCatalogMetadata(t *testing.T) {
	provider := NewProvider()
	if provider.GameKey() != domain.GameMinecraft || provider.Key() != domain.ProviderMinecraft {
		t.Fatalf("unexpected provider identity: %s %s", provider.GameKey(), provider.Key())
	}
	if !provider.Capabilities().ConsoleCommands || !provider.Capabilities().PlayerList {
		t.Fatalf("expected console and player list support, got %+v", provider.Capabilities())
	}
	if !provider.Capabilities().KickPlayer || !provider.Capabilities().BanPlayer {
		t.Fatalf("expected kick and ban support, got %+v", provider.Capabilities())
	}
	names := map[string]bool{}
	for _, field := range provider.ConfigSchema() {
		names[field.Name] = true
	}
	for _, expected := range []string{"serverName", "worldName", "maxPlayers", "gameMode", "difficulty", "onlineMode", "whitelistEnabled", "eulaAccepted"} {
		if !names[expected] {
			t.Fatalf("expected config schema field %q, got %+v", expected, provider.ConfigSchema())
		}
	}
}

func TestImageAndVersions(t *testing.T) {
	provider := NewProvider()
	if provider.ImageFor("") != "smartcat99999/minecraft-server:2026.6.0-java21" {
		t.Fatalf("unexpected minecraft image: %s", provider.ImageFor(""))
	}
	if provider.ImageFor("1.21.4") != "smartcat99999/minecraft-server:2026.6.0-java21" {
		t.Fatalf("minecraft game versions should be passed via VERSION env, got image %s", provider.ImageFor("1.21.4"))
	}
	for _, version := range provider.Versions() {
		if version == "latest" {
			t.Fatalf("provider versions must be concrete, got %v", provider.Versions())
		}
	}
	if len(provider.Versions()) < 2 {
		t.Fatalf("expected multiple versions, got %v", provider.Versions())
	}
}

func TestNormalizeAndValidateConfig(t *testing.T) {
	config := normalizeConfig(Config{ServerName: "Friends", WorldName: "world", MaxPlayers: 20, EULAAccepted: true})
	if config.Port != DefaultInternalPort {
		t.Fatalf("expected internal port %d, got %d", DefaultInternalPort, config.Port)
	}
	if err := validateConfig(config); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
	bad := config
	bad.EULAAccepted = false
	if err := validateConfig(bad); err == nil {
		t.Fatal("expected missing EULA to fail")
	}
}

func TestRuntimeOptionsRenderMinecraftFiles(t *testing.T) {
	config := normalizeConfig(Config{
		ServerName:   "Friends Server",
		WorldName:    "survival-island",
		MaxPlayers:   16,
		EULAAccepted: true,
		OnlineMode:   true,
	})
	options := runtimeOptions(config)

	if options.PortProtocol != "tcp" {
		t.Fatalf("expected TCP port protocol, got %q", options.PortProtocol)
	}
	properties := options.Files["data/server.properties"]
	for _, expected := range []string{
		"max-players=16",
		"motd=Friends Server",
		"level-name=survival-island",
		"gamemode=survival",
		"difficulty=normal",
		"online-mode=true",
		"white-list=false",
	} {
		if !strings.Contains(properties, expected) {
			t.Fatalf("expected server.properties to contain %q, got:\n%s", expected, properties)
		}
	}
	if got := options.Files["data/eula.txt"]; got != "eula=true\n" {
		t.Fatalf("expected eula=true, got %q", got)
	}
	if !containsEnv(options.Env, "EULA=TRUE") {
		t.Fatalf("expected EULA=TRUE env, got %v", options.Env)
	}
	if !containsEnv(options.Env, "VERSION=1.21.4") {
		t.Fatalf("expected default VERSION=1.21.4 env, got %v", options.Env)
	}
}

func containsEnv(env []string, target string) bool {
	for _, e := range env {
		if e == target {
			return true
		}
	}
	return false
}

func TestServerRuntimeUsesSemanticConfigPayload(t *testing.T) {
	provider := NewProvider()
	runtimeConfig, err := provider.RuntimeConfigForResource(domain.GameServer{
		Spec: domain.ServerSpec{Config: map[string]any{
			"serverName":       "Payload Server",
			"worldName":        "payload-world",
			"maxPlayers":       float64(24),
			"gameMode":         "creative",
			"difficulty":       "hard",
			"onlineMode":       false,
			"whitelistEnabled": true,
			"eulaAccepted":     true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	options := runtimeConfig.Options
	if !containsEnv(options.Env, "VERSION=1.21.4") {
		t.Fatalf("expected default VERSION=1.21.4 env, got %v", options.Env)
	}
	properties := options.Files["data/server.properties"]
	for _, expected := range []string{
		"motd=Payload Server",
		"level-name=payload-world",
		"max-players=24",
		"gamemode=creative",
		"difficulty=hard",
		"online-mode=false",
		"white-list=true",
	} {
		if !strings.Contains(properties, expected) {
			t.Fatalf("expected server.properties to contain %q, got:\n%s", expected, properties)
		}
	}
}

func TestConfigFromPayloadAndEnrich(t *testing.T) {
	payload := map[string]any{
		"serverName":   "Test",
		"worldName":    "test-world",
		"maxPlayers":   float64(8),
		"eulaAccepted": true,
		"gameMode":     "adventure",
	}
	config := configFromPayload(payload, Config{})
	if config.ServerName != "Test" || config.WorldName != "test-world" {
		t.Fatalf("unexpected config: %+v", config)
	}
	if !config.EULAAccepted {
		t.Fatal("expected eulaAccepted to map to Secure=true")
	}
	enriched := payloadFromConfig(config)
	if enriched["gameMode"] != "adventure" {
		t.Fatalf("expected enriched game mode adventure, got %v", enriched["gameMode"])
	}
	if enriched["eulaAccepted"] != true {
		t.Fatalf("expected enriched eulaAccepted true, got %v", enriched["eulaAccepted"])
	}
}

func TestJoinInfo(t *testing.T) {
	server := domain.GameServer{
		Name: "Survival",
		Spec: domain.ServerSpec{
			Network: domain.ServerNetworkSpec{Port: 25565, HostPort: 25565},
		},
	}
	info := NewProvider().JoinInfo(server)
	if info.Port != 25565 {
		t.Fatalf("expected port 25565, got %d", info.Port)
	}
	if !strings.Contains(info.InviteText, "25565") || !strings.Contains(info.InviteText, "Minecraft") {
		t.Fatalf("unexpected invite text: %s", info.InviteText)
	}
}

func TestKickBanCommands(t *testing.T) {
	provider := NewProvider()
	if cmd := provider.KickCommand("Steve"); cmd != "kick Steve" {
		t.Fatalf("expected minecraft kick command, got %q", cmd)
	}
	if cmd := provider.BanCommand("Alex"); cmd != "ban Alex" {
		t.Fatalf("expected minecraft ban command, got %q", cmd)
	}
	if cmd := provider.WhitelistAddCommand("Steve"); cmd != "whitelist add Steve" {
		t.Fatalf("expected minecraft whitelist add command, got %q", cmd)
	}
	if cmd := provider.WhitelistRemoveCommand("Alex"); cmd != "whitelist remove Alex" {
		t.Fatalf("expected minecraft whitelist remove command, got %q", cmd)
	}
	if cmd := provider.WhitelistListCommand(); cmd != "whitelist list" {
		t.Fatalf("expected minecraft whitelist list command, got %q", cmd)
	}
	if cmd := provider.KickCommand("name\ninjection"); cmd != "kick nameinjection" {
		t.Fatalf("expected sanitized kick command, got %q", cmd)
	}
}
