package palworld

import "github.com/smartcat999/game-panel-lite/apps/api/internal/domain"

func configSchema() []domain.ProviderConfigField {
	return []domain.ProviderConfigField{
		{Name: "serverName", Label: "服务器名称", Type: "text", Required: true, Default: "Palworld Server"},
		{Name: "saveName", Label: "存档名称", Type: "text", Required: true, Default: "Palworld Save"},
		{Name: "maxPlayers", Label: "最大玩家数", Type: "number", Required: true, Default: 8},
		{Name: "serverPassword", Label: "服务器密码", Type: "password", Required: false},
		{Name: "adminPassword", Label: "管理员密码", Type: "password", Required: true, Help: "用于 Palworld 管理员操作。"},
		{Name: "eggHatchingTime", Label: "巨大蛋孵化时间（小时）", Type: "number", Required: true, Default: 72, Help: "设为 0 可立即孵化。"},
		{Name: "expRate", Label: "经验倍率", Type: "number", Required: true, Default: 1},
		{Name: "captureRate", Label: "捕获倍率", Type: "number", Required: true, Default: 1},
		{Name: "palSpawnRate", Label: "帕鲁刷新倍率", Type: "number", Required: true, Default: 1, Help: "提高此项会增加服务器负载。"},
		{Name: "enemyDropRate", Label: "敌人掉落倍率", Type: "number", Required: true, Default: 1},
		{Name: "collectionDropRate", Label: "采集掉落倍率", Type: "number", Required: true, Default: 1},
		{Name: "dayTimeSpeedRate", Label: "白天速度", Type: "number", Required: true, Default: 1},
		{Name: "nightTimeSpeedRate", Label: "夜晚速度", Type: "number", Required: true, Default: 1},
		{Name: "baseCampMaxNum", Label: "服务器据点总数", Type: "number", Required: true, Default: 128},
		{Name: "baseCampMaxNumInGuild", Label: "每公会据点上限", Type: "number", Required: true, Default: 4, Help: "官方上限为 10。"},
		{Name: "baseCampWorkerMaxNum", Label: "每据点工作帕鲁上限", Type: "number", Required: true, Default: 15, Help: "官方上限为 50。"},
		{Name: "guildPlayerMaxNum", Label: "公会人数上限", Type: "number", Required: true, Default: 20},
		{Name: "buildingDeteriorationRate", Label: "建筑自然劣化倍率", Type: "number", Required: true, Default: 1},
		{Name: "deathPenalty", Label: "死亡惩罚", Type: "select", Required: true, Default: "All", Options: []domain.ProviderConfigFieldOption{{Value: "None", Label: "不掉落"}, {Value: "Item", Label: "掉落物品"}, {Value: "ItemAndEquipment", Label: "掉落物品和装备"}, {Value: "All", Label: "掉落全部及队伍帕鲁"}}},
		{Name: "enableInvaderEnemy", Label: "开启入侵事件", Type: "boolean", Default: true},
		{Name: "enableFastTravel", Label: "开启快速传送", Type: "boolean", Default: true},
		{Name: "pvp", Label: "开启 PVP", Type: "boolean", Default: false},
	}
}
