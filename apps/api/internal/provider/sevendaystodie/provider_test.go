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
	next := p.DefaultConfigPayload()
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
