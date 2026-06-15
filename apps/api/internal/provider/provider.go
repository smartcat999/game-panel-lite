package provider

import (
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

type GameProvider interface {
	Key() domain.ProviderKey
	Name() string
	Image() string
	Versions() []string
	ImageFor(version string) string
	DefaultConfig() domain.TerrariaConfig
	ValidateConfig(domain.TerrariaConfig) error
	RenderConfig(domain.TerrariaConfig) (string, error)
	RuntimeOptions(domain.TerrariaConfig) runtime.ContainerOptions
}

type PlayerListProvider interface {
	PlayerListCommand() string
	ParsePlayerListOutput([]string) []domain.Player
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
