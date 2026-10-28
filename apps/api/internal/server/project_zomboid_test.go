package server

import (
	"context"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/projectzomboid"
)

func TestProjectZomboidIsolatedUDPWorkload(t *testing.T) {
	p := projectzomboid.NewProvider()
	s := domain.GameServer{ID: "pz-test", ProviderKey: p.Key(), Spec: domain.ServerSpec{Config: p.DefaultConfigPayload(), Network: domain.ServerNetworkSpec{HostPort: 30000}}}
	w, err := NewProviderWorkloadBuilder(provider.NewRegistry(p)).BuildWorkloadSpec(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if w.Network.Port != 16261 || w.Network.HostPort != 30000 || w.Network.Protocol != "udp" {
		t.Fatal("incorrect UDP primary mapping")
	}
	if len(w.Network.AdditionalPorts) != 1 || w.Network.AdditionalPorts[0].HostPort != 30001 || w.Network.AdditionalPorts[0].Port != 16262 || w.Network.AdditionalPorts[0].Protocol != "udp" {
		t.Fatal("incorrect UDP secondary mapping")
	}
	if len(w.Options.Files) != 3 || len(w.Options.DataMounts) != 1 || w.Options.DataMounts[0] != "/data" {
		t.Fatal("incorrect isolated data contract")
	}
}
