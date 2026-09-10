package tmodloader

import (
	"context"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliverycontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/nodeworkload"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
)

func TestProviderMaterializesControlledModLock(t *testing.T) {
	port := 31778
	spec, err := (Provider{}).Materialize(context.Background(), nodeworkload.Intent{
		LogicalInstanceID: "lin_modded", ProviderReleaseID: "gpr_tmodloader", GameVersion: GameVersion,
		DesiredState: "running", DataScope: "instances/lin_modded", CPUMilli: 2000, MemoryMiB: 4096, FencingToken: 9,
		Configuration: map[string]any{"worldName": "ReadinessSmoke", "worldSize": "large", "difficulty": "expert", "maxPlayers": 12},
		ModLock: []providercontract.ModLockEntry{
			{ModID: CalamityWorkshopID, Version: CalamityVersion, Digest: CalamityDigest, Direct: true},
			{ModID: CalamityMusicWorkshopID, Version: CalamityMusicVersion, Digest: CalamityMusicDigest, Direct: false},
		},
		Endpoints: []deliverycontrol.EndpointBinding{{Port: &port, Transports: []string{"tcp"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Artifact != Image || spec.Files["tml-data/Mods/enabled.json"] != `["CalamityMod","CalamityModMusic"]` || spec.Files["tml-data/Mods/install.txt"] != CalamityWorkshopID+"\n"+CalamityMusicWorkshopID+"\n" || !strings.Contains(spec.Files["tml-data/Mods/artifacts.lock"], strings.TrimPrefix(CalamityDigest, "sha256:")) || spec.Mounts["Worlds"] != "/data/Worlds" || spec.Mounts["calamity-home"] != "/opt/tmodloader/~" || spec.Mounts["tml-data"] != "/home/tml/.local/share/Terraria/tModLoader" || !spec.ExecutableTemp || spec.ReadyLogMarker != "Server started" || spec.Listeners[0].HostPort != port {
		t.Fatalf("spec=%#v", spec)
	}
}

func TestProviderRejectsUnknownModAndUnsafeWorld(t *testing.T) {
	port := 31778
	base := nodeworkload.Intent{LogicalInstanceID: "lin_modded", ProviderReleaseID: "gpr_tmodloader", GameVersion: GameVersion, DesiredState: "running", DataScope: "instances/lin_modded", CPUMilli: 1000, MemoryMiB: 2048, FencingToken: 1, Endpoints: []deliverycontrol.EndpointBinding{{Port: &port, Transports: []string{"tcp"}}}}
	base.Configuration = map[string]any{"worldName": "../escape"}
	if _, err := (Provider{}).Materialize(context.Background(), base); err == nil {
		t.Fatal("accepted unsafe world")
	}
	base.Configuration = map[string]any{"worldName": "Safe"}
	base.ModLock = []providercontract.ModLockEntry{{ModID: "unknown", Version: "1", Digest: "sha256:" + strings.Repeat("b", 64)}}
	if _, err := (Provider{}).Materialize(context.Background(), base); err == nil {
		t.Fatal("accepted uncontrolled mod")
	}
}
