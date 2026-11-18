# Project Zomboid 42.21 配置元数据与官方预设核验

审计日期：2026-10-28。对象：官方 42.21 / Steam App 380870 / Build 25485538。依据为配置对象、实际 Lua 和 manifest。预设、INI roundtrip 和默认出生区已在无网络临时容器运行原版类核验，没有启动游戏服务器或操作游戏 UI。完整安装由镜像任务统一下载，本任务只读使用。交付限定元数据、提取脚本和本文，不改 Provider/UI/runtime、不提交 Git。

## 关键结论

- 两个目标 Lua 文件中，**有效 `singlePlayerOnly` 声明为 0 项，消费该属性的过滤代码也为 0 处**。唯一文本命中是 S:4491 的注释 `LootRespawn`；B 文件没有这个标记。不能把该注释加入字段清单，也不能凭字段名称判断仅限单人。
- 静态沙盒设置表 257 项；单人非调试 consumer 创建 255 项控件，调试 consumer 创建 257 项。服务器 consumer 同样创建 255 项，但排除的是另外两项：server 跳过 `WaterShut`、`ElecShut`，单人非调试跳过 `WaterShutModifier`、`ElecShutModifier`。这不证明隐藏字段在引擎里无效。
- `steamOnly` 是**页面创建条件**：`Steam` 和 `SteamWorkshop`。条件为 `not page.steamOnly or getSteamModeActive()`，不是清除底层选项。
- 单人的地图／经验倍率依赖只改变文字颜色；没有禁用控件、隐藏字段或丢弃保存值。服务器 consumer 没有这些变灰条件。不能将其提炼成全局 `disabledWhen`。
- `Zombies` 回调依赖 `media/lua/shared/defines.lua:73` 的 `ZombiePopulationMultiplierTable`。真实 1…6 值为 `[2.5,1.6,1.2,0.65,0.15,0.0]`，已从同 Build 文件解析写入 onChange。
- 出生区域可以引用官方地图文件，但编辑按钮只允许符合条件的本地出生点文件。限制的是编辑入口，不是禁止引用、增删区域或修改 INI `SpawnPoint`。

## 证据与可复核边界

下文 `S:行号` 指 [ServerSettingsScreen.lua](/private/tmp/gpl-pz-export-source/media/lua/client/OptionScreens/ServerSettingsScreen.lua)，`B:行号` 指 [SandboxOptions.lua](/private/tmp/gpl-pz-export-source/media/lua/client/OptionScreens/SandboxOptions.lua)。行号按原始文件换行计算，未格式化源文件。表格中的字段行均是 S 的有效声明起始行。

| 材料 | 用途与边界 |
|---|---|
| S，5199 行 | SettingsTable、server consumer、定制面板、保存路径与回调的直接证据 |
| B，1167 行 | 单人 consumer、调试条件、快捷档位、开局转换的直接证据 |
| [appmanifest_380870.acf](/private/tmp/gpl-pz-export-source/steamapps/appmanifest_380870.acf:12) | `buildid=25485538`；app-info.log:265–270 另记 public 分支 buildid |
| [options-CN.json](/private/tmp/gpl-pz-config-audit-20261028/options-CN.json) / options-EN.json | 既有配置对象导出：版本 42.21；144 server + 269 sandbox；类型、默认值、边界、枚举文案来自此处，不声称本次重跑 JVM |
| [ui-inventory.json](/private/tmp/gpl-pz-config-audit-20261028/ui-inventory.json) | 静态顺序及映射；本次与有效声明逐项比对；不是最终控件可见性 |
| [现有交互报告](/private/tmp/gpl-pz-config-audit-20261028/交互与预设核验.md) / interaction-rules.json | 线索及旧结论；本次以原始 Lua 复核，不能代替直接证据 |
| [提取器](/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/scripts/extract-project-zomboid-config.py) | 本次实现并执行；重建 Java 提取器、整合元数据与预设 fixture、离线完整性校验 |

源文件 SHA256：

- `ServerSettingsScreen.lua`：`617349ddbd1e5023c5f4d83a248a89423dfc608db01496458b31a6b1a3d51a19`。
- `SandboxOptions.lua`：`22319f9de25c5da14146e54758a1711347f019272d1277860db66d75a2cd0833`。

S 的哈希与 inventory 的 `sourceSha256` 完全一致。没有使用其他 Build 的数值。后来补取的 `media/lua/shared/defines.lua:65–89` 定义了三个倍率常量组；INI、预设构造与默认出生区另外通过同 Build Java 对象及字节码核验。随机停水停电运行行为仍未测试。

## consumer 差异与字段条件

