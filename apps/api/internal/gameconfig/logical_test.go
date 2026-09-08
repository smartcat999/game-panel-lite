package gameconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
)

func TestLogicalTerrariaConfiguration(t *testing.T) {
	for _, p := range []provider.GameProvider{terraria.NewVanillaProvider(), terraria.NewTModLoaderProvider()} {
		t.Run(string(p.Key()), func(t *testing.T) {
			registry, err := provider.NewRegistry(p)
			if err != nil {
				t.Fatal(err)
			}
			n := LogicalNormalizer{Providers: registry, MaxBytes: 4096}
			ctx := context.Background()
			version := p.Versions()[0]
			schema := p.CatalogMetadata().ConfigVersion
			one, err := n.Normalize(ctx, string(p.Key()), version, schema, []byte(`{"password":"secret","maxPlayers":5}`))
			if err != nil {
				t.Fatal(err)
			}
			two, err := n.Normalize(ctx, string(p.Key()), version, schema, []byte(`{ "maxPlayers": 5, "password": "secret" }`))
			if err != nil || !bytes.Equal(one, two) {
				t.Fatalf("unstable normalization: %v", err)
			}
			var config map[string]any
			if err := json.Unmarshal(one, &config); err != nil {
				t.Fatal(err)
			}
			if _, ok := config["port"]; ok {
				t.Fatal("regional port persisted in global config")
			}
			if config["password"] != "secret" || config["maxPlayers"] != float64(5) {
				t.Fatal("user configuration lost")
			}
			for _, raw := range []string{`{"port":7777}`, `{"nodeId":"node"}`, `{"worldPath":"/host"}`, `{"typo":1}`, `{"maxPlayers":0}`, `{"maxPlayers":2.5}`, `{"maxPlayers":null}`, `{"password":"a\nport=1234"}`, `{"worldName":"../world"}`, `{"password":"first","PASSWORD":"second"}`, `null`, `[]`, `{} {}`, `{"maxPlayers":"8"}`} {
				if _, err := n.Normalize(ctx, string(p.Key()), version, schema, []byte(raw)); !errors.Is(err, ErrInvalidLogicalConfiguration) {
					t.Fatalf("accepted invalid input: %s %v", raw, err)
				}
			}
			if _, err := n.Normalize(ctx, string(p.Key()), "unknown-game-version", schema, []byte(`{}`)); err == nil {
				t.Fatal("unknown game version accepted")
			}
			if _, err := n.Normalize(ctx, string(p.Key()), version, 0, []byte(`{}`)); err == nil {
				t.Fatal("legacy schema alias accepted")
			}
		})
	}
}
