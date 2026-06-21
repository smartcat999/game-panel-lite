package http

import (
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/minecraft"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
)

func TestPlayerManagementGatedByProviderCapability(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	palworldServer := testServer("palworld-players", cfg.DataDir)
	palworldServer.GameKey = domain.GamePalworld
	palworldServer.ProviderKey = domain.ProviderPalworld
	createTestServer(t, db, palworldServer)

	palworldPlayers := httptest.NewRecorder()
	router.ServeHTTP(palworldPlayers, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/palworld-players/players", nil))
	if palworldPlayers.Code != stdhttp.StatusOK {
		t.Fatalf("expected players 200, got %d: %s", palworldPlayers.Code, palworldPlayers.Body.String())
	}
	var palworldResp struct {
		Supported bool            `json:"supported"`
		Players   []domain.Player `json:"players"`
	}
	if err := json.Unmarshal(palworldPlayers.Body.Bytes(), &palworldResp); err != nil {
		t.Fatal(err)
	}
	if palworldResp.Supported {
		t.Fatal("expected Palworld player list to be unsupported")
	}

	palworldKick := httptest.NewRecorder()
	router.ServeHTTP(palworldKick, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/palworld-players/players/Alice/kick", nil))
	if palworldKick.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected Palworld kick to be rejected 400, got %d: %s", palworldKick.Code, palworldKick.Body.String())
	}

	palworldBan := httptest.NewRecorder()
	router.ServeHTTP(palworldBan, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/palworld-players/players/Alice/ban", nil))
	if palworldBan.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected Palworld ban to be rejected 400, got %d: %s", palworldBan.Code, palworldBan.Body.String())
	}

	terrariaServer := testServer("terraria-players", cfg.DataDir)
	terrariaServer.GameKey = domain.GameTerraria
	terrariaServer.ProviderKey = domain.ProviderTerrariaVanilla
	createTestServer(t, db, terrariaServer)
	terrariaPlayers := httptest.NewRecorder()
	router.ServeHTTP(terrariaPlayers, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/terraria-players/players", nil))
	if terrariaPlayers.Code != stdhttp.StatusOK {
		t.Fatalf("expected terraria players 200, got %d: %s", terrariaPlayers.Code, terrariaPlayers.Body.String())
	}
	var terrariaResp struct {
		Supported bool            `json:"supported"`
		Players   []domain.Player `json:"players"`
	}
	if err := json.Unmarshal(terrariaPlayers.Body.Bytes(), &terrariaResp); err != nil {
		t.Fatal(err)
	}
	if !terrariaResp.Supported {
		t.Fatal("expected Terraria player list to be supported")
	}

	terrariaKick := httptest.NewRecorder()
	router.ServeHTTP(terrariaKick, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/terraria-players/players/Alice/kick", nil))
	if terrariaKick.Code != stdhttp.StatusConflict {
		t.Fatalf("expected terraria kick conflict for stopped server, got %d: %s", terrariaKick.Code, terrariaKick.Body.String())
	}
}

func TestMinecraftWhitelistManagement(t *testing.T) {
	adapter := newCommandCaptureAdapter()
	router, db, cfg := newTestRouterWithAdapter(t, adapter)

	minecraftServer := testServer("minecraft-whitelist", cfg.DataDir)
	minecraftServer.GameKey = domain.GameMinecraft
	minecraftServer.ProviderKey = domain.ProviderMinecraft
	minecraftServer.Status = domain.StatusRunning
	minecraftServer.ContainerID = "container-minecraft-whitelist"
	minecraftServer.Port = minecraft.DefaultInternalPort
	minecraftServer.Config = terraria.Config{
		ServerName: "Minecraft Server",
		WorldName:  "world",
		MaxPlayers: 20,
		Port:       minecraft.DefaultInternalPort,
		Secure:     true,
	}
	minecraftServer.ConfigPayload = map[string]any{
		"serverName":       minecraftServer.Config.ServerName,
		"worldName":        minecraftServer.Config.WorldName,
		"maxPlayers":       minecraftServer.Config.MaxPlayers,
		"gameMode":         "survival",
		"difficulty":       "normal",
		"onlineMode":       true,
		"whitelistEnabled": true,
		"eulaAccepted":     true,
	}
	createTestServer(t, db, minecraftServer)

	status := httptest.NewRecorder()
	router.ServeHTTP(status, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/minecraft-whitelist/whitelist", nil))
	if status.Code != stdhttp.StatusOK {
		t.Fatalf("expected whitelist status 200, got %d: %s", status.Code, status.Body.String())
	}
	var statusResp struct {
		Supported bool `json:"supported"`
		Running   bool `json:"running"`
	}
	if err := json.Unmarshal(status.Body.Bytes(), &statusResp); err != nil {
		t.Fatal(err)
	}
	if !statusResp.Supported || !statusResp.Running {
		t.Fatalf("expected supported running whitelist response, got %+v", statusResp)
	}

	add := httptest.NewRecorder()
	router.ServeHTTP(add, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/minecraft-whitelist/whitelist/Steve", nil))
	if add.Code != stdhttp.StatusOK {
		t.Fatalf("expected whitelist add 200, got %d: %s", add.Code, add.Body.String())
	}
	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/minecraft-whitelist/whitelist/Alex", nil))
	if remove.Code != stdhttp.StatusOK {
		t.Fatalf("expected whitelist remove 200, got %d: %s", remove.Code, remove.Body.String())
	}
	if !reflect.DeepEqual(adapter.commands, []string{"whitelist add Steve", "whitelist remove Alex"}) {
		t.Fatalf("unexpected whitelist commands: %+v", adapter.commands)
	}

	palworldServer := testServer("palworld-whitelist", cfg.DataDir)
	palworldServer.GameKey = domain.GamePalworld
	palworldServer.ProviderKey = domain.ProviderPalworld
	createTestServer(t, db, palworldServer)
	palworldStatus := httptest.NewRecorder()
	router.ServeHTTP(palworldStatus, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/palworld-whitelist/whitelist", nil))
	if palworldStatus.Code != stdhttp.StatusOK {
		t.Fatalf("expected Palworld whitelist status 200, got %d: %s", palworldStatus.Code, palworldStatus.Body.String())
	}
	var palworldResp struct {
		Supported bool `json:"supported"`
	}
	if err := json.Unmarshal(palworldStatus.Body.Bytes(), &palworldResp); err != nil {
		t.Fatal(err)
	}
	if palworldResp.Supported {
		t.Fatal("expected Palworld whitelist to be unsupported")
	}
	palworldAdd := httptest.NewRecorder()
	router.ServeHTTP(palworldAdd, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/palworld-whitelist/whitelist/Alice", nil))
	if palworldAdd.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected unsupported Palworld whitelist add 400, got %d: %s", palworldAdd.Code, palworldAdd.Body.String())
	}
}
