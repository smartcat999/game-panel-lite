package sevendaystodie

import (
	"encoding/xml"
	"fmt"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestConfigXMLAndNetwork(t *testing.T) {
	p := NewProvider()
	config := p.DefaultConfigPayload()
	config["serverName"] = `好友 & "基地" <test>`
	config["serverPassword"] = `a<&"b`
	config["eacEnabled"] = false
	server := domain.GameServer{ProviderKey: p.Key(), Spec: domain.ServerSpec{Config: config}}
	runtime, err := p.RuntimeConfigForResource(server)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Properties []struct {
			Name  string `xml:"name,attr"`
			Value string `xml:"value,attr"`
		} `xml:"property"`
	}
	text := runtime.Options.Files["gamepanel-serverconfig.xml"]
	if err := xml.Unmarshal([]byte(text), &document); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for _, prop := range document.Properties {
		values[prop.Name] = prop.Value
	}
	if values["ServerName"] != config["serverName"] || values["ServerPassword"] != config["serverPassword"] || values["EACEnabled"] != "false" {
		t.Fatalf("XML did not preserve config: %+v", values)
	}
	if values["SandboxCode"] == "" || values["GameDifficulty"] != "" || values["BloodMoonFrequency"] != "" {
		t.Fatal("3.x must use SandboxCode")
	}
	if values["UserDataFolder"] != "/data/userdata" || values["GameName"] != "Friends" {
		t.Fatalf("unstable save identity: %+v", values)
	}
	if _, exists := values["SaveGameFolder"]; exists {
		t.Fatal("3.3 rejects SaveGameFolder; saves must use UserDataFolder/Saves")
	}
	if runtime.Protocol != "tcp" || len(runtime.PortBindings) != 3 || runtime.PortBindings[0].Port != 26900 || runtime.PortBindings[2].Port != 26902 || runtime.PortBindings[0].Protocol != "udp" {
		t.Fatalf("invalid network: %+v", runtime)
	}
}

func TestConfigRejectsInvalidValues(t *testing.T) {
	p := NewProvider()
	for _, tc := range []struct {
		field string
		value any
	}{
		{"worldName", "../save"}, {"worldName", "save\\other"}, {"worldName", ""},
		{"maxPlayers", 0}, {"maxPlayers", 65}, {"maxPlayers", 1.5}, {"maxPlayers", "8"},
		{"gameWorld", "../../world"}, {"worldGenSize", 5000}, {"port", 65534},
		{"sandboxCode", "not a code"}, {"sandboxCode", strings.Repeat("A", 4097)},
	} {
		t.Run(fmt.Sprintf("%s=%v", tc.field, tc.value), func(t *testing.T) {
			config := p.DefaultConfigPayload()
			config[tc.field] = tc.value
			if _, err := p.NormalizeConfigPayload(config); err == nil {
				t.Fatalf("accepted %s=%v", tc.field, tc.value)
			}
		})
	}
}

func TestDefaultsRoundTripAndCapabilities(t *testing.T) {
	p := NewProvider()
	config, err := p.NormalizeConfigPayload(p.DefaultConfigPayload())
	if err != nil {
		t.Fatal(err)
	}
	summary, err := p.ConfigSummary(config)
	if err != nil || summary.Port != 26900 || summary.MaxPlayers != 8 {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
	if !p.Capabilities().Backups || p.Capabilities().Mods || p.Capabilities().SaveSnapshots || p.Capabilities().ConsoleCommands {
		t.Fatal("advertised unsupported capabilities")
	}
	if p.BackupSubtree(domain.GameServer{}) != "userdata" || p.SteamUpdateConfig().AppID != "294420" {
		t.Fatal("invalid save/update layout")
	}
}

func TestConfigUpdatePreservesWorldIdentity(t *testing.T) {
	p := NewProvider()
	current := p.DefaultConfigPayload()
	next, err := p.NormalizeConfigPayload(current)
	if err != nil {
		t.Fatal(err)
	}
	next["serverName"] = "Renamed server"
	next["serverPassword"] = "new-password"
	if err := p.ValidateConfigUpdate(current, next); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"worldName", "gameWorld", "worldGenSeed", "worldGenSize"} {
		changed := p.DefaultConfigPayload()
		switch field {
		case "worldGenSize":
			changed[field] = 4096
		case "gameWorld":
			changed[field] = "RWG"
		default:
			changed[field] = "Different"
		}
		if err := p.ValidateConfigUpdate(current, changed); err == nil {
			t.Fatalf("allowed changing save identity: %s", field)
		}
	}
}

