package palworld

import "github.com/smartcat999/game-panel-lite/apps/api/internal/domain"

func configSchema() []domain.ProviderConfigField {
	return []domain.ProviderConfigField{
		{Name: "serverName", Label: "服务器名称", Type: "text", Required: true, Default: "Palworld Server"},
		{Name: "saveName", Label: "存档名称", Type: "text", Required: true, Default: "Palworld Save"},
		{Name: "maxPlayers", Label: "最大玩家数", Type: "number", Required: true, Default: 8},
		{Name: "serverPassword", Label: "服务器密码", Type: "password", Required: false},
		{Name: "adminPassword", Label: "管理员密码", Type: "password", Required: true, Help: "用于 Palworld 管理员操作。"},
	}
}
