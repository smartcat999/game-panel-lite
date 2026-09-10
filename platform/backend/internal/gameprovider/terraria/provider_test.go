package terraria

import (
	"context"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliverycontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/nodeworkload"
)

func TestProviderBuildsValidatedVanillaWorkload(t *testing.T) {
	port := 31777
	spec, err := (Provider{}).Materialize(context.Background(), nodeworkload.Intent{LogicalInstanceID: "lin_test", ProviderReleaseID: "gpr_terraria", GameVersion: GameVersion, DesiredState: "running", DataScope: "instances/lin_test", CPUMilli: 1000, MemoryMiB: 2048, FencingToken: 7, Configuration: map[string]any{"worldName": "Moon Garden", "maxPlayers": 12}, Endpoints: []deliverycontrol.EndpointBinding{{Port: &port, Transports: []string{"tcp"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Artifact != Image || spec.Listeners[0].InternalPort != 7777 || spec.Listeners[0].HostPort != port || spec.Mounts["Worlds"] != "/home/container/Worlds" {
		t.Fatalf("spec=%#v", spec)
	}
	for _, expected := range []string{"world=/home/container/Worlds/Moon Garden.wld", "maxplayers=12", "upnp=0"} {
		if !strings.Contains(spec.Files["serverconfig.txt"], expected) {
			t.Fatalf("config missing %q: %s", expected, spec.Files["serverconfig.txt"])
		}
	}
}

func TestProviderRejectsUnsupportedGameVersion(t *testing.T) {
	_, err := (Provider{}).Materialize(context.Background(), nodeworkload.Intent{LogicalInstanceID: "lin_test", ProviderReleaseID: "gpr_terraria", GameVersion: "1.4.4.9", DesiredState: "running", DataScope: "instances/lin_test"})
	if err == nil {
		t.Fatal("accepted a game version that does not match the runtime image")
	}
}

func TestProviderRejectsUnknownTraversalAndMultilineConfiguration(t *testing.T) {
	port := 31777
	base := nodeworkload.Intent{LogicalInstanceID: "lin_test", ProviderReleaseID: "gpr_terraria", GameVersion: GameVersion, DesiredState: "running", DataScope: "instances/lin_test", CPUMilli: 1000, MemoryMiB: 2048, FencingToken: 7, Endpoints: []deliverycontrol.EndpointBinding{{Port: &port, Transports: []string{"tcp"}}}}
	for _, configuration := range []map[string]any{{"unknown": true}, {"worldName": "../escape"}, {"motd": "first\nsecond"}} {
		base.Configuration = configuration
		if _, err := (Provider{}).Materialize(context.Background(), base); err == nil {
			t.Fatalf("accepted unsafe configuration %#v", configuration)
		}
	}
}
