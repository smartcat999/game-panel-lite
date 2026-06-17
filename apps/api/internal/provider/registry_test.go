package provider

import (
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/palworld"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
)

func TestRegistryFindsTerrariaProviders(t *testing.T) {
	registry := NewRegistry(terraria.NewVanillaProvider(), terraria.NewTModLoaderProvider())
	if _, ok := registry.Get(domain.ProviderTerrariaVanilla); !ok {
		t.Fatal("expected vanilla provider")
	}
	if _, ok := registry.Get(domain.ProviderTerrariaTModLoader); !ok {
		t.Fatal("expected tModLoader provider")
	}
}

func TestRegistryBuildsGameCatalog(t *testing.T) {
	registry := NewRegistry(terraria.NewVanillaProvider(), terraria.NewTModLoaderProvider(), palworld.NewProvider())
	games := registry.Games()
	if len(games) < 2 {
		t.Fatalf("expected available Terraria and Palworld entries, got %+v", games)
	}
	terrariaGame, ok := registry.Game(domain.GameTerraria)
	if !ok {
		t.Fatal("expected Terraria game catalog entry")
	}
	if terrariaGame.Status != "available" || len(terrariaGame.Providers) != 2 {
		t.Fatalf("expected available Terraria entry with two providers, got %+v", terrariaGame)
	}
	if !terrariaGame.Providers[0].Capabilities.ConsoleCommands || !terrariaGame.Providers[0].Capabilities.SaveSnapshots {
		t.Fatalf("expected Terraria capabilities to be exposed, got %+v", terrariaGame.Providers[0].Capabilities)
	}
	if len(terrariaGame.Providers[0].ConfigSchema) == 0 {
		t.Fatal("expected Terraria config schema")
	}
	palworldGame, ok := registry.Game(domain.GamePalworld)
	if !ok {
		t.Fatal("expected Palworld game catalog entry")
	}
	if palworldGame.Status != "available" || len(palworldGame.Providers) != 1 {
		t.Fatalf("expected available Palworld entry with provider, got %+v", palworldGame)
	}
	if palworldGame.Providers[0].Key != domain.ProviderPalworld || !palworldGame.Providers[0].Capabilities.SaveSnapshots {
		t.Fatalf("expected Palworld provider capabilities, got %+v", palworldGame.Providers[0])
	}
}
