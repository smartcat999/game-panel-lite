package productioncatalog

import (
	"context"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/gameprovider/terraria"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/gameprovider/tmodloader"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/nodeworkload"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
)

const (
	TerrariaReleaseID           = "gpr_terraria_1458_v2"
	TModLoaderReleaseID         = "gpr_tmodloader_202607_v3"
	legacyTerrariaReleaseID     = "gpr_terraria_1458_v1"
	legacyTModLoaderReleaseIDV1 = "gpr_tmodloader_202607_v1"
	legacyTModLoaderReleaseIDV2 = "gpr_tmodloader_202607_v2"
)

var publishedAt = time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)

func Publish(ctx context.Context, store providercontract.Store, signingKey []byte) (*providercontract.Registry, map[string]nodeworkload.GameProvider, error) {
	registry := providercontract.NewRegistry(store, signingKey)
	for _, manifest := range Manifests() {
		if _, err := registry.Publish(ctx, manifest, publishedAt); err != nil {
			return nil, nil, err
		}
	}
	providers := map[string]nodeworkload.GameProvider{
		TerrariaReleaseID:           terraria.Provider{},
		legacyTerrariaReleaseID:     terraria.Provider{},
		TModLoaderReleaseID:         tmodloader.Provider{},
		legacyTModLoaderReleaseIDV1: tmodloader.Provider{},
		legacyTModLoaderReleaseIDV2: tmodloader.Provider{},
	}
	return registry, providers, nil
}

