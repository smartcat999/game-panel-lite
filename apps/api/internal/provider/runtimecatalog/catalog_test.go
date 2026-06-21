package runtimecatalog

import (
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestFromCatalogAppliesActiveRegistry(t *testing.T) {
	catalog := Catalog{
		ActiveRegistry: "cn",
		Registries: map[string]string{
			"global": "smartcat99999",
			"cn":     "registry.cn-hangzhou.aliyuncs.com/gamepanel-lite",
		},
		Providers: map[domain.ProviderKey]RuntimeConfig{
			domain.ProviderTerrariaVanilla: {
				ImageTemplate: "{registry}/terraria-vanilla:{version}",
				Versions:      []string{"1.4.5.6"},
			},
		},
	}

	config := FromCatalog([]Catalog{catalog}, domain.ProviderTerrariaVanilla, RuntimeConfig{})
	if got := config.ImageFor("1.4.5.6"); got != "registry.cn-hangzhou.aliyuncs.com/gamepanel-lite/terraria-vanilla:1.4.5.6" {
		t.Fatalf("expected active registry image, got %q", got)
	}
}

func TestCatalogActiveRegistryCanBeOverriddenByRegion(t *testing.T) {
	catalog := Catalog{
		ActiveRegistry: "global",
		Registries: map[string]string{
			"global": "smartcat99999",
			"cn":     "registry.cn-hangzhou.aliyuncs.com/gamepanel-lite",
		},
		Providers: map[domain.ProviderKey]RuntimeConfig{
			domain.ProviderTerrariaVanilla: {
				ImageTemplate: "{registry}/terraria-vanilla:{version}",
				Versions:      []string{"1.4.5.6"},
			},
		},
	}.WithActiveRegistry("cn")

	config := FromCatalog([]Catalog{catalog}, domain.ProviderTerrariaVanilla, RuntimeConfig{})
	if got := config.ImageFor(""); got != "registry.cn-hangzhou.aliyuncs.com/gamepanel-lite/terraria-vanilla:1.4.5.6" {
		t.Fatalf("expected region override to select cn registry, got %q", got)
	}
}

func TestRuntimeConfigFiltersLatestVersions(t *testing.T) {
	config := RuntimeConfig{
		ImageTemplate: "example/runtime:{version}",
		Versions:      []string{"latest", "1.0.0", "1.0.0", ""},
	}

	versions := config.VersionList()
	if len(versions) != 1 || versions[0] != "1.0.0" {
		t.Fatalf("expected concrete version list, got %v", versions)
	}
	if got := config.ImageFor("latest"); got != "example/runtime:1.0.0" {
		t.Fatalf("expected latest input to resolve to concrete version, got %q", got)
	}
}

func TestWithFallbackUsesConcreteFallbackWhenCatalogOnlyHasLatest(t *testing.T) {
	config := RuntimeConfig{
		ImageTemplate: "example/runtime:{version}",
		Versions:      []string{"latest"},
	}.WithFallback(RuntimeConfig{Versions: []string{"2.0.0"}})

	versions := config.VersionList()
	if len(versions) != 1 || versions[0] != "2.0.0" {
		t.Fatalf("expected concrete fallback version, got %v", versions)
	}
}
