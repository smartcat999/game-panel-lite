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
}
