package server

import (
	"context"
	"errors"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

type portsProvider struct{ terraria.VanillaProvider }

func (portsProvider) RuntimeConfigForResource(domain.GameServer) (domain.ProviderRuntimeConfig, error) {
	return domain.ProviderRuntimeConfig{Port: 7777, AdditionalPorts: []int{7778}, Protocol: "udp"}, nil
}
func TestWorkloadBuilderResolvesDefaultAndShiftedPorts(t *testing.T) {
	p := portsProvider{terraria.NewVanillaProvider()}
	builder := NewProviderWorkloadBuilder(mustRegistry(t, p))
	for _, host := range []int{0, 40000, 65535} {
		instance := domain.GameServer{ID: "ports", ProviderKey: p.Key(), Spec: domain.ServerSpec{Network: domain.ServerNetworkSpec{HostPort: host}}}
		spec, err := builder.BuildWorkloadSpec(context.Background(), instance)
		if host == 65535 {
			if !errors.Is(err, workload.ErrInvalidNetwork) {
				t.Fatalf("offset overflow: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		expected := host
		if expected == 0 {
			expected = 7777
		}
		if spec.Network.HostPort != expected || len(spec.Network.AdditionalPorts) != 1 || spec.Network.AdditionalPorts[0].HostPort != expected+1 {
			t.Fatalf("host=%d network=%+v", host, spec.Network)
		}
	}
}
