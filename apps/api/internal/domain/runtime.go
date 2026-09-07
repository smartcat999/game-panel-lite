package domain

import "github.com/smartcat999/game-panel-lite/internal/workload"

// Aliases retain domain call sites while using one shared workload wire shape.
type WorkloadResources = workload.Resources
type WorkloadNetwork = workload.Network
type WorkloadPort = workload.Port
type WorkloadOptions = workload.Options
type WorkloadSpec = workload.Spec

type ProviderRuntimeConfig struct {
	Port            int             `json:"port,omitempty"`
	AdditionalPorts []int           `json:"additionalPorts,omitempty"`
	Protocol        string          `json:"protocol,omitempty"`
	Options         WorkloadOptions `json:"options,omitempty"`
}

type WorkloadStatus struct {
	RuntimeID string            `json:"runtimeId,omitempty"`
	State     ServerActualState `json:"state"`
	Message   string            `json:"message,omitempty"`
}
