# 七日杀配置覆盖与来源

本次以用户提供的游戏截图校对布局与简体中文，以当前缓存的 Steam App 294420、Build 25661908 游戏文件校对编号、默认值和允许值。截图当前选值不作为默认值。

## 页面与保存方式

| 页面 | 内容 | 保存位置 |
| --- | --- | --- |
| 基础 | 存档、游戏世界、随机地图参数、沙盒规则 | 现有世界属性与 SandboxCode |
| 服务器 | 名称、密码、地区、语言、可见性、玩家上限、EAC | serverconfig.xml |
| 高级 | 区块重置、创造模式、固定存档、限制镜头、服务器资源参数 | serverconfig.xml |
| 多人 | 玩家杀戮、土地征用、睡袋、队伍共享杀戮范围 | serverconfig.xml |

沙盒弹窗单独保留玩家、实体、世界、资源、制作、商人、任务、其他八类；固定高度、分类内搜索，中文和英文来自游戏本地化文件。编辑仅更新代码中对应规则，不把沙盒选项伪装为独立 XML 属性。应用后仍需保存配置并重启。

新增 XML 参数：MaxChunkAge=-1、BuildCreate=false、PersistentPlayerProfiles=false、CameraRestrictionMode=0、BedrollDeadZoneSize=15、BedrollExpiryTime=45、PartySharedKillRange=100。旧实例未设置这些属性时不补写，避免一次普通保存改变现有游戏行为。已保存的自定义数字即使不在菜单常用值中也保留显示。

区块重置启用前二次确认；默认关闭。截图的“区块保持时间”来自 SaveDataLimit 策略，与 MaxChunkAge 不同。本次不把存档容量限制当作无风险的清理选项开放。客户端“服务器已启用/离线”由面板的启动/停止控制，不添加同名虚构参数。跨平台联机仍需独立核对专服与平台约束，当前不增加未经验证的开关。

## 规则代码验证

- 当前规则代码版本：A。随后每三个大写字母为一个覆盖项。
- 前两个字母按 26 进制表示选项编号，第三个字母表示允许值的索引。
- A 无覆盖项，代表游戏内各项原始默认值；服务器模板的 AAAJABJACJADJARFBNC 是另一组覆盖，不能静默替换成 A。
- 只修改目标覆盖项，保留其他项与未知编号。恢复某项原始默认值时移除该项覆盖。
- 错误格式、重复编号和其他版本禁止可视化改写；原代码输入入口保留。
- 单一父项依赖使用原生禁用公式且保留子项值。BloodMoonWarning 的多个父项覆盖顺序尚未验证，暂保留可编辑，不添加自创合并规则。
- 165 项共 1,258 个允许值，单元测试逐项遍历读写；使用游戏模板代码作为已知向量。尚未用游戏客户端逐个应用全部组合。

## 可复核来源

- Data/Config/XUi_Menu/windows.xml：四个外层分类和常规参数控件。
- Data/Config/Localization.csv：字段、选值、分类的官方中英文。
- serverconfig.xml：XML 参数及默认值（所选字段 fixture 位于 API provider 的 testdata）。
- 7DaysToDieServer_Data/Managed/Assembly-CSharp.dll：SandboxOptionManager.SetupOptions、SandboxOptions 枚举、BaseSandboxOption 文本键、SandboxOptionPreset.saveOptionsToCode、LoadOptionsFromCode。
- sandbox_overrides.xml 的注释清单仅作参考，不能证明完整数量或类型。

程序集 SHA-256：`e16ff9d539b8fbf64904c0bc090d502286a62464742a9396f0455936ad0beb87`。

## 重新生成

使用自己安装的同一游戏构建，不提交游戏二进制或完整本地化文件。提取脚本仅读取程序集，不执行它；程序集校验和不匹配时拒绝生成，新版本需重新审计。

```sh
python3 -m venv /tmp/seven-days-extract
/tmp/seven-days-extract/bin/pip install dnfile dncil
/tmp/seven-days-extract/bin/python scripts/extract-seven-days-sandbox.py \
  --assembly "$GAME_FILES/7DaysToDieServer_Data/Managed/Assembly-CSharp.dll" \
  --localization "$GAME_FILES/Data/Config/Localization.csv" \
  --output /tmp/seven-days-sandbox-data.json
cmp /tmp/seven-days-sandbox-data.json apps/web/lib/seven-days-sandbox-data.json
```

输出应逐字一致。当前生成文件也已经实际复现并通过 cmp。

## 完整规则清单

默认值为不带覆盖项的 A 代码对应值；百分比、开关、枚举按游戏控件显示。具体允许值见生成的 JSON 元数据。

