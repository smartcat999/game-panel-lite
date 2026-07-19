package dst

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

//go:embed dst_world_options.json
var dstWorldOptionsJSON []byte

type dstWorldOptionManifest struct {
	SourceBuild string           `json:"sourceBuild"`
	Options     []dstWorldOption `json:"options"`
}

type dstWorldOption struct {
	Key              string   `json:"key"`
	Label            string   `json:"label"`
	LabelEn          string   `json:"labelEn"`
	Category         string   `json:"category"`
	Group            string   `json:"group"`
	Default          string   `json:"default"`
	Values           []string `json:"values"`
	Worlds           []string `json:"worlds"`
	MasterControlled bool     `json:"masterControlled"`
}

var dstWorldManifest = mustLoadDSTWorldManifest()

func mustLoadDSTWorldManifest() dstWorldOptionManifest {
	var manifest dstWorldOptionManifest
	if err := json.Unmarshal(dstWorldOptionsJSON, &manifest); err != nil {
		panic(fmt.Sprintf("load embedded DST world options: %v", err))
	}
	if len(manifest.Options) == 0 {
		panic("embedded DST world options are empty")
	}
	return manifest
}

func configSchema() []domain.ProviderConfigField {
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
	fields = append(fields, worldOptionFields("world", "forest")...)
	fields = append(fields, worldOptionFields("caves", "cave")...)
	return fields
}

func worldOptionFields(prefix string, world string) []domain.ProviderConfigField {
	fields := make([]domain.ProviderConfigField, 0, len(dstWorldManifest.Options))
	for _, item := range dstWorldManifest.Options {
		if !containsString(item.Worlds, world) || (world == "cave" && item.MasterControlled) {
			continue
		}
		values := valuesForWorld(item, world)
		options := make([]domain.ProviderConfigFieldOption, 0, len(values))
		for _, value := range values {
			options = append(options, domain.ProviderConfigFieldOption{Value: value, Label: dstOptionLabel(value)})
		}
		fields = append(fields, domain.ProviderConfigField{
			Name:    prefix + ".overrides." + item.Key,
			Label:   item.Label,
			Type:    "select",
			Default: item.Default,
			Options: options,
			Group:   strings.Join([]string{"dst", prefix, item.Category, item.Group}, "."),
		})
	}
	return fields
}

func valuesForWorld(item dstWorldOption, world string) []string {
	if item.Key == "task_set" {
		if world == "cave" {
			return []string{"cave_default"}
		}
		return []string{"default", "classic"}
	}
	if item.Key == "start_location" {
		if world == "cave" {
			return []string{"caves"}
		}
		return []string{"default", "plus", "darkness"}
	}
	return item.Values
}

func dstOptionLabel(value string) string {
	labels := map[string]string{
		"0": "立即", "5": "5 天", "15": "15 天", "20": "20 天",
		"always": "总是", "autumn|spring": "秋季或春季", "autumn|winter|spring|summer": "随机季节",
		"cave_default": "默认洞穴", "caves": "洞穴入口", "classic": "经典", "darkness": "黑暗", "default": "默认",
		"enabled": "启用", "fast": "快", "few": "少", "fixed": "固定出生门", "highly random": "高度随机",
		"huge": "巨大", "insane": "疯狂", "least": "最少", "longday": "长白天", "longdusk": "长黄昏",
		"longnight": "长夜晚", "longseason": "长", "many": "多", "max": "最大", "medium": "中等",
		"more": "更多伤害", "most": "最多", "mostly": "很多", "never": "从不", "noday": "无白天",
		"nodusk": "无黄昏", "none": "无", "nonight": "无夜晚", "nonlethal": "非致命", "noseason": "无",
		"often": "较多", "onlyday": "仅白天", "onlydusk": "仅黄昏", "onlynight": "仅夜晚", "plus": "额外资源",
		"random": "随机", "rare": "稀少", "scatter": "随机出生", "shortseason": "短", "slow": "慢",
		"small": "小", "spring": "春季", "summer": "夏季", "uncommon": "较少", "veryfast": "非常快",
		"verylongseason": "非常长", "veryshortseason": "非常短", "veryslow": "非常慢", "winter": "冬季",
		"winter|summer": "冬季或夏季",
		"ocean_never":   "从不", "ocean_rare": "稀少", "ocean_uncommon": "较少", "ocean_default": "默认",
		"ocean_often": "较多", "ocean_mostly": "很多", "ocean_always": "总是", "ocean_insane": "疯狂",
	}
	if label := labels[value]; label != "" {
		return label
	}
	return value
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func floatPtr(value float64) *float64 { return &value }
