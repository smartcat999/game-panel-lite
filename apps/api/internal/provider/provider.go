package provider

import "github.com/smartcat999/game-panel-lite/apps/api/internal/domain"

type GameProvider interface {
	Key() domain.ProviderKey
	Name() string
	Image() string
	DefaultConfig() domain.TerrariaConfig
	ValidateConfig(domain.TerrariaConfig) error
	RenderConfig(domain.TerrariaConfig) (string, error)
}

type Registry struct {
	providers map[domain.ProviderKey]GameProvider
}

func NewRegistry(providers ...GameProvider) *Registry {
	registry := &Registry{providers: map[domain.ProviderKey]GameProvider{}}
	for _, item := range providers {
		registry.providers[item.Key()] = item
	}
	return registry
}

func (r *Registry) Get(key domain.ProviderKey) (GameProvider, bool) {
	item, ok := r.providers[key]
	return item, ok
}

func (r *Registry) List() []GameProvider {
	out := make([]GameProvider, 0, len(r.providers))
	for _, item := range r.providers {
		out = append(out, item)
	}
	return out
}