| 对象 | server-settings consumer | single-player consumer | 直接证据 |
|---|---|---|---|
| 表的来源 | SettingsTable 的 INI 与 Sandbox | 从相同 Sandbox 表复制，移除预设定制页 | S:2417–2433、4039–4045；B:529–543 |
| `singlePlayerOnly` | 无有效属性、无过滤读取 | 无此标记、无过滤读取 | S:4491 注释；完整两文件搜索及两条 createPanel 路径 |
| `WaterShut`、`ElecShut` | createPanel 首个分支直接跳过，后续不创建 label/control | 正常创建 enum | S:2528–2529、2579–2586；B:584–589、615–626、660–680 |
| `WaterShutModifier`、`ElecShutModifier` | 正常创建整数输入 | 仅 `getDebug()` 时创建，标签加调试前缀 | S:4853–4856；B:584–589、668–669 |
| `Map.AllowMiniMap`、`Map.MapAllKnown` | 无关联禁用／变灰 | `Map.AllowWorldMap=false` 时标签变灰 | B:59–63、83–98；S:131–170 |
| `Map.MapNeedsLight` | 正常控件 | 不在上述 maps 集合，不随该条件变灰 | S:4960–4963；B:59–63 |
| 35 项独立技能倍率 | 无关联禁用／变灰 | `MultiplierConfig.GlobalToggle=true` 时标签及文本框文字变灰 | B:53–57、83–98；完整字段见后表 |
| `MultiplierConfig.Global` | 无关联禁用／变灰 | `GlobalToggle=false` 时标签及文本框文字变灰 | B:86–98 |
| 32 项 `advancedCombo` | 普通数字输入；忽略 advancedCombo | 简易档位／高级数字输入切换 | S:2535–2543；B:253–303、346–353、559–564、601–614 |
| 日数 | 按 firstYear、StartMonth 更新列表并向月底截断 | 同样处理 | S:2761–2774；B:391–404、922 |
| `StartYear` | 对象存在，静态表无此项，不创建控件 | 同样不创建 | S:4369–4377；后附缺失项清单 |
| 模组动态选项 | `isCustom()` 且 pageName 非 nil 时补入现有／新页面 | 获取共享 Sandbox 表，沿用补入项 | S:5128–5146；B:530 |

上述“35 项”来自 B:53 的完整显式名单，不代表所有 `MultiplierConfig.*` 都适用。变灰后依旧走 B:795–814 的通用保存路径。搜索命中还会在 B:101–102 将标签设为绿色；因此颜色也不是引擎依赖的可靠证明。S:131–170 没有复制单人变灰代码。

### 应补充的 onChange / disabledWhen 元数据

建议按 `consumers.serverSettings`、`consumers.singlePlayer` 保存 UI 元数据；底层 option 的类型、值和边界保持共享。以下是合同建议，并非现有 JSON 已有字段。

| 字段／事件 | 建议元数据 | consumer 与来源 |
|---|---|---|
| `Zombies.onChange` | `{kind:"lookup",indexBase:1,targets:{"ZombieConfig.PopulationMultiplier":[2.5,1.6,1.2,0.65,0.15,0]}}` | 两个 consumer；S:2734–2739、B:756–761、defines.lua:65–73 |
| `ZombieRespawn.onChange` | `{kind:"lookup",indexBase:1,targets:{"ZombieConfig.RespawnHours":[16,72,216,0],"ZombieConfig.RespawnUnseenHours":[6,16,48,0],"ZombieConfig.RespawnMultiplier":[0.5,0.1,0.05,0]}}` | 两个 consumer；S:2740–2747、B:762–769；提取器已包含对应值，建议显式 indexBase |
| `ZombieMigrate.onChange` | `{kind:"boolean",target:"ZombieConfig.RedistributeHours",true:12,false:0}` | 两个 consumer；S:2751–2758、B:746–753；提取器已包含 |
| `StartMonth.onChange`／日数同步 | `{kind:"days-in-month",yearSource:"getFirstYear",monthBase:1,target:"StartDay",overflow:"clamp"}`；若消费方使用约束重算，保留现有 StartDay.constraint 即可 | 两个 consumer；S:2761–2774、B:391–404；原版由可见更新周期触发，不是显式 combo 回调 |
| 地图两个子项 | singlePlayer 的 `dimmedWhen:{field:"Map.AllowWorldMap",equals:false}`；`disabledWhen:[]` | 仅单人；B:59–98 |
| 35 项独立倍率 | singlePlayer 的 `dimmedWhen:{field:"MultiplierConfig.GlobalToggle",equals:true}`；`disabledWhen:[]` | 仅单人；B:53–57、83–98 |
| `MultiplierConfig.Global` | singlePlayer 的 `dimmedWhen:{field:"MultiplierConfig.GlobalToggle",equals:false}`；`disabledWhen:[]` | 仅单人；B:86–98 |
| `ZombieConfig.PopulationMultiplier` 快捷档位 | `onChange` 限定事件 `selectQuickChoice`：非 Custom 时 `Zombies=selectedIndex`；手输值不触发该反向赋值 | 仅单人快捷控件；B:279–285 |
| `WaterShut`／`ElecShut` | serverSettings `visible:false`；singlePlayer `visible:true`；开局派生事件另存 `onApply`，不要写成 onChange | S:2528；B:946–953 |
| 两个 Modifier | serverSettings `visible:true`；singlePlayer `visibleWhen:{context:"debug",equals:true}`，单人 `onApply` 时被区间随机结果覆盖 | B:584–589、946–953 |
| `Steam`／`SteamWorkshop` 页面 | `visibleWhen:{context:"steamModeActive",equals:true}`，保留原值 | S:2424、4161、4179 |
| 出生区“编辑”操作 | `disabledWhen` 为“无选中项，或 file 以 media 开头，或含 /，或含反斜杠，或不以 spawnpoints.lua 结尾” | S:617–629；这是实际禁用按钮条件，不是通用字段依赖 |

**server 的地图／技能倍率 `disabledWhen` 和 `dimmedWhen` 都应为空。** 若产品决定强化禁用，需另记 `origin:"product-policy"`，不能标记为官方 server 行为。按 UI 事件执行 onChange；初次导入、装载已有配置、应用整份预设时不要对每个字段顺次重放回调，否则可能覆写预设里独立设置的高级倍率。证据：S:2838–2856、B:773–790 直接设控件值，不主动调用上述回调。

