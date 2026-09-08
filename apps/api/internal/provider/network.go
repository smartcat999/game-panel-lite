package provider

import (
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

// RuntimeNetwork maps every provider port using the primary host-port offset.
// Scheduling and runtime construction must use the same mapping.
func RuntimeNetwork(config domain.ProviderRuntimeConfig, hostPort int) (workload.Network, error) {
	if hostPort == 0 {
		hostPort = config.Port
	}
	if config.Port < 0 || config.Port > 65535 || hostPort < 0 || hostPort > 65535 {
		return workload.Network{}, workload.ErrInvalidNetwork
	}
	network := workload.Network{Port: config.Port, HostPort: hostPort, Protocol: config.Protocol}
	for _, port := range config.AdditionalPorts {
		if port < 1 || port > 65535 || config.Port < 1 {
			return workload.Network{}, workload.ErrInvalidNetwork
		}
		translated := hostPort + (port - config.Port)
		if translated < 1 || translated > 65535 {
			return workload.Network{}, workload.ErrInvalidNetwork
		}
		network.AdditionalPorts = append(network.AdditionalPorts, workload.Port{
			Port: port, HostPort: translated, Protocol: config.Protocol,
		})
	}
	if _, err := workload.ResolvePortBindings(network); err != nil {
		return workload.Network{}, err
	}
	return network, nil
}
