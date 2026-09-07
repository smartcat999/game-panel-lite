package palworld

import "github.com/smartcat999/game-panel-lite/apps/api/internal/domain"

func (Provider) CatalogMetadata() domain.ProviderCatalogMetadata {
	return domain.ProviderCatalogMetadata{GameName: "Palworld", GameDescription: "Survival crafting server for small friend groups. Provider implementation is next on the roadmap.", CoverImage: "palworld", Priority: 100}
}