最终元数据面向 server-settings，保留 `singlePlayerOnly:false`，隐藏 Water/Elec，并记录两份 Lua 及 defines 的 SHA256。单人灰字／快捷控件只在本文审计，不加入服务器禁用条件。`vanillaSupported`、模组隐藏、身份字段隐藏和 `officialOnly` 都是产品策略：原版仍显示 Mods（S:4184–4186），Map 允许手工地图名（S:1203–1213），SpawnPoint 没有“只允许官方坐标”的检查。JSON 单独标明 product-policy 来源。

## 自定义面板、存储与联动

| 面板 | 真实字段／资源 | 读取与写回 | 注意事项与源行 |
|---|---|---|---|
| SteamWorkshop | INI `WorkshopItems` | 按 `;` 分割，跳过空项；按列表顺序用 `;` 拼回，不加尾分号 | 工坊条目 ID，不是 Mod ID；S:1419–1434、1453–1461；手填验证不含分号并调用 isValidSteamID，S:1470–1471 |
| Mods | INI `Mods` | 分号分割，跳过空项；按列表顺序拼接 modID | S:822–848；原版页面非 Steam-only，S:4183–4186 |
| Map | INI `Map` | 分号分割，跳过空项；按用户列表顺序拼回 | S:1095–1119；上移／下移 S:1066–1084；禁止自动排序 |
| SpawnRegions | 区域 name/file 列表 + INI `SpawnPoint` | 先提交当前编辑行，再 clearSpawnRegions，按行调用 addSpawnRegion；SpawnPoint 文本直接 setValue | S:667–688；不应把区域列表编码进 SpawnPoint |
| 职业出生点 Page7 | 职业键 → points 数组 | loadSpawnPointsFile(region.file)；收集 professionType 对应 points 后 saveSpawnPointsFile | S:3750–3756、3872–3877；文件名／最终文件布局由 Java 管理，本次不臆测 |
| SandboxPresetPanel | 整个 Sandbox 对象 | resetToDefault 后 loadPresetFile 或 loadGameFile，回填 Sandbox 控件 | S:1787–1796；不覆盖 INI；S:4363–4367 |

定制页由 S:2484–2491 提前返回专用 panel；初始化 S:2922–2926 调用其 setSettings，保存 S:2890–2895 调用 settingsFromUI。静态 settings 数组为空不表示没有配置功能。

Map 面板收到 addedMod 时把该 Mod 新地图插在列表开头，按其返回顺序插入且避重；removedMod 时移除该 Mod 的所有匹配地图，再通知区域页（S:1122–1155）。地图添加／删除通知使区域页增删 `media/maps/<mapFolder>/spawnpoints.lua` 引用（S:646–664），增加时按完整 file 去重。不要把“所有原版面板都会自动联动”等概括当作证据：例如手填 Workshop ID 的 S:1486–1494 没有发送 addedWorkshopItem，而已安装条目按钮 S:1483 会发送。

### 官方地图与出生区的准确限制

Map 已安装列表明确加入 `Muldraugh, KY`（S:1161）。紧随其后的 S:1162–1163 **注释**说明 Riverside、Rosewood、West Point 不必加入，因为各自没有 objects.lua；这是开发者解释，不是禁止这三个名称的执行分支。实际代码继续列出所选 Mods 的地图（S:1164–1175），手填其他地图只检查非空、不含 `;`、不重复（S:1203–1213）。不能把注释转换成地图白名单。

SpawnRegions 的有效编辑条件（S:617–623）：

```text
selected != nil
AND NOT startsWith(file, "media")
AND NOT contains(file, "/")
AND NOT contains(file, "\\")
AND endsWith(file, "spawnpoints.lua")
```

`media` 是原代码的前缀字面量，并非只匹配 `media/`。因此官方地图引用 `media/maps/.../spawnpoints.lua` 会禁用编辑，部分 Mod 的 media 路径也受同一条件影响。该判断不检查文件实际存在，也不证明通过者可成功保存。提示分两类：media 前缀对应 tooltip1，其他不合法文件对应 tooltip2（S:624–629）。选中官方区域仍可删除区域（S:631、599–603）；name/file 编辑只写回字符串（S:448–460、677–686）。Page7 的编辑回调及保存函数没有再次执行同一 guard（S:606–612、3872–3877），所以这不是后端路径安全保障。

职业出生点手填为恰好三个逗号分量，逐个 `tonumber`；要求 x≥0、y≥0、0≤z≤7，存 `{posX=x,posY=y,posZ=z}`（S:3846–3858）。没有整数检查、地图边界检查，也没有 worldX/worldY 分块转换。不要套用其他 Build 的 300 格坐标换算。INI SpawnPoint 是另一项，源码只是文本输入及原样保存（S:532–544、688），不能把 Page7 验证直接归给 SpawnPoint。

面板实现建议：官方／模组 media 引用保留为只读引用；编辑内容前复制成实例管理的本地文件，给受控 ID，不接受任意宿主路径；后端单独验证文件名、路径和内容。复制及后端验证属于产品实现建议，并非这里已验证的官方行为。

## 类型、转换与默认值

| 底层类型 | 读入控件 | 保存 | 来源 |
|---|---|---|---|
| boolean | selected[1]=getValue() | selected[1] == true | S:2838–2887；B:773–814 |
| integer / double | getValueAsString() → 文本 | option:parse(text)，不是直接 setValue(text) | 同上；整数 onlyNumbers=true，浮点 false，S:5100–5114、5161–5181 |
| enum | selected=getValue() | setValue(selected)，**1-based**，不是 UI 文案或 0-based 索引 | 同上；选项文本来自同 Build 翻译，S:5104–5110、5165–5177 |
| string / text | getValue() → 文本 | 原样 setValue(text) | 同上；server text 为多行、最多 64 行，S:2569–2577 |

