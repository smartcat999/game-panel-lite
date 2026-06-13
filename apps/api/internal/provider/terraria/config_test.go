package terraria

import (
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestPresetsValidate(t *testing.T) {
	if len(Presets) != 5 {
		t.Fatalf("expected 5 presets, got %d", len(Presets))
	}
	for _, preset := range Presets {
		if err := ValidateConfig(preset.Config); err != nil {
			t.Fatalf("preset %s did not validate: %v", preset.Key, err)
		}
	}
}

func TestRenderServerConfig(t *testing.T) {
	rendered, err := RenderServerConfig(domain.TerrariaConfig{
		WorldName: "Moon Garden", WorldSize: "large", Difficulty: "master",
		MaxPlayers: 12, Port: 7778, Password: "stars", MOTD: "Mind the wyverns",
		Seed: "05162020", Secure: true, Language: "en-US",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"world=worlds/Moon Garden.wld", "autocreate=3", "difficulty=3",
		"maxplayers=12", "port=7778", "password=stars", "secure=1",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected rendered config to contain %q, got:\n%s", expected, rendered)
		}
	}
}

func TestValidateConfigRejectsUnsafeValues(t *testing.T) {
	base := Presets[0].Config
	cases := []domain.TerrariaConfig{
		func() domain.TerrariaConfig { c := base; c.Port = 80; return c }(),
		func() domain.TerrariaConfig { c := base; c.MaxPlayers = 0; return c }(),
		func() domain.TerrariaConfig { c := base; c.WorldName = "../outside"; return c }(),
	}
	for _, item := range cases {
		if err := ValidateConfig(item); err == nil {
			t.Fatalf("expected invalid config to fail: %+v", item)
		}
	}
}
