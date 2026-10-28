package projectzomboid

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestVerifiedMetadataAndNativePresets(t *testing.T) {
	if len(definitions.Server) != 144 || len(definitions.Sandbox) != 269 || len(definitions.Presets) != 5 {
		t.Fatal("incomplete official metadata")
	}
	p := NewProvider()
	base := p.DefaultConfigPayload()
	for _, preset := range definitions.Presets {
		if len(preset.Values) != 269 {
			t.Fatalf("incomplete preset %s", preset.ID)
		}
		changed, err := ApplyPreset(base, preset.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.NormalizeConfigPayload(changed); err != nil {
			t.Fatalf("preset %s: %v", preset.ID, err)
		}
		for name, expected := range preset.Values {
			actual, found := getPath(changed["sandbox"].(map[string]any), name)
			if !found || !reflect.DeepEqual(actual, expected) {
				t.Fatalf("preset %s lost %s", preset.ID, name)
			}
		}
		if !reflect.DeepEqual(changed["server"], base["server"]) || changed["adminPassword"] != base["adminPassword"] {
			t.Fatal("preset changed server or credentials")
		}
	}
	for _, preset := range definitions.Presets {
		if preset.ID == "Apocalypse" {
			for name, expected := range preset.Values {
				actual, _ := getPath(base["sandbox"].(map[string]any), name)
				if !reflect.DeepEqual(actual, expected) {
					t.Fatalf("native default differs at %s", name)
				}
			}
		}
	}
}

func TestRuntimeWritesNativeFilesAndStableIdentity(t *testing.T) {
	p := NewProvider()
	config := p.DefaultConfigPayload()
	config["server"].(map[string]any)["ResetID"] = float64(1234567890)
	config["server"].(map[string]any)["Password"] = "join-secret"
	resource := domain.GameServer{ProviderKey: p.Key(), Spec: domain.ServerSpec{Config: config}}
	runtime, err := p.RuntimeConfigForResource(resource)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Protocol != "udp" || runtime.Port != 16261 || len(runtime.AdditionalPorts) != 1 || runtime.AdditionalPorts[0] != 16262 {
		t.Fatal("incorrect game ports")
	}
	if _, overwritesNative := runtime.Options.Files["Zomboid/Server/gamepanel.ini"]; overwritesNative {
		t.Fatal("Provider must not overwrite native identity updated when creating a world")
	}
	ini := runtime.Options.Files["Zomboid/Server/gamepanel.ini.pending"]
	if !strings.Contains(ini, "ResetID=1234567890\n") {
		t.Fatal("native integer identity was not rendered as decimal")
	}
	if !strings.Contains(ini, "Password=join-secret\n") || !strings.Contains(ini, "UPnP=false\n") || !strings.Contains(ini, "RCONPassword=\n") {
		t.Fatal("incorrect INI policy")
	}
	if strings.Contains(ini, config["adminPassword"].(string)) {
		t.Fatal("bootstrap admin secret leaked to INI")
	}
	lua := runtime.Options.Files["Zomboid/Server/gamepanel_SandboxVars.lua"]
	if !strings.HasPrefix(lua, "SandboxVars = {") || !strings.Contains(lua, "[\"VERSION\"] = 6") || strings.Contains(lua, "[\"Version\"]") || !strings.Contains(lua, "[\"ZombieLore\"] = {") {
		t.Fatal("sandbox was not serialized as native nested Lua")
	}
	regions := runtime.Options.Files["Zomboid/Server/gamepanel_spawnregions.lua"]
	if strings.Count(regions, "spawnpoints.lua") != 11 || strings.Index(regions, "West Point, KY") > strings.Index(regions, "Rosewood, KY") {
		t.Fatal("native spawn regions missing or reordered")
	}
	if p.BackupSubtree(resource) != "Zomboid" || p.RestoreSubtree(resource) != "Zomboid" || !p.Capabilities().BackupRequiresStop {
		t.Fatal("unsafe backup policy")
	}
	if err := p.ValidateConfigUpdate(config, clone(config)); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"server.ResetID", "server.ServerPlayerID", "adminPassword"} {
		next := clone(config)
		v, _ := getPath(next, name)
		if name == "server.ResetID" {
			setPath(next, name, float64(1))
		} else {
			setPath(next, name, "replacement")
		}
		if err := p.ValidateConfigUpdate(config, next); err == nil {
			t.Fatalf("allowed replacing %s (%T)", name, v)
		}
	}
	second := p.DefaultConfigPayload()
	if second["adminPassword"] == config["adminPassword"] {
		t.Fatal("shared initial admin password")
	}
}