输入校验 S:149–164、B:107–121 主要改变错误边框；不能推断整个保存按钮会因所有无效字段而锁定。Java parse 对非法值的最终行为需要后续对象测试，不能用 Lua 调用本身证明。

开始日的月索引传入 daysInMonth 前减 1；UI 和存储仍为 1-based（S:2761–2774；B:391–404）。日期使用 `getFirstYear()`，没有从隐藏 StartYear 枚举推算年份。S:5169–5172 的 StartYear／StartDay 显示转换分支是通用辅助代码；StartYear 没有当前静态声明，所以不能据此添加可见年份控件。

单人开局 B:946–953 先保存控件，再 `randomWaterShut(WaterShut)`、`randomElectricityShut(ElecShut)` 写入两个 Modifier，最后 toLua。这是 **onApply 转换**，不是选择枚举时的 onChange；调试模式手填 Modifier 同样会被这一步覆盖。具体随机分布、立即／禁用的数字哨兵值不在这两份文件中，待同 Build 实测补齐。server 保存只见通用 parse，不见这两个随机函数调用。

### 全部 32 组 advancedCombo 快捷转换

仅单人 consumer 使用。下表值按源码档位顺序；每个值前缀为翻译键，等号后是送入数字输入框的文本或外部表达式。`quickDefault` 是快捷控件声明值，不是 option 默认值：例如 FoodLootNew 的对象默认 0.8，不等于 quickDefault=4 的 0.4（options-CN.json；S:4537–4548）。不要把它当作新房间默认规则。

普通模式显示 combo，高级模式显示 entry（B:288–299）。Custom 是追加的最后一项，选择它不会改 entry；简易档位写 `tostring(value.text)`（B:268–285）。回填和切回普通模式按**字符串完全相等**匹配，否则显示 Custom（B:294–297、346–353）；例如 `2` 与 `2.0` 可能显示不同档位，但均按数字 parse 保存。高级数字值始终是保存真值，不保存快捷项序号。

