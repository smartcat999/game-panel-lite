package provider

import (
	"fmt"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

type NodeRequirementsProvider interface {
	NodeRequirements() domain.NodeRequirements
}

func (r *Registry) NodeRequirements(key domain.ProviderKey) (domain.NodeRequirements, error) {
	item, ok := r.Get(key)
	if !ok {
		return domain.NodeRequirements{}, fmt.Errorf("unknown provider %q", key)
	}
	if placement, ok := item.(NodeRequirementsProvider); ok {
		requirements := placement.NodeRequirements()
		requirements.Architectures = append([]string(nil), requirements.Architectures...)
		return requirements, nil
	}
	return domain.NodeRequirements{}, nil
}
