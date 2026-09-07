package minecraft

import "github.com/smartcat999/game-panel-lite/apps/api/internal/domain"

func (Provider) CatalogMetadata() domain.ProviderCatalogMetadata {
	return domain.ProviderCatalogMetadata{PluginVersion: "1.0.0", ConfigVersion: 1, GameName: "Minecraft Java", GameDescription: "Vanilla Minecraft Java Edition server for friends.", CoverImage: "minecraft", Priority: 100}
}
