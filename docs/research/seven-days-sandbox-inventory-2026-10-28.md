# Seven Days 沙盒选项原始名称与标识清单（2026-10-28）

> 更新：文末 DLL 补充已验证名称绑定与 165 项完整分类；前文仅 CSV/XML 阶段的未知结论以补充章节为准。

## 结论与范围

用户指定“任务”10 项、“其他”11 项中文，在原始 Localization.csv 中全部逐字匹配，英文和本地化键已核实；相应候选标识全部存在于 sandbox_overrides.xml 注释清单。但字符串分别存在不等于已证明运行时绑定：只有 QuestProgressionDailyLimit 具有明确的浏览控件 name/title 证据，其余 20 项的同名对应仍为候选关系，不宣称是服务器配置属性。

8 个分类名称已完整核实。完整八类成员、排序和可见数量未核实。沙盒页面动态生成，旧注释清单不能当成完整当前分类表。任务／其他分组依据用户本次提供的截图字段列表；本子任务未收到截图像素，未独立检查截图布局、值或版本标记。所有下表英文、中文保留游戏文本原样，包括不自然的译文。

## 来源与构建

通过 ssh um773 读取已缓存镜像 smartcat99999/seven-days-to-die:sha-cd0510d3；仅使用 docker run --rm --pull never --network none --read-only --entrypoint python3，不启动游戏、不挂载主机目录、不拉取镜像或访问业务服务。

镜像内 steamapps/appmanifest_294420.acf 声明 App ID 294420、7 Days to Die Dedicated Server、buildid 和 TargetBuildID 均为 25661908，depot 294422 的 manifest 为 3031329678505041777。没有独立核验 Steam depot 完整性。v3.3 来自用户上下文，本次文件没有提供足以独立确认的语义版本字符串。镜像 tag 已记录，但未取得不可变 image ID / registry digest；tag 不等于内容摘要。

引用约定：L = Localization.csv，O = sandbox_overrides.xml，W = XUi_Menu/windows.xml，M = Steam 清单。L 按 Key 精确定位并附 CSV 记录结束行号；XML 使用物理行号。行号仅适用于以下 SHA-256 对应内容。

| 引用 | 原始路径 | 字节数 | SHA-256 |
| --- | --- | --- | --- |
| L | /opt/seven-days/Data/Config/Localization.csv | 16840111 | 74fe44e9ced90cd04ea6c821d535fd4ec02240307954d0b4b2884b72988b0a1f |
| O | /opt/seven-days/Data/Config/sandbox_overrides.xml | 5212 | fa3f11fe295f9c2ac30c683be91b31d5a288f2c5c6bbb0cd072640dc563bbb94 |
| W | /opt/seven-days/Data/Config/XUi_Menu/windows.xml | 232970 | f6d7612c5618be7fd7754e581b050f77062e240ebc2847e302467f0ada8c0de7 |
| M | /opt/seven-days/steamapps/appmanifest_294420.acf | 742 | 06477a9405a4807a0819a43a2ed6a72fcf9deb699ee748bcfdeec78b1600b571 |

本地已有 /private/tmp/gpl-seven-menu-windows.xml 和 /private/tmp/gpl-seven-sandbox-overrides.xml 与镜像对应文件逐字节摘要一致。CSV 用 csv.DictReader、UTF-8 BOM 兼容解码及正确换行处理读取；go* / sandboxOption* 范围无重复键。XML 正常解析。没有使用应用现有翻译或第三方攻略。

## 八类名称

仅证明分类键与中英文存在；不证明成员。General 的英文是 Player，Misc 的英文是 Misc。

| 本地化键 | English | 简体中文 | 证据 |
| --- | --- | --- | --- |
| sandboxOptionCategoryGeneral | Player | 玩家 | L:21159 |
| sandboxOptionCategoryEntities | Entities | 实体 | L:21158 |
| sandboxOptionCategoryWorld | World | 世界 | L:21164 |
| sandboxOptionCategoryResources | Resources | 资源 | L:21161 |
| sandboxOptionCategoryCrafting | Crafting | 制作 | L:21157 |
| sandboxOptionCategoryTraders | Traders | 商人 | L:21163 |
| sandboxOptionCategoryTasks | Tasks | 任务 | L:21162 |
| sandboxOptionCategoryMisc | Misc | 其他 | L:21160 |

## 任务：全部 10 项

标识自身已验证在 O 注释中存在。“候选”指绑定关系待核实，不表示中英文是猜测。

| English 原文 | 简体中文原文 | 本地化键 | O 注释标识 | 证据／绑定状态 |
| --- | --- | --- | --- | --- |
| Challenges | 挑战 | goChallengesEnabled | ChallengesEnabled | L:20619；O:165；同名候选，未见显式绑定 |
| Intro Challenges | 入门挑战 | goIntroChallengesEnabled | IntroChallengesEnabled | L:20777；O:166；同名候选，未见显式绑定 |
| Trade Routes | 商贸路线 | goTraderToTraderQuestsEnabled | TraderToTraderQuestsEnabled | L:21074；O:146；同名候选，未见显式绑定 |
| POI Quests | 目标点任务 | goPOIQuestsEnabled | POIQuestsEnabled | L:20916；O:151；同名候选，未见显式绑定 |
| Quests Per Day | 每日任务数 | goQuestProgressionDailyLimit | QuestProgressionDailyLimit | L:20922；O:149；W:838 明确 name/title（浏览页） |
| Quests | 任务 | goQuestsEnabled | QuestsEnabled | L:20924；O:144；同名候选，未见显式绑定 |
| Intro Quest | 入门任务 | goIntroQuestEnabled | IntroQuestEnabled | L:20779；O:145；同名候选，未见显式绑定 |
| Buried Quests | 埋藏任务 | goBuriedQuestsEnabled | BuriedQuestsEnabled | L:20616；O:150；同名候选，未见显式绑定 |
| Quests Per Tier | 每级任务 | goQuestsPerTier | QuestsPerTier | L:20926；O:148；同名候选，未见显式绑定 |
| Base Skill Points | 基础技能点 | goStarterSkillPoints | StarterSkillPoints | L:21032；O:147；同名候选，未见显式绑定 |

相应官方中文说明如下；描述键存在不证明默认值、允许值或有效配置属性。

| 说明键 | schinese 原文 | 证据 |
| --- | --- | --- |
| goChallengesEnabledDesc | 开启游戏内挑战系统。 | L:20620 |
| goIntroChallengesEnabledDesc | 开启新手引导挑战任务。 | L:20778 |
| goTraderToTraderQuestsEnabledDesc | 允许接受去找其他商人的任务。 | L:21075 |
| goPOIQuestsEnabledDesc | 是否在商人任务列表刷新目标点任务。如果关闭，则仅刷新埋藏任务。 | L:20917 |
| goQuestProgressionDailyLimitDesc | 计入商人进度的每日任务数量。额外的任务仍会给予经验值和奖励，但不会推进商人等级进度。 | L:20923 |
| goQuestsEnabledDesc | 启用或禁用游戏内任务。 | L:20925 |
| goIntroQuestEnabledDesc | 跳过开头的埋藏补给任务，直接解锁完整任务流程。 | L:20780 |
| goBuriedQuestsEnabledDesc | 是否在商人任务列表刷新埋藏任务。如果关闭，则仅刷新目标点任务。 | L:20617 |
| goQuestsPerTierDesc | 解锁下一等级所需完成的任务数量。数值越高，升级的进度越慢。无 = 无限制，可直接访问全部任务等级。 | L:20927 |
| goStarterSkillPointsDesc | 完成新手引导任务可获得的技能点数。如果关闭引导，则开局直接发放对应点数。 | L:21033 |

## 其他：全部 11 项

标识自身已验证在 O 注释中存在。“候选”指绑定关系待核实，不表示中英文是猜测。

| English 原文 | 简体中文原文 | 本地化键 | O 注释标识 | 证据／绑定状态 |
| --- | --- | --- | --- | --- |
| Vehicle Fuel Usage | 载具燃料使用量 | goVehicleFuelUsage | VehicleFuelUsage | L:21083；O:167；同名候选，未见显式绑定 |
| Vehicle Block Damage | 载具物块伤害 | goVehicleBlockDamage | VehicleBlockDamage | L:21079；O:169；同名候选，未见显式绑定 |
| Electrical Output | 电力输出 | goElectricalOutput | ElectricalOutput | L:20708；O:171；同名候选，未见显式绑定 |
| Celebrate Kills | 庆祝击杀 | goSillyCelebrate | SillyCelebrate | L:21009；O:172；同名候选，未见显式绑定 |
| Tiny Zombies | 迷你丧尸 | goSillyTinyZombies | SillyTinyZombies | L:21015；O:174；同名候选，未见显式绑定 |
| Silly Sounds | 滑稽声音 | goSillySounds | SillySounds | L:21013；O:175；同名候选，未见显式绑定 |
| Vehicle Entity Damage | 载具实体伤害 | goVehicleEntityDamage | VehicleEntityDamage | L:21081；O:168；同名候选，未见显式绑定 |
| Vehicle Self Damage | 载具自身伤害 | goVehicleSelfDamage | VehicleSelfDamage | L:21085；O:170；同名候选，未见显式绑定 |
| Big Heads | 大头 | goSillyBigHeads | SillyBigHeads | L:21005；O:173；同名候选，未见显式绑定 |
| Gravity | 重力 | goSillyLowGravity | SillyLowGravity | L:21011；O:176；同名候选，未见显式绑定 |
| Black and White | 黑白 | goSillyBlackandWhite | SillyBlackandWhite | L:21007；O:177；同名候选，未见显式绑定 |