func Manifests() []providercontract.Manifest {
	minimumPlayers, maximumPlayers := 1.0, 255.0
	commonFields := map[string]providercontract.Field{
		"worldName":  {Type: "string", Title: "世界名称", Localizations: localizedDefaults("世界名称", "World name", "泰拉瑞亚世界", "GamePanel World"), ApplyBehavior: providercontract.ApplyCreateOnly, Default: "GamePanel World", MinLength: intPointer(1), MaxLength: intPointer(64), Pattern: `^[^/\\\r\n\x00.][^/\\\r\n\x00]*$`},
		"worldSize":  {Type: "enum", Title: "世界大小", Localizations: localizedEnum("世界大小", "World size", map[string]string{"small": "小型", "medium": "中型", "large": "大型"}, map[string]string{"small": "Small", "medium": "Medium", "large": "Large"}), ApplyBehavior: providercontract.ApplyCreateOnly, Default: "medium", Enum: []any{"small", "medium", "large"}},
		"difficulty": {Type: "enum", Title: "难度", Localizations: localizedEnum("难度", "Difficulty", map[string]string{"journey": "旅途", "classic": "经典", "expert": "专家", "master": "大师"}, map[string]string{"journey": "Journey", "classic": "Classic", "expert": "Expert", "master": "Master"}), ApplyBehavior: providercontract.ApplyCreateOnly, Default: "classic", Enum: []any{"journey", "classic", "expert", "master"}},
		"maxPlayers": {Type: "integer", Title: "最大玩家数", Localizations: localized("最大玩家数", "Maximum players"), ApplyBehavior: providercontract.ApplyRestart, Default: 8, Minimum: &minimumPlayers, Maximum: &maximumPlayers},
		"motd":       {Type: "string", Title: "欢迎语", Localizations: localizedDefaults("欢迎语", "Welcome message", "欢迎来到服务器", "Welcome to GamePanel"), ApplyBehavior: providercontract.ApplyRestart, Default: "Welcome to GamePanel", MaxLength: intPointer(256)},
		"password":   {Type: "secret", Title: "服务器密码", Localizations: localized("服务器密码", "Server password"), ApplyBehavior: providercontract.ApplyRestart, Default: "", MaxLength: intPointer(64)},
	}
	ui := providercontract.UISchema{Sections: []providercontract.UISection{{ID: "general", Title: "游戏配置", Localizations: map[string]string{"zh-CN": "游戏配置", "en": "Game configuration"}, Order: 1}}, Fields: map[string]providercontract.UIField{
		"worldName": {Section: "general", Order: 1, Control: "text"}, "worldSize": {Section: "general", Order: 2, Control: "select"}, "difficulty": {Section: "general", Order: 3, Control: "select"}, "maxPlayers": {Section: "general", Order: 4, Control: "number"}, "motd": {Section: "general", Order: 5, Control: "text"}, "password": {Section: "general", Order: 6, Control: "password"},
	}}
	listener := []contract.ListenerRequirement{{Name: "game", Purpose: "join", Transports: []string{"tcp"}, InternalPort: 7777, ExternalPortPolicy: "allocated", AddressMode: "ip-port", Primary: true}}
	terrariaFields := cloneFields(commonFields)
	terrariaFields["worldEvil"] = providercontract.Field{Type: "enum", Title: "世界邪恶", Localizations: localizedEnum("世界邪恶", "World evil", map[string]string{"random": "随机", "corruption": "腐化", "crimson": "猩红"}, map[string]string{"random": "Random", "corruption": "Corruption", "crimson": "Crimson"}), ApplyBehavior: providercontract.ApplyCreateOnly, Default: "random", Enum: []any{"random", "corruption", "crimson"}}
	terrariaFields["secure"] = providercontract.Field{Type: "boolean", Title: "安全校验", Localizations: localized("安全校验", "Secure validation"), ApplyBehavior: providercontract.ApplyRestart, Default: true}
	terrariaFields["language"] = providercontract.Field{Type: "enum", Title: "语言", Localizations: map[string]providercontract.FieldLocalization{
		"zh-CN": {Title: "语言", Default: "zh-Hans", EnumLabels: map[string]string{"en-US": "英语（美国）", "de-DE": "德语", "it-IT": "意大利语", "fr-FR": "法语", "es-ES": "西班牙语", "ru-RU": "俄语", "zh-Hans": "简体中文", "pt-BR": "葡萄牙语（巴西）", "pl-PL": "波兰语"}},
		"en":    {Title: "Language", Default: "en-US", EnumLabels: map[string]string{"en-US": "English (United States)", "de-DE": "German", "it-IT": "Italian", "fr-FR": "French", "es-ES": "Spanish", "ru-RU": "Russian", "zh-Hans": "Simplified Chinese", "pt-BR": "Portuguese (Brazil)", "pl-PL": "Polish"}},
	}, ApplyBehavior: providercontract.ApplyRestart, Default: "en-US", Enum: []any{"en-US", "de-DE", "it-IT", "fr-FR", "es-ES", "ru-RU", "zh-Hans", "pt-BR", "pl-PL"}}
	terrariaUI := cloneUI(ui)
	terrariaUI.Fields["worldEvil"] = providercontract.UIField{Section: "general", Order: 7, Control: "select"}
	terrariaUI.Fields["secure"] = providercontract.UIField{Section: "general", Order: 8, Control: "switch"}
	terrariaUI.Fields["language"] = providercontract.UIField{Section: "general", Order: 9, Control: "select"}
	tmodFields := cloneFields(commonFields)
	tmodUI := cloneUI(ui)
	return []providercontract.Manifest{
		{ProviderReleaseID: TerrariaReleaseID, GameKey: terraria.GameKey, DisplayName: "Terraria", ReleaseVersion: "2", GameVersions: []string{terraria.GameVersion}, SchemaVersion: 1, ConfigurationSchema: providercontract.ConfigurationSchema{Type: "object", Properties: terrariaFields, Required: []string{"worldName", "worldSize", "worldEvil", "difficulty", "maxPlayers", "secure", "language"}}, UISchema: terrariaUI, ListenerRequirements: listener, Capabilities: []string{"configuration", "console", "logs", "backup"}},
		{ProviderReleaseID: TModLoaderReleaseID, GameKey: tmodloader.GameKey, DisplayName: "tModLoader", ReleaseVersion: "3", GameVersions: []string{tmodloader.GameVersion}, SchemaVersion: 1, ConfigurationSchema: providercontract.ConfigurationSchema{Type: "object", Properties: tmodFields, Required: []string{"worldName", "worldSize", "difficulty", "maxPlayers"}}, UISchema: tmodUI, ListenerRequirements: listener, Capabilities: []string{"configuration", "mods", "console", "logs", "backup"}, ModCatalog: &providercontract.ModCatalog{Revision: 1, Entries: []providercontract.ModCatalogEntry{
			{ModID: tmodloader.CalamityWorkshopID, DisplayName: "Calamity Mod", Versions: []providercontract.ModVersion{{Version: tmodloader.CalamityVersion, Digest: tmodloader.CalamityDigest, Dependencies: []providercontract.ModDependency{{ModID: tmodloader.CalamityMusicWorkshopID, Version: tmodloader.CalamityMusicVersion}}}}},
			{ModID: tmodloader.CalamityMusicWorkshopID, DisplayName: "Calamity Mod Music", Versions: []providercontract.ModVersion{{Version: tmodloader.CalamityMusicVersion, Digest: tmodloader.CalamityMusicDigest}}},
		}}},
	}
}

func cloneFields(source map[string]providercontract.Field) map[string]providercontract.Field {
	result := make(map[string]providercontract.Field, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneUI(source providercontract.UISchema) providercontract.UISchema {
	result := providercontract.UISchema{Sections: append([]providercontract.UISection(nil), source.Sections...), Fields: make(map[string]providercontract.UIField, len(source.Fields))}
	for key, value := range source.Fields {
		result.Fields[key] = value
	}
	return result
}

func intPointer(value int) *int { return &value }

func localized(zhCN, en string) map[string]providercontract.FieldLocalization {
	return map[string]providercontract.FieldLocalization{"zh-CN": {Title: zhCN}, "en": {Title: en}}
}

func localizedDefaults(zhCN, en string, zhCNDefault, enDefault any) map[string]providercontract.FieldLocalization {
	return map[string]providercontract.FieldLocalization{"zh-CN": {Title: zhCN, Default: zhCNDefault}, "en": {Title: en, Default: enDefault}}
}

func localizedEnum(zhCN, en string, zhCNLabels, enLabels map[string]string) map[string]providercontract.FieldLocalization {
	return map[string]providercontract.FieldLocalization{"zh-CN": {Title: zhCN, EnumLabels: zhCNLabels}, "en": {Title: en, EnumLabels: enLabels}}
}
