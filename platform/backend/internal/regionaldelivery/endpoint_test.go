package regionaldelivery

import (
	"errors"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliverycontrol"
)

func TestNodeDirectEndpointIsExplicitlyChangeable(t *testing.T) {
	start, end := 32000, 32001
	state := State{ID: "rdp_one", LogicalInstanceID: "lin_one", ListenerRequirements: []deliverycontrol.ListenerRequirement{{Name: "game", Purpose: "join", Transports: []string{"udp"}, InternalPort: 7777, ExternalPortPolicy: "allocated", AddressMode: "ip-port", Primary: true}}}
	allocations, err := chooseEndpoints(state, []EndpointPool{{ID: "pool_node", DeliveryMode: "node-direct", Address: "198.51.100.8", PortStart: &start, PortEnd: &end, Stability: "may-change", Active: true}}, map[string]bool{}, time.Now())
	if err != nil || len(allocations) != 1 || allocations[0].Stability != "may-change" || allocations[0].DisplayAddress != "198.51.100.8:32000" {
		t.Fatalf("allocations=%#v err=%v", allocations, err)
	}
}

func TestDefaultRequiredPortMustBelongToPool(t *testing.T) {
	start, end := 32000, 32001
	state := State{ID: "rdp_one", LogicalInstanceID: "lin_one", ListenerRequirements: []deliverycontrol.ListenerRequirement{{Name: "game", Purpose: "join", Transports: []string{"udp"}, InternalPort: 7777, ExternalPortPolicy: "default-required", AddressMode: "ip-port", Primary: true}}}
	if _, err := chooseEndpoints(state, []EndpointPool{{ID: "pool_gateway", DeliveryMode: "gateway", Address: "play.example", PortStart: &start, PortEnd: &end, Stability: "stable", Active: true}}, map[string]bool{}, time.Now()); !errors.Is(err, ErrNoEndpoint) {
		t.Fatalf("default-required error=%v", err)
	}
}
