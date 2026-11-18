package minecraft

import "github.com/smartcat999/game-panel-lite/apps/api/internal/domain"

func configSchema() []domain.ProviderConfigField {
	return []domain.ProviderConfigField{
		{Name: "serverName", Label: "服务器名称 / MOTD", Type: "text", Required: true, Default: "Friends Server"},
		{Name: "worldName", Label: "世界名称", Type: "text", Required: true, Default: "world"},
		{Name: "maxPlayers", Label: "最大玩家数", Type: "number", Required: true, Default: 20},
		{
			Name:     "gameMode",
			Label:    "游戏模式",
			Type:     "select",
			Required: true,
			Default:  "survival",
			Options: []domain.ProviderConfigFieldOption{
				{Value: "survival", Label: "生存"},
				{Value: "creative", Label: "创造"},
				{Value: "adventure", Label: "冒险"},
				{Value: "spectator", Label: "旁观"},
			},
		},
		{
			Name:     "difficulty",
			Label:    "难度",
			Type:     "select",
			Required: true,
			Default:  "normal",
			Options: []domain.ProviderConfigFieldOption{
				{Value: "peaceful", Label: "和平"},
				{Value: "easy", Label: "简单"},
				{Value: "normal", Label: "普通"},
				{Value: "hard", Label: "困难"},
			},
		},
		{Name: "onlineMode", Label: "正版验证 (online-mode)", Type: "boolean", Required: false, Default: true, Help: "关闭后允许非正版账号加入，建议仅用于私人好友服。"},
		{Name: "whitelistEnabled", Label: "启用白名单", Type: "boolean", Required: false, Default: false, Help: "开启后仅白名单内玩家可加入，需要后续手动添加玩家。"},
		{Name: "eulaAccepted", Label: "我已阅读并接受 Minecraft EULA", Type: "boolean", Required: true, Default: false, Help: "运行 Minecraft 服务器必须接受最终用户许可协议。"},
	}
}
