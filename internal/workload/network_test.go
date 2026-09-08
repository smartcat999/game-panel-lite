package workload

import (
	"errors"
	"reflect"
	"testing"
)

func TestResolvePortBindings(t *testing.T) {
	network := Network{Port: 7777, AdditionalPorts: []Port{{Port: 8888, HostPort: 48888, Protocol: "udp"}, {Port: 7777}, {Port: 7777, Protocol: "udp"}}}
	want := []Port{{Port: 7777, HostPort: 7777, Protocol: "tcp"}, {Port: 7777, HostPort: 7777, Protocol: "udp"}, {Port: 8888, HostPort: 48888, Protocol: "udp"}}
	got, err := ResolvePortBindings(network)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("bindings=%+v err=%v", got, err)
	}
	if network.AdditionalPorts[1].HostPort != 0 || network.AdditionalPorts[1].Protocol != "" {
		t.Fatal("resolver mutated input")
	}
	for _, network := range []Network{
		{Port: 65536}, {Port: -1}, {HostPort: 7777}, {Port: 7777, HostPort: -1}, {Port: 7777, HostPort: 65536}, {Port: 7777, Protocol: "sctp"},
		{AdditionalPorts: []Port{{Port: 0}}},
		{Port: 7777, HostPort: 9000, AdditionalPorts: []Port{{Port: 8888, HostPort: 9000}}},
	} {
		if _, err := ResolvePortBindings(network); !errors.Is(err, ErrInvalidNetwork) {
			t.Errorf("accepted %+v: %v", network, err)
		}
	}
}