相应官方中文说明如下；描述键存在不证明默认值、允许值或有效配置属性。

| 说明键 | schinese 原文 | 证据 |
| --- | --- | --- |
| goVehicleFuelUsageDesc | 调整载具消耗燃料的速率。 | L:21084 |
| goVehicleBlockDamageDesc | 载具对物块造成的伤害量。 | L:21080 |
| goElectricalOutputDesc | 控制发电量。 | L:20709 |
| goSillyCelebrateDesc | 开启击杀实体时的庆祝五彩纸屑特效。“仅限爆头”选项仅会在爆头击杀时触发五彩纸屑特效，因此可以射击动物的身体以收集肉类。“仅限爆头”选项不适用于非敌对动物。 | L:21010 |
| goSillyTinyZombiesDesc | 启用缩小版的丧尸。 | L:21016 |
| goSillySoundsDesc | 为游戏内的动作开启滑稽音效。 | L:21014 |
| goVehicleEntityDamageDesc | 载具对实体造成的伤害量。 | L:21082 |
| goVehicleSelfDamageDesc | 你的载具发生碰撞时自身受到的伤害量。 | L:21086 |
| goSillyBigHeadsDesc | 开启丧尸、动物和商人的“大头模式”。 | L:21006 |
| goSillyLowGravityDesc | 控制游戏内重力的大小。 | L:21012 |
| goSillyBlackandWhiteDesc | 启用经典黑白色调（灵感源自 20 世纪 60 年代的恐怖电影）。 | L:21008 |

## XML 明确关系与限制

- W:2696 使用 SandboxOptions 控制器。W:2745–2746 用重复网格及 `<sandbox_tabbutton name="${repeat_i}" />` 生成分类；W:2752–2758 动态生成容器及 `<sandbox_option />`。repeat_count="10" 是模板容量，不能推导实际有 10 类或具体成员。
- W:838 明确 `name="QuestProgressionDailyLimit" title="goQuestProgressionDailyLimit"`，但位于 W:818 的 xuiGameSettingsAdvanced 浏览页，不能当成沙盒分类归属证据。
- W:822 明确 `name="BlockDamagePlayer" sandboxoption="BlockDamage" title="goBlockDamagePlayer"`；W:825 明确 `name="LootAbundance" sandboxoption="GlobalLootCount" title="goLootAbundance"`。不能机械拼出不存在的 goBlockDamage、goGlobalLootCount。
- W:811 明确 `name="AirDropMarker" title="goMarkAirDrops"`；不存在 goAirDropMarker。该证据限于这个浏览控件。
- O:22 说明 sandbox_override 将命名选项设为 Default 并禁止修改；O:3–7 仅有已注释示例。O:28–177 的 150 个选项名全部是注释，不能保证当前仍有效或可作为 serverconfig.xml 属性。
- O:144–151 列出八个任务相关标识，ChallengesEnabled 与 IntroChallengesEnabled 却在 O:165–166；注释顺序不能直接当成截图分类。

## 注释清单全部 150 项

严格保留 O 顺序，不伪造分类。147 项存在同名 go + 标识键；另外 3 项使用 XML 明确的浏览标题关系。除注明显式关系外，只证明两边字符串分别存在，未证明运行时绑定。

