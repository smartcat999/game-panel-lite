package sevendaystodie

import "github.com/smartcat999/game-panel-lite/apps/api/internal/domain"

func (Provider) ConfigSchema() []domain.ProviderConfigField {
	n := func(v float64) *float64 { return &v }
	return []domain.ProviderConfigField{
		{Name: "serverName", Label: "服务器名称", LabelEn: "Server name", Type: "text", Required: true, Default: "7 Days to Die Server"},
		{Name: "worldName", Label: "存档名称", LabelEn: "Save name", Type: "text", Required: true, Default: "Friends"},
		{Name: "serverPassword", Label: "服务器密码", LabelEn: "Server password", Type: "password", Default: ""},
		{Name: "maxPlayers", Label: "最大玩家数", LabelEn: "Max players", Type: "number", Required: true, Default: 8, Min: n(1), Max: n(64), Step: n(1)},
		{Name: "gameWorld", Label: "地图", LabelEn: "World", Type: "select", Default: "Navezgane", Options: []domain.ProviderConfigFieldOption{{Value: "Navezgane", Label: "纳维戈恩", LabelEn: "Navezgane"}, {Value: "RWG", Label: "随机生成", LabelEn: "Random generation"}}},
		{Name: "worldGenSeed", Label: "地图种子", LabelEn: "World seed", Type: "text", Default: "GamePanelLite"},
		{Name: "worldGenSize", Label: "随机地图大小", LabelEn: "Random world size", Type: "number", Required: true, Default: 6144, Min: n(4096), Max: n(8192), Step: n(2048)},
		{Name: "serverVisibility", Label: "可见性", LabelEn: "Visibility", Type: "select", Default: "2", Options: []domain.ProviderConfigFieldOption{{Value: "0", Label: "不公开", LabelEn: "Hidden"}, {Value: "1", Label: "好友", LabelEn: "Friends"}, {Value: "2", Label: "公开", LabelEn: "Public"}}},
		{Name: "playerKillingMode", Label: "玩家伤害", LabelEn: "Player damage", Type: "select", Default: "0", Options: []domain.ProviderConfigFieldOption{{Value: "0", Label: "关闭", LabelEn: "Disabled"}, {Value: "1", Label: "仅盟友", LabelEn: "Allies only"}, {Value: "2", Label: "仅非盟友", LabelEn: "Strangers only"}, {Value: "3", Label: "所有玩家", LabelEn: "Everyone"}}},
		{Name: "eacEnabled", Label: "启用反作弊", LabelEn: "Enable anti-cheat", Type: "boolean", Default: true},
		{Name: "sandboxCode", Label: "沙盒规则代码", LabelEn: "Sandbox code", Type: "text", Required: true, Default: "AAAJABJACJADJARFBNC", Help: "在游戏的新建游戏 → 沙盒选项中设置难度、血月和掉落等规则，复制代码并粘贴到这里。请使用相同游戏版本生成代码。", HelpEn: "Set difficulty, blood moons and loot in New Game → Sandbox Options, then copy the code here. Generate it with the same game version."},
	}
}
