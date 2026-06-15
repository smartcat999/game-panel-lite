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
		WorldName: "Moon Garden", WorldSize: "large", WorldEvil: "corruption", Difficulty: "master",
		MaxPlayers: 12, Port: 7778, Password: "stars", MOTD: "Mind the wyverns",
		Seed: "05162020", Secure: true, Language: "en-US",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"world=worlds/Moon Garden.wld", "autocreate=3", "worldevil=1", "difficulty=3",
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

func TestTModLoaderRuntimeOptionsUseNonInteractiveConfig(t *testing.T) {
	config := domain.TerrariaConfig{
		WorldName: "Modded Smoke", WorldSize: "small", Difficulty: "classic",
		MaxPlayers: 4, Port: 17784, MOTD: "Mods online", Secure: true, Language: "zh-Hans",
	}
	provider := NewTModLoaderProvider()
	options := provider.RuntimeOptions(config)

	if provider.Image() != "smartcat99999/tmodloader:v2026.04.3.0" {
		t.Fatalf("unexpected tModLoader image: %s", provider.Image())
	}
	if got := strings.Join(options.Cmd, " "); !strings.Contains(got, "-config /home/container/serverconfig.txt") {
		t.Fatalf("expected non-interactive config command, got %q", got)
	}
	env := strings.Join(options.Env, "\n")
	for _, expected := range []string{"WORLD_NAME=Modded Smoke", "WORLD_SIZE=1"} {
		if !strings.Contains(env, expected) {
			t.Fatalf("expected tModLoader env to contain %q, got:\n%s", expected, env)
		}
	}
	rendered := options.Files["serverconfig.txt"]
	for _, expected := range []string{
		"world=/home/container/Worlds/Modded Smoke.wld",
		"autocreate=1",
		"worldname=Modded Smoke",
		"maxplayers=4",
		"port=17784",
		"motd=Mods online",
		"worldpath=/home/container/Worlds",
		"secure=1",
		"language=zh-Hans",
		"upnp=0",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected tModLoader runtime config to contain %q, got:\n%s", expected, rendered)
		}
	}
}