| O 注释标识 | 本地化键 | English | 简体中文 | 证据／关系状态 |
| --- | --- | --- | --- | --- |
| RangedDamage | goRangedDamage | Ranged Damage | 远程伤害 | O:28；L:20930；同名候选，绑定待验证 |
| MeleeDamage | goMeleeDamage | Melee Damage | 近战伤害 | O:29；L:20862；同名候选，绑定待验证 |
| BlockDamage | goBlockDamagePlayer | Block Damage | 物块伤害 | O:30；L:20593；W:822 显式 sandboxoption |
| TerrainDamage | goTerrainDamage | Terrain Damage | 地形伤害 | O:31；L:21049；同名候选，绑定待验证 |
| HeadshotMultiplier | goHeadshotMultiplier | Headshot Multiplier | 爆头倍数 | O:32；L:20761；同名候选，绑定待验证 |
| CrouchSpeed | goCrouchSpeed | Crouch Speed | 蹲伏速度 | O:33；L:20647；同名候选，绑定待验证 |
| WalkSpeed | goWalkSpeed | Walk Speed | 行走速度 | O:34；L:21099；同名候选，绑定待验证 |
| RunSpeed | goRunSpeed | Run Speed | 奔跑速度 | O:35；L:20966；同名候选，绑定待验证 |
| JumpStrength | goJumpStrength | Jump Strength | 跳跃力量 | O:36；L:20787；同名候选，绑定待验证 |
| StaminaUsage | goStaminaUsage | Stamina Usage | 体力值消耗 | O:37；L:21030；同名候选，绑定待验证 |
| StaminaRegen | goStaminaRegen | Stamina Regen | 体力值回复 | O:38；L:21028；同名候选，绑定待验证 |
| PlayerLevelBonusApplied | goPlayerLevelBonusApplied | Level Stat Bonus | 等级属性加成 | O:39；L:20910；同名候选，绑定待验证 |
| JarRefund | goJarRefund | Jar Refund | 罐子退款 | O:40；L:20785；同名候选，绑定待验证 |
| ShowHealthBars | goShowHealthBars | Entity Health Bars | 实体生命值条 | O:41；L:20999；同名候选，绑定待验证 |
| ShowEnemyDamage | goShowEnemyDamage | Show Entity Damage | 显示实体伤害 | O:42；L:20995；同名候选，绑定待验证 |
| NewbieCoat | goNewbieCoat | New Player Buff | 新玩家增益 | O:43；L:20877；同名候选，绑定待验证 |
| HeadshotMode | goHeadshotMode | Headshot Mode | 爆头模式 | O:44；L:20759；同名候选，绑定待验证 |
| IncomingDamage | goIncomingDamage | Player Damage In | 玩家所受伤害 | O:45；L:20769；同名候选，绑定待验证 |
| XPMultiplier | goXPMultiplier | XP Multiplier | 经验值倍数 | O:46；L:21106；同名候选，绑定待验证 |
| ShowXP | goShowXP | Show XP | 显示经验值 | O:47；L:21003；同名候选，绑定待验证 |
| EncumbranceModifier | goEncumbranceModifier | Encumbrance Slots | 负重栏位 | O:48；L:20711；同名候选，绑定待验证 |
| ItemDegradation | goItemDegradation | Item Degradation | 物品损坏 | O:49；L:20783；同名候选，绑定待验证 |
| LoseItemsOnDeathType | goLoseItemsOnDeathType | Lose On Death | 死亡时丢失 | O:50；L:20838；同名候选，绑定待验证 |
| LoseItemsOnDeathCount | goLoseItemsOnDeathCount | Death Loss Count | 死亡丢失数量 | O:51；L:20836；同名候选，绑定待验证 |
| DegradeItemsOnDeath | goDegradeItemsOnDeath | Death Degradation | 死亡损耗 | O:52；L:20668；同名候选，绑定待验证 |
| DegradeAmountOnDeath | goDegradeAmountOnDeath | Death Degradation Amount | 死亡损耗量 | O:53；L:20666；同名候选，绑定待验证 |
| DeathPenalty | goDeathPenalty | Death Penalty | 死亡惩罚 | O:54；L:20663；同名候选，绑定待验证 |
| DropOnDeath | goDropOnDeath | Drop On Death | 死亡丢失物品 | O:55；L:20691；同名候选，绑定待验证 |
| DropOnQuit | goDropOnQuit | Drop On Quit | 退出丢失物品 | O:56；L:20693；同名候选，绑定待验证 |
| InfectionRate | goInfectionRate | Infection Rate | 感染速率 | O:57；L:20773；同名候选，绑定待验证 |
| EnemySpawnMode | goEnemySpawnMode | Enemy Spawning | 敌人生成 | O:58；L:20718；同名候选，绑定待验证 |
| EntityDamage | goEntityDamage | Enemy Damage Dealt | 敌人造成的伤害 | O:59；L:20720；同名候选，绑定待验证 |
| BlockDamageAI | goBlockDamageAI | Enemy Block Damage | 敌人物块伤害 | O:60；L:20589；同名候选，绑定待验证 |
| BlockDamageAIBM | goBlockDamageAIBM | BM Block Damage | 血月物块伤害 | O:61；L:20590；同名候选，绑定待验证 |
| ZombieMove | goZombieMove | Zombie Day Speed | 丧尸白天速度 | O:62；L:21125；同名候选，绑定待验证 |
| ZombieMoveNight | goZombieMoveNight | Zombie Night Speed | 丧尸夜间速度 | O:63；L:21127；同名候选，绑定待验证 |
| ZombieFeralMove | goZombieFeralMove | Zombie Feral Speed | 凶残丧尸速度 | O:64；L:21120；同名候选，绑定待验证 |
| ZombieBMMove | goZombieBMMove | Zombie BM Speed | 丧尸血月速度 | O:65；L:21118；同名候选，绑定待验证 |
| ZombieFeralSense | goZombieFeralSense | Zombie Feral Sense | 凶残丧尸感知 | O:66；L:21122；同名候选，绑定待验证 |
| AISmellMode | goAISmellMode | AI Smell Mode | AI 嗅觉模式 | O:67；L:20526；同名候选，绑定待验证 |
| AllowZombieDigging | goAllowZombieDigging | Zombie Digging | 丧尸挖掘 | O:68；L:20536；同名候选，绑定待验证 |
| ZombieRageChance | goZombieRageChance | Zombie Rage | 丧尸狂暴 | O:69；L:21131；同名候选，绑定待验证 |
| EntityIncomingDamage | goEntityIncomingDamage | Entity Damage In | 实体所受伤害 | O:70；L:20722；同名候选，绑定待验证 |
| MaxEnemyTier | goMaxEnemyTier | Max Enemy Tier | 最大敌人等级 | O:71；L:20849；同名候选，绑定待验证 |
| BiomeZombieRespawn | goBiomeZombieRespawn | Enemy Respawn | 敌人重生 | O:72；L:20587；同名候选，绑定待验证 |
| BiomeAnimalRespawn | goBiomeAnimalRespawn | Biome Animal Respawn | 环境动物重生 | O:73；L:20561；同名候选，绑定待验证 |
| BiomeEnemyDensity | goBiomeEnemyDensity | Enemy Density | 敌人密度 | O:74；L:20571；同名候选，绑定待验证 |
| ZombiesEatAnimals | goZombiesEatAnimals | Zombies Eat Animals | 丧尸吃动物 | O:75；L:21133；同名候选，绑定待验证 |
| BloodMoonFrequency | goBloodMoonFrequency | Blood Moon Frequency | 血月频率 | O:76；L:20602；同名候选，绑定待验证 |
| BloodMoonRange | goBloodMoonRange | Blood Moon Range | 血月范围 | O:77；L:20604；同名候选，绑定待验证 |
| BloodMoonWarning | goBloodMoonWarning | Blood Moon Warning | 血月警告 | O:78；L:20606；同名候选，绑定待验证 |
| BloodMoonEnemyCount | goBloodMoonEnemyCount | Blood Moon Count | 血月计数 | O:79；L:20600；同名候选，绑定待验证 |
| AirDropFrequency | goAirDropFrequency | Air Drops | 空投 | O:80；L:20518；同名候选，绑定待验证 |
| AirDropMarker | goMarkAirDrops | Mark Air Drops | 标记空投 | O:81；L:20841；W:811 name/title |
| AirDropRandomTime | goAirDropRandomTime | Air Drop Variance | 空投变化 | O:82；L:20520；同名候选，绑定待验证 |
| BiomeProgression | goBiomeProgression | Biome Progression | 生态环境进度 | O:83；L:20585；同名候选，绑定待验证 |
| TemperatureSurvival | goTemperatureSurvival | Temperature Survival | 温度生存 | O:84；L:21047；同名候选，绑定待验证 |
| StormFreq | goStormFreq | Storm Frequency | 风暴频率 | O:85；L:21038；同名候选，绑定待验证 |
| StormWarning | goStormWarning | Storm Warning | 风暴警告 | O:86；L:21040；同名候选，绑定待验证 |
| HeatMapSensitivity | goHeatMapSensitivity | Heat Map Sensitivity | 热力图敏感度 | O:87；L:20764；同名候选，绑定待验证 |
| GlobalGSModifier | goGlobalGSModifier | Global Game Stage | 全局游戏阶段 | O:88；L:20750；同名候选，绑定待验证 |
| BiomeGSModifier | goBiomeGSModifier | Biome Game Stage | 环境游戏阶段 | O:89；L:20573；同名候选，绑定待验证 |
| GlobalLSModifier | goGlobalLSModifier | Global Loot Stage | 全局战利品等级 | O:90；L:20752；同名候选，绑定待验证 |
| BiomeLSModifier | goBiomeLSModifier | Biome Loot Stage | 环境战利品等级 | O:91；L:20575；同名候选，绑定待验证 |
| POITierLSModifier | goPOITierLSModifier | POI Loot Stage | 目标点战利品等级 | O:92；L:20918；同名候选，绑定待验证 |
| GlobalTSModifier | goGlobalTSModifier | Global Trader Stage | 全局商人阶段 | O:93；L:20754；同名候选，绑定待验证 |
| DayNightLength | goDayNightLength | 24 Hour Cycle | 24 小时周期 | O:94；L:20659；同名候选，绑定待验证 |
| DayLightLength | goDayLightLength | Daylight Length | 白昼时长 | O:95；L:20655；同名候选，绑定待验证 |
| AllowMap | goAllowMap | Allow Map | 允许地图 | O:96；L:20532；同名候选，绑定待验证 |
| AllowCompass | goAllowCompass | Allow Compass | 允许指南针 | O:97；L:20530；同名候选，绑定待验证 |
| AllowScreenMarkers | goAllowScreenMarkers | Allow Screen Markers | 允许屏幕标记 | O:98；L:20534；同名候选，绑定待验证 |
| ShowLocationInfo | goShowLocationInfo | Show Location Info | 显示地点信息 | O:99；L:21001；同名候选，绑定待验证 |
| ShowDayTime | goShowDayTime | Show Day/Time | 显示天数/时间 | O:100；L:20993；同名候选，绑定待验证 |
| WorkstationsInTheWild | goWorkstationsInTheWild | Workstations In The Wild | 野外工作站 | O:101；L:21104；同名候选，绑定待验证 |
| MaxTechType | goMaxTechType | Max Tech Type | 最大科技类型 | O:102；L:20857；同名候选，绑定待验证 |
| LootRespawnDays | goLootRespawnDays | Loot Respawn Days | 战利品刷新天数 | O:103；L:20830；同名候选，绑定待验证 |
| LootTimer | goLootTimer | Loot Timer | 开箱时间 | O:104；L:20834；同名候选，绑定待验证 |
| LootMaxTier | goLootMaxTier | Max Loot Quality | 最大战利品品质 | O:105；L:20828；同名候选，绑定待验证 |
| GlobalLootCount | goLootAbundance | Loot Abundance | 掉落率 | O:106；L:20822；W:825 显式 sandboxoption |
| FoodLootCount | goFoodLootCount | Food Abundance | 食物丰富度 | O:107；L:20728；同名候选，绑定待验证 |
| DrinkLootCount | goDrinkLootCount | Drink Abundance | 饮料丰富度 | O:108；L:20688；同名候选，绑定待验证 |
| MedicalLootCount | goMedicalLootCount | Medical Abundance | 药品丰富度 | O:109；L:20859；同名候选，绑定待验证 |
| AmmoLootCount | goAmmoLootCount | Ammo Abundance | 弹药丰富度 | O:110；L:20541；同名候选，绑定待验证 |
| ResourceLootCount | goResourceLootCount | Resource Abundance | 资源丰富度 | O:111；L:20964；同名候选，绑定待验证 |
| ArmorLootCount | goArmorLootCount | Armor Abundance | 护甲丰富度 | O:112；L:20551；同名候选，绑定待验证 |
| MeleeLootCount | goMeleeLootCount | Melee Abundance | 近战武器丰富度 | O:113；L:20864；同名候选，绑定待验证 |
| RangedLootCount | goRangedLootCount | Ranged Abundance | 远程武器丰富度 | O:114；L:20932；同名候选，绑定待验证 |
| DukesLootCount | goDukesLootCount | Currency Abundance | 货币丰富度 | O:115；L:20695；同名候选，绑定待验证 |
| CraftingMagazinesLootCount | goCraftingMagazinesLootCount | Magazine Abundance | 杂志丰富度 | O:116；L:20631；同名候选，绑定待验证 |
| TreasureMapChance | goTreasureMapChance | Treasure Map Chance | 藏宝图生成概率 | O:117；L:21076；同名候选，绑定待验证 |
| LootBagChance | goLootBagChance | Loot Bag Drop | 战利品袋掉落 | O:118；L:20826；同名候选，绑定待验证 |
| CropOutput | goCropOutput | Crop Yield | 农作物产出 | O:119；L:20643；同名候选，绑定待验证 |
| SeedDropOutput | goSeedDropOutput | Seed Drop | 种子掉落 | O:120；L:20974；同名候选，绑定待验证 |
| CropGrowthSpeed | goCropGrowthSpeed | Crop Growth | 农作物生长 | O:121；L:20641；同名候选，绑定待验证 |
| BackpackCrafting | goBackpackCrafting | Backpack Crafting | 背包制作 | O:122；L:20554；同名候选，绑定待验证 |
| WorkstationCrafting | goWorkstationCrafting | Workstation Crafting | 工作台制作 | O:123；L:21102；同名候选，绑定待验证 |
| CraftingProgression | goCraftingProgression | Crafting Progression | 制作进度 | O:124；L:20637；同名候选，绑定待验证 |
| CraftingTime | goCraftingTime | Crafting Timer | 制作计时器 | O:125；L:20639；同名候选，绑定待验证 |
| CraftingInput | goCraftingInput | Crafting Cost | 制作消耗 | O:126；L:20629；同名候选，绑定待验证 |
| CraftingOutput | goCraftingOutput | Crafting Yield | 制作产出 | O:127；L:20635；同名候选，绑定待验证 |
| CraftingMaxTier | goCraftingMaxTier | Crafting Max Quality | 制作最高品质 | O:128；L:20633；同名候选，绑定待验证 |
| MiningOutput | goMiningOutput | Mining Yield | 采矿产出 | O:129；L:20867；同名候选，绑定待验证 |
| HarvestingOutput | goHarvestingOutput | Resource Yield | 资源产出 | O:130；L:20756；同名候选，绑定待验证 |
| ScrappingOutput | goScrappingOutput | Scrapping Yield | 分解产出 | O:131；L:20970；同名候选，绑定待验证 |
| SmeltingType | goSmeltingType | Smelter Type | 熔炉类型 | O:132；L:21023；同名候选，绑定待验证 |
| DewCollectorTime | goDewCollectorTime | Dew Collector Time | 露水收集器时间 | O:133；L:20675；同名候选，绑定待验证 |
| DewCollectorOutput | goDewCollectorOutput | Dew Collector Yield | 露水收集器产出 | O:134；L:20673；同名候选，绑定待验证 |
| DewCollectorInput | goDewCollectorInput | Dew Collector Input | 露水收集器原料 | O:135；L:20671；同名候选，绑定待验证 |
| ApiaryTime | goApiaryTime | Apiary Time | 蜂箱时长 | O:136；L:20549；同名候选，绑定待验证 |
| ApiaryOutput | goApiaryOutput | Apiary Yield | 蜂箱产出 | O:137；L:20547；同名候选，绑定待验证 |
| ApiaryInput | goApiaryInput | Apiary Input | 蜂箱原料 | O:138；L:20545；同名候选，绑定待验证 |
| RepairTypes | goRepairTypes | Repair Method | 修理方式 | O:139；L:20953；同名候选，绑定待验证 |
| MaxDegradationAmount | goMaxDegradationAmount | Repair Degradation | 修理损坏 | O:140；L:20847；同名候选，绑定待验证 |
| PointsPerMagazine | goPointsPerMagazine | Magazine Progress | 杂志进度 | O:141；L:20914；同名候选，绑定待验证 |
| SkillGainRate | goSkillGainRate | Skill Gain Rate | 技能获得速率 | O:142；L:21017；同名候选，绑定待验证 |
| SkillPointsPerLevel | goSkillPointsPerLevel | Skill Gain Amount | 技能获得数量 | O:143；L:21019；同名候选，绑定待验证 |
| QuestsEnabled | goQuestsEnabled | Quests | 任务 | O:144；L:20924；同名候选，绑定待验证 |
| IntroQuestEnabled | goIntroQuestEnabled | Intro Quest | 入门任务 | O:145；L:20779；同名候选，绑定待验证 |
| TraderToTraderQuestsEnabled | goTraderToTraderQuestsEnabled | Trade Routes | 商贸路线 | O:146；L:21074；同名候选，绑定待验证 |
| StarterSkillPoints | goStarterSkillPoints | Base Skill Points | 基础技能点 | O:147；L:21032；同名候选，绑定待验证 |
| QuestsPerTier | goQuestsPerTier | Quests Per Tier | 每级任务 | O:148；L:20926；同名候选，绑定待验证 |
| QuestProgressionDailyLimit | goQuestProgressionDailyLimit | Quests Per Day | 每日任务数 | O:149；L:20922；W:838 name/title |
| BuriedQuestsEnabled | goBuriedQuestsEnabled | Buried Quests | 埋藏任务 | O:150；L:20616；同名候选，绑定待验证 |
| POIQuestsEnabled | goPOIQuestsEnabled | POI Quests | 目标点任务 | O:151；L:20916；同名候选，绑定待验证 |
| TraderDialog | goTraderDialog | Trading Dialog | 交易对话 | O:152；L:21058；同名候选，绑定待验证 |
| TraderHours | goTraderHours | Trader Hours | 商人营业时间 | O:153；L:21060；同名候选，绑定待验证 |
| TradersEnabled | goTradersEnabled | Trading Enabled | 交易开启 | O:154；L:21072；同名候选，绑定待验证 |
| VendingEnabled | goVendingEnabled | Vending Enabled | 贩卖机开启 | O:155；L:21087；同名候选，绑定待验证 |
| TraderSellPrices | goTraderSellPrices | Sell Price | 出售价格 | O:156；L:21070；同名候选，绑定待验证 |
| TraderBuyPrices | goTraderBuyPrices | Buy Price | 购买价格 | O:157；L:21056；同名候选，绑定待验证 |
| TraderProtection | goTraderProtection | Trader Protection | 商人保护 | O:158；L:21066；同名候选，绑定待验证 |
| TraderResetInterval | goTraderResetInterval | Trader Reset | 商人重置 | O:159；L:21068；同名候选，绑定待验证 |
| TraderItemAbundance | goTraderItemAbundance | Trader Item Count | 商人物品数量 | O:160；L:21062；同名候选，绑定待验证 |
| TraderBuyLimit | goTraderBuyLimit | Trader Buy Limit | 商人收购上限 | O:161；L:21054；同名候选，绑定待验证 |
| TraderMaxTier | goTraderMaxTier | Trader Max Quality | 商人最高品质 | O:162；L:21064；同名候选，绑定待验证 |
| VendingResetInterval | goVendingResetInterval | Vending Reset | 贩卖机重置 | O:163；L:21091；同名候选，绑定待验证 |
| VendingItemAbundance | goVendingItemAbundance | Vending Item Count | 贩卖机物品数量 | O:164；L:21089；同名候选，绑定待验证 |
| ChallengesEnabled | goChallengesEnabled | Challenges | 挑战 | O:165；L:20619；同名候选，绑定待验证 |
| IntroChallengesEnabled | goIntroChallengesEnabled | Intro Challenges | 入门挑战 | O:166；L:20777；同名候选，绑定待验证 |
| VehicleFuelUsage | goVehicleFuelUsage | Vehicle Fuel Usage | 载具燃料使用量 | O:167；L:21083；同名候选，绑定待验证 |
| VehicleEntityDamage | goVehicleEntityDamage | Vehicle Entity Damage | 载具实体伤害 | O:168；L:21081；同名候选，绑定待验证 |
| VehicleBlockDamage | goVehicleBlockDamage | Vehicle Block Damage | 载具物块伤害 | O:169；L:21079；同名候选，绑定待验证 |
| VehicleSelfDamage | goVehicleSelfDamage | Vehicle Self Damage | 载具自身伤害 | O:170；L:21085；同名候选，绑定待验证 |
| ElectricalOutput | goElectricalOutput | Electrical Output | 电力输出 | O:171；L:20708；同名候选，绑定待验证 |
| SillyCelebrate | goSillyCelebrate | Celebrate Kills | 庆祝击杀 | O:172；L:21009；同名候选，绑定待验证 |
| SillyBigHeads | goSillyBigHeads | Big Heads | 大头 | O:173；L:21005；同名候选，绑定待验证 |
| SillyTinyZombies | goSillyTinyZombies | Tiny Zombies | 迷你丧尸 | O:174；L:21015；同名候选，绑定待验证 |
| SillySounds | goSillySounds | Silly Sounds | 滑稽声音 | O:175；L:21013；同名候选，绑定待验证 |
| SillyLowGravity | goSillyLowGravity | Gravity | 重力 | O:176；L:21011；同名候选，绑定待验证 |
| SillyBlackandWhite | goSillyBlackandWhite | Black and White | 黑白 | O:177；L:21007；同名候选，绑定待验证 |