| 字段 | S 行 | quickDefault | 顺序映射（翻译键 → 文本／表达式） |
|---|---:|---:|---|
| `ZombieLore.SprinterPercentage` | 4388 | 4 | `Sandbox_Insane=100`; `Sandbox_VeryHigh=90`; `Sandbox_High=50`; `Sandbox_Normal=33`; `Sandbox_Low=6`; `Sandbox_None=0` |
| `ZombieLore.DoorOpeningPercentage` | 4406 | 4 | `Sandbox_Insane=100`; `Sandbox_VeryHigh=90`; `Sandbox_High=50`; `Sandbox_Normal=33`; `Sandbox_Low=6`; `Sandbox_None=0` |
| `ZombieConfig.PopulationMultiplier` | 4437 | 4 | `Sandbox_Insane=ZombiePopulationMultiplier.Insane`; `Sandbox_VeryHigh=ZombiePopulationMultiplier.VeryHigh`; `Sandbox_High=ZombiePopulationMultiplier.High`; `Sandbox_Normal=ZombiePopulationMultiplier.Normal`; `Sandbox_Low=ZombiePopulationMultiplier.Low`; `Sandbox_None=ZombiePopulationMultiplier.None` |
| `ZombieConfig.PopulationStartMultiplier` | 4449 | 4 | `Sandbox_Insane=ZombiePopulationStartMultiplier.Insane`; `Sandbox_VeryHigh=ZombiePopulationStartMultiplier.VeryHigh`; `Sandbox_High=ZombiePopulationStartMultiplier.High`; `Sandbox_Normal=ZombiePopulationStartMultiplier.Normal`; `Sandbox_Low=ZombiePopulationStartMultiplier.Low`; `Sandbox_None=ZombiePopulationStartMultiplier.None` |
| `ZombieConfig.PopulationPeakMultiplier` | 4461 | 4 | `Sandbox_Insane=ZombiePopulationPeakMultiplier.Insane`; `Sandbox_VeryHigh=ZombiePopulationPeakMultiplier.VeryHigh`; `Sandbox_High=ZombiePopulationPeakMultiplier.High`; `Sandbox_Normal=ZombiePopulationPeakMultiplier.Normal`; `Sandbox_Low=ZombiePopulationPeakMultiplier.Low`; `Sandbox_None=ZombiePopulationPeakMultiplier.None` |
| `MaximumLooted` | 4496 | 4 | `Sandbox_Insane=200`; `Sandbox_VeryHigh=150`; `Sandbox_High=100`; `Sandbox_Normal=50`; `Sandbox_Low=25`; `Sandbox_None=0` |
| `RuralLooted` | 4509 | 4 | `Sandbox_Insane=2.0`; `Sandbox_VeryHigh=1.5`; `Sandbox_High=1.0`; `Sandbox_Normal=0.5`; `Sandbox_Low=0.25`; `Sandbox_None=0.0` |
| `MaximumDiminishedLoot` | 4521 | 4 | `Sandbox_Insane=100`; `Sandbox_VeryHigh=80`; `Sandbox_High=60`; `Sandbox_Medium=40`; `Sandbox_Low=20`; `Sandbox_None=0` |
| `FoodLootNew` | 4537 | 4 | `Sandbox_None=0.0`; `Sandbox_Insane=0.05`; `Sandbox_ExtremelyRare=0.2`; `Sandbox_Rare=0.4`; `Sandbox_Normal=0.6`; `Sandbox_Common=2`; `Sandbox_Abundant=3` |
| `CannedFoodLootNew` | 4551 | 4 | `Sandbox_None=0.0`; `Sandbox_Insane=0.05`; `Sandbox_ExtremelyRare=0.2`; `Sandbox_Rare=0.4`; `Sandbox_Normal=0.6`; `Sandbox_Common=2`; `Sandbox_Abundant=3` |
| `WeaponLootNew` | 4565 | 4 | `Sandbox_None=0.0`; `Sandbox_Insane=0.05`; `Sandbox_ExtremelyRare=0.2`; `Sandbox_Rare=0.4`; `Sandbox_Normal=0.6`; `Sandbox_Common=2`; `Sandbox_Abundant=3` |
| `RangedWeaponLootNew` | 4579 | 4 | `Sandbox_None=0.0`; `Sandbox_Insane=0.05`; `Sandbox_ExtremelyRare=0.2`; `Sandbox_Rare=0.4`; `Sandbox_Normal=0.6`; `Sandbox_Common=2`; `Sandbox_Abundant=3` |
| `AmmoLootNew` | 4593 | 4 | `Sandbox_None=0.0`; `Sandbox_Insane=0.05`; `Sandbox_ExtremelyRare=0.2`; `Sandbox_Rare=0.4`; `Sandbox_Normal=0.6`; `Sandbox_Common=2`; `Sandbox_Abundant=3` |
| `MedicalLootNew` | 4607 | 4 | `Sandbox_None=0.0`; `Sandbox_Insane=0.05`; `Sandbox_ExtremelyRare=0.2`; `Sandbox_Rare=0.4`; `Sandbox_Normal=0.6`; `Sandbox_Common=2`; `Sandbox_Abundant=3` |
| `SurvivalGearsLootNew` | 4621 | 4 | `Sandbox_None=0.0`; `Sandbox_Insane=0.05`; `Sandbox_ExtremelyRare=0.2`; `Sandbox_Rare=0.4`; `Sandbox_Normal=0.6`; `Sandbox_Common=2`; `Sandbox_Abundant=3` |
| `MechanicsLootNew` | 4635 | 4 | `Sandbox_None=0.0`; `Sandbox_Insane=0.05`; `Sandbox_ExtremelyRare=0.2`; `Sandbox_Rare=0.4`; `Sandbox_Normal=0.6`; `Sandbox_Common=2`; `Sandbox_Abundant=3` |
| `SkillBookLoot` | 4649 | 4 | `Sandbox_None=0.0`; `Sandbox_Insane=0.05`; `Sandbox_ExtremelyRare=0.2`; `Sandbox_Rare=0.4`; `Sandbox_Normal=0.6`; `Sandbox_Common=2`; `Sandbox_Abundant=3` |
| `RecipeResourceLoot` | 4663 | 4 | `Sandbox_None=0.0`; `Sandbox_Insane=0.05`; `Sandbox_ExtremelyRare=0.2`; `Sandbox_Rare=0.4`; `Sandbox_Normal=0.6`; `Sandbox_Common=2`; `Sandbox_Abundant=3` |
| `LiteratureLootNew` | 4677 | 4 | `Sandbox_None=0.0`; `Sandbox_Insane=0.05`; `Sandbox_ExtremelyRare=0.2`; `Sandbox_Rare=0.4`; `Sandbox_Normal=0.6`; `Sandbox_Common=2`; `Sandbox_Abundant=3` |
| `ClothingLootNew` | 4691 | 4 | `Sandbox_None=0.0`; `Sandbox_Insane=0.05`; `Sandbox_ExtremelyRare=0.2`; `Sandbox_Rare=0.4`; `Sandbox_Normal=0.6`; `Sandbox_Common=2`; `Sandbox_Abundant=3` |
| `ContainerLootNew` | 4705 | 4 | `Sandbox_None=0.0`; `Sandbox_Insane=0.05`; `Sandbox_ExtremelyRare=0.2`; `Sandbox_Rare=0.4`; `Sandbox_Normal=0.6`; `Sandbox_Common=2`; `Sandbox_Abundant=3` |
| `KeyLootNew` | 4719 | 4 | `Sandbox_None=0.0`; `Sandbox_Insane=0.05`; `Sandbox_ExtremelyRare=0.2`; `Sandbox_Rare=0.4`; `Sandbox_Normal=0.6`; `Sandbox_Common=2`; `Sandbox_Abundant=3` |
| `MediaLootNew` | 4733 | 4 | `Sandbox_None=0.0`; `Sandbox_Insane=0.05`; `Sandbox_ExtremelyRare=0.2`; `Sandbox_Rare=0.4`; `Sandbox_Normal=0.6`; `Sandbox_Common=2`; `Sandbox_Abundant=3` |
| `MementoLootNew` | 4747 | 4 | `Sandbox_None=0.0`; `Sandbox_Insane=0.05`; `Sandbox_ExtremelyRare=0.2`; `Sandbox_Rare=0.4`; `Sandbox_Normal=0.6`; `Sandbox_Common=2`; `Sandbox_Abundant=3` |
| `CookwareLootNew` | 4761 | 4 | `Sandbox_None=0.0`; `Sandbox_Insane=0.05`; `Sandbox_ExtremelyRare=0.2`; `Sandbox_Rare=0.4`; `Sandbox_Normal=0.6`; `Sandbox_Common=2`; `Sandbox_Abundant=3` |
| `MaterialLootNew` | 4775 | 4 | `Sandbox_None=0.0`; `Sandbox_Insane=0.05`; `Sandbox_ExtremelyRare=0.2`; `Sandbox_Rare=0.4`; `Sandbox_Normal=0.6`; `Sandbox_Common=2`; `Sandbox_Abundant=3` |
| `FarmingLootNew` | 4789 | 4 | `Sandbox_None=0.0`; `Sandbox_Insane=0.05`; `Sandbox_ExtremelyRare=0.2`; `Sandbox_Rare=0.4`; `Sandbox_Normal=0.6`; `Sandbox_Common=2`; `Sandbox_Abundant=3` |
| `ToolLootNew` | 4803 | 4 | `Sandbox_None=0.0`; `Sandbox_Insane=0.05`; `Sandbox_ExtremelyRare=0.2`; `Sandbox_Rare=0.4`; `Sandbox_Normal=0.6`; `Sandbox_Common=2`; `Sandbox_Abundant=3` |
| `OtherLootNew` | 4817 | 4 | `Sandbox_None=0.0`; `Sandbox_Insane=0.05`; `Sandbox_ExtremelyRare=0.2`; `Sandbox_Rare=0.4`; `Sandbox_Normal=0.6`; `Sandbox_Common=2`; `Sandbox_Abundant=3` |
| `ZombiePopLootEffect` | 4837 | 2 | `Sandbox_None=0`; `Sandbox_Normal=10`; `Sandbox_Abundant=20` |
| `FuelStationGasMin` | 4865 | 1 | `Sandbox_FuelStationGas_option1=0.0`; `Sandbox_FuelStationGas_option2=0.01`; `Sandbox_FuelStationGas_option3=0.1`; `Sandbox_FuelStationGas_option4=0.5`; `Sandbox_FuelStationGas_option5=0.7`; `Sandbox_FuelStationGas_option6=0.8`; `Sandbox_FuelStationGas_option7=0.9`; `Sandbox_FuelStationGas_option8=1.0` |
| `FuelStationGasMax` | 4880 | 5 | `Sandbox_FuelStationGas_option1=0.0`; `Sandbox_FuelStationGas_option2=0.01`; `Sandbox_FuelStationGas_option3=0.1`; `Sandbox_FuelStationGas_option4=0.5`; `Sandbox_FuelStationGas_option5=0.7`; `Sandbox_FuelStationGas_option6=0.8`; `Sandbox_FuelStationGas_option7=0.9`; `Sandbox_FuelStationGas_option8=1.0` |

