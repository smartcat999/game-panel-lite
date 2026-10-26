package server

import (
	"context"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/sevendaystodie"
)

func TestSevenDaysMixedProtocolWorkload(t *testing.T) {
	p := sevendaystodie.NewProvider()
	builder := NewProviderWorkloadBuilder(provider.NewRegistry(p))
	server := domain.GameServer{ProviderKey: p.Key(), Spec: domain.ServerSpec{Config: p.DefaultConfigPayload(), Network: domain.ServerNetworkSpec{HostPort: 30000}}}
	spec, err := builder.BuildWorkloadSpec(context.Background(), server)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Network.Protocol != "tcp" || spec.Network.HostPort != 30000 || len(spec.Network.AdditionalPorts) != 3 {
		t.Fatalf("invalid spec: %+v", spec.Network)
	}
	for i, port := range spec.Network.AdditionalPorts {
		if port.Protocol != "udp" || port.Port != 26900+i || port.HostPort != 30000+i {
			t.Fatalf("invalid port %d: %+v", i, port)
		}
	}
}