## 清单外的 76 项本地化标题

为避免将旧清单称为完整当前选项，下表收录所有未在上表中出现且有同名 Desc 配对的 go* 标题。包括其他设置、服务器浏览或旧功能，不保证仍被使用，不代表沙盒成员或分类。该筛选不覆盖没有 Desc 配对的全部本地化键。

例如 goBiomeDay*、goBiomeNight*、goChickenCoop*、goBookLootCount、goCrouchRunSpeed、goStackSizeMultiplier 的名称可确认存在，但不能直接将去掉 go 的字符串当作已验证选项标识。

| 本地化键 | English | 简体中文 | 证据 |
| --- | --- | --- | --- |
| goCameraRestrictionMode | Restrict Camera Mode | 限制镜头模式 | L:20068 |
| goBedrollDeadZoneSize | Bedroll Deadzone | 睡袋死区 | L:20557 |
| goBedrollExpiryTime | Bedroll Duration | 睡袋持续时间 | L:20559 |
| goBiomeDayAnimalDensity | Day Animal Density | 白天动物密度 | L:20563 |
| goBiomeDayAnimalRespawn | Day Animal Respawn | 白天动物重新生成 | L:20565 |
| goBiomeDayEnemyDensity | Day Enemy Density | 白天敌人密度 | L:20567 |
| goBiomeDayZombieRespawn | Day Enemy Respawn | 白天敌人重新生成 | L:20569 |
| goBiomeNightAnimalDensity | Night Animal Density | 夜间动物密度 | L:20577 |
| goBiomeNightAnimalRespawn | Night Animal Respawn | 夜间动物重新生成 | L:20579 |
| goBiomeNightEnemyDensity | Night Enemy Density | 夜间敌人密度 | L:20581 |
| goBiomeNightZombieRespawn | Night Enemy Respawn | 夜间敌人重新生成 | L:20583 |
| goBlockDurability | Block Durability | 物块耐久度 | L:20596 |
| goBookLootCount | Book Abundance | 书籍丰富度 | L:20608 |
| goBuildCreate | Creative Mode | 创意模式 | L:20610 |
| goChickenCoopInput | Chicken Coop Input | 鸡舍投入量 | L:20621 |
| goChickenCoopOutput | Chicken Coop Yield | 鸡舍产量 | L:20623 |
| goChickenCoopTime | Chicken Coop Time | 鸡舍生产时间 | L:20625 |
| goCrouchRunSpeed | Crouch Run Speed | 蹲伏奔跑速度 | L:20645 |
| goCurrentPlayers | Current Players | 当前玩家 | L:20649 |
| goDayCount | Match Length | 比赛时长 | L:20653 |
| goDifficulty | Difficulty Settings | 难度设置 | L:20677 |
| goEacAndSanctions | EAC & Sanctions | EAC与制裁 | L:20697 |
| goEacEnabled | EAC Protected | EAC保护 | L:20699 |
| goEnemyDifficulty | Enemy Aggression | 敌人攻击性 | L:20714 |
| goFragLimit | Frag Limit | 击杀上限 | L:20731 |
| goFreePlayerSlots | Free Slots | 空置栏位 | L:20734 |
| goFullChickenStressEvent | Chicken Stress Event | 鸡只压力事件 | L:20736 |
| goGameHost | Server Name | 服务器名称 | L:20738 |
| goGameMode | Game Mode | 游戏模式 | L:20740 |
| goGameName | Game Name | 游戏名称 | L:20742 |
| goGameWorld | Game World | 游戏世界 | L:20744 |
| goGameWorldSize | World Size | 世界规模 | L:20746 |
| goGameWorldType | Game World Type | 世界类型 | L:20748 |
| goHungerMultiplier | Hunger Multiplier | 饥饿度倍数 | L:20767 |
| goInfectionChance | Infection Chance | 感染几率 | L:20771 |
| goIp | Server IP | 服务器IP | L:20781 |
| goLandClaimDeadZone | Claim Deadzone | 死区距离 | L:20789 |
| goLandClaimDecayMode | Claim Decay Mode | 所有权衰减模式 | L:20793 |
| goLandClaimExpiryTime | Claim Duration | 所有权期限 | L:20800 |
| goLandClaimOfflineDelay | Claim Offline Delay | 领地离线延迟 | L:20805 |
| goLandClaimOfflineDurabilityModifier | Claim Health Offline | 离线所有权生命值 | L:20807 |
| goLandClaimOnlineDurabilityModifier | Claim Health Online | 在线所有权生命值 | L:20811 |
| goLandClaimSize | Claim Size | 所有权范围 | L:20815 |
| goLanguage | Language | 语言 | L:20819 |
| goMatchLength | Match Length | 比赛时长 | L:20843 |
| goMaxChunkAge | Chunk Reset Timer | 区块重置计时器 | L:20845 |
| goMaxPlayers | Max Players | 玩家上限 | L:20851 |
| goMaxSpawnedAnimals | Max Animals | 动物上限 | L:20853 |
| goMaxSpawnedZombies | Max Zombies | 丧尸上限 | L:20855 |
| goModdedConfig | Modded Configs | 已修改配置 | L:20872 |
| goPartySharedKillRange | Party Shared Kill Range | 队伍共享杀戮范围 | L:20888 |
| goPassword | Game Password | 游戏密码 | L:20891 |
| goPasswordProtected | Password Protected | 密码保护 | L:20893 |
| goPersistentCharacters | Persistent Profiles | 固定存档 | L:20897 |
| goPing | Ping | Ping | L:20899 |
| goPlayerKillingMode | Player Killing | 玩家杀戮 | L:20904 |
| goPort | Game Port | 游戏端口 | L:20920 |
| goRebuildMap | Rebuild Map | 重建地图 | L:20934 |
| goRegion | Region | 区域 | L:20940 |
| goRequiresMod | Requires Mod | 需要模组 | L:20955 |
| goResetUnprotectedChunks | Reset Unprotected Chunks | 重置无保护区块 | L:20960 |
| goSanctionsIgnored | Ignores Sanctions | 忽略惩罚 | L:20968 |
| goServerAllowCrossplay | Crossplay Enabled | 已启用跨平台联机 | L:20976 |
| goServerEnabled | Server Enabled | 服务器已启用 | L:20978 |
| goServerSandboxPreset | Sandbox Preset | 沙盒预设 | L:20982 |
| goServerTime | Game Time | 游戏时间 | L:20984 |
| goServerTimeDays | Game Time (Day) | 游戏时间（天数） | L:20985 |
| goServerVisibility | Server Visibility | 服务器可见性 | L:20988 |
| goShowFriendPlayerOnMap | Show Friends On Map | 在地图上显示好友 | L:20997 |
| goStackSizeMultiplier | Stack Size Multiplier | 堆叠上限倍率 | L:21026 |
| goStockFiles | Vanilla Files | 通用文件 | L:21034 |
| goStockSettings | Vanilla Settings | 通用设置 | L:21036 |
| goThirstMultiplier | Thirst Multiplier | 口渴度倍数 | L:21051 |
| goVersion | Game Version | 游戏版本 | L:21093 |
| goZombiePlayers | Zombie Players | 丧尸玩家 | L:21129 |
| goSandboxPreset | Preset | 预设 | L:21748 |