同 Build defines.lua:65–89 已补齐这些定义：PopulationMultiplier `[2.5,1.6,1.2,0.65,0.15,0]`；PopulationStartMultiplier 与 PopulationPeakMultiplier 均为 `[4,2,1.5,1,0.5,0]`。原生服务器表单只应用实际存在的 Zombies → PopulationMultiplier 联动，不加入单人快捷控件。

## 导出范围

完整字段、类型、边界、默认值与中英文枚举写在 `apps/api/internal/provider/projectzomboid/data/options.json`，不在本文重复维护第二份列表。当前静态映射与对象导出逐项核对：

| 范围 | 对象字段 | 静态映射 | 原生服务器表单创建 |
|---|---:|---:|---:|
| 服务器 | 144 | 99 | 99，另有四个自定义面板字段 |
| 沙盒 | 269 | 257 | 255，跳过 WaterShut/ElecShut |

产品的 vanilla 可见策略另扣除身份字段和模组字段，加入官方地图／出生点编辑字段后，可见 99 server + 255 sandbox。`visible` 面向服务器管理；不是底层值是否有效的标记。`hiddenReason` 区分原版控件缺失、身份字段和 vanilla 产品策略。

### 对象存在但未在静态 Sandbox 表声明的全部 12 项

这些选项既不因“未映射”自动变成单人专属，也不应全部塞入高级表单。没有静态声明行；存在证据为 options-CN.json 的对应 name，consumer 只按声明创建控件（S:2519–2592；B:660–688）。

| 字段 | 类型 | 对象默认 | 数值范围 |
|---|---|---|---|
| `StartYear` | enum | `1` | 1.0…100.0 |
| `AlarmDecayModifier` | integer | `14` | -1.0…2147483647.0 |
| `InsaneLootFactor` | double | `0.05` | 0.0…0.2 |
| `ExtremeLootFactor` | double | `0.2` | 0.05…0.6 |
| `RareLootFactor` | double | `0.6` | 0.2…1.0 |
| `NormalLootFactor` | double | `1.0` | 0.6…2.0 |
| `CommonLootFactor` | double | `2.0` | 1.0…3.0 |
| `AbundantLootFactor` | double | `3.0` | 2.0…4.0 |
| `Farming` | enum | `3` | 1.0…5.0 |
| `PlantAbundance` | enum | `3` | 1.0…5.0 |
| `NightLength` | enum | `3` | 1.0…5.0 |
| `AnimalMetaStatsModifier` | enum | `4` | 1.0…6.0 |

动态 Mod 选项是 S:5134–5145 的独立扩展路径；当前清单限定原版对象，不承诺覆盖 Mod 运行后的全集。原版 server 静态字段为 99 项，四个定制字段另行覆盖 `WorkshopItems`、`Mods`、`Map`、`SpawnPoint`，因此 45 项静态未映射不能全算隐藏；去掉这四项，仍有 41 项未由本次原生表单／专用字段编辑器直接覆盖。缺少 UI 覆盖不证明缺少运行时作用。

