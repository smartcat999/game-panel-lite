package provider

import (
	"sort"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

type GameProvider interface {
	GameKey() domain.GameKey
	Key() domain.ProviderKey
	Name() string
	Description() string
	Capabilities() domain.ProviderCapabilities
	ConfigSchema() []domain.ProviderConfigField
	Image() string
	Versions() []string
	ImageFor(version string) string
	DefaultConfig() domain.TerrariaConfig
	ValidateConfig(domain.TerrariaConfig) error
	RenderConfig(domain.TerrariaConfig) (string, error)
	RuntimeOptions(domain.TerrariaConfig) runtime.ContainerOptions
}

type ServerRuntimeProvider interface {
	RenderServerConfig(domain.GameServerInstance) (string, error)
	RuntimeOptionsForServer(domain.GameServerInstance) (runtime.ContainerOptions, error)
}

type PlayerListProvider interface {
	PlayerListCommand(domain.TerrariaConfig) string
	ParsePlayerListOutput([]string) []domain.Player
}

type PlayerActivityProvider interface {
	ParsePlayerLogEvent(string) (domain.PlayerLogEvent, bool)
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
	sort.Slice(out, func(i, j int) bool {
		return out[i].Key() < out[j].Key()
	})
	return out
}

func (r *Registry) Games() []domain.GameCatalogEntry {
	games := map[domain.GameKey]domain.GameCatalogEntry{
		domain.GameTerraria: {
			Key:         domain.GameTerraria,
			Name:        "Terraria",
			Description: "2D sandbox adventure server for vanilla and tModLoader worlds.",
			Status:      "available",
		},
		domain.GamePalworld: {
			Key:         domain.GamePalworld,
			Name:        "Palworld",
			Description: "Survival crafting server for small friend groups. Provider implementation is next on the roadmap.",
			Status:      "planned",
		},
	}
	for _, item := range r.List() {
		entry := games[item.GameKey()]
		if entry.Key == "" {
			entry = domain.GameCatalogEntry{
				Key:         item.GameKey(),
				Name:        string(item.GameKey()),
				Description: item.Description(),
				Status:      "available",
			}
		}
		entry.Providers = append(entry.Providers, domain.ProviderCatalog{
			Key:          item.Key(),
			Name:         item.Name(),
			Description:  item.Description(),
			Recommended:  len(entry.Providers) == 0,
			Versions:     append([]string{}, item.Versions()...),
			Capabilities: item.Capabilities(),
			ConfigSchema: append([]domain.ProviderConfigField{}, item.ConfigSchema()...),
		})
		entry.Status = "available"
		games[item.GameKey()] = entry
	}
	out := make([]domain.GameCatalogEntry, 0, len(games))
	for _, item := range games {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Status != out[j].Status {
			return out[i].Status == "available"
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func (r *Registry) Game(key domain.GameKey) (domain.GameCatalogEntry, bool) {
	for _, item := range r.Games() {
		if item.Key == key {
			return item, true
		}
	}
	return domain.GameCatalogEntry{}, false
}