## 未知项与交接

1. 当前八类的全部成员、显示顺序、可见数量未取得原始静态声明，不能承诺 150 项就是全部沙盒选项，也不能直接把补充名称纳入八类。
2. 任务／其他 21 项中的 20 项尚缺运行时标识到本地化键的显式绑定证据；CSV 名称和注释标识已分别验证。
3. 未调查值类型、允许值、默认值、依赖、禁用条件、服务器可配置性、编码格式或编解码器。官方描述不替代这些验证，主任务分别处理配置／UI与编码调查。
4. v3.3 未独立验证，已验证源文件摘要及 Steam 清单 build 25661908；客户端截图与专用服务端是否为完全相同构建尚未知。
5. 本次只新增此研究文档，没有修改应用或其他文档，没有访问生产、部署、发布或给其他聊天发消息。核查覆盖八类名称、21 个用户指定中文逐字匹配、150 个唯一注释标识、147 个同名标题与 3 个 XML 浏览标题关系、补充标题及文件摘要。未运行应用测试，本任务没有应用改动。


## 补充核验：DLL 实际本地化算法与 165 项分类（同日）

本节补足前文仅检查 CSV/XML 时的名称绑定与分类未知项。前文相关限制为早期证据范围，以本节为准；截图像素、运行时可见性及 v3.3 语义版本仍未独立核验。

使用 /private/tmp/gpl-dotnet-inspect/bin/python 的 dnfile/dncil 静态读取 DLL 实际方法及 SandboxOptions 枚举常量；按父线程 SetupOptions 提取 JSON 的 id 关联枚举并生成键，逐项核对完整 CSV。分类和注册顺序来源是父线程线性 IL 提取，本补充重新统计，未独立重做 SetupOptions 提取器。未调查允许值或编解码。

### 来源摘要

| 本地输入 | SHA-256 |
| --- | --- |
| /private/tmp/gpl-seven-Assembly-CSharp.dll | e16ff9d539b8fbf64904c0bc090d502286a62464742a9396f0455936ad0beb87 |
| /private/tmp/gpl-seven-Localization.csv | 74fe44e9ced90cd04ea6c821d535fd4ec02240307954d0b4b2884b72988b0a1f |
| /private/tmp/gpl-sandbox-extracted.json | 5f4db5ce1a320d91015bc8d04983f38b3c5504e1c4feecbd8b921e3895c123ea |

CSV 摘要与前文镜像原始 CSV 一致；DLL 与指定镜像同源依赖父线程说明，未另行 SSH 回读 DLL。

### 实际算法及优先级

实际方法为 BaseSandboxOption.get_OptionNameText（token 0x0600C2C6，RVA 0x0047A2E4）及 get_DescriptionText（token 0x0600C2C7，RVA 0x0047A378），不是 get_LocalizedName/get_LocalizedDescription。

名称 getter：先返回非 null 的 optionNameText 缓存；否则若 OverrideOptionName 非 null，只使用这个覆盖键；否则使用 "go" + Option.ToString()。Option 是 SandboxOptions 枚举，不是 JSON 中的英文 name 或 OptionName 字段。键存在时调用 Localization.Get(key, false, null)。默认键不存在时返回 "*" + OptionName；覆盖键不存在时返回空字符串，绝不回退默认键。结果缓存到 optionNameText。

描述 getter：先返回非 null 的 descriptionText 缓存；否则优先使用非 null 的 OverrideDescriptionName；无覆盖时生成 "go" + Option.ToString() + "Desc"。无论默认键还是覆盖键不存在均返回空字符串，并缓存。描述覆盖独立于名称覆盖，不能自动从 OverrideOptionName 拼 Desc。

