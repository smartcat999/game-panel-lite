package sevendaystodie

import (
	"encoding/xml"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
)

func configProperties(t *testing.T, input map[string]any) map[string]string {
	t.Helper()
	c, err := parseConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	data, err := renderConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Properties []property `xml:"property"`
	}
	if err := xml.Unmarshal([]byte(data), &doc); err != nil {
		t.Fatal(err)
	}
	result := map[string]string{}
	for _, p := range doc.Properties {
		result[p.Name] = p.Value
	}
	return result
}

func TestAdvancedSettingsMatchVerifiedGameTemplate(t *testing.T) {
	data, err := os.ReadFile("testdata/settings-v3.3.0.xml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Properties []property `xml:"property"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	rendered := configProperties(t, NewProvider().DefaultConfigPayload())
	schema := advancedConfigSchema()
	if len(doc.Properties) != 26 || len(schema) != len(doc.Properties) {
		t.Fatal("settings do not match audited fixture")
	}
	for i, p := range doc.Properties {
		if rendered[p.Name] != p.Value {
			t.Errorf("%s: got %q want template default %q", p.Name, rendered[p.Name], p.Value)
		}
		f := schema[i]
		if strings.ToUpper(f.Name[:1])+f.Name[1:] != p.Name || fmt.Sprint(f.Default) != p.Value {
			t.Errorf("schema mismatch for %s", p.Name)
		}
		if f.LabelEn == "" || (f.Help != "" && f.HelpEn == "") {
			t.Errorf("missing English text for %s", f.Name)
		}
		if f.Max != nil && f.Name != "serverMaxAllowedViewDistance" && f.Name != "landClaimDecayMode" {
			t.Errorf("undocumented official maximum for %s", f.Name)
		}
	}
}

func TestAdvancedSettingsPersistZeroFalseAndIntegerMultipliers(t *testing.T) {
	p := NewProvider()
	input := p.DefaultConfigPayload()
	changes := map[string]any{
		"region": "Asia", "language": "Chinese & English", "maxSpawnedZombies": 80, "maxSpawnedAnimals": 0,
		"serverMaxAllowedViewDistance": 6, "serverMaxWorldTransferSpeedKiBs": 768, "maxQueuedMeshLayers": 400,
		"dynamicMeshEnabled": false, "dynamicMeshLandClaimOnly": false, "dynamicMeshLandClaimBuffer": 0,
		"dynamicMeshMaxItemCache": 2, "landClaimCount": 0, "landClaimSize": 51, "landClaimDeadZone": 40,
		"landClaimExpiryTime": 14, "landClaimDecayMode": 2, "landClaimOnlineDurabilityModifier": 0,
		"landClaimOfflineDurabilityModifier": 2, "landClaimOfflineDelay": 15, "maxChunkAge": -1, "buildCreate": false, "persistentPlayerProfiles": true, "cameraRestrictionMode": 2, "bedrollDeadZoneSize": 0, "bedrollExpiryTime": 60, "partySharedKillRange": 500,
	}
	for k, v := range changes {
		input[k] = v
	}
	normalized, err := p.NormalizeConfigPayload(input)
	if err != nil {
		t.Fatal(err)
	}
	again, err := p.NormalizeConfigPayload(normalized)
	if err != nil {
		t.Fatal(err)
	}
	properties := configProperties(t, again)
	for k, want := range changes {
		name := strings.ToUpper(k[:1]) + k[1:]
		if properties[name] != fmt.Sprint(want) {
			t.Errorf("%s: got %q want %v", name, properties[name], want)
		}
	}
	if err := p.ValidateConfigUpdate(input, normalized); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyConfigKeepsUntouchedTemplateSettings(t *testing.T) {
	p := NewProvider()
	current := payload(defaultConfig())
	current["worldName"] = "旧存档"
	current["worldGenSize"] = 4096
	current["worldGenSeed"] = ""
	next, err := p.NormalizeConfigPayload(current)
	if err != nil {
		t.Fatal(err)
	}
	next["serverPassword"] = "new-password"
	if err := p.ValidateConfigUpdate(current, next); err != nil {
		t.Fatal(err)
	}
	properties := configProperties(t, next)
	for _, f := range advancedConfigSchema() {
		if _, ok := next[f.Name]; ok {
			t.Errorf("normalization invented legacy setting %s", f.Name)
		}
		name := strings.ToUpper(f.Name[:1]) + f.Name[1:]
		if _, ok := properties[name]; ok {
			t.Errorf("runtime overrides unmanaged template field %s", name)
		}
	}
}

func TestAdvancedSettingsRejectInvalidInputs(t *testing.T) {
	p := NewProvider()
	for _, tc := range []struct {
		field string
		value any
	}{
		{"maxChunkAge", -2}, {"cameraRestrictionMode", 3}, {"bedrollDeadZoneSize", -1}, {"bedrollExpiryTime", -1}, {"partySharedKillRange", -1}, {"buildCreate", "false"},
		{"region", "Atlantis"}, {"region", "asia"}, {"language", "  "}, {"language", "English\nChinese"}, {"language", strings.Repeat("x", 129)},
		{"serverMaxAllowedViewDistance", 5}, {"serverMaxAllowedViewDistance", 13},
		{"maxSpawnedZombies", -1}, {"maxSpawnedZombies", 1.5}, {"maxSpawnedAnimals", "50"},
		{"maxQueuedMeshLayers", int64(math.MaxInt32) + 1}, {"serverMaxWorldTransferSpeedKiBs", -1},
		{"dynamicMeshEnabled", "false"}, {"dynamicMeshLandClaimBuffer", -1}, {"dynamicMeshMaxItemCache", -1},
		{"landClaimCount", -1}, {"landClaimSize", 0}, {"landClaimDeadZone", -1}, {"landClaimExpiryTime", -1},
		{"landClaimDecayMode", -1}, {"landClaimDecayMode", 3}, {"landClaimDecayMode", 1.5},
		{"landClaimOnlineDurabilityModifier", -1}, {"landClaimOfflineDurabilityModifier", 2.5}, {"landClaimOfflineDurabilityModifier", math.Inf(1)},
		{"landClaimOfflineDurabilityModifier", math.NaN()}, {"landClaimOfflineDelay", -1},
	} {
		t.Run(fmt.Sprintf("%s=%v", tc.field, tc.value), func(t *testing.T) {
			input := p.DefaultConfigPayload()
			input[tc.field] = tc.value
			if _, err := p.NormalizeConfigPayload(input); err == nil {
				t.Fatalf("accepted invalid %s=%v", tc.field, tc.value)
			}
		})
	}
}
