package provider

import (
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
)

type versionedProvider struct {
	terraria.VanillaProvider
	plugin string
	config int
}

func (p versionedProvider) CatalogMetadata() domain.ProviderCatalogMetadata {
	metadata := p.VanillaProvider.CatalogMetadata()
	metadata.PluginVersion = p.plugin
	metadata.ConfigVersion = p.config
	return metadata
}
func TestRegistryRejectsInvalidVersionDeclarations(t *testing.T) {
	for _, version := range []string{"", "1", "1.0", "v1.0.0", "1.-1.0", "01.0.0", "1.0.0.0"} {
		if _, err := NewRegistry(versionedProvider{terraria.NewVanillaProvider(), version, 1}); err == nil {
			t.Fatalf("accepted %q", version)
		}
	}
	for _, config := range []int{0, -1} {
		if _, err := NewRegistry(versionedProvider{terraria.NewVanillaProvider(), "1.0.0", config}); err == nil {
			t.Fatalf("accepted config %d", config)
		}
	}
}
func TestConfigCompatibilityDoesNotTreatLegacyAsLatest(t *testing.T) {
	for _, current := range []int{1, 2} {
		p := versionedProvider{terraria.NewVanillaProvider(), "1.0.0", current}
		for _, stored := range []int{-1, 0, 1, 2, 3} {
			wantOK := stored == current || stored == 0 && current == 1
			if err := CheckConfigVersion(p, stored); (err == nil) != wantOK {
				t.Fatalf("current=%d stored=%d err=%v", current, stored, err)
			}
		}
	}
}
