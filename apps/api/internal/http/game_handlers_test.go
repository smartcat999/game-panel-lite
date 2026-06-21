package http

import (
	"encoding/json"
	"fmt"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestGameCatalogEndpoints(t *testing.T) {
	router, _, _ := newTestRouter(t)

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/games", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected game catalog 200, got %d: %s", list.Code, list.Body.String())
	}
	var games []domain.GameCatalogEntry
	if err := json.Unmarshal(list.Body.Bytes(), &games); err != nil {
		t.Fatal(err)
	}
	var terrariaGame *domain.GameCatalogEntry
	var palworldGame *domain.GameCatalogEntry
	var minecraftGame *domain.GameCatalogEntry
	for index := range games {
		switch games[index].Key {
		case domain.GameTerraria:
			terrariaGame = &games[index]
		case domain.GamePalworld:
			palworldGame = &games[index]
		case domain.GameMinecraft:
			minecraftGame = &games[index]
		}
	}
	if terrariaGame == nil || terrariaGame.Status != "available" || len(terrariaGame.Providers) != 2 {
		t.Fatalf("expected available Terraria entry with two providers, got %+v", terrariaGame)
	}
	if palworldGame == nil || palworldGame.Status != "available" || len(palworldGame.Providers) != 1 {
		t.Fatalf("expected available Palworld entry with provider, got %+v", palworldGame)
	}
	if minecraftGame == nil || minecraftGame.Status != "available" || len(minecraftGame.Providers) != 1 {
		t.Fatalf("expected available Minecraft entry with provider, got %+v", minecraftGame)
	}
	if palworldGame.Providers[0].Key != domain.ProviderPalworld || palworldGame.Providers[0].Capabilities.ConsoleCommands {
		t.Fatalf("expected Palworld provider without console commands, got %+v", palworldGame.Providers[0])
	}
	if len(terrariaGame.Providers[0].ConfigSchema) == 0 || !terrariaGame.Providers[0].Capabilities.ConsoleCommands {
		t.Fatalf("expected provider schema and capabilities, got %+v", terrariaGame.Providers[0])
	}

	versions := httptest.NewRecorder()
	router.ServeHTTP(versions, httptest.NewRequest(stdhttp.MethodGet, "/api/games/terraria/versions", nil))
	if versions.Code != stdhttp.StatusOK {
		t.Fatalf("expected game versions 200, got %d: %s", versions.Code, versions.Body.String())
	}
	var versionPayload map[domain.ProviderKey][]string
	if err := json.Unmarshal(versions.Body.Bytes(), &versionPayload); err != nil {
		t.Fatal(err)
	}
	if len(versionPayload[domain.ProviderTerrariaVanilla]) == 0 || len(versionPayload[domain.ProviderTerrariaTModLoader]) == 0 {
		t.Fatalf("expected provider versions, got %+v", versionPayload)
	}

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(stdhttp.MethodGet, "/api/games/unknown-game", nil))
	if missing.Code != stdhttp.StatusNotFound {
		t.Fatalf("expected unknown game 404, got %d: %s", missing.Code, missing.Body.String())
	}
}

func TestGameCatalogServerCountsAndRecommendedVersion(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	for i := 0; i < 2; i++ {
		server := testServer("mc-"+fmt.Sprint(i), cfg.DataDir)
		server.GameKey = domain.GameMinecraft
		server.ProviderKey = domain.ProviderMinecraft
		createTestServer(t, db, server)
	}
	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/games", nil))
	var games []domain.GameCatalogEntry
	if err := json.Unmarshal(list.Body.Bytes(), &games); err != nil {
		t.Fatal(err)
	}
	var minecraftGame *domain.GameCatalogEntry
	var terrariaGame *domain.GameCatalogEntry
	for index := range games {
		if games[index].Key == domain.GameMinecraft {
			minecraftGame = &games[index]
		}
		if games[index].Key == domain.GameTerraria {
			terrariaGame = &games[index]
		}
	}
	if minecraftGame == nil {
		t.Fatal("expected minecraft game in catalog")
	}
	if terrariaGame == nil {
		t.Fatal("expected terraria game in catalog")
	}
	if minecraftGame.ServerCount != 2 {
		t.Fatalf("expected minecraft server count 2, got %d", minecraftGame.ServerCount)
	}
	if minecraftGame.CoverImage != "minecraft" {
		t.Fatalf("expected minecraft cover image, got %q", minecraftGame.CoverImage)
	}
	if minecraftGame.Providers[0].RecommendedVersion != "1.21.4" {
		t.Fatalf("expected minecraft to recommend stable version after latest, got %+v", minecraftGame.Providers[0])
	}
	var vanillaProvider *domain.ProviderCatalog
	for index := range terrariaGame.Providers {
		if terrariaGame.Providers[index].Key == domain.ProviderTerrariaVanilla {
			vanillaProvider = &terrariaGame.Providers[index]
		}
	}
	if vanillaProvider == nil {
		t.Fatal("expected Terraria vanilla provider in catalog")
	}
	if vanillaProvider.RecommendedVersion != "1.4.5.6" {
		t.Fatalf("expected Terraria vanilla to recommend latest explicit version, got %+v", vanillaProvider)
	}
}
