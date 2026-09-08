package workload

import (
	"errors"
	"fmt"
	"sort"
)

var ErrInvalidNetwork = errors.New("invalid workload network")

// ResolvePortBindings returns the complete host bindings used by the runtime.
// Zero host ports mean the container port, not an ephemeral allocation. Exact
// duplicates are collapsed; one host/protocol pair cannot target two ports.
func ResolvePortBindings(network Network) ([]Port, error) {
	items := append([]Port(nil), network.AdditionalPorts...)
	if network.Port != 0 {
		items = append(items, Port{Port: network.Port, HostPort: network.HostPort, Protocol: network.Protocol})
	} else if network.HostPort != 0 {
		return nil, fmt.Errorf("%w: host port requires a container port", ErrInvalidNetwork)
	}
	type hostBinding struct {
		port     int
		protocol string
	}
	owners := map[hostBinding]int{}
	resolved := make([]Port, 0, len(items))
	for _, item := range items {
		if item.HostPort == 0 {
			item.HostPort = item.Port
		}
		if item.Protocol == "" {
			item.Protocol = "tcp"
		}
		if item.Port < 1 || item.Port > 65535 || item.HostPort < 1 || item.HostPort > 65535 || (item.Protocol != "tcp" && item.Protocol != "udp") {
			return nil, fmt.Errorf("%w: ports must be 1..65535 with tcp or udp protocol", ErrInvalidNetwork)
		}
		key := hostBinding{item.HostPort, item.Protocol}
		if existing, ok := owners[key]; ok {
			if existing != item.Port {
				return nil, fmt.Errorf("%w: host port %d/%s targets multiple container ports", ErrInvalidNetwork, item.HostPort, item.Protocol)
			}
			continue
		}
		owners[key] = item.Port
		resolved = append(resolved, item)
	}
	sort.Slice(resolved, func(i, j int) bool {
		if resolved[i].HostPort != resolved[j].HostPort {
			return resolved[i].HostPort < resolved[j].HostPort
		}
		return resolved[i].Protocol < resolved[j].Protocol
	})
	return resolved, nil
}

// OffsetHostPorts preserves container ports and protocols while shifting every
// effective host binding by the same offset. It never mutates the input slice.
func OffsetHostPorts(network Network, offset int) (Network, error) {
	if offset < -65535 || offset > 65535 {
		return Network{}, ErrInvalidNetwork
	}
	if _, err := ResolvePortBindings(network); err != nil {
		return Network{}, err
	}
	shifted := network
	shifted.AdditionalPorts = append([]Port(nil), network.AdditionalPorts...)
	shift := func(port, host int) (int, error) {
		if host == 0 {
			host = port
		}
		host += offset
		if host < 1 || host > 65535 {
			return 0, ErrInvalidNetwork
		}
		return host, nil
	}
	var err error
	if network.Port != 0 {
		shifted.HostPort, err = shift(network.Port, network.HostPort)
		if err != nil {
			return Network{}, err
		}
	}
	for i, port := range shifted.AdditionalPorts {
		shifted.AdditionalPorts[i].HostPort, err = shift(port.Port, port.HostPort)
		if err != nil {
			return Network{}, err
		}
	}
	if network.Port == 0 && len(network.AdditionalPorts) == 0 && offset != 0 {
		return Network{}, ErrInvalidNetwork
	}
	return shifted, nil
}
