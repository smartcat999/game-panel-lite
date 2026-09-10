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
	TerrariaReleaseID         = "gpr_terraria_1458_v1"
	TModLoaderReleaseID       = "gpr_tmodloader_202607_v2"
	legacyTModLoaderReleaseID = "gpr_tmodloader_202607_v1"
)

var publishedAt = time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)

func Publish(ctx context.Context, store providercontract.Store, signingKey []byte) (*providercontract.Registry, map[string]nodeworkload.GameProvider, error) {
	registry := providercontract.NewRegistry(store, signingKey)
	for _, manifest := range Manifests() {
		if _, err := registry.Publish(ctx, manifest, publishedAt); err != nil {
			return nil, nil, err
		}
	}
	providers := map[string]nodeworkload.GameProvider{TerrariaReleaseID: terraria.Provider{}, TModLoaderReleaseID: tmodloader.Provider{}, legacyTModLoaderReleaseID: tmodloader.Provider{}}
	return registry, providers, nil
}

func Manifests() []providercontract.Manifest {
	minimumPlayers, maximumPlayers := 1.0, 255.0
	commonFields := map[string]providercontract.Field{
		"worldName":  {Type: "string", Title: "世界名称", ApplyBehavior: providercontract.ApplyCreateOnly, Default: "GamePanel World", MinLength: intPointer(1), MaxLength: intPointer(64), Pattern: `^[^/\\\r\n\x00.][^/\\\r\n\x00]*$`},
		"worldSize":  {Type: "enum", Title: "世界大小", ApplyBehavior: providercontract.ApplyCreateOnly, Default: "medium", Enum: []any{"small", "medium", "large"}},
		"difficulty": {Type: "enum", Title: "难度", ApplyBehavior: providercontract.ApplyCreateOnly, Default: "classic", Enum: []any{"journey", "classic", "expert", "master"}},
		"maxPlayers": {Type: "integer", Title: "最大玩家数", ApplyBehavior: providercontract.ApplyRestart, Default: 8, Minimum: &minimumPlayers, Maximum: &maximumPlayers},
		"motd":       {Type: "string", Title: "欢迎语", ApplyBehavior: providercontract.ApplyRestart, Default: "Welcome to GamePanel", MaxLength: intPointer(256)},
		"password":   {Type: "secret", Title: "服务器密码", ApplyBehavior: providercontract.ApplyRestart, Default: "", MaxLength: intPointer(64)},
	}
	ui := providercontract.UISchema{Sections: []providercontract.UISection{{ID: "general", Title: "游戏配置", Order: 1}}, Fields: map[string]providercontract.UIField{
		"worldName": {Section: "general", Order: 1, Control: "text"}, "worldSize": {Section: "general", Order: 2, Control: "select"}, "difficulty": {Section: "general", Order: 3, Control: "select"}, "maxPlayers": {Section: "general", Order: 4, Control: "number"}, "motd": {Section: "general", Order: 5, Control: "text"}, "password": {Section: "general", Order: 6, Control: "password"},
	}}
	listener := []contract.ListenerRequirement{{Name: "game", Purpose: "join", Transports: []string{"tcp"}, InternalPort: 7777, ExternalPortPolicy: "allocated", AddressMode: "ip-port", Primary: true}}
	terrariaFields := cloneFields(commonFields)
	terrariaFields["serverName"] = providercontract.Field{Type: "string", Title: "服务器名称", ApplyBehavior: providercontract.ApplyRestart, Default: "GamePanel Server", MinLength: intPointer(1), MaxLength: intPointer(64)}
	terrariaFields["worldEvil"] = providercontract.Field{Type: "enum", Title: "世界邪恶", ApplyBehavior: providercontract.ApplyCreateOnly, Default: "random", Enum: []any{"random", "corruption", "crimson"}}
	terrariaFields["secure"] = providercontract.Field{Type: "boolean", Title: "安全校验", ApplyBehavior: providercontract.ApplyRestart, Default: true}
	terrariaFields["language"] = providercontract.Field{Type: "string", Title: "语言", ApplyBehavior: providercontract.ApplyRestart, Default: "en-US", MinLength: intPointer(2), MaxLength: intPointer(16)}
	terrariaUI := cloneUI(ui)
	terrariaUI.Fields["serverName"] = providercontract.UIField{Section: "general", Order: 0, Control: "text"}
	terrariaUI.Fields["worldEvil"] = providercontract.UIField{Section: "general", Order: 7, Control: "select"}
	terrariaUI.Fields["secure"] = providercontract.UIField{Section: "general", Order: 8, Control: "switch"}
	terrariaUI.Fields["language"] = providercontract.UIField{Section: "general", Order: 9, Control: "text"}
	tmodFields := cloneFields(commonFields)
	tmodFields["serverName"] = providercontract.Field{Type: "string", Title: "服务器名称", ApplyBehavior: providercontract.ApplyRestart, Default: "GamePanel tModLoader", MinLength: intPointer(1), MaxLength: intPointer(64)}
	tmodUI := cloneUI(ui)
	tmodUI.Fields["serverName"] = providercontract.UIField{Section: "general", Order: 0, Control: "text"}
	return []providercontract.Manifest{
		{ProviderReleaseID: TerrariaReleaseID, GameKey: terraria.GameKey, DisplayName: "Terraria", ReleaseVersion: "1", GameVersions: []string{terraria.GameVersion}, SchemaVersion: 1, ConfigurationSchema: providercontract.ConfigurationSchema{Type: "object", Properties: terrariaFields, Required: []string{"serverName", "worldName", "worldSize", "worldEvil", "difficulty", "maxPlayers", "secure", "language"}}, UISchema: terrariaUI, ListenerRequirements: listener, Capabilities: []string{"configuration", "console", "logs", "backup"}},
		{ProviderReleaseID: TModLoaderReleaseID, GameKey: tmodloader.GameKey, DisplayName: "tModLoader", ReleaseVersion: "2", GameVersions: []string{tmodloader.GameVersion}, SchemaVersion: 1, ConfigurationSchema: providercontract.ConfigurationSchema{Type: "object", Properties: tmodFields, Required: []string{"serverName", "worldName", "worldSize", "difficulty", "maxPlayers"}}, UISchema: tmodUI, ListenerRequirements: listener, Capabilities: []string{"configuration", "mods", "console", "logs", "backup"}, ModCatalog: &providercontract.ModCatalog{Revision: 1, Entries: []providercontract.ModCatalogEntry{
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
