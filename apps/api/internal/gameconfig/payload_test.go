package gameconfig

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/dst"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
)

type mutatingConfigPlugin struct {
	terraria.VanillaProvider
	defaults map[string]any
	fail     bool
}

func (p mutatingConfigPlugin) DefaultConfigPayload() map[string]any { return p.defaults }
func (p mutatingConfigPlugin) NormalizeConfigPayload(payload map[string]any) (map[string]any, error) {
	payload["nested"].(map[string]any)["value"] = "changed"
	if p.fail {
		return nil, errors.New("normalization rejected")
	}
	return payload, nil
}

func TestNormalizeIsolatesProviderMutationAndRejectsVersion(t *testing.T) {
	original := map[string]any{"nested": map[string]any{"value": "original"}}
	for _, fail := range []bool{false, true} {
		p := mutatingConfigPlugin{VanillaProvider: terraria.NewVanillaProvider(), defaults: original, fail: fail}
		registry, err := provider.NewRegistry(p)
		if err != nil {
			t.Fatal(err)
		}
		service := NewService(registry, &configStore{}, p.Key())
		for _, fallback := range []map[string]any{nil, original} {
			result, encoded, err := service.Normalize(p.Key(), 1, json.RawMessage(`{"other":true}`), fallback)
			if (err != nil) != fail {
				t.Fatalf("normalization result: %v", err)
			}
			if !fail && (result["other"] != true || !strings.Contains(encoded, "changed")) {
				t.Fatal("overlay or normalization lost")
			}
			if original["nested"].(map[string]any)["value"] != "original" {
				t.Fatal("provider mutated caller or defaults")
			}
		}
		if _, _, err := service.Normalize(p.Key(), 999, nil, original); err == nil {
			t.Fatal("accepted incompatible version")
		}
		for _, raw := range []string{`[]`, `42`, `{"broken":`} {
			if _, _, err := service.Normalize(p.Key(), 1, json.RawMessage(raw), original); err == nil {
				t.Fatal("accepted non-object config")
			}
		}
	}
}

func TestPublicConfigRemovesNestedAndDottedSecrets(t *testing.T) {
	p := dst.NewProvider()
	registry, err := provider.NewRegistry(p)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(registry, &configStore{}, p.Key())
	original := map[string]any{
		"identity":          map[string]any{"password": "join-secret", "clusterToken": "klei-secret", "serverName": "Friends"},
		"identity.password": "dotted-secret", "identity.clusterToken": "dotted-token",
		"clusterToken": "legacy-secret", "gameplay": map[string]any{"maxPlayers": float64(8)},
	}
	before, _ := json.Marshal(original)
	clean, encoded, err := service.PublicConfig(p.Key(), original)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"join-secret", "klei-secret", "dotted-secret", "dotted-token", "legacy-secret"} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("secret retained: %s", secret)
		}
	}
	if clean["identity"].(map[string]any)["serverName"] != "Friends" || !reflect.DeepEqual(clean["gameplay"], original["gameplay"]) {
		t.Fatal("non-secret fields changed")
	}
	after, _ := json.Marshal(original)
	if string(before) != string(after) {
		t.Fatal("redaction modified caller")
	}
	// Historical fields may disagree; every outward representation is redacted.
	preset, err := service.PublicPreset(domain.ConfigPreset{ProviderKey: p.Key(), Config: original, ConfigPayload: original, ConfigPayloadJSON: string(before)})
	if err != nil {
		t.Fatal(err)
	}
	response, _ := json.Marshal(preset)
	if strings.Contains(string(response), "secret") || strings.Contains(string(response), "dotted-token") {
		t.Fatal("historical representation leaked")
	}
	if _, err := service.PublicPreset(domain.ConfigPreset{ProviderKey: p.Key(), ConfigPayloadJSON: "broken"}); err == nil {
		t.Fatal("malformed historical JSON accepted")
	}
}
