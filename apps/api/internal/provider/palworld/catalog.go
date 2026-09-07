package palworld

import "github.com/smartcat999/game-panel-lite/apps/api/internal/domain"

func (Provider) CatalogMetadata() domain.ProviderCatalogMetadata {
	return domain.ProviderCatalogMetadata{PluginVersion: "1.0.0", ConfigVersion: 1, GameName: "Palworld", GameDescription: "Survival crafting server for small friend groups. Provider implementation is next on the roadmap.", CoverImage: "palworld", Priority: 100}
}