func TestNativeTypesRangesAndINIInjection(t *testing.T) {
	p := NewProvider()
	base := p.DefaultConfigPayload()
	for _, test := range []struct {
		path  string
		value any
	}{
		{"server.PublicName", "first\nPVP=false"}, {"server.PublicDescription", "left=middle=right"},
		{"server.Password", "trailing "}, {"server.Password", "bad\rvalue"}, {"server.Password", "bad\x00value"},
		{"server.Password", "join\x01"},
		{"server.MaxPlayers", 0}, {"server.MaxPlayers", 255}, {"server.MaxPlayers", "8"},
		{"sandbox.Zombies", 1.5}, {"sandbox.Zombies", 7}, {"sandbox.ZombieLore.Speed", 0},
		{"server.UPnP", true}, {"server.RCONPassword", "remote"}, {"server.Mods", "unsupported"},
		{"server.WorkshopItems", "123"}, {"server.Map", "../../outside"}, {"server.DefaultPort", 16263},
		{"server.SpawnPoint", "-1,0,0"}, {"server.SpawnPoint", "0,0"}, {"server.PVP", "false"},
	} {
		input := clone(base)
		setPath(input, test.path, test.value)
		if _, err := p.NormalizeConfigPayload(input); err == nil {
			t.Errorf("accepted invalid %s", test.path)
		}
	}
	valid := clone(base)
	setPath(valid, "server.PublicDescription", `first\nsecond<LINE>third`)
	if _, err := p.NormalizeConfigPayload(valid); err != nil {
		t.Fatal("rejected native literal text:", err)
	}
	invalid := clone(base)
	setPath(invalid, "sandbox.StartMonth", float64(2))
	setPath(invalid, "sandbox.StartDay", float64(31))
	if _, err := p.NormalizeConfigPayload(invalid); err == nil {
		t.Fatal("accepted invalid calendar day")
	}
	encoded := luaString("\";os.execute('unsafe')--\n中文")
	if strings.Contains(encoded, "--\n") || !strings.Contains(encoded, "\\\"") || !strings.Contains(encoded, "\\n") {
		t.Fatal("Lua string was not escaped")
	}
}

// Export the actual Provider output for optional readback through the licensed
// game's own SandboxOptions.readLuaFile. Never downloads or starts a server.
func TestNativeConfigFixtureExport(t *testing.T) {
	root := os.Getenv("GAMEPANEL_PZ_RENDER_DIR")
	if root == "" {
		t.Skip("set GAMEPANEL_PZ_RENDER_DIR for native Lua readback fixtures")
	}
	p := NewProvider()
	base := p.DefaultConfigPayload()
	inputs := map[string]map[string]any{"defaults": base}
	for _, preset := range definitions.Presets {
		config, err := ApplyPreset(base, preset.ID)
		if err != nil {
			t.Fatal(err)
		}
		inputs[preset.ID] = config
	}
	for name, config := range inputs {
		runtime, err := p.RuntimeConfigForResource(domain.GameServer{Spec: domain.ServerSpec{Config: config}})
		if err != nil {
			t.Fatal(err)
		}
		for path, content := range runtime.Options.Files {
			file := filepath.Join(root, name, path)
			if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestSchemaContainsOnlyNativeVisibleOptions(t *testing.T) {
	p := NewProvider()
	schema := p.ConfigSchema()
	seen := map[string]bool{}
	for _, field := range schema {
		if seen[field.Name] {
			t.Fatalf("duplicate field %s", field.Name)
		}
		seen[field.Name] = true
		if !strings.HasPrefix(field.Group, "pz.") {
			t.Fatal("lost native grouping")
		}
		if field.Name == "server.UPnP" && field.Default != false {
			t.Fatal("automatic mapping must be disabled")
		}
	}
	for _, hidden := range []string{"server.ResetID", "server.ServerPlayerID", "server.Mods", "server.WorkshopItems", "sandbox.WaterShut", "sandbox.ElecShut"} {
		if seen[hidden] {
			t.Fatalf("exposed hidden field %s", hidden)
		}
	}
	if !seen["server.PublicName"] || !seen["sandbox.Zombies"] || !seen["adminPassword"] {
		t.Fatal("missing supported fields")
	}
	if _, err := json.Marshal(schema); err != nil {
		t.Fatal(err)
	}
}
