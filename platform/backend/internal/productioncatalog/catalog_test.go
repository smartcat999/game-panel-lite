package productioncatalog

import (
	"context"
	"testing"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
)

func TestProductionCatalogPublishesSignedProviders(t *testing.T) {
	key := []byte("production-provider-key-0123456789")
	registry, providers, err := Publish(context.Background(), providercontract.NewMemoryStore(), key)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{TerrariaReleaseID, TModLoaderReleaseID} {
		manifest, err := registry.Verified(context.Background(), id)
		if err != nil || providers[id] == nil || manifest.ProviderReleaseID != id {
			t.Fatalf("id=%s manifest=%#v provider=%v err=%v", id, manifest, providers[id], err)
		}
	}
	for _, id := range []string{legacyTerrariaReleaseID, legacyTModLoaderReleaseIDV1, legacyTModLoaderReleaseIDV2} {
		if providers[id] == nil {
			t.Fatalf("legacy Provider Release %s lost its runtime adapter", id)
		}
	}
}

func TestProductionCatalogLocalizesConfigurationWithoutChangingValues(t *testing.T) {
	for _, manifest := range Manifests() {
		if _, exists := manifest.ConfigurationSchema.Properties["serverName"]; exists {
			t.Fatalf("provider=%s exposes unused serverName beside the platform instance name", manifest.ProviderReleaseID)
		}
		for key, field := range manifest.ConfigurationSchema.Properties {
			for _, locale := range []string{"zh-CN", "en"} {
				localization, ok := field.Localizations[locale]
				if !ok || localization.Title == "" {
					t.Fatalf("provider=%s field=%s locale=%s is not localized", manifest.ProviderReleaseID, key, locale)
				}
				if field.Type == "enum" && len(localization.EnumLabels) != len(field.Enum) {
					t.Fatalf("provider=%s field=%s locale=%s enum labels=%v", manifest.ProviderReleaseID, key, locale, localization.EnumLabels)
				}
			}
		}
		for _, section := range manifest.UISchema.Sections {
			for _, locale := range []string{"zh-CN", "en"} {
				if section.Localizations[locale] == "" {
					t.Fatalf("provider=%s section=%s locale=%s is not localized", manifest.ProviderReleaseID, section.ID, locale)
				}
			}
		}
	}

	manifest := Manifests()[0]
	worldSize := manifest.ConfigurationSchema.Properties["worldSize"]
	if worldSize.Default != "medium" || worldSize.Localizations["zh-CN"].EnumLabels["medium"] != "中型" || worldSize.Localizations["en"].EnumLabels["medium"] != "Medium" {
		t.Fatalf("localized labels must preserve the provider value: %#v", worldSize)
	}
	language := manifest.ConfigurationSchema.Properties["language"]
	if language.Default != "en-US" || language.Localizations["zh-CN"].Default != "zh-Hans" || language.Localizations["zh-CN"].EnumLabels["zh-Hans"] != "简体中文" {
		t.Fatalf("localized language must preserve a supported locale code: %#v", language)
	}
}
