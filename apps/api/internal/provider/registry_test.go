package provider

import (
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
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
