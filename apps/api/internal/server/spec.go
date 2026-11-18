package server

import "github.com/smartcat999/game-panel-lite/apps/api/internal/domain"

func initialSpec(spec domain.ServerSpec) domain.ServerSpec {
	if spec.Generation <= 0 {
		spec.Generation = 1
	}
	if spec.DesiredState == "" {
		spec.DesiredState = domain.DesiredStopped
	}
	if spec.Config == nil {
		spec.Config = map[string]any{}
	}
	return spec
}

func bumpSpecGeneration(spec *domain.ServerSpec) {
	spec.Generation++
	if spec.Generation <= 0 {
		spec.Generation = 1
	}
}