## 注释、辅助分支与证据强弱

| 内容 | 状态 | 审计处理 |
|---|---|---|
| S:4491 `LootRespawn, singlePlayerOnly=true` | 整行注释，且当前导出对象没有 LootRespawn | 不计字段、不添加单人过滤、不复活旧功能 |
| S:4495 `MaximumLooted` 普通声明 | 注释 | 后面 S:4496 的 advancedCombo 才是有效声明，只计一次 |
| S:4535 `UniqueHomeVHS` | 注释 | 不计字段 |
| S:1162–1163 三个城镇说明 | 注释 | 保留为解释；地图选择真正执行的是 1161 与 1164–1175 |
| S:2495、2602 groupBox 辅助路径 | 代码存在，当前 SettingsTable 无 groupBox 声明 | 不添加折叠分组开关；B:467 的高级模式确实启用，两者不要混淆 |
| S:2562–2568 StartYear spinbox；5169–5170 显示标签 | 辅助分支存在，当前无 StartYear 控件声明 | 不将年份控件计入有效 257 项 |
| B:83–98 灰字条件 | 生效绘制代码 | 只记录单人视觉依赖，不声称 disable 或运行时忽略 |
| S:617–623 编辑按钮 setEnable | 生效交互限制 | 可记录操作级 disabledWhen；不是后端授权／路径边界 |

## 官方预设与默认构造函数

完整安装位于 `um773:/var/tmp/gpl-pz-42.21-build-25485538/server`，manifest 为 App 380870 / Build 25485538 / StateFlags 4。旧 `/tmp/gpl-pz-research.EazJDp` 只保留导出及编译器。游戏 jar SHA256 为 `e1a69eb743ede60b213a0fe7f8b83d4fcab773036d256cc4543a336f3b058a33`，与旧审计归档一致。

同 Build `SandboxOptions.<init>()` 字节码偏移 3032–3056：先 RunLua(`lua/shared/defines.lua`)，再 `loadGameFile("Apocalypse")`，最后 `setDefaultsToCurrentValues()`。`ServerSettings.resetToDefault()` 创建新的 SandboxOptions，所以新建默认已经来自 Apocalypse。`getFirstYear()` 字节码为 `sipush 1993; ireturn`，对象实际也返回 1993。

服务器预设页先 clear 再按 Apocalypse、Outbreak、Extinction、Rising、SixMonthsLater 添加（S:1806–1813）；随包 `ISScrollingListBox.lua:340–342` clear 把 selected 设为 1，所以初选 Apocalypse。单人 B:549–550 也显式选中 Apocalypse。选择列表项与点击“应用”是两个动作，后者才执行 reset/load。

预设源文件位于 `media/lua/shared/Sandbox/<id>.lua`，默认总表为同目录 `SandboxVars.lua`。SixMonthsLater 仍为旧 VERSION=5 且只包含部分字段，必须通过官方 loader 完成转换与默认继承。

| 官方预设 | 中文 | 全量有效值 | 与 reset 后默认不同的字段数 |
|---|---|---:|---:|
| Apocalypse | 启示录 | 269 | 0 |
| Outbreak | 爆发 | 269 | 69 |
| Extinction | 绝灭 | 269 | 77 |
| Rising | 复兴 | 269 | 63 |
| SixMonthsLater | 六个月后 | 269 | 37 |

每项都验证 reset 恢复、loadGameFile 返回 true、重置后重复加载一致。五份完整映射各不相同。Apocalypse 与默认相同是实际 loader 结果。身份 ResetID、ServerPlayerID 不属于沙盒预设；server 元数据将 defaultValue 设为 null、generated=true、visible=false，也移除了 tooltip 中随机默认值后缀。

## INI 文本写入核验

同 Build jar 中 `ServerOptions.saveServerTextFile` 将选项交给 `ConfigFile.write`，后者把 `getValueAsString()` 原样拼成 name=value。`TextServerOption` 继承 StringConfigOption，没有单独转义；text 仅代表多行控件。`ConfigFile.read` 逐行读取、trim 整行、按等号分割取第二段，没有通用引号、反斜杠或多行解码协议。实际对象 roundtrip：

| PublicDescription 输入 | 官方读回 |
|---|---|
| 字面反斜杠加 n | 原样保留，不变成换行 |
| 实际 LF：first 后接 PVP=false | 两个配置项 PublicDescription=first、PVP=false |
| left=middle=right | 仅 left |
| 首尾带空格的 padded | 前导空格保留，末尾空格丢失 |

产品应拒绝 INI string/text 的实际 CR、LF、NUL 和等号，以避免注入和截断；这是产品策略，不能称为官方 escape 格式。欢迎消息可使用游戏的 `<LINE>` 显示标记。help 中换行只用于展示，不能用于 INI 渲染。metadata.serialization 的 semicolon-separated 只描述 Mods／WorkshopItems／Map 列表；通用 INI 规则在 source.ini。

## 官方地图与出生区域

Map 原生非模组列表只有 `Muldraugh, KY`（S:1161）。vanilla 产品据此设置 Map.allowedValues，并标明 officialOnlyOrigin=product-policy，不声称原生禁止其他手工地图名。

