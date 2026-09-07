package terraria

import "github.com/smartcat999/game-panel-lite/apps/api/internal/domain"

func (VanillaProvider) CatalogMetadata() domain.ProviderCatalogMetadata {
	return domain.ProviderCatalogMetadata{PluginVersion: "1.0.0", ConfigVersion: 1, GameName: "Terraria", GameDescription: "2D sandbox adventure server for vanilla and tModLoader worlds.", CoverImage: "terraria", Priority: 10}
}

func (TModLoaderProvider) CatalogMetadata() domain.ProviderCatalogMetadata {
	return domain.ProviderCatalogMetadata{PluginVersion: "1.0.0", ConfigVersion: 1, GameName: "Terraria", GameDescription: "2D sandbox adventure server for vanilla and tModLoader worlds.", CoverImage: "terraria", Priority: 20}
}
