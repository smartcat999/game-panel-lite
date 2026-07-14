package dst

import "github.com/smartcat999/game-panel-lite/apps/api/internal/domain"

const (
	groupWorldBasics    = "dst.world.basics"
	groupWorldSeasons   = "dst.world.seasons"
	groupWorldResources = "dst.world.resources"
	groupWorldCreatures = "dst.world.creatures"
	groupWorldThreats   = "dst.world.threats"
	groupCaveWorld      = "dst.caves.world"
	groupCaveResources  = "dst.caves.resources"
	groupCaveThreats    = "dst.caves.threats"
)

func options(values ...string) []domain.ProviderConfigFieldOption {
	result := make([]domain.ProviderConfigFieldOption, 0, len(values))
	for _, value := range values {
		result = append(result, domain.ProviderConfigFieldOption{Value: value, Label: value})
	}
	return result
}

func overrideField(prefix string, key string, label string, group string, values ...string) domain.ProviderConfigField {
	return domain.ProviderConfigField{
		Name: prefix + ".overrides." + key, Label: label, Type: "select", Default: "default",
		Options: options(values...), Group: group,
	}
}

func configSchema() []domain.ProviderConfigField {
	frequency := []string{"never", "rare", "default", "often", "always"}
	worldgenFrequency := []string{"never", "rare", "uncommon", "default", "often", "mostly", "always", "insane"}
	seasonLength := []string{"noseason", "veryshortseason", "shortseason", "default", "longseason", "verylongseason", "random"}
	speed := []string{"never", "veryslow", "slow", "default", "fast", "veryfast"}

	fields := []domain.ProviderConfigField{
		{Name: "identity.serverName", Label: "服务器名称", Type: "text", Required: true, Default: "DST Friends"},
		{Name: "identity.clusterName", Label: "集群名称", Type: "text", Required: true, Default: "GamePanelLite"},
		{Name: "identity.description", Label: "服务器描述", Type: "text", Required: false, Default: "Managed by GamePanel Lite"},
		{Name: "identity.password", Label: "服务器密码", Type: "password", Required: false},
		{Name: "identity.clusterToken", Label: "Klei 服务器令牌", Type: "password", Required: true, Help: "在 Klei 账号页面创建专用服务器令牌后填入。"},
		{Name: "identity.visibility", Label: "可见性", Type: "select", Required: true, Default: "public", Options: []domain.ProviderConfigFieldOption{{Value: "public", Label: "公开"}, {Value: "lan", Label: "局域网"}, {Value: "offline", Label: "离线"}}},
		{Name: "gameplay.gameMode", Label: "游戏模式", Type: "select", Required: true, Default: "survival", Options: []domain.ProviderConfigFieldOption{{Value: "survival", Label: "生存"}, {Value: "endless", Label: "无尽"}, {Value: "wilderness", Label: "荒野"}}},
		{Name: "gameplay.maxPlayers", Label: "最大玩家数", Type: "number", Required: true, Default: 6, Min: floatPtr(1), Max: floatPtr(64), Step: floatPtr(1)},
		{Name: "gameplay.pvp", Label: "开启 PVP", Type: "boolean", Default: false},
		{Name: "gameplay.pauseWhenEmpty", Label: "无人时暂停", Type: "boolean", Default: true},
		{Name: "gameplay.consoleEnabled", Label: "启用控制台", Type: "boolean", Default: true},
		{Name: "world.preset", Label: "世界预设", Type: "select", Required: true, Default: "forest_default", Options: []domain.ProviderConfigFieldOption{{Value: "forest_default", Label: "默认森林"}, {Value: "forest_classic", Label: "经典森林"}, {Value: "forest_survival", Label: "生存森林"}}},
		{Name: "caves.enabled", Label: "启用洞穴", Type: "boolean", Default: false, Help: "创建额外洞穴分片配置。"},
	}

	fields = append(fields,
		overrideField("world", "world_size", "世界大小", groupWorldBasics, "small", "medium", "default", "huge"),
		overrideField("world", "day", "昼夜长度", groupWorldBasics, "default", "longday", "longdusk", "longnight", "noday", "nodusk", "nonight", "onlyday", "onlydusk", "onlynight"),
		overrideField("world", "season_start", "起始季节", groupWorldSeasons, "default", "winter", "spring", "summer", "autumn|spring", "winter|summer", "autumn|winter|spring|summer"),
		overrideField("world", "autumn", "秋季长度", groupWorldSeasons, seasonLength...),
		overrideField("world", "winter", "冬季长度", groupWorldSeasons, seasonLength...),
		overrideField("world", "spring", "春季长度", groupWorldSeasons, seasonLength...),
		overrideField("world", "summer", "夏季长度", groupWorldSeasons, seasonLength...),
		overrideField("world", "grass", "草丛数量", groupWorldResources, worldgenFrequency...),
		overrideField("world", "sapling", "树苗数量", groupWorldResources, worldgenFrequency...),
		overrideField("world", "berrybush", "浆果丛数量", groupWorldResources, worldgenFrequency...),
		overrideField("world", "flint", "燧石数量", groupWorldResources, worldgenFrequency...),
		overrideField("world", "rock", "岩石数量", groupWorldResources, worldgenFrequency...),
		overrideField("world", "rabbits", "兔子洞数量", groupWorldCreatures, worldgenFrequency...),
		overrideField("world", "pigs", "猪人房数量", groupWorldCreatures, worldgenFrequency...),
		overrideField("world", "beefalo", "皮弗娄牛数量", groupWorldCreatures, worldgenFrequency...),
		overrideField("world", "spiders", "蜘蛛巢数量", groupWorldThreats, worldgenFrequency...),
		overrideField("world", "houndmound", "猎犬丘数量", groupWorldThreats, worldgenFrequency...),
		overrideField("world", "hounds", "猎犬袭击", groupWorldThreats, frequency...),
		overrideField("world", "wildfires", "自燃", groupWorldThreats, frequency...),
		overrideField("world", "deerclops", "独眼巨鹿", groupWorldThreats, frequency...),
		overrideField("world", "bearger", "熊獾", groupWorldThreats, frequency...),
		overrideField("world", "goosemoose", "麋鹿鹅", groupWorldThreats, frequency...),
		overrideField("world", "dragonfly", "龙蝇", groupWorldThreats, frequency...),
		overrideField("world", "regrowth", "资源再生速度", groupWorldResources, speed...),
		overrideField("caves", "world_size", "洞穴大小", groupCaveWorld, "small", "medium", "default", "huge"),
		overrideField("caves", "cavelight", "洞穴光照变化", groupCaveWorld, speed...),
		overrideField("caves", "grass", "洞穴草丛数量", groupCaveResources, worldgenFrequency...),
		overrideField("caves", "sapling", "洞穴树苗数量", groupCaveResources, worldgenFrequency...),
		overrideField("caves", "berrybush", "洞穴浆果丛数量", groupCaveResources, worldgenFrequency...),
		overrideField("caves", "flower_cave", "荧光花数量", groupCaveResources, worldgenFrequency...),
		overrideField("caves", "wormlights", "发光浆果数量", groupCaveResources, worldgenFrequency...),
		overrideField("caves", "cave_spiders", "洞穴蜘蛛巢", groupCaveThreats, worldgenFrequency...),
		overrideField("caves", "bats", "蝙蝠洞", groupCaveThreats, worldgenFrequency...),
		overrideField("caves", "earthquakes", "地震", groupCaveThreats, frequency...),
		overrideField("caves", "wormattacks", "洞穴蠕虫袭击", groupCaveThreats, frequency...),
		overrideField("caves", "toadstool", "毒菌蟾蜍", groupCaveThreats, frequency...),
	)
	return fields
}

func floatPtr(value float64) *float64 { return &value }
