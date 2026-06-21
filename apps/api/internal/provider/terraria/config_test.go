package terraria

import (
	"strings"
	"testing"
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
	rendered, err := RenderServerConfig(Config{
		WorldName: "Moon Garden", WorldSize: "large", WorldEvil: "corruption", Difficulty: "master",
		MaxPlayers: 12, Port: 7778, Password: "stars", MOTD: "Mind the wyverns",
		Seed: "05162020", Secure: true, Language: "zh-Hans",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"world=/home/container/Worlds/Moon Garden.wld", "autocreate=3", "worldname=Moon Garden", "worldevil=1", "difficulty=3",
		"maxplayers=12", "port=7778", "password=stars", "secure=1",
		"worldpath=/home/container/Worlds", "language=zh-Hans", "upnp=0",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected rendered config to contain %q, got:\n%s", expected, rendered)
		}
	}
}

func TestRenderServerConfigDefaultsLanguageWhenUnset(t *testing.T) {
	config := Presets[0].Config
	config.Language = ""
	rendered, err := RenderServerConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "language=en-US") {
		t.Fatalf("expected unset language to default to en-US, got:\n%s", rendered)
	}
}

func TestValidateConfigRejectsUnsafeValues(t *testing.T) {
	base := Presets[0].Config
	cases := []Config{
		func() Config { c := base; c.Port = 80; return c }(),
		func() Config { c := base; c.MaxPlayers = 0; return c }(),
		func() Config { c := base; c.WorldName = "../outside"; return c }(),
	}
	for _, item := range cases {
		if err := ValidateConfig(item); err == nil {
			t.Fatalf("expected invalid config to fail: %+v", item)
		}
	}
}

func TestVanillaRuntimeOptionsUseSelfBuiltImageAndConfig(t *testing.T) {
	config := Config{
		WorldName: "Vanilla Smoke", WorldSize: "medium", WorldEvil: "crimson", Difficulty: "expert",
		MaxPlayers: 8, Port: 17777, MOTD: "Vanilla online", Secure: true, Language: "zh-Hans",
	}
	provider := NewVanillaProvider()
	options := provider.RuntimeOptions(config)

	if provider.Image() != "smartcat99999/terraria-vanilla:1.4.5.6" {
		t.Fatalf("unexpected vanilla image: %s", provider.Image())
	}
	if versions := provider.Versions(); len(versions) != 1 || versions[0] != "1.4.5.6" {
		t.Fatalf("unexpected vanilla versions: %v", versions)
	}
	if got := strings.Join(options.Cmd, " "); !strings.Contains(got, "./server/gamepanel-terraria-entrypoint.sh") || !strings.Contains(got, "-config /home/container/serverconfig.txt") {
		t.Fatalf("expected vanilla self-built image command, got %q", got)
	}
	env := strings.Join(options.Env, "\n")
	for _, expected := range []string{"WORLD_NAME=Vanilla Smoke", "WORLD_SIZE=2"} {
		if !strings.Contains(env, expected) {
			t.Fatalf("expected vanilla env to contain %q, got:\n%s", expected, env)
		}
	}
	rendered := options.Files["serverconfig.txt"]
	for _, expected := range []string{
		"world=/home/container/Worlds/Vanilla Smoke.wld",
		"autocreate=2",
		"worldname=Vanilla Smoke",
		"worldevil=2",
		"difficulty=2",
		"maxplayers=8",
		"port=17777",
		"motd=Vanilla online",
		"worldpath=/home/container/Worlds",
		"secure=1",
		"language=zh-Hans",
		"upnp=0",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected vanilla runtime config to contain %q, got:\n%s", expected, rendered)
		}
	}
}

func TestTModLoaderRuntimeOptionsUseNonInteractiveConfig(t *testing.T) {
	config := Config{
		WorldName: "Modded Smoke", WorldSize: "small", Difficulty: "classic",
		MaxPlayers: 4, Port: 17784, MOTD: "Mods online", Secure: true, Language: "zh-Hans",
	}
	provider := NewTModLoaderProvider()
	options := provider.RuntimeOptions(config)

	if provider.Image() != "smartcat99999/tmodloader:v2026.04.3.0" {
		t.Fatalf("unexpected tModLoader image: %s", provider.Image())
	}
	gotCmd := strings.Join(options.Cmd, " ")
	if !strings.Contains(gotCmd, "-config /home/container/serverconfig.txt") {
		t.Fatalf("expected non-interactive config command, got %q", gotCmd)
	}
	if !strings.Contains(gotCmd, "-nosteam") {
		t.Fatalf("expected tModLoader command to disable Steam prompts, got %q", gotCmd)
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
