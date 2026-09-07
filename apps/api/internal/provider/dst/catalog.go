package dst

import "github.com/smartcat999/game-panel-lite/apps/api/internal/domain"

func (Provider) CatalogMetadata() domain.ProviderCatalogMetadata {
	return domain.ProviderCatalogMetadata{GameName: "Don't Starve Together", GameDescription: "Co-op survival server for private friend groups.", CoverImage: "dont-starve-together", Priority: 100}
}
