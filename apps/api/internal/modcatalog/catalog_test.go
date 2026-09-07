package modcatalog

import (
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"testing"
)

func TestRecommendedDSTModsUsesMaintainedFastTravel(t *testing.T) {
	items, err := RecommendedDSTMods()
	if err != nil {
		t.Fatalf("load recommended DST mods: %v", err)
	}

	var foundMaintained bool
	for _, item := range items {
		if item.WorkshopID == "458587300" {
			t.Fatal("legacy Fast Travel workshop item must not be recommended")
		}
		if item.WorkshopID == "1530801499" {
			foundMaintained = true
			if item.Title != "Fast Travel (GUI)" {
				t.Fatalf("unexpected maintained Fast Travel title %q", item.Title)
			}
		}
	}
	if !foundMaintained {
		t.Fatal("maintained Fast Travel workshop item is missing")
	}
}

func TestCatalogLookupDoesNotFallBackAcrossProviders(t *testing.T) {
	items, err := RecommendedTModLoaderMods()
	if err != nil || len(items) == 0 {
		t.Fatalf("catalog: %v", err)
	}
	for _, item := range items {
		if item.ModName == "" || item.WorkshopID == "" {
			continue
		}
		if _, ok := RecommendedModByProviderAndModName(domain.ProviderTerrariaTModLoader, item.ModName); !ok {
			t.Fatal("missing matching mod")
		}
		for _, key := range []domain.ProviderKey{domain.ProviderPalworld, domain.ProviderMinecraft, "unknown", ""} {
			if _, ok := RecommendedModByProviderAndWorkshopID(key, item.WorkshopID); ok {
				t.Fatalf("workshop fallback for %s", key)
			}
			if _, ok := RecommendedModByProviderAndModName(key, item.ModName); ok {
				t.Fatalf("name fallback for %s", key)
			}
		}
		return
	}
	t.Fatal("no named workshop fixture")
}
