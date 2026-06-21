package dst

import "github.com/smartcat999/game-panel-lite/apps/api/internal/domain"

func configSchema() []domain.ProviderConfigField {
	return []domain.ProviderConfigField{
		{Name: "serverName", Label: "服务器名称", Type: "text", Required: true, Default: "DST Friends"},
		{Name: "clusterName", Label: "存档名称", Type: "text", Required: true, Default: "GamePanelLite"},
		{Name: "maxPlayers", Label: "最大玩家数", Type: "number", Required: true, Default: 6},
		{Name: "serverPassword", Label: "服务器密码", Type: "password", Required: false},
		{Name: "clusterToken", Label: "Klei 服务器令牌", Type: "password", Required: true, Help: "在 Klei 账号页面创建专用服务器令牌后填入。"},
		{
			Name:     "worldPreset",
			Label:    "世界预设",
			Type:     "select",
			Required: true,
			Default:  "forest_default",
			Options: []domain.ProviderConfigFieldOption{
				{Value: "forest_default", Label: "默认森林"},
				{Value: "forest_classic", Label: "经典森林"},
				{Value: "forest_survival", Label: "生存森林"},
			},
		},
		{
			Name:     "gameMode",
			Label:    "游戏模式",
			Type:     "select",
			Required: true,
			Default:  "survival",
			Options: []domain.ProviderConfigFieldOption{
				{Value: "survival", Label: "生存"},
				{Value: "endless", Label: "无尽"},
				{Value: "wilderness", Label: "荒野"},
			},
		},
		{Name: "clusterDescription", Label: "服务器描述", Type: "text", Required: false, Default: "Managed by GamePanel Lite"},
		{Name: "cavesEnabled", Label: "启用洞穴", Type: "boolean", Required: false, Default: false, Help: "创建额外洞穴分片配置。"},
		{Name: "pvp", Label: "开启 PVP", Type: "boolean", Required: false, Default: false},
		{Name: "pauseWhenEmpty", Label: "无人时暂停", Type: "boolean", Required: false, Default: true},
		{Name: "offlineServer", Label: "离线服务器", Type: "boolean", Required: false, Default: false},
		{Name: "consoleEnabled", Label: "启用控制台", Type: "boolean", Required: false, Default: true},
	}
}
