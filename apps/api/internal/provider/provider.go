package provider

import (
	"fmt"
	"sort"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

type GameProvider interface {
	GameKey() domain.GameKey
	Key() domain.ProviderKey
	Name() string
	Description() string
	CatalogMetadata() domain.ProviderCatalogMetadata
	Capabilities() domain.ProviderCapabilities
	ConfigSchema() []domain.ProviderConfigField
	Image() string
	Versions() []string
	ImageFor(version string) string
}

type ConfigPayloadProvider interface {
	DefaultConfigPayload() map[string]any
	NormalizeConfigPayload(map[string]any) (map[string]any, error)
	ValidateConfigPayload(map[string]any) error
}

type ConfigSummaryProvider interface {
	ConfigSummary(map[string]any) (domain.ProviderConfigSummary, error)
}

type ResourceRuntimeProvider interface {
	RuntimeConfigForResource(domain.GameServer) (domain.ProviderRuntimeConfig, error)
}

type JoinInfoProvider interface {
	JoinInfo(domain.GameServer) domain.ServerJoinInfo
}

type SaveMetadataProvider interface {
	SaveDisplayName() string
}

type WorldRegenerationProvider interface {
	WorldRegenerationPlan(domain.GameServer) (domain.WorldRegenerationPlan, error)
}

type PlayerListProvider interface {
	PlayerListCommand(domain.GameServer) string
	ParsePlayerListOutput([]string) []domain.Player
}

type PlayerCommandProvider interface {
	KickCommand(player string) string
	BanCommand(player string) string
}

type WhitelistCommandProvider interface {
	WhitelistAddCommand(player string) string
	WhitelistRemoveCommand(player string) string
	WhitelistListCommand() string
}

type PlayerActivityProvider interface {
	ParsePlayerLogEvent(string) (domain.PlayerLogEvent, bool)
}

// PlayerCountLogProvider derives the current online count from a bounded
// workload log snapshot. A nil result means the snapshot is incomplete.
type PlayerCountLogProvider interface {
	ParsePlayerCount([]string) *int
}

type Registry struct {
	providers map[domain.ProviderKey]GameProvider
}

// NewRegistry constructs a registry that is read-only after startup. Duplicate IDs
// are configuration errors rather than implicit overrides.
func NewRegistry(providers ...GameProvider) (*Registry, error) {
	registry := &Registry{providers: map[domain.ProviderKey]GameProvider{}}
	for _, item := range providers {
		if item == nil || item.Key() == "" || item.GameKey() == "" {
			return nil, fmt.Errorf("provider and game IDs are required")
		}
		if _, exists := registry.providers[item.Key()]; exists {
			return nil, fmt.Errorf("duplicate provider ID: %s", item.Key())
		}
		registry.providers[item.Key()] = item
	}
	return registry, nil
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
	games := map[domain.GameKey]domain.GameCatalogEntry{}
	for _, item := range r.List() {
		metadata := item.CatalogMetadata()
		entry := games[item.GameKey()]
		if entry.Key == "" {
			entry = domain.GameCatalogEntry{
				Key:         item.GameKey(),
				Name:        metadata.GameName,
				Description: metadata.GameDescription,
				CoverImage:  metadata.CoverImage,
				Status:      "available",
			}
		}
		entry.Providers = append(entry.Providers, domain.ProviderCatalog{
			Key:                item.Key(),
			Name:               item.Name(),
			Description:        item.Description(),
			Recommended:        false,
			Versions:           append([]string{}, item.Versions()...),
			RecommendedVersion: recommendedVersion(item.Versions()),
			Capabilities:       item.Capabilities(),
			ConfigSchema:       append([]domain.ProviderConfigField{}, item.ConfigSchema()...),
			SaveDisplayName:    saveDisplayNameFor(item),
		})
		entry.Status = "available"
		games[item.GameKey()] = entry
	}
	out := make([]domain.GameCatalogEntry, 0, len(games))
	for _, item := range games {
		r.sortProviderCatalog(item.Providers)
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

func (r *Registry) sortProviderCatalog(providers []domain.ProviderCatalog) {
	sort.SliceStable(providers, func(i, j int) bool {
		leftPriority := r.providers[providers[i].Key].CatalogMetadata().Priority
		rightPriority := r.providers[providers[j].Key].CatalogMetadata().Priority
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		return providers[i].Key < providers[j].Key
	})
	for index := range providers {
		providers[index].Recommended = index == 0
	}
}

func (r *Registry) Game(key domain.GameKey) (domain.GameCatalogEntry, bool) {
	for _, item := range r.Games() {
		if item.Key == key {
			return item, true
		}
	}
	return domain.GameCatalogEntry{}, false
}

func saveDisplayNameFor(item GameProvider) string {
	if saveProvider, ok := item.(SaveMetadataProvider); ok {
		return saveProvider.SaveDisplayName()
	}
	return "save"
}

func recommendedVersion(versions []string) string {
	if len(versions) == 0 {
		return ""
	}
	return versions[0]
}