这里 null 判断来自 IL 的 brtrue 引用分支，不是 IsNullOrEmpty；空字符串覆盖仍优先于默认键，空字符串缓存也直接返回。未调查外部清缓存行为。调用的 MethodDef 0x0600966C、0x0600966B 元数据所有者分别验证为 Localization.Exists 和 Localization.Get；Exists 第二参数为 false。

IL 定位（dncil 输出偏移包含方法头，非去头后的 IL 标签）：名称 0x0D/0x12 读取／判断缓存，0x15/0x1A 判断 OverrideOptionName，0x1C–0x32 拼 go + 枚举，0x42–0x4D 拼星号备用名，0x63–0x86 处理覆盖。描述 0x15/0x1A 判断独立覆盖，0x1C–0x37 拼 go + 枚举 + Desc，0x5D–0x80 处理覆盖。构造器 token 0x0600C2CB，RVA 0x0047A418，分别写入 Option、OptionName、CategoryName。

例如 id=123 的 JSON name 是 Quests per Day，实际枚举为 QuestProgressionDailyLimit，故键为 goQuestProgressionDailyLimit；id=146 的 name 是 Celebrate Kills，枚举为 SillyCelebrate，故键为 goSillyCelebrate。去掉英文 name 的空格不是实际算法。

### 三组覆盖键

165 条提取记录中只有以下三项设置覆盖字段。字段值来自父线程提取 JSON，优先级已从实际 getter 独立核验。

| 枚举 | 标题覆盖 | 描述覆盖 | 中文 |
| --- | --- | --- | --- |
| BlockDamage | goBlockDamagePlayer | goBlockDamagePlayerDesc | 物块伤害 |
| AirDropMarker | goMarkAirDrops | goMarkAirDropsDesc | 标记空投 |
| GlobalLootCount | goLootAbundance | goLootAbundanceDesc | 掉落率 |

### 八类数量与全部绑定

165 个 id 唯一；生成并应用覆盖后，165 个标题键、165 个描述键均存在，标题均有非空 schinese。以下按提取文件注册顺序列出，不承诺截图行列顺序或运行中可见性。

| 分类 | 官方中文 | 数量 |
| --- | --- | --- |
| General | 玩家 | 33 |
| Entities | 实体 | 26 |
| World | 世界 | 23 |
| Resources | 资源 | 25 |
| Crafting | 制作 | 23 |
| Traders | 商人 | 14 |
| Tasks | 任务 | 10 |
| Misc | 其他 | 11 |
| 合计 |  | 165 |

用户列出的任务 10 项与 Tasks 中文集合完全一致，其他 11 项与 Misc 中文集合完全一致，无缺项或额外项。这 21 项均无覆盖字段，标题／描述直接按 go + 枚举及 go + 枚举 + Desc 生成。前文的同名候选已由实际算法、枚举 id、SetupOptions 提取分类和 CSV 文本联合验证。

#### 玩家 / General

| id | SandboxOptions 枚举 | 实际标题键 | english 原文 | schinese 原文 |
| --- | --- | --- | --- | --- |
| 0 | RangedDamage | goRangedDamage | Ranged Damage | 远程伤害 |
| 1 | MeleeDamage | goMeleeDamage | Melee Damage | 近战伤害 |
| 2 | BlockDamage | goBlockDamagePlayer | Block Damage | 物块伤害 |
| 3 | TerrainDamage | goTerrainDamage | Terrain Damage | 地形伤害 |
| 4 | HeadshotMultiplier | goHeadshotMultiplier | Headshot Multiplier | 爆头倍数 |
| 17 | IncomingDamage | goIncomingDamage | Player Damage In | 玩家所受伤害 |
| 6 | WalkSpeed | goWalkSpeed | Walk Speed | 行走速度 |
| 7 | RunSpeed | goRunSpeed | Run Speed | 奔跑速度 |
| 5 | CrouchSpeed | goCrouchSpeed | Crouch Speed | 蹲伏速度 |
| 112 | CrouchRunSpeed | goCrouchRunSpeed | Crouch Run Speed | 蹲伏奔跑速度 |
| 8 | JumpStrength | goJumpStrength | Jump Strength | 跳跃力量 |
| 10 | StaminaRegen | goStaminaRegen | Stamina Regen | 体力值回复 |
| 9 | StaminaUsage | goStaminaUsage | Stamina Usage | 体力值消耗 |
| 18 | XPMultiplier | goXPMultiplier | XP Multiplier | 经验值倍数 |
| 19 | ShowXP | goShowXP | Show XP | 显示经验值 |
| 11 | PlayerLevelBonusApplied | goPlayerLevelBonusApplied | Level Stat Bonus | 等级属性加成 |
| 116 | SkillGainRate | goSkillGainRate | Skill Gain Rate | 技能获得速率 |
| 117 | SkillPointsPerLevel | goSkillPointsPerLevel | Skill Gain Amount | 技能获得数量 |
| 15 | NewbieCoat | goNewbieCoat | New Player Buff | 新玩家增益 |
| 26 | DeathPenalty | goDeathPenalty | Death Penalty | 死亡惩罚 |
| 22 | LoseItemsOnDeathType | goLoseItemsOnDeathType | Lose On Death | 死亡时丢失 |
| 23 | LoseItemsOnDeathCount | goLoseItemsOnDeathCount | Death Loss Count | 死亡丢失数量 |
| 24 | DegradeItemsOnDeath | goDegradeItemsOnDeath | Death Degradation | 死亡损耗 |
| 25 | DegradeAmountOnDeath | goDegradeAmountOnDeath | Death Degradation Amount | 死亡损耗量 |
| 27 | DropOnDeath | goDropOnDeath | Drop On Death | 死亡丢失物品 |
| 28 | DropOnQuit | goDropOnQuit | Drop On Quit | 退出丢失物品 |
| 161 | HungerMultiplier | goHungerMultiplier | Hunger Multiplier | 饥饿度倍数 |
| 162 | ThirstMultiplier | goThirstMultiplier | Thirst Multiplier | 口渴度倍数 |
| 160 | InfectionChance | goInfectionChance | Infection Chance | 感染几率 |
| 29 | InfectionRate | goInfectionRate | Infection Rate | 感染速率 |
| 163 | StackSizeMultiplier | goStackSizeMultiplier | Stack Size Multiplier | 堆叠上限倍率 |
| 20 | EncumbranceModifier | goEncumbranceModifier | Encumbrance Slots | 负重栏位 |
| 12 | JarRefund | goJarRefund | Jar Refund | 罐子退款 |

#### 实体 / Entities

| id | SandboxOptions 枚举 | 实际标题键 | english 原文 | schinese 原文 |
| --- | --- | --- | --- | --- |
| 30 | EnemySpawnMode | goEnemySpawnMode | Enemy Spawning | 敌人生成 |
| 43 | MaxEnemyTier | goMaxEnemyTier | Max Enemy Tier | 最大敌人等级 |
| 46 | BiomeDayEnemyDensity | goBiomeDayEnemyDensity | Day Enemy Density | 白天敌人密度 |
| 44 | BiomeDayZombieRespawn | goBiomeDayZombieRespawn | Day Enemy Respawn | 白天敌人重新生成 |
| 155 | BiomeDayAnimalDensity | goBiomeDayAnimalDensity | Day Animal Density | 白天动物密度 |
| 45 | BiomeDayAnimalRespawn | goBiomeDayAnimalRespawn | Day Animal Respawn | 白天动物重新生成 |
| 158 | BiomeNightEnemyDensity | goBiomeNightEnemyDensity | Night Enemy Density | 夜间敌人密度 |
| 156 | BiomeNightZombieRespawn | goBiomeNightZombieRespawn | Night Enemy Respawn | 夜间敌人重新生成 |
| 159 | BiomeNightAnimalDensity | goBiomeNightAnimalDensity | Night Animal Density | 夜间动物密度 |
| 157 | BiomeNightAnimalRespawn | goBiomeNightAnimalRespawn | Night Animal Respawn | 夜间动物重新生成 |
| 31 | EntityDamage | goEntityDamage | Enemy Damage Dealt | 敌人造成的伤害 |
| 42 | EntityIncomingDamage | goEntityIncomingDamage | Entity Damage In | 实体所受伤害 |
| 32 | BlockDamageAI | goBlockDamageAI | Enemy Block Damage | 敌人物块伤害 |
| 33 | BlockDamageAIBM | goBlockDamageAIBM | BM Block Damage | 血月物块伤害 |
| 16 | HeadshotMode | goHeadshotMode | Headshot Mode | 爆头模式 |
| 13 | ShowHealthBars | goShowHealthBars | Entity Health Bars | 实体生命值条 |
| 14 | ShowEnemyDamage | goShowEnemyDamage | Show Entity Damage | 显示实体伤害 |
| 34 | ZombieMove | goZombieMove | Zombie Day Speed | 丧尸白天速度 |
| 35 | ZombieMoveNight | goZombieMoveNight | Zombie Night Speed | 丧尸夜间速度 |
| 36 | ZombieFeralMove | goZombieFeralMove | Zombie Feral Speed | 凶残丧尸速度 |
| 37 | ZombieBMMove | goZombieBMMove | Zombie BM Speed | 丧尸血月速度 |
| 38 | ZombieFeralSense | goZombieFeralSense | Zombie Feral Sense | 凶残丧尸感知 |
| 39 | AISmellMode | goAISmellMode | AI Smell Mode | AI 嗅觉模式 |
| 41 | ZombieRageChance | goZombieRageChance | Zombie Rage | 丧尸狂暴 |
| 40 | AllowZombieDigging | goAllowZombieDigging | Zombie Digging | 丧尸挖掘 |
| 47 | ZombiesEatAnimals | goZombiesEatAnimals | Zombies Eat Animals | 丧尸吃动物 |