func TestNewWorldSeedIsPersistedAndLegacySeedsArePreserved(t *testing.T) {
	p := NewProvider()
	input := p.DefaultConfigPayload()
	input["gameWorld"] = "RWG"
	first, err := p.NormalizeConfigPayload(input)
	if err != nil {
		t.Fatal(err)
	}
	seed, ok := first["worldGenSeed"].(string)
	if !ok || len(seed) != 16 || seed == "GamePanelLite" {
		t.Fatalf("expected generated seed, got %v", first["worldGenSeed"])
	}
	second, err := p.NormalizeConfigPayload(first)
	if err != nil || second["worldGenSeed"] != seed {
		t.Fatalf("seed changed after normalization: %v, %v", second, err)
	}
	for _, value := range []string{"", "GamePanelLite", "Friends-custom-seed"} {
		input["worldGenSeed"] = value
		normalized, err := p.NormalizeConfigPayload(input)
		if err != nil || normalized["worldGenSeed"] != value {
			t.Fatalf("existing seed changed: %v, %v", normalized, err)
		}
	}
	delete(input, "worldGenSeed")
	parsed, err := parseConfig(input)
	if err != nil || parsed.WorldGenSeed != "GamePanelLite" {
		t.Fatalf("legacy runtime fallback changed: %+v, %v", parsed, err)
	}
}

func TestPackagedMapsAndTenKWorld(t *testing.T) {
	p := NewProvider()
	for _, name := range []string{"Navezgane", "RWG", "Pregen06k01", "Pregen06k02", "Pregen08k01", "Pregen08k02"} {
		t.Run(name, func(t *testing.T) {
			config := p.DefaultConfigPayload()
			config["gameWorld"] = name
			config["worldGenSize"] = 10240
			config["worldGenSeed"] = "fixed"
			if _, err := p.NormalizeConfigPayload(config); err != nil {
				t.Fatal(err)
			}
			runtime, err := p.RuntimeConfigForResource(domain.GameServer{Spec: domain.ServerSpec{Config: config}})
			if err != nil || !strings.Contains(runtime.Options.Files["gamepanel-serverconfig.xml"], `name="GameWorld" value="`+name+`"`) {
				t.Fatalf("map not rendered: %v", err)
			}
		})
	}
	for _, name := range []string{"Playtesting", "Empty", "../Pregen06k01"} {
		config := p.DefaultConfigPayload()
		config["gameWorld"] = name
		if err := p.ValidateConfigPayload(config); err == nil {
			t.Fatalf("accepted unintended world %q", name)
		}
	}
}

func TestLegacyFourKSaveRemainsEditableWithoutChangingIdentity(t *testing.T) {
	p := NewProvider()
	current := p.DefaultConfigPayload()
	current["worldGenSeed"] = "GamePanelLite"
	current["worldGenSize"] = 4096
	current["worldName"] = "旧的中文存档"
	next, err := p.NormalizeConfigPayload(current)
	if err != nil {
		t.Fatal(err)
	}
	next["serverPassword"] = "new-password"
	if err := p.ValidateConfigUpdate(current, next); err != nil {
		t.Fatal(err)
	}
	for _, field := range p.ConfigSchema() {
		if field.Name == "worldGenSize" && (field.Min == nil || *field.Min != 6144 || field.Max == nil || *field.Max != 10240) {
			t.Fatal("new-world choices must match the current template")
		}
	}
}
