package minecraft

import "github.com/smartcat999/game-panel-lite/apps/api/internal/domain"

func (Provider) CatalogMetadata() domain.ProviderCatalogMetadata {
	return domain.ProviderCatalogMetadata{GameName: "Minecraft Java", GameDescription: "Vanilla Minecraft Java Edition server for friends.", CoverImage: "minecraft", Priority: 100}
}