#### 世界 / World

| id | SandboxOptions 枚举 | 实际标题键 | english 原文 | schinese 原文 |
| --- | --- | --- | --- | --- |
| 60 | GlobalGSModifier | goGlobalGSModifier | Global Game Stage | 全局游戏阶段 |
| 61 | BiomeGSModifier | goBiomeGSModifier | Biome Game Stage | 环境游戏阶段 |
| 55 | BiomeProgression | goBiomeProgression | Biome Progression | 生态环境进度 |
| 56 | TemperatureSurvival | goTemperatureSurvival | Temperature Survival | 温度生存 |
| 74 | MaxTechType | goMaxTechType | Max Tech Type | 最大科技类型 |
| 73 | WorkstationsInTheWild | goWorkstationsInTheWild | Workstations In The Wild | 野外工作站 |
| 48 | BloodMoonFrequency | goBloodMoonFrequency | Blood Moon Frequency | 血月频率 |
| 49 | BloodMoonRange | goBloodMoonRange | Blood Moon Range | 血月范围 |
| 51 | BloodMoonEnemyCount | goBloodMoonEnemyCount | Blood Moon Count | 血月计数 |
| 50 | BloodMoonWarning | goBloodMoonWarning | Blood Moon Warning | 血月警告 |
| 52 | AirDropFrequency | goAirDropFrequency | Air Drops | 空投 |
| 54 | AirDropRandomTime | goAirDropRandomTime | Air Drop Variance | 空投变化 |
| 57 | StormFreq | goStormFreq | Storm Frequency | 风暴频率 |
| 58 | StormWarning | goStormWarning | Storm Warning | 风暴警告 |
| 59 | HeatMapSensitivity | goHeatMapSensitivity | Heat Map Sensitivity | 热力图敏感度 |
| 66 | DayNightLength | goDayNightLength | 24 Hour Cycle | 24 小时周期 |
| 67 | DayLightLength | goDayLightLength | Daylight Length | 白昼时长 |
| 53 | AirDropMarker | goMarkAirDrops | Mark Air Drops | 标记空投 |
| 68 | AllowMap | goAllowMap | Allow Map | 允许地图 |
| 69 | AllowCompass | goAllowCompass | Allow Compass | 允许指南针 |
| 70 | AllowScreenMarkers | goAllowScreenMarkers | Allow Screen Markers | 允许屏幕标记 |
| 71 | ShowLocationInfo | goShowLocationInfo | Show Location Info | 显示地点信息 |
| 72 | ShowDayTime | goShowDayTime | Show Day/Time | 显示天数/时间 |

#### 资源 / Resources

| id | SandboxOptions 枚举 | 实际标题键 | english 原文 | schinese 原文 |
| --- | --- | --- | --- | --- |
| 77 | LootMaxTier | goLootMaxTier | Max Loot Quality | 最大战利品品质 |
| 62 | GlobalLSModifier | goGlobalLSModifier | Global Loot Stage | 全局战利品等级 |
| 63 | BiomeLSModifier | goBiomeLSModifier | Biome Loot Stage | 环境战利品等级 |
| 64 | POITierLSModifier | goPOITierLSModifier | POI Loot Stage | 目标点战利品等级 |
| 75 | LootRespawnDays | goLootRespawnDays | Loot Respawn Days | 战利品刷新天数 |
| 76 | LootTimer | goLootTimer | Loot Timer | 开箱时间 |
| 90 | LootBagChance | goLootBagChance | Loot Bag Drop | 战利品袋掉落 |
| 78 | GlobalLootCount | goLootAbundance | Loot Abundance | 掉落率 |
| 79 | FoodLootCount | goFoodLootCount | Food Abundance | 食物丰富度 |
| 80 | DrinkLootCount | goDrinkLootCount | Drink Abundance | 饮料丰富度 |
| 81 | MedicalLootCount | goMedicalLootCount | Medical Abundance | 药品丰富度 |
| 82 | AmmoLootCount | goAmmoLootCount | Ammo Abundance | 弹药丰富度 |
| 83 | ResourceLootCount | goResourceLootCount | Resource Abundance | 资源丰富度 |
| 84 | ArmorLootCount | goArmorLootCount | Armor Abundance | 护甲丰富度 |
| 85 | MeleeLootCount | goMeleeLootCount | Melee Abundance | 近战武器丰富度 |
| 86 | RangedLootCount | goRangedLootCount | Ranged Abundance | 远程武器丰富度 |
| 87 | DukesLootCount | goDukesLootCount | Currency Abundance | 货币丰富度 |
| 88 | CraftingMagazinesLootCount | goCraftingMagazinesLootCount | Magazine Abundance | 杂志丰富度 |
| 111 | BookLootCount | goBookLootCount | Book Abundance | 书籍丰富度 |
| 89 | TreasureMapChance | goTreasureMapChance | Treasure Map Chance | 藏宝图生成概率 |
| 101 | MiningOutput | goMiningOutput | Mining Yield | 采矿产出 |
| 91 | CropOutput | goCropOutput | Crop Yield | 农作物产出 |
| 92 | SeedDropOutput | goSeedDropOutput | Seed Drop | 种子掉落 |
| 102 | HarvestingOutput | goHarvestingOutput | Resource Yield | 资源产出 |
| 93 | CropGrowthSpeed | goCropGrowthSpeed | Crop Growth | 农作物生长 |

#### 制作 / Crafting

| id | SandboxOptions 枚举 | 实际标题键 | english 原文 | schinese 原文 |
| --- | --- | --- | --- | --- |
| 96 | CraftingProgression | goCraftingProgression | Crafting Progression | 制作进度 |
| 100 | CraftingMaxTier | goCraftingMaxTier | Crafting Max Quality | 制作最高品质 |
| 115 | PointsPerMagazine | goPointsPerMagazine | Magazine Progress | 杂志进度 |
| 94 | BackpackCrafting | goBackpackCrafting | Backpack Crafting | 背包制作 |
| 95 | WorkstationCrafting | goWorkstationCrafting | Workstation Crafting | 工作台制作 |
| 104 | SmeltingType | goSmeltingType | Smelter Type | 熔炉类型 |
| 97 | CraftingTime | goCraftingTime | Crafting Timer | 制作计时器 |
| 98 | CraftingInput | goCraftingInput | Crafting Cost | 制作消耗 |
| 99 | CraftingOutput | goCraftingOutput | Crafting Yield | 制作产出 |
| 103 | ScrappingOutput | goScrappingOutput | Scrapping Yield | 分解产出 |
| 105 | DewCollectorTime | goDewCollectorTime | Dew Collector Time | 露水收集器时间 |
| 106 | DewCollectorOutput | goDewCollectorOutput | Dew Collector Yield | 露水收集器产出 |
| 107 | DewCollectorInput | goDewCollectorInput | Dew Collector Input | 露水收集器原料 |
| 108 | ApiaryTime | goApiaryTime | Apiary Time | 蜂箱时长 |
| 109 | ApiaryOutput | goApiaryOutput | Apiary Yield | 蜂箱产出 |
| 110 | ApiaryInput | goApiaryInput | Apiary Input | 蜂箱原料 |
| 152 | ChickenCoopTime | goChickenCoopTime | Chicken Coop Time | 鸡舍生产时间 |
| 153 | ChickenCoopOutput | goChickenCoopOutput | Chicken Coop Yield | 鸡舍产量 |
| 154 | ChickenCoopInput | goChickenCoopInput | Chicken Coop Input | 鸡舍投入量 |
| 164 | FullChickenStressEvent | goFullChickenStressEvent | Chicken Stress Event | 鸡只压力事件 |
| 21 | ItemDegradation | goItemDegradation | Item Degradation | 物品损坏 |
| 113 | RepairTypes | goRepairTypes | Repair Method | 修理方式 |
| 114 | MaxDegradationAmount | goMaxDegradationAmount | Repair Degradation | 修理损坏 |

#### 商人 / Traders

