package sevendaystodie

import (
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

// Optional settings preserve existing instance templates until explicitly configured.
// Names/defaults verified against v3.3.0 Steam Build 25661908 serverconfig.xml.
type advancedConfig struct {
	Region                             *string `json:"region,omitempty"`
	Language                           *string `json:"language,omitempty"`
	MaxSpawnedZombies                  *int    `json:"maxSpawnedZombies,omitempty"`
	MaxSpawnedAnimals                  *int    `json:"maxSpawnedAnimals,omitempty"`
	ServerMaxAllowedViewDistance       *int    `json:"serverMaxAllowedViewDistance,omitempty"`
	ServerMaxWorldTransferSpeedKiBs    *int    `json:"serverMaxWorldTransferSpeedKiBs,omitempty"`
	MaxQueuedMeshLayers                *int    `json:"maxQueuedMeshLayers,omitempty"`
	DynamicMeshEnabled                 *bool   `json:"dynamicMeshEnabled,omitempty"`
	DynamicMeshLandClaimOnly           *bool   `json:"dynamicMeshLandClaimOnly,omitempty"`
	DynamicMeshLandClaimBuffer         *int    `json:"dynamicMeshLandClaimBuffer,omitempty"`
	DynamicMeshMaxItemCache            *int    `json:"dynamicMeshMaxItemCache,omitempty"`
	LandClaimCount                     *int    `json:"landClaimCount,omitempty"`
	LandClaimSize                      *int    `json:"landClaimSize,omitempty"`
	LandClaimDeadZone                  *int    `json:"landClaimDeadZone,omitempty"`
	LandClaimExpiryTime                *int    `json:"landClaimExpiryTime,omitempty"`
	LandClaimDecayMode                 *int    `json:"landClaimDecayMode,omitempty"`
	LandClaimOnlineDurabilityModifier  *int    `json:"landClaimOnlineDurabilityModifier,omitempty"`
	LandClaimOfflineDurabilityModifier *int    `json:"landClaimOfflineDurabilityModifier,omitempty"`
	LandClaimOfflineDelay              *int    `json:"landClaimOfflineDelay,omitempty"`
}

func advancedConfigSchema() []domain.ProviderConfigField {
	n := func(v float64) *float64 { return &v }
	return []domain.ProviderConfigField{
		{Name: "region", Label: "服务器地区", LabelEn: "Server region", Type: "select", Default: "NorthAmericaEast", Required: true, Options: []domain.ProviderConfigFieldOption{{Value: "NorthAmericaEast", Label: "北美东部", LabelEn: "North America East"}, {Value: "NorthAmericaWest", Label: "北美西部", LabelEn: "North America West"}, {Value: "CentralAmerica", Label: "中美洲", LabelEn: "Central America"}, {Value: "SouthAmerica", Label: "南美洲", LabelEn: "South America"}, {Value: "Europe", Label: "欧洲", LabelEn: "Europe"}, {Value: "Russia", Label: "俄罗斯", LabelEn: "Russia"}, {Value: "Asia", Label: "亚洲", LabelEn: "Asia"}, {Value: "MiddleEast", Label: "中东", LabelEn: "Middle East"}, {Value: "Africa", Label: "非洲", LabelEn: "Africa"}, {Value: "Oceania", Label: "大洋洲", LabelEn: "Oceania"}}},
		{Name: "language", Label: "服务器语言", LabelEn: "Server language", Type: "text", Default: "English", Required: true, Help: "列表检索用英文语言名，如 Chinese。", HelpEn: "Use an English language name for server searches, e.g. Chinese."},
		{Name: "maxSpawnedZombies", Label: "丧尸上限", LabelEn: "Max zombies", Type: "number", Default: 64, Required: true, Group: "sevenDays.performance", Min: n(0), Step: n(1), Help: "血月、沉睡僵尸等场景会应用额外系数。", HelpEn: "Blood moons and sleepers apply additional scaling."},
		{Name: "maxSpawnedAnimals", Label: "动物上限", LabelEn: "Max animals", Type: "number", Default: 50, Required: true, Group: "sevenDays.performance", Min: n(0), Step: n(1)},
		{Name: "serverMaxAllowedViewDistance", Label: "最大可视距离", LabelEn: "Max view distance", Type: "number", Default: 12, Required: true, Group: "sevenDays.performance", Min: n(6), Max: n(12), Step: n(1)},
		{Name: "serverMaxWorldTransferSpeedKiBs", Label: "地图传输上限（KiB/s）", LabelEn: "World transfer limit (KiB/s)", Type: "number", Default: 512, Required: true, Group: "sevenDays.performance", Min: n(0), Step: n(1), Help: "仅首次加入下载地图；实际最高约 1300 KiB/s。", HelpEn: "Initial world download only; practical maximum is about 1300 KiB/s."},
		{Name: "maxQueuedMeshLayers", Label: "网格排队层数", LabelEn: "Queued mesh layers", Type: "number", Default: 1000, Required: true, Group: "sevenDays.performance", Min: n(0), Step: n(1), Help: "降低可节省内存，但可能延长区块生成。", HelpEn: "Lower values save memory but may slow chunk generation."},
		{Name: "dynamicMeshEnabled", Label: "动态网格", LabelEn: "Dynamic mesh", Type: "boolean", Default: true, Group: "sevenDays.performance"},
		{Name: "dynamicMeshLandClaimOnly", Label: "仅限征用区域", LabelEn: "Dynamic mesh in claimed areas only", Type: "boolean", Default: true, Group: "sevenDays.performance"},
		{Name: "dynamicMeshLandClaimBuffer", Label: "征用区域缓冲（区块）", LabelEn: "Claim mesh radius (chunks)", Type: "number", Default: 3, Required: true, Group: "sevenDays.performance", Min: n(0), Step: n(1)},
		{Name: "dynamicMeshMaxItemCache", Label: "同时处理网格数", LabelEn: "Concurrent dynamic mesh items", Type: "number", Default: 3, Required: true, Group: "sevenDays.performance", Min: n(0), Step: n(1), Help: "提高会增加内存占用。", HelpEn: "Higher values use more memory."},
		{Name: "landClaimCount", Label: "每人征用块数量", LabelEn: "Claim blocks per player", Type: "number", Default: 5, Required: true, Group: "sevenDays.landClaims", Min: n(0), Step: n(1)},
		{Name: "landClaimSize", Label: "保护范围（物块）", LabelEn: "Claim size (blocks)", Type: "number", Default: 41, Required: true, Group: "sevenDays.landClaims", Min: n(1), Step: n(1)},
		{Name: "landClaimDeadZone", Label: "保护区间距（物块）", LabelEn: "Claim separation (blocks)", Type: "number", Default: 30, Required: true, Group: "sevenDays.landClaims", Min: n(0), Step: n(1), Help: "好友之间不受此间距限制。", HelpEn: "This separation does not apply between friends."},
		{Name: "landClaimExpiryTime", Label: "征用权期限（现实天）", LabelEn: "Offline expiry (real days)", Type: "number", Default: 7, Required: true, Group: "sevenDays.landClaims", Min: n(0), Step: n(1)},
		{Name: "landClaimDecayMode", Label: "征用权衰减方式", LabelEn: "Offline decay", Type: "number", Default: 0, Required: true, Group: "sevenDays.landClaims", Min: n(0), Max: n(2), Step: n(1), Options: []domain.ProviderConfigFieldOption{{Value: "0", Label: "缓慢（线性）", LabelEn: "Slow (linear)"}, {Value: "1", Label: "快速（指数）", LabelEn: "Fast (exponential)"}, {Value: "2", Label: "到期前不衰减", LabelEn: "Full protection until expiry"}}},
		{Name: "landClaimOnlineDurabilityModifier", Label: "在线耐久倍率", LabelEn: "Online durability multiplier", Type: "number", Default: 4, Required: true, Group: "sevenDays.landClaims", Min: n(0), Step: n(1), Options: []domain.ProviderConfigFieldOption{{Value: "0", Label: "无限", LabelEn: "Infinite"}, {Value: "1", Label: "1 倍", LabelEn: "1x"}, {Value: "2", Label: "2 倍", LabelEn: "2x"}, {Value: "4", Label: "4 倍", LabelEn: "4x"}, {Value: "8", Label: "8 倍", LabelEn: "8x"}, {Value: "16", Label: "16 倍", LabelEn: "16x"}, {Value: "32", Label: "32 倍", LabelEn: "32x"}, {Value: "64", Label: "64 倍", LabelEn: "64x"}, {Value: "128", Label: "128 倍", LabelEn: "128x"}, {Value: "256", Label: "256 倍", LabelEn: "256x"}}},
		{Name: "landClaimOfflineDurabilityModifier", Label: "离线耐久倍率", LabelEn: "Offline durability multiplier", Type: "number", Default: 4, Required: true, Group: "sevenDays.landClaims", Min: n(0), Step: n(1), Options: []domain.ProviderConfigFieldOption{{Value: "0", Label: "无限", LabelEn: "Infinite"}, {Value: "1", Label: "1 倍", LabelEn: "1x"}, {Value: "2", Label: "2 倍", LabelEn: "2x"}, {Value: "4", Label: "4 倍", LabelEn: "4x"}, {Value: "8", Label: "8 倍", LabelEn: "8x"}, {Value: "16", Label: "16 倍", LabelEn: "16x"}, {Value: "32", Label: "32 倍", LabelEn: "32x"}, {Value: "64", Label: "64 倍", LabelEn: "64x"}, {Value: "128", Label: "128 倍", LabelEn: "128x"}, {Value: "256", Label: "256 倍", LabelEn: "256x"}}},
		{Name: "landClaimOfflineDelay", Label: "离线切换延迟（分钟）", LabelEn: "Offline protection delay (minutes)", Type: "number", Default: 0, Required: true, Group: "sevenDays.landClaims", Min: n(0), Step: n(1)},
	}
}

func validateAdvancedConfig(c advancedConfig) error {
	if c.Region != nil {
		switch *c.Region {
		case "NorthAmericaEast", "NorthAmericaWest", "CentralAmerica", "SouthAmerica", "Europe", "Russia", "Asia", "MiddleEast", "Africa", "Oceania":
		default:
			return fmt.Errorf("unsupported server region")
		}
	}
	if c.Language != nil {
		if strings.TrimSpace(*c.Language) == "" || len(*c.Language) > 128 || strings.IndexFunc(*c.Language, unicode.IsControl) >= 0 {
			return fmt.Errorf("server language must contain 1-128 bytes without control characters")
		}
	}
	// These are panel input-safety bounds, not undocumented official maximums.
	for _, field := range []struct {
		name    string
		value   *int
		minimum int
	}{
		{"MaxSpawnedZombies", c.MaxSpawnedZombies, 0},
		{"MaxSpawnedAnimals", c.MaxSpawnedAnimals, 0},
		{"ServerMaxAllowedViewDistance", c.ServerMaxAllowedViewDistance, 6},
		{"ServerMaxWorldTransferSpeedKiBs", c.ServerMaxWorldTransferSpeedKiBs, 0},
		{"MaxQueuedMeshLayers", c.MaxQueuedMeshLayers, 0},
		{"DynamicMeshLandClaimBuffer", c.DynamicMeshLandClaimBuffer, 0},
		{"DynamicMeshMaxItemCache", c.DynamicMeshMaxItemCache, 0},
		{"LandClaimCount", c.LandClaimCount, 0},
		{"LandClaimSize", c.LandClaimSize, 1},
		{"LandClaimDeadZone", c.LandClaimDeadZone, 0},
		{"LandClaimExpiryTime", c.LandClaimExpiryTime, 0},
		{"LandClaimDecayMode", c.LandClaimDecayMode, 0},
		{"LandClaimOfflineDelay", c.LandClaimOfflineDelay, 0},
		{"LandClaimOnlineDurabilityModifier", c.LandClaimOnlineDurabilityModifier, 0},
		{"LandClaimOfflineDurabilityModifier", c.LandClaimOfflineDurabilityModifier, 0},
	} {
		if field.value != nil && (*field.value < field.minimum || *field.value > math.MaxInt32) {
			return fmt.Errorf("%s must be an integer between %d and %d", field.name, field.minimum, math.MaxInt32)
		}
	}
	if c.ServerMaxAllowedViewDistance != nil && *c.ServerMaxAllowedViewDistance > 12 {
		return fmt.Errorf("view distance must be between 6 and 12")
	}
	if c.LandClaimDecayMode != nil && *c.LandClaimDecayMode > 2 {
		return fmt.Errorf("land claim decay mode must be 0, 1 or 2")
	}
	return nil
}

func optionalProperty[T string | int | bool](name string, value *T) *property {
	if value == nil {
		return nil
	}
	return &property{Name: name, Value: fmt.Sprint(*value)}
}

func advancedProperties(c advancedConfig) []property {
	var properties []property
	for _, value := range []*property{
		optionalProperty("Region", c.Region),
		optionalProperty("Language", c.Language),
		optionalProperty("MaxSpawnedZombies", c.MaxSpawnedZombies),
		optionalProperty("MaxSpawnedAnimals", c.MaxSpawnedAnimals),
		optionalProperty("ServerMaxAllowedViewDistance", c.ServerMaxAllowedViewDistance),
		optionalProperty("ServerMaxWorldTransferSpeedKiBs", c.ServerMaxWorldTransferSpeedKiBs),
		optionalProperty("MaxQueuedMeshLayers", c.MaxQueuedMeshLayers),
		optionalProperty("DynamicMeshEnabled", c.DynamicMeshEnabled),
		optionalProperty("DynamicMeshLandClaimOnly", c.DynamicMeshLandClaimOnly),
		optionalProperty("DynamicMeshLandClaimBuffer", c.DynamicMeshLandClaimBuffer),
		optionalProperty("DynamicMeshMaxItemCache", c.DynamicMeshMaxItemCache),
		optionalProperty("LandClaimCount", c.LandClaimCount),
		optionalProperty("LandClaimSize", c.LandClaimSize),
		optionalProperty("LandClaimDeadZone", c.LandClaimDeadZone),
		optionalProperty("LandClaimExpiryTime", c.LandClaimExpiryTime),
		optionalProperty("LandClaimDecayMode", c.LandClaimDecayMode),
		optionalProperty("LandClaimOnlineDurabilityModifier", c.LandClaimOnlineDurabilityModifier),
		optionalProperty("LandClaimOfflineDurabilityModifier", c.LandClaimOfflineDurabilityModifier),
		optionalProperty("LandClaimOfflineDelay", c.LandClaimOfflineDelay),
	} {
		if value != nil {
			properties = append(properties, *value)
		}
	}
	return properties
}
