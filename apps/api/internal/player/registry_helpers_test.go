package player

import (
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"testing"
)

func mustRegistry(t *testing.T, providers ...provider.GameProvider) *provider.Registry {
	t.Helper()
	registry, err := provider.NewRegistry(providers...)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}