| id | SandboxOptions 枚举 | 实际标题键 | english 原文 | schinese 原文 |
| --- | --- | --- | --- | --- |
| 128 | TradersEnabled | goTradersEnabled | Trading Enabled | 交易开启 |
| 129 | VendingEnabled | goVendingEnabled | Vending Enabled | 贩卖机开启 |
| 127 | TraderHours | goTraderHours | Trader Hours | 商人营业时间 |
| 132 | TraderProtection | goTraderProtection | Trader Protection | 商人保护 |
| 126 | TraderDialog | goTraderDialog | Trading Dialog | 交易对话 |
| 65 | GlobalTSModifier | goGlobalTSModifier | Global Trader Stage | 全局商人阶段 |
| 136 | TraderMaxTier | goTraderMaxTier | Trader Max Quality | 商人最高品质 |
| 134 | TraderItemAbundance | goTraderItemAbundance | Trader Item Count | 商人物品数量 |
| 138 | VendingItemAbundance | goVendingItemAbundance | Vending Item Count | 贩卖机物品数量 |
| 133 | TraderResetInterval | goTraderResetInterval | Trader Reset | 商人重置 |
| 137 | VendingResetInterval | goVendingResetInterval | Vending Reset | 贩卖机重置 |
| 130 | TraderSellPrices | goTraderSellPrices | Sell Price | 出售价格 |
| 131 | TraderBuyPrices | goTraderBuyPrices | Buy Price | 购买价格 |
| 135 | TraderBuyLimit | goTraderBuyLimit | Trader Buy Limit | 商人收购上限 |

#### 任务 / Tasks

| id | SandboxOptions 枚举 | 实际标题键 | english 原文 | schinese 原文 |
| --- | --- | --- | --- | --- |
| 139 | ChallengesEnabled | goChallengesEnabled | Challenges | 挑战 |
| 118 | QuestsEnabled | goQuestsEnabled | Quests | 任务 |
| 140 | IntroChallengesEnabled | goIntroChallengesEnabled | Intro Challenges | 入门挑战 |
| 119 | IntroQuestEnabled | goIntroQuestEnabled | Intro Quest | 入门任务 |
| 120 | TraderToTraderQuestsEnabled | goTraderToTraderQuestsEnabled | Trade Routes | 商贸路线 |
| 124 | BuriedQuestsEnabled | goBuriedQuestsEnabled | Buried Quests | 埋藏任务 |
| 125 | POIQuestsEnabled | goPOIQuestsEnabled | POI Quests | 目标点任务 |
| 122 | QuestsPerTier | goQuestsPerTier | Quests Per Tier | 每级任务 |
| 123 | QuestProgressionDailyLimit | goQuestProgressionDailyLimit | Quests Per Day | 每日任务数 |
| 121 | StarterSkillPoints | goStarterSkillPoints | Base Skill Points | 基础技能点 |

#### 其他 / Misc

| id | SandboxOptions 枚举 | 实际标题键 | english 原文 | schinese 原文 |
| --- | --- | --- | --- | --- |
| 141 | VehicleFuelUsage | goVehicleFuelUsage | Vehicle Fuel Usage | 载具燃料使用量 |
| 142 | VehicleEntityDamage | goVehicleEntityDamage | Vehicle Entity Damage | 载具实体伤害 |
| 143 | VehicleBlockDamage | goVehicleBlockDamage | Vehicle Block Damage | 载具物块伤害 |
| 144 | VehicleSelfDamage | goVehicleSelfDamage | Vehicle Self Damage | 载具自身伤害 |
| 145 | ElectricalOutput | goElectricalOutput | Electrical Output | 电力输出 |
| 146 | SillyCelebrate | goSillyCelebrate | Celebrate Kills | 庆祝击杀 |
| 147 | SillyBigHeads | goSillyBigHeads | Big Heads | 大头 |
| 148 | SillyTinyZombies | goSillyTinyZombies | Tiny Zombies | 迷你丧尸 |
| 150 | SillyLowGravity | goSillyLowGravity | Gravity | 重力 |
| 149 | SillySounds | goSillySounds | Silly Sounds | 滑稽声音 |
| 151 | SillyBlackandWhite | goSillyBlackandWhite | Black and White | 黑白 |

### 旧清单差异

旧 150 项与当前 165 项有 147 项交集，旧清单有 3 项未出现在当前注册提取中，当前多出 18 项，净增加 15 项。不能把这些未注册旧名字直接作为当前选项，也不能仅凭此声称它们已在所有代码中移除。

仅旧清单存在：BiomeAnimalRespawn, BiomeEnemyDensity, BiomeZombieRespawn。

| 当前新增枚举 | 分类 | 中文 |
| --- | --- | --- |
| CrouchRunSpeed | General | 蹲伏奔跑速度 |
| HungerMultiplier | General | 饥饿度倍数 |
| ThirstMultiplier | General | 口渴度倍数 |
| InfectionChance | General | 感染几率 |
| StackSizeMultiplier | General | 堆叠上限倍率 |
| BiomeDayEnemyDensity | Entities | 白天敌人密度 |
| BiomeDayZombieRespawn | Entities | 白天敌人重新生成 |
| BiomeDayAnimalDensity | Entities | 白天动物密度 |
| BiomeDayAnimalRespawn | Entities | 白天动物重新生成 |
| BiomeNightEnemyDensity | Entities | 夜间敌人密度 |
| BiomeNightZombieRespawn | Entities | 夜间敌人重新生成 |
| BiomeNightAnimalDensity | Entities | 夜间动物密度 |
| BiomeNightAnimalRespawn | Entities | 夜间动物重新生成 |
| BookLootCount | Resources | 书籍丰富度 |
| ChickenCoopTime | Crafting | 鸡舍生产时间 |
| ChickenCoopOutput | Crafting | 鸡舍产量 |
| ChickenCoopInput | Crafting | 鸡舍投入量 |
| FullChickenStressEvent | Crafting | 鸡只压力事件 |

本补充仅修改此研究文档。没有修改应用，未访问生产、部署、发布或调查值组／编解码；截图对应语义版本、实际布局及可见性仍待主任务核验。


## 补充核验：应用标签元数据与子项禁用规则

只读核查 `apps/web/lib/seven-days-sandbox-data.json`，不修改应用。核查时该文件 SHA-256 为 `681dd6540b883c576add59b552d8feeda6f8b300323d6718c8dfcb597bb05125`；父线程可能继续修改它，此结论仅针对该内容。`/private/tmp/gpl-sandbox-enum.json` 与本补充独立读取的 DLL 枚举，在全部 165 个注册 id 上一致。应用 165 个 key、分类、中英文 label 全部与实际枚举／本地化算法及 CSV 一致。分类数量为 33/26/23/25/23/14/10/11。未审查该文件的 choices、defaultIndex 或 codeVersion。

### DisabledOptionsOnValue 的方向与公式

`DisabledOptionsOnValue` 是父项持有的规则；其 DisabledOptions 数组是受控制的子项 id，不是“本项依赖的父项列表”。Boolean/Float/Int 嵌套规则构造器将参数依次写入 DisabledOptions、Value、Inverted、AlwaysShowValuesOnEnabled（分别见 token 0x0600C2F4 / 0x0600C308 / 0x0600C31B）。应用 14 组 ids/value/inverted 与父线程提取规则逐项一致。

直接核查 SandboxOptionManager.SetOption 三个重载：

- int 重载 token 0x0600C330，RVA 0x0047DB98。
- float 重载 token 0x0600C331，RVA 0x0047DD3C。
- bool 重载 token 0x0600C332，RVA 0x0047DE08。

三者先设置父项值，再取得其 DisabledOptions 规则。正常同类型分支中，dncil 偏移 0x3E–0x5C：Inverted=false 时先 ceq 再与 0 做 ceq，得到“不相等”；Inverted=true 时只做 ceq，得到“相等”。计算结果直接写入子项 IsEnabled（偏移 0xAC），因此：

```text
equal = (parentActualValue == rule.Value)
child.IsEnabled = Inverted ? equal : !equal
childDisabled = equal XOR Inverted
```

所以用户给出的禁用公式正确；这里要区分“禁用布尔量”与“启用布尔量”。原生使用精确类型比较；Web 若以 bool 表示布尔选项而依赖 Value 为 0/1，应先统一表示后比较。本补充没有审查 UI 实际代码。int 重载另含 float 转换分支，不能把该特殊输入路径当成同类型父实际值比较的通用定义。

| Inverted | 父值等于 Value | childDisabled |
| --- | --- | --- |
| false | false | false |
| false | true | true |
| true | false | true |
| true | true | false |

### 不强制清除子项值及多父项边界

这些 SetOption 依赖处理循环只写子项 IsEnabled 和 DisabledByText；禁用时提示使用 xuiSandboxDisabledBy、父项本地化名和当前值显示文本，启用时清空提示。循环中没有对子项调用 SetInt/SetFloat/SetBool、SetToDefault 或写入子项 CurrentValue，也不递归更新子项的依赖。因此“禁用不强制清除 child”与本次检查的原生 setter 一致。这是这些方法的局部结论，不声称其他尚未检查的初始化、保存或运行时消费路径也绝不改变值。

原生 setter 直接赋值，未在此处合并多个父项条件。当前规则里 child id=50（BloodMoonWarning）由父 id=48（BloodMoonFrequency）及 id=72（ShowDayTime）共同涉及；一次 SetOption 调用只根据该次父项条件覆盖子项 IsEnabled。在这个方法范围内表现为后一次写入生效，不能从中推出“任一父项禁用即禁用”的 OR 或 AND 策略。要声称 UI 完全复刻多父项行为，仍需另查初始化／调用顺序；本补充未扩展到该项。

没有修改应用、提取允许值或编解码。研究文档检查通过，应用标签 165 项和依赖规则 14 组均完成自动逐项比对。
