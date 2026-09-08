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

func TestOffsetHostPorts(t *testing.T) {
	original := Network{Port: 7777, AdditionalPorts: []Port{{Port: 7778, Protocol: "udp"}}}
	shifted, err := OffsetHostPorts(original, 24223)
	if err != nil {
		t.Fatal(err)
	}
	ports, err := ResolvePortBindings(shifted)
	if err != nil || len(ports) != 2 || ports[0].HostPort != 32000 || ports[0].Port != 7777 || ports[1].HostPort != 32001 || ports[1].Protocol != "udp" {
		t.Fatalf("shifted bindings: %v %v", ports, err)
	}
	if original.HostPort != 0 || original.AdditionalPorts[0].HostPort != 0 {
		t.Fatal("input mutated")
	}
	for _, offset := range []int{-7777, 65535, 65536, -65536} {
		if _, err := OffsetHostPorts(original, offset); err == nil {
			t.Fatalf("invalid offset %d", offset)
		}
	}
	if _, err := OffsetHostPorts(Network{}, 1); err == nil {
		t.Fatal("empty network shifted")
	}
}
