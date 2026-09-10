package terraria

import (
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/nodeexecution"
)

func TestProviderBuildsValidatedVanillaWorkload(t *testing.T) {
	spec, err := (Provider{}).Build(nodeexecution.WorkloadIntent{LogicalInstanceID: "lin_test", GameKey: GameKey, GameVersion: GameVersion, DesiredState: "running", DataDir: t.TempDir(), CPUUnits: 1000, MemoryMegabytes: 2048, Configuration: map[string]any{"worldName": "Moon Garden", "maxPlayers": 12}})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Image != Image || spec.Port != 7777 || spec.DataMounts["Worlds"] != "/home/container/Worlds" {
		t.Fatalf("spec=%#v", spec)
	}
	for _, expected := range []string{"world=/home/container/Worlds/Moon Garden.wld", "maxplayers=12", "upnp=0"} {
		if !strings.Contains(spec.Files["serverconfig.txt"], expected) {
			t.Fatalf("config missing %q: %s", expected, spec.Files["serverconfig.txt"])
		}
	}
}

func TestProviderRejectsUnsupportedGameVersion(t *testing.T) {
	_, err := (Provider{}).Build(nodeexecution.WorkloadIntent{LogicalInstanceID: "lin_test", GameKey: GameKey, GameVersion: "1.4.4.9", DesiredState: "running", DataDir: t.TempDir(), CPUUnits: 1000, MemoryMegabytes: 2048})
	if err == nil {
		t.Fatal("accepted a game version that does not match the runtime image")
	}
}

func TestProviderRejectsUnknownTraversalAndMultilineConfiguration(t *testing.T) {
	base := nodeexecution.WorkloadIntent{LogicalInstanceID: "lin_test", GameKey: GameKey, DesiredState: "running", DataDir: t.TempDir(), CPUUnits: 1000, MemoryMegabytes: 2048}
	for _, configuration := range []map[string]any{{"unknown": true}, {"worldName": "../escape"}, {"motd": "first\nsecond"}} {
		base.Configuration = configuration
		if _, err := (Provider{}).Build(base); err == nil {
			t.Fatalf("accepted unsafe configuration %#v", configuration)
		}
	}
}
