package provider_test

import (
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/dst"
)

type placementProvider struct {
	dst.Provider
	architectures []string
}

func (placementProvider) Key() domain.ProviderKey { return "custom-placement" }
func (p placementProvider) NodeRequirements() domain.NodeRequirements {
	return domain.NodeRequirements{Architectures: p.architectures}
}

func TestRegistryNodeRequirements(t *testing.T) {
	custom := placementProvider{Provider: dst.NewProvider(), architectures: []string{"arm64"}}
	registry, err := provider.NewRegistry(dst.NewProvider(), custom)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		key  domain.ProviderKey
		arch string
	}{{domain.ProviderDST, "amd64"}, {custom.Key(), "arm64"}} {
		requirements, err := registry.NodeRequirements(tc.key)
		if err != nil || len(requirements.Architectures) != 1 || requirements.Architectures[0] != tc.arch {
			t.Fatalf("requirements=%+v err=%v", requirements, err)
		}
		requirements.Architectures[0] = "changed-by-caller"
		again, err := registry.NodeRequirements(tc.key)
		if err != nil || again.Architectures[0] != tc.arch {
			t.Fatalf("caller mutated provider requirements: %+v err=%v", again, err)
		}
	}
	if _, err := registry.NodeRequirements("unknown"); err == nil {
		t.Fatal("unknown provider accepted")
	}
}
