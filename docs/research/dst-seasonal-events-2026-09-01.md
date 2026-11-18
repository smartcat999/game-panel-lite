# Don't Starve Together 季节活动配置调研（2026-09-01）

## 结论

当前 DST 专用服务器支持 13 个可配置活动。产品界面不应把它建模为“13 选 1”，而应建模为：

- 一个“跟随官方当前活动”的总开关；
- 13 个可独立开启的“强制活动”开关；
- 强制活动支持同时启用多个。

这些配置属于 `World Settings`，不是只能在首次生成地图时使用的 `World Generation` 选项。已有世界可以修改，无需重建；专用服务器应持久化到 `leveldataoverride.lua`，然后完整重启 Master/Caves 分片使其重新加载。

## 证据范围

本结论交叉核对了以下一手来源：

1. Klei 官方 DST Dedicated Server（Steam App `343050`）当前发布包中的 `data/databundles/scripts.zip`，生产环境所用脚本与项目清单均对应游戏构建 `740477`。Klei 官方 Dedicated Server 指南说明了集群和分片配置目录；官方 Steam 公告确认当前公开构建为 `740477`。  
   来源：[Klei Dedicated Server Command Line Options Guide](https://support.klei.com/hc/en-us/articles/360029556192-Dedicated-Server-Command-Line-Options-Guide)、[DST 官方公告：Hotfix 740477](https://steamcommunity.com/app/322330/announcements/)
2. 官方脚本 `scripts/constants.lua`：定义 `SPECIAL_EVENTS`、`WORLD_SPECIAL_EVENT`、`WORLD_EXTRA_EVENTS` 和 `GetAllActiveEvents()`。
3. 官方脚本 `scripts/map/customize.lua`：将活动列在 `WORLDSETTINGS_GROUP.events`；`specialevent` 仅有 `none/default`，每个独立活动仅有 `default/enabled`。
4. 官方脚本 `scripts/gamelogic.lua`：加载世界时先应用 `specialevent`，再遍历全部 `SPECIAL_EVENTS`，对每个值为 `enabled` 的覆盖项调用 `ApplyExtraEvent()`。
5. 官方脚本 `scripts/components/specialeventsetup.lua`：比较上一次保存的活动集合和本次加载的活动集合，在已有世界中执行新增活动的 setup、移除活动的 shutdown，并保存当前活动集合。

以上脚本可通过 Klei 官方 Dedicated Server 包公开下载复核；这里不使用第三方 Wiki 或非官方脚本镜像作为结论依据。

## 当前活动及配置字段

下表中的内部值来自 Klei 官方构建 `740477` 的 `SPECIAL_EVENTS` 和 `customize.lua`；“官方活动公告”用于逐项确认活动名称或其官方发布记录。

| 中文名称 | `leveldataoverride.lua` 字段 / `SPECIAL_EVENTS` 值 | 一手来源 |
|---|---|---|
| 盛夏鸦年华 | `crow_carnival` | [Klei/DST 官方 2026 盛夏鸦年华公告](https://steamcommunity.com/games/322330/announcements/detail/716782947012706692) |
| 万圣夜 | `hallowed_nights` | [Klei/DST 官方 Hallowed Nights 公告](https://steamcommunity.com/ogg/322330/announcements/detail/4551542220586216359) |
| 冬季盛宴 | `winters_feast` | [Klei/DST 官方公告归档（Winter's Feast）](https://steamcommunity.com/app/322330/announcements/) |
| 火鸡之年 | `year_of_the_gobbler` | [Klei/DST 官方公告归档（Year of the Gobbler）](https://steamcommunity.com/app/322330/announcements/) |
| 座狼之年 | `year_of_the_varg` | [Klei/DST 官方 Year of the Varg 公告](https://steamcommunity.com/ogg/322330/announcements/detail/1652128636342506973) |
| 猪王之年 | `year_of_the_pig` | [Klei/DST 官方 Year of the Pig King 公告](https://steamcommunity.com/games/322330/announcements/detail/1692692744031596720) |
| 胡萝卜鼠之年 | `year_of_the_carrat` | [Klei/DST 官方 Year of the Carrat 公告](https://steamcommunity.com/ogg/322330/announcements/detail/3583111098707078000) |
| 皮弗娄牛之年 | `year_of_the_beefalo` | [Klei/DST 官方 Year of the Beefalo 公告](https://steamcommunity.com/games/322330/announcements/detail/3035957920295179497) |
| 浣猫之年 | `year_of_the_catcoon` | [Klei/DST 官方 Year of the Catcoon 公告](https://steamcommunity.com/games/322330/announcements/detail/3114803256552345240) |
| 兔人之年 | `year_of_the_bunnyman` | [Klei/DST 官方 Year of the Bunnyman 公告](https://steamcommunity.com/ogg/322330/announcements/detail/3632750190487731178) |
| 龙蝇之年 | `year_of_the_dragonfly` | [Klei/DST 官方 Year of the Dragonfly 公告](https://steamcommunity.com/ogg/322330/announcements/detail/4023472338859612609) |
| 洞穴蠕虫之年 | `year_of_the_snake` | [Klei/DST 官方 Year of the Depths Worm 公告](https://steamcommunity.com/ogg/322330/announcements/detail/506192738774941991) |
| 发条骑士之年 | `year_of_the_knight` | [Klei/DST 官方 Year of the Clockwork Knight 公告](https://steamcommunity.com/ogg/322330/announcements/detail/516360716930777222) |

注意：`year_of_the_snake` 是内部字段名，当前官方用户可见名称是 **Year of the Depths Worm / 洞穴蠕虫之年**。不要在界面上把它直译为“蛇年”。

Forge（熔炉）和 Gorge/Quagmire（暴食）不在上述清单内。官方脚本把它们归入单独的 `FESTIVAL_EVENTS` 旧游戏模式，不属于普通生存世界的这组季节活动开关。

## 配置字段与推荐写法

活动配置写入地面分片的 `leveldataoverride.lua`：

```lua
return {
  desc = "The standard Don't Starve Together experience.",
  hideminimap = false,
  id = "SURVIVAL_TOGETHER",
  location = "forest",
  max_playlist_position = 999,
  min_playlist_position = 0,
  name = "Default",
  numrandom_set_pieces = 4,
  override_level_string = false,
  overrides = {
    specialevent = "none",
    hallowed_nights = "enabled",
    winters_feast = "enabled",
    year_of_the_snake = "enabled",
    year_of_the_knight = "enabled",
  },
  playstyle = "survival",
  random_set_pieces = {
    "Sculptures_1",
    "Sculptures_2",
    "Sculptures_3",
    "Sculptures_4",
    "Sculptures_5",
  },
  required_prefabs = { "multiplayer_portal" },
  required_setpieces = { "Sculptures_1", "Maxwell5" },
  substitutes = {},
  version = 4,
  worldgen_desc = "The standard Don't Starve Together experience.",
  worldgen_id = "SURVIVAL_TOGETHER",
  worldgen_name = "Default",
}
```

面板只需要维护 `overrides` 中的活动字段，不应重写用户其他世界配置。

### 三种常用组合

```lua
-- 跟随官方活动，并额外强制开启多个活动
specialevent = "default",
hallowed_nights = "enabled",
winters_feast = "enabled",

-- 不跟随官方活动，只启用指定的多个活动
specialevent = "none",
hallowed_nights = "enabled",
winters_feast = "enabled",

-- 全部关闭
specialevent = "none",
-- 其余 13 个字段均为 "default" 或不写
```

## 自动活动与强制活动的区别

### `specialevent = "default"`

保留游戏构建中 Klei 指定的 `WORLD_SPECIAL_EVENT`。它不是服务器按本地日期自行计算，也不是从网络日历实时获取；服务器升级到新的官方游戏构建并重启后，才会跟随该构建中指定的当前活动。构建 `740477` 的官方脚本中当前值为 `crow_carnival`。

### `specialevent = "none"`

关闭构建内置的自动/主活动。它不会关闭各个独立字段中显式设置为 `enabled` 的附加活动。

### `<event> = "enabled"`

把对应活动加入 `WORLD_EXTRA_EVENTS`，不受官方当前活动周期影响。恢复为 `default` 后，在下一次完整加载世界时取消该强制活动。

## 是否支持多选

支持。

这不是推测：官方脚本把 `WORLD_EXTRA_EVENTS` 定义为集合；`gamelogic.lua` 遍历所有活动字段，把每个 `enabled` 项加入该集合；`GetAllActiveEvents()` 再把一个主活动与任意数量的附加活动合并。因此：

- 可同时强制开启多个活动；
- 可以“自动活动 + 多个强制活动”并存；
- 多个 Year of the ... 活动也可同时开启。

活动内容可能在配方、神龛、掉落物或世界实体上互相影响，Klei 脚本允许组合不等于所有组合都经过官方重点平衡测试。产品可以显示轻量提示，但不应禁止多选。

## 创建世界时与已有世界的差异

### 创建世界时

在首次生成前写入 `leveldataoverride.lua`，活动会从第一次加载开始生效。活动需要的世界初始化和 setup 会在首次加载时执行。

### 世界生成后

也支持修改。活动位于官方 `WORLDSETTINGS_GROUP`，不是 `WORLDGEN_GROUP`；官方脚本在每次加载保存世界时读取 overrides，比较上一次活动集合与新的活动集合，并执行新增/关闭活动的 setup/shutdown。

推荐流程：

1. 停止 Master 和 Caves；
2. 修改 Master 的 `leveldataoverride.lua` 活动 overrides；
3. 保留世界存档，不执行重新生成；
4. 完整启动 Master 和 Caves；
5. 从日志确认 `[Special Event] Setting up ...` 或 `Shutting down ...`。

Klei 官方曾明确把 World Settings 描述为可在世界生成后调整的一组设置；这与当前脚本实现一致。  
来源：[Klei/DST 官方 Quality of Life / World Settings 公告](https://store.steampowered.com/news/posts/?appids=322330&enddate=1617410559&feed=steam_community_announcements)

少数活动会向已有世界补充实体或一次性执行 setup；停用活动不会承诺删除玩家已经制作、拾取或放置的全部活动物品。因此“关闭活动”和“清理所有活动遗留物”不是同一个操作。

## 控制台命令

官方脚本存在以下全局函数：

```lua
ApplySpecialEvent(SPECIAL_EVENTS.HALLOWED_NIGHTS)
ApplyExtraEvent(SPECIAL_EVENTS.WINTERS_FEAST)
```

但它们是内部脚本接口，不应作为 GamePanel 的持久配置方案：

- 只修改当前进程内存，重启后丢失；
- `ApplyExtraEvent()` 没有对称的持久禁用命令；
- 活动资源和 backend prefabs 在加载世界时预载；
- `specialeventsetup` 的完整 setup/shutdown 流程发生在世界加载阶段。

因此面板应使用 `leveldataoverride.lua` + 完整重启，并在 UI 上显示“保存后重启生效”。不要提供“即时执行控制台命令即可永久切换”的误导性操作。

## 对 GamePanel Lite 的实现建议

当前仓库的 `apps/api/internal/provider/dst/dst_world_options.json` 已包含构建 `740477` 的 13 个独立活动字段和 `specialevent`，后端配置能力基本具备。需要补的是产品表达：

1. 在“游戏规则与房间设置”增加“季节活动”区域。
2. 顶部提供“跟随官方活动”开关：
   - 开：`specialevent = "default"`
   - 关：`specialevent = "none"`
3. 下方提供 13 个独立复选框，允许多选：
   - 选中：`enabled`
   - 未选：`default`
4. 文案使用“跟随官方当前活动”，不要写“自动按日期切换”。
5. 保存后标记服务器配置待应用，并提示重启生效；不要重新生成世界。
6. 仅 Master 展示和保存，避免用户误以为 Master/Caves 要分别选择；这些字段在官方定义中是 `master_controlled`。