随包存在 11 个官方 spawnpoints.lua，`SpawnRegions.getDefaultServerRegions()` 实际也返回这 11 个区域。默认顺序为 West Point、Rosewood、Echo Creek、Ekron、Valley Station、March Ridge、Irvington、Muldraugh、Fallas Lake、Riverside、Brandenburg（均为 `, KY`）。JSON 的 SpawnRegions.officialRegions 提供受控原生相对路径，defaultRegions 保留对象实际顺序；不能套旧版四区域列表，也不能暴露任意宿主路径。

## 提取命令与验证

生成可复现的临时 JVM 提取器：

```sh
python3 scripts/extract-project-zomboid-config.py --emit-java /private/tmp/ExportPresets.java
```

本次使用 um773 的 debian:bookworm-slim 临时容器，`--network none --read-only`；官方目录只读挂载 /game，旧审计 ecj.jar 只读挂载 /ecj.jar；/research、/tmp、/root 使用 tmpfs。通过标准输入传入 Java，不在宿主创建游戏副本。cwd=/game，核心命令：

```sh
./jre64/bin/java -jar /ecj.jar -21 -d /research /research/ExportPresets.java
./jre64/bin/java -Duser.home=/research/home -Djava.awt.headless=true -Djava.library.path=linux64 -cp java/projectzomboid.jar:/research ExportPresets /research/presets-runtime.json
```

fixture 经 stdout 取回 `/private/tmp/gpl-pz-config-audit-20261028/presets-runtime.json`，纯文本 Lua／manifest 取回 `/private/tmp/gpl-pz-export-source`。以下提取及离线检查已实际执行：

```sh
python3 scripts/extract-project-zomboid-config.py --audit-dir /private/tmp/gpl-pz-config-audit-20261028 --runtime /private/tmp/gpl-pz-config-audit-20261028/presets-runtime.json --source-dir /private/tmp/gpl-pz-export-source
python3 scripts/extract-project-zomboid-config.py --check apps/api/internal/provider/projectzomboid/data/options.json
```

输出 source.status=verified：144 server、269 sandbox、五个各 269 项的完整预设、22 server pages、10 sandbox pages、98 枚举／644 个选值。vanilla 可见 99 server + 255 sandbox，其他字段仍保留元数据。源 SHA256 写入 source.sha256，没有将游戏 binary 放入仓库。

尚未进行的游戏实测：

1. Steam／非 Steam、单人 debug／非 debug、地图和技能灰字的实际交互、单人快捷／高级模式。
2. 随机停水停电的运行结果、真实服务器生命周期与客户端加入。
3. 出生点编辑、区域增删、后端路径隔离。UI 的字符串检查不能代替后端验证。

## 本次验证结果

已验证：两份 Lua 的完整标记搜索、关键执行分支及调用路径、源哈希；257 项有效声明与 inventory 的集合完全相等且无重复；269−257=12 项静态未映射；32 组 advancedCombo；两种 consumer 各自 255 个非调试沙盒控件的条件推导。数量是静态代码推导，不是运行截图计数。所有选项类型、边界和 enum 序号来自既有 exact-build 导出。

另已验证对象预设加载、默认出生区、INI roundtrip、字段唯一性、全量键集、真实类型、范围、字符串长度、连续枚举索引、五预设不同、身份清理与 Java 源码重建。错误类型／越界／过长 fixture 及 pending 状态均被拒绝。Go／前端测试不属于本数据任务，未运行；不将配置对象执行称为游戏 UI 或服务器启动测试。没有修改 Provider/UI/runtime、现有房间、生产或 release，未执行 Git 提交。

官方 [SandboxOptions API](https://projectzomboid.com/modding/zombie/SandboxOptions.html) 辅助确认方法签名；实际值和版本结论以随包 jar、Lua、manifest 及本次对象执行为准。

## 最终集成证据补记（2026-10-28）

1. **实际 Provider 输出已通过官方服务器 loader 回读。** 据父任务回传，Provider 生成的默认配置及 Apocalypse、Outbreak、Extinction、Rising、SixMonthsLater 五个预设文件，均已在 um773 使用同 Build 25485538 官方类、通过 `PzReadServerLua.java` 调用 `SandboxOptions.readLuaFile` 回读，再用 `compare-pz-readback.py` 逐项比较完整键集、底层类型和值；六组各 269 项全部通过（6 × 269 typed PASS）。这次输入是实际 Provider 生成的服务器 Lua，不只检查字符串或原版预设源文件。

   服务器文件必须写大写 `VERSION=6`。本任务的最小官方对象负例已证明：同样输入 DayLength=4、CarSpawnRate=3、RecentlySurvivorVehicles=2，写 `Version=6` 时 loader 虽返回 true，却读回 4、4、3；写 `VERSION=6` 时保持 4、3、2。原因是服务器读取路径只查找 `VERSION`，缺失时按版本 0 执行旧配置迁移。Provider 已修复大小写，Go 回归断言也已禁止错误的 `Version`。

2. **Java trim 导致的 INI 密码差异已修复。** 官方 `ConfigFile.read` 对整行调用 Java `String.trim()`，会去除尾部所有不大于 U+0020 的字符；Go `unicode.IsSpace` 不覆盖其中全部控制字符。官方对象实测将 `Password` 值 `join\u0001`（末尾为实际 U+0001）写入后读回 `join`，长度为 4，证明仅检查 Unicode 空白不足以保证密码 roundtrip。据父任务回传，INI 文本校验已补充拒绝 ASCII `<0x20` 控制字符，相关回归测试通过。

本补记仅记录配置文件回读与校验修复结果。um773 面板部署、真实服务器生命周期和客户端加入由父任务继续验证，不计入上述通过结论。