| 类别 | 编号 | 参数标识 | 游戏中文 | 原始默认值 |
| --- | ---: | --- | --- | --- |
| 玩家 | 0 | RangedDamage | 远程伤害 | 100% |
| 玩家 | 1 | MeleeDamage | 近战伤害 | 100% |
| 玩家 | 2 | BlockDamage | 物块伤害 | 100% |
| 玩家 | 3 | TerrainDamage | 地形伤害 | 100% |
| 玩家 | 4 | HeadshotMultiplier | 爆头倍数 | 100% |
| 玩家 | 17 | IncomingDamage | 玩家所受伤害 | 100% |
| 玩家 | 6 | WalkSpeed | 行走速度 | 100% |
| 玩家 | 7 | RunSpeed | 奔跑速度 | 100% |
| 玩家 | 5 | CrouchSpeed | 蹲伏速度 | 100% |
| 玩家 | 112 | CrouchRunSpeed | 蹲伏奔跑速度 | 100% |
| 玩家 | 8 | JumpStrength | 跳跃力量 | 100% |
| 玩家 | 10 | StaminaRegen | 体力值回复 | 100% |
| 玩家 | 9 | StaminaUsage | 体力值消耗 | 100% |
| 玩家 | 18 | XPMultiplier | 经验值倍数 | 100% |
| 玩家 | 19 | ShowXP | 显示经验值 | 所有 |
| 玩家 | 11 | PlayerLevelBonusApplied | 等级属性加成 | 是 |
| 玩家 | 116 | SkillGainRate | 技能获得速率 | 1 |
| 玩家 | 117 | SkillPointsPerLevel | 技能获得数量 | 1 |
| 玩家 | 15 | NewbieCoat | 新玩家增益 | 是 |
| 玩家 | 26 | DeathPenalty | 死亡惩罚 | 仅经验值 |
| 玩家 | 22 | LoseItemsOnDeathType | 死亡时丢失 | 无 |
| 玩家 | 23 | LoseItemsOnDeathCount | 死亡丢失数量 | 1-3 |
| 玩家 | 24 | DegradeItemsOnDeath | 死亡损耗 | 无 |
| 玩家 | 25 | DegradeAmountOnDeath | 死亡损耗量 | 10% |
| 玩家 | 27 | DropOnDeath | 死亡丢失物品 | 所有 |
| 玩家 | 28 | DropOnQuit | 退出丢失物品 | 无 |
| 玩家 | 161 | HungerMultiplier | 饥饿度倍数 | 100% |
| 玩家 | 162 | ThirstMultiplier | 口渴度倍数 | 100% |
| 玩家 | 160 | InfectionChance | 感染几率 | 100% |
| 玩家 | 29 | InfectionRate | 感染速率 | 100% |
| 玩家 | 163 | StackSizeMultiplier | 堆叠上限倍率 | 100% |
| 玩家 | 20 | EncumbranceModifier | 负重栏位 | 默认 |
| 玩家 | 12 | JarRefund | 罐子退款 | 60% |
| 实体 | 30 | EnemySpawnMode | 敌人生成 | 是 |
| 实体 | 43 | MaxEnemyTier | 最大敌人等级 | 精英 |
| 实体 | 46 | BiomeDayEnemyDensity | 白天敌人密度 | 默认 |
| 实体 | 44 | BiomeDayZombieRespawn | 白天敌人重新生成 | 默认 |
| 实体 | 155 | BiomeDayAnimalDensity | 白天动物密度 | 默认 |
| 实体 | 45 | BiomeDayAnimalRespawn | 白天动物重新生成 | 默认 |
| 实体 | 158 | BiomeNightEnemyDensity | 夜间敌人密度 | 默认 |
| 实体 | 156 | BiomeNightZombieRespawn | 夜间敌人重新生成 | 默认 |
| 实体 | 159 | BiomeNightAnimalDensity | 夜间动物密度 | 默认 |
| 实体 | 157 | BiomeNightAnimalRespawn | 夜间动物重新生成 | 默认 |
| 实体 | 31 | EntityDamage | 敌人造成的伤害 | 100% |
| 实体 | 42 | EntityIncomingDamage | 实体所受伤害 | 100% |
| 实体 | 32 | BlockDamageAI | 敌人物块伤害 | 100% |
| 实体 | 33 | BlockDamageAIBM | 血月物块伤害 | 100% |
| 实体 | 16 | HeadshotMode | 爆头模式 | 无 |
| 实体 | 13 | ShowHealthBars | 实体生命值条 | 否 |
| 实体 | 14 | ShowEnemyDamage | 显示实体伤害 | 否 |
| 实体 | 34 | ZombieMove | 丧尸白天速度 | 行走 |
| 实体 | 35 | ZombieMoveNight | 丧尸夜间速度 | 冲刺 |
| 实体 | 36 | ZombieFeralMove | 凶残丧尸速度 | 冲刺 |
| 实体 | 37 | ZombieBMMove | 丧尸血月速度 | 冲刺 |
| 实体 | 38 | ZombieFeralSense | 凶残丧尸感知 | 关闭 |
| 实体 | 39 | AISmellMode | AI 嗅觉模式 | 奔跑 |
| 实体 | 41 | ZombieRageChance | 丧尸狂暴 | 15% |
| 实体 | 40 | AllowZombieDigging | 丧尸挖掘 | 是 |
| 实体 | 47 | ZombiesEatAnimals | 丧尸吃动物 | 是 |
| 世界 | 60 | GlobalGSModifier | 全局游戏阶段 | 默认 |
| 世界 | 61 | BiomeGSModifier | 环境游戏阶段 | 默认 |
| 世界 | 55 | BiomeProgression | 生态环境进度 | 是 |
| 世界 | 56 | TemperatureSurvival | 温度生存 | 是 |
| 世界 | 74 | MaxTechType | 最大科技类型 | 科技 3 |
| 世界 | 73 | WorkstationsInTheWild | 野外工作站 | 无 |
| 世界 | 48 | BloodMoonFrequency | 血月频率 | 7 日 |
| 世界 | 49 | BloodMoonRange | 血月范围 | 0 日 |
| 世界 | 51 | BloodMoonEnemyCount | 血月计数 | 8 敌人 |
| 世界 | 50 | BloodMoonWarning | 血月警告 | 早晨 |
| 世界 | 52 | AirDropFrequency | 空投 | 3 日 |
| 世界 | 54 | AirDropRandomTime | 空投变化 | 无 |
| 世界 | 57 | StormFreq | 风暴频率 | 100% |
| 世界 | 58 | StormWarning | 风暴警告 | 是 |
| 世界 | 59 | HeatMapSensitivity | 热力图敏感度 | 默认 |
| 世界 | 66 | DayNightLength | 24 小时周期 | 60 |
| 世界 | 67 | DayLightLength | 白昼时长 | 18 |
| 世界 | 53 | AirDropMarker | 标记空投 | 是 |
| 世界 | 68 | AllowMap | 允许地图 | 是 |
| 世界 | 69 | AllowCompass | 允许指南针 | 是 |
| 世界 | 70 | AllowScreenMarkers | 允许屏幕标记 | 是 |
| 世界 | 71 | ShowLocationInfo | 显示地点信息 | 是 |
| 世界 | 72 | ShowDayTime | 显示天数/时间 | 是 |
| 资源 | 77 | LootMaxTier | 最大战利品品质 | 默认 |
| 资源 | 62 | GlobalLSModifier | 全局战利品等级 | 默认 |
| 资源 | 63 | BiomeLSModifier | 环境战利品等级 | 默认 |
| 资源 | 64 | POITierLSModifier | 目标点战利品等级 | 默认 |
| 资源 | 75 | LootRespawnDays | 战利品刷新天数 | 7 |
| 资源 | 76 | LootTimer | 开箱时间 | 100% |
| 资源 | 90 | LootBagChance | 战利品袋掉落 | 100% |
| 资源 | 78 | GlobalLootCount | 掉落率 | 100% |
| 资源 | 79 | FoodLootCount | 食物丰富度 | 100% |
| 资源 | 80 | DrinkLootCount | 饮料丰富度 | 100% |
| 资源 | 81 | MedicalLootCount | 药品丰富度 | 100% |
| 资源 | 82 | AmmoLootCount | 弹药丰富度 | 100% |
| 资源 | 83 | ResourceLootCount | 资源丰富度 | 100% |
| 资源 | 84 | ArmorLootCount | 护甲丰富度 | 100% |
| 资源 | 85 | MeleeLootCount | 近战武器丰富度 | 100% |
| 资源 | 86 | RangedLootCount | 远程武器丰富度 | 100% |
| 资源 | 87 | DukesLootCount | 货币丰富度 | 100% |
| 资源 | 88 | CraftingMagazinesLootCount | 杂志丰富度 | 100% |
| 资源 | 111 | BookLootCount | 书籍丰富度 | 100% |
| 资源 | 89 | TreasureMapChance | 藏宝图生成概率 | 100% |
| 资源 | 101 | MiningOutput | 采矿产出 | 100% |
| 资源 | 91 | CropOutput | 农作物产出 | 100% |
| 资源 | 92 | SeedDropOutput | 种子掉落 | 100% |
| 资源 | 102 | HarvestingOutput | 资源产出 | 100% |
| 资源 | 93 | CropGrowthSpeed | 农作物生长 | 100% |
| 制作 | 96 | CraftingProgression | 制作进度 | 是 |
| 制作 | 100 | CraftingMaxTier | 制作最高品质 | 默认 |
| 制作 | 115 | PointsPerMagazine | 杂志进度 | 1 |
| 制作 | 94 | BackpackCrafting | 背包制作 | 是 |
| 制作 | 95 | WorkstationCrafting | 工作台制作 | 是 |
| 制作 | 104 | SmeltingType | 熔炉类型 | 熔炉 |
| 制作 | 97 | CraftingTime | 制作计时器 | 100% |
| 制作 | 98 | CraftingInput | 制作消耗 | 100% |
| 制作 | 99 | CraftingOutput | 制作产出 | 100% |
| 制作 | 103 | ScrappingOutput | 分解产出 | 100% |
| 制作 | 105 | DewCollectorTime | 露水收集器时间 | 100% |
| 制作 | 106 | DewCollectorOutput | 露水收集器产出 | 100% |
| 制作 | 107 | DewCollectorInput | 露水收集器原料 | 100% |
| 制作 | 108 | ApiaryTime | 蜂箱时长 | 100% |
| 制作 | 109 | ApiaryOutput | 蜂箱产出 | 100% |
| 制作 | 110 | ApiaryInput | 蜂箱原料 | 100% |
| 制作 | 152 | ChickenCoopTime | 鸡舍生产时间 | 100% |
| 制作 | 153 | ChickenCoopOutput | 鸡舍产量 | 100% |
| 制作 | 154 | ChickenCoopInput | 鸡舍投入量 | 100% |
| 制作 | 164 | FullChickenStressEvent | 鸡只压力事件 | 随机 |
| 制作 | 21 | ItemDegradation | 物品损坏 | 默认 |
| 制作 | 113 | RepairTypes | 修理方式 | 均可 |
| 制作 | 114 | MaxDegradationAmount | 修理损坏 | 无 |
| 商人 | 128 | TradersEnabled | 交易开启 | 是 |
| 商人 | 129 | VendingEnabled | 贩卖机开启 | 是 |
| 商人 | 127 | TraderHours | 商人营业时间 | 默认 |
| 商人 | 132 | TraderProtection | 商人保护 | 是 |
| 商人 | 126 | TraderDialog | 交易对话 | 是 |
| 商人 | 65 | GlobalTSModifier | 全局商人阶段 | 默认 |
| 商人 | 136 | TraderMaxTier | 商人最高品质 | 默认 |
| 商人 | 134 | TraderItemAbundance | 商人物品数量 | 默认 |
| 商人 | 138 | VendingItemAbundance | 贩卖机物品数量 | 默认 |
| 商人 | 133 | TraderResetInterval | 商人重置 | 默认 |
| 商人 | 137 | VendingResetInterval | 贩卖机重置 | 默认 |
| 商人 | 130 | TraderSellPrices | 出售价格 | 100% |
| 商人 | 131 | TraderBuyPrices | 购买价格 | 100% |
| 商人 | 135 | TraderBuyLimit | 商人收购上限 | 3 |
| 任务 | 139 | ChallengesEnabled | 挑战 | 是 |
| 任务 | 118 | QuestsEnabled | 任务 | 是 |
| 任务 | 140 | IntroChallengesEnabled | 入门挑战 | 是 |
| 任务 | 119 | IntroQuestEnabled | 入门任务 | 是 |
| 任务 | 120 | TraderToTraderQuestsEnabled | 商贸路线 | 是 |
| 任务 | 124 | BuriedQuestsEnabled | 埋藏任务 | 是 |
| 任务 | 125 | POIQuestsEnabled | 目标点任务 | 是 |
| 任务 | 122 | QuestsPerTier | 每级任务 | 10 |
| 任务 | 123 | QuestProgressionDailyLimit | 每日任务数 | 4 |
| 任务 | 121 | StarterSkillPoints | 基础技能点 | 4 |
| 其他 | 141 | VehicleFuelUsage | 载具燃料使用量 | 100% |
| 其他 | 142 | VehicleEntityDamage | 载具实体伤害 | 100% |
| 其他 | 143 | VehicleBlockDamage | 载具物块伤害 | 100% |
| 其他 | 144 | VehicleSelfDamage | 载具自身伤害 | 100% |
| 其他 | 145 | ElectricalOutput | 电力输出 | 100% |
| 其他 | 146 | SillyCelebrate | 庆祝击杀 | 否 |
| 其他 | 147 | SillyBigHeads | 大头 | 否 |
| 其他 | 148 | SillyTinyZombies | 迷你丧尸 | 否 |
| 其他 | 150 | SillyLowGravity | 重力 | 100% |
| 其他 | 149 | SillySounds | 滑稽声音 | 否 |
| 其他 | 151 | SillyBlackandWhite | 黑白 | 否 |
