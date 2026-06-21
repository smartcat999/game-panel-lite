package provider

import (
	"sort"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
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
			CoverImage:  "terraria",
		},
		domain.GamePalworld: {
			Key:         domain.GamePalworld,
			Name:        "Palworld",
			Description: "Survival crafting server for small friend groups. Provider implementation is next on the roadmap.",
			Status:      "planned",
			CoverImage:  "palworld",
		},
		domain.GameDST: {
			Key:         domain.GameDST,
			Name:        "Don't Starve Together",
			Description: "Co-op survival server for private friend groups.",
			Status:      "planned",
			CoverImage:  "dont-starve-together",
		},
		domain.GameMinecraft: {
			Key:         domain.GameMinecraft,
			Name:        "Minecraft Java",
			Description: "Vanilla Minecraft Java Edition server for friends.",
			Status:      "planned",
			CoverImage:  "minecraft",
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
		sortProviderCatalog(item.Providers)
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

func sortProviderCatalog(providers []domain.ProviderCatalog) {
	sort.SliceStable(providers, func(i, j int) bool {
		leftPriority := providerCatalogPriority(providers[i].Key)
		rightPriority := providerCatalogPriority(providers[j].Key)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		return providers[i].Key < providers[j].Key
	})
	for index := range providers {
		providers[index].Recommended = index == 0
	}
}

func providerCatalogPriority(key domain.ProviderKey) int {
	switch key {
	case domain.ProviderTerrariaVanilla:
		return 10
	case domain.ProviderTerrariaTModLoader:
		return 20
	default:
		return 100
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
	if versions[0] != "latest" {
		return versions[0]
	}
	for _, v := range versions[1:] {
		if v != "latest" {
			return v
		}
	}
	return versions[0]
}
