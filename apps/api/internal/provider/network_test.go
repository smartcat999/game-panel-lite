package provider

import (
	"errors"
	"reflect"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func TestRuntimeNetworkCompleteOffset(t *testing.T) {
	config := domain.ProviderRuntimeConfig{Port: 27015, Protocol: "udp", AdditionalPorts: []int{27016, 27014, 27016}}
	network, err := RuntimeNetwork(config, 32000)
	if err != nil {
		t.Fatal(err)
	}
	got, err := workload.ResolvePortBindings(network)
	want := []workload.Port{{Port: 27014, HostPort: 31999, Protocol: "udp"}, {Port: 27015, HostPort: 32000, Protocol: "udp"}, {Port: 27016, HostPort: 32001, Protocol: "udp"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("incomplete binding: %v %v", got, err)
	}
	for _, host := range []int{-1, 1, 65535, 65536} {
		if _, err := RuntimeNetwork(config, host); !errors.Is(err, workload.ErrInvalidNetwork) {
			t.Fatalf("invalid translated port accepted: %d %v", host, err)
		}
	}
	for _, invalid := range []domain.ProviderRuntimeConfig{
		{Port: -1}, {Port: 65536}, {Port: 7777, Protocol: "sctp"}, {AdditionalPorts: []int{7777}},
		{Port: 7777, AdditionalPorts: []int{0}}, {Port: 7777, AdditionalPorts: []int{65536}},
	} {
		if _, err := RuntimeNetwork(invalid, 0); err == nil {
			t.Fatal("invalid provider network accepted")
		}
	}
}
