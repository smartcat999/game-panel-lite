# 七日杀配置完整性与交互审计

调研日期：2026-10-27。目标为项目已部署的 **v3.3.0 / Steam Build 25661908**，代码基线 `cd0510d3f24b9200ab4238a68ee482a21bd2e1da`。本文区分游戏模板事实、当前实现和产品建议；后者均未实施。

## 结论

1. `GamePanelLite` 是面板自定的地图种子，不是游戏要求。建议新建随机地图时随机生成一次并保存，允许手动输入和重新随机；已有服务器的种子不随启动、升级、语言切换变化。
2. 种子和尺寸只对 `RWG` 随机地图生效。固定地图不应展示这两个可编辑参数。当前面板的 4K/6K/8K 与目标镜像模板声明的 6K/8K/10K 不一致；4K 的实际引擎接受情况未测试，不能据此说必定崩溃。
3. 沙盒代码有第三方网页工具，不必限定为只能在游戏内获取。官方说明仍是游戏内配置并复制；本次没有发现官方网页生成器或账户申领入口。工具外链应标明第三方和版本。
4. 当前 UI 的不足不等于服务端缺少所有默认参数。面板开放 11 个游戏字段，另有通用端口；其余主要继承游戏 XML 模板。应先修正误导和联动，再补常用性能、领地、地区参数。
5. 自动备份不能仅靠显示开关解决：当前提供方声明必须停服备份。在线备份需要内部控制通道、保存完成确认和一致性验证。

## 证据基线与来源

### 游戏自身的配置定义（第一手资料）

通过 um773 已缓存的目标镜像，以禁网、只读容器读取 `/opt/seven-days/serverconfig.xml`；未启动游戏服务。模板包含 **68 个生效 property**，`UserDataFolder` 在模板中为注释；面板补入后通常为 69 项。文件 SHA-256：

`21dd90603e12c2aa88c73edf4d1c44ee1bc60358b94275259c4aa24970a8d879`

可复核的同一运行镜像引用：

`registry.cn-hangzhou.aliyuncs.com/gamepanel-lite/seven-days-to-die@sha256:d5bffabbe8d96f492d7d7c725692df9221cdd31bf82f20a3bee863284a62f3f5`

读取方式（镜像已在本地时）：

```sh
docker run --rm --pull never --network none --read-only \
  --entrypoint cat \
  registry.cn-hangzhou.aliyuncs.com/gamepanel-lite/seven-days-to-die@sha256:d5bffabbe8d96f492d7d7c725692df9221cdd31bf82f20a3bee863284a62f3f5 \
  /opt/seven-days/serverconfig.xml
```

下文关于模板默认值、字段范围、地图名称和参数风险的事实，均来自这份游戏原始模板；附录列出全部生效参数。网页旧教程出现不同尺寸或旧难度字段时，不覆盖本构建的定义。

### 当前实现

- [配置 schema](../../apps/api/internal/provider/sevendaystodie/config.go)：字段、文案、默认值和前端可用范围。
- [提供方](../../apps/api/internal/provider/sevendaystodie/provider.go)：验证、渲染、能力和存档身份保护。
- [运行时准备](../../docker/seven-days-to-die/prepare_runtime.py)与[配置合并](../../docker/seven-days-to-die/merge_config.py)：首次复制模板到实例目录，此后每次启动叠加面板托管字段，输出 `gamepanel-active.xml`。已有实例模板不一定自动等于新版镜像模板，升级时需单独比较默认参数差异。
- [配置编辑器](../../apps/web/components/provider-config-editor.tsx)、[高级参数分类](../../apps/web/lib/provider-config.ts)、[创建向导](../../apps/web/components/create-server-wizard.tsx)、[加入信息](../../apps/web/components/server-lobby-banner.tsx)。

### 官方公告与工具

- [V3.0 官方开发者公告（Steam 正文）](https://store.steampowered.com/news/posts/?appids=251570&enddate=1781236667&feed=steam_community_announcements)：玩法规则迁移至 SandboxCode、游戏内生成方法及默认 Adventurer 代码。
- [V3.1 官方公告入口](https://steamcommunity.com/games/251570/announcements/detail/695393383984663328)、[官方公告列表正文](https://steamcommunity.com/app/251570/announcements)：旧代码的规则兼容风险。单篇抓取未返回完整正文，研究核对的是列表中的 2026-07-27 条目。
- 第三方工具仅作为其自身功能的证据，详见沙盒章节；它们不替代官方游戏定义。

## 地图与种子的设计

| 项目 | 当前实现 / 游戏依据 | 建议 |
| --- | --- | --- |
| 地图类型 | 面板只允许 `Navezgane`、`RWG`。镜像实际另含四张 Pregen 地图。 | 开放固定原版、预生成、随机生成三种选择；底层使用明确白名单。 |
| 随机种子 | 面板 `GamePanelLite`；游戏模板 `MyGame`。模板说明仅 RWG 有效。 | 创建 RWG 时生成一次默认种子；输入框旁放重新随机按钮，不用固定品牌词。 |
| 随机尺寸 | UI/后端接受 4096、6144、8192；模板声明 6144、8192、10240。 | 用 6K、8K、10K 离散选项替代数字步进；默认 6K，结合资源说明 10K 成本，不默认选最大。 |
| 固定地图 | UI 仍显示种子和尺寸。 | 隐藏无效字段；切回 RWG 可保留本次尚未提交的值。固定地图导出的无效值应统一规范化，避免无意义差异。 |
| 存档名称 | `GameName` 命名存档，还影响树木等装饰的放置；不决定 RWG 基础布局。 | 与房间显示名称区分；不要把种子等同于存档名称，也不要承诺跨版本同种子必定完全相同。 |
| 内部标识 | 游戏模板注释允许 `[A-Za-z0-9_-. ]`；向导自动生成中文世界名，后端只做路径安全等校验。 | 新实例内部存档 ID 可用安全字符，界面名称独立本地化；先验证游戏 Unicode 行为，不批量重命名旧存档。 |
| 既有世界 | 后端已禁止修改地图、种子、尺寸、存档名称。 | 保留限制；需要新地图时新建实例，避免误选其他存档或重置。 |

镜像内实际目录核对为 `Navezgane`、`Pregen06k01`、`Pregen06k02`、`Pregen08k01`、`Pregen08k02`，另有 `Playtesting`、`Empty`。后两项不应作为普通玩家推荐地图。这里确认了文件存在，尚未逐张启动测试。导入自定义地图放后续，必须校验归档路径和文件结构。

随机种子是新实例的产品默认值建议，尚未验证游戏对空种子的处理，因此不能用“留空会自动随机”替代实际生成和持久化。

## 沙盒规则代码：获取入口与版本边界

官方路径是游戏内“新建游戏 → 沙盒选项”调整并复制代码，粘贴到 `SandboxCode`；不需要说明成必须先创建并游玩存档。准确按钮位置尚未亲自打开目标版本验证。官方 V3.0 公告介绍了这条配置路径；目标 **v3.3.0 模板本身也将 `AAAJABJACJADJARFBNC` 标为 Adventurer 默认**。因此保留当前默认有直接依据，但不能进一步声称已逐项核对它与当前客户端预设完全一致。

这是可分享的玩法配置字符串，不是账号凭证。本次未发现官方网页生成入口。第三方页面可以导入、编辑和生成：

| 网页 | 作者页面声明的功能 / 版本 | 本次建议 |
| --- | --- | --- |
| [7D2D.net / Supercraft](https://7d2d.net/7-days-to-die-sandbox-code-generator) | 生成、导入、预设、分享链接；列出 165 项，声明 3.3 兼容，页面已更新至 3.4。 | 可作为候选外链，明确第三方身份；尚未在 Build25661908 验证全部输出。 |
| [GHosting](https://ghosting.gg/tools/7-days-to-die-sandbox-code-generator) | 生成、解析、复制代码/XML/分享链接；自标 3.1 schema。 | 补充参考。它与其他工具对部分规则标签存在分歧，不作为确定的解码依据。 |
| [TopServer.Network](https://topserver.network/tools/7-days-to-die-server-settings-generator/) | 导入、生成及完整 XML 导出；声明核验 3.2。 | 不建议用它的整份 XML 覆盖面板托管配置。 |

[BoQsc 网页](https://boqsc.github.io/7dtd-sandbox-code-tools/)和[Labyricorn 网页](https://labyricorn.github.io/7D2D-Sandbox-Settings-Reports/)也存在，但作者项目面向 3.0。背景研究核对了 [BoQsc URL 读取源码](https://github.com/BoQsc/7dtd-sandbox-code-tools/blob/102f3fb60ed94c44439da0c29b71c113a5dfbbb4/index.html#L467-L493)以及 [Labyricorn URL 读取源码](https://github.com/Labyricorn/7D2D-Sandbox-Settings-Reports/blob/ddd238b9c64306ab76bed8f32807dea20f83006c/index.html#L4667-L4681)，支持 `?code=` 导入；未做浏览器端到端操作。Labyricorn 的[生成逻辑](https://github.com/Labyricorn/7D2D-Sandbox-Settings-Reports/blob/ddd238b9c64306ab76bed8f32807dea20f83006c/index.html#L3822-L3910)只输出已知项，存在丢失新版本未知项的风险，不适合作为 3.3 默认编辑器。

[官方论坛玩家分享区](https://community.thefunpimps.com/threads/share-your-sandbox-code.47498/)也能寻找规则代码，但玩家帖子不等于官方预设或版本兼容保证。

**交互建议：**一个代码输入框，旁边“获取方法”“网页工具 ↗”，网页入口明确“第三方”；允许粘贴和恢复目标版本默认。工具未验证前，不自动展示确定的完整规则解析。不要向外链附带密码、管理凭据或完整 XML；即便工具支持深链，也要核对参数协议后再使用。

**完整性建议：**难度、经验、掉落、昼夜、血月等属于新版沙盒规则，不应照旧教程追加已经迁移的独立 XML 参数。第一批保留外部生成与粘贴；内置玩法预设必须从相同版本实际导出。进一步内置常用规则编辑时，要有按构建验证的编解码映射、未知项保留策略和服务端生效值核验；不能靠正则验证大写字母便声称规则有效。

官方 V3.1 对旧代码的兼容风险给过明确提示，不能承诺所有 3.x 代码通用。第三方工具作者的兼容声明与面板验证结论要分开显示。详细规则数目不作为面板已实现的参数数量。

## 当前参数完整性与优先级

面板渲染 18 项：11 个游戏字段、通用端口，以及 6 个固定托管字段（其中 `UserDataFolder` 不在模板生效的 68 项中）。其余模板字段保留默认；未知 JSON 属性不会自动变成高级配置，提供方结构体规范化会丢弃它们。

| 优先级 | 调整 | 验收重点 |
| --- | --- | --- |
| P0 | 修快捷模板误匹配 | 当前七日杀没有专属分支，创建页模板文字回落为泰拉瑞亚描述，点击进入 Terraria 默认分支；应禁用不适用模板或添加七日杀模板，点击后不得切换游戏。 |
| P0 | 地图、种子、尺寸联动 | 加四张预生成地图；仅 RWG 显示种子和 6K/8K/10K；后端同样按地图和版本验证。 |
| P0 | 沙盒获取入口 | 保留当前模板默认，提供游戏内步骤和第三方网页；没有经过目标构建核验不显示“规则已验证”。 |
| P0 | 存档身份与名字 | 分清房间名称、存档内部 ID；兼容原有存档，不自动迁移或重置。 |
| P0 | 加入信息文案 | “无密码 (公开)”目前只看密码，可能与列表可见性冲突；改为“无密码”，可见性单独来自实际配置。好友可见需要首次 IP 直连，隐藏列表不等于禁止访问。 |
| P1 | 地区与语言 | 开放 Region / Language；模板默认 NorthAmericaEast / English。列表语言用英文语言名称，不与面板界面语言切换自动绑定。 |
| P1 | 性能与地图传输 | 僵尸/动物总量、视距、动态网格、下载地图限速；区分玩法密度与服务端全局性能上限。 |
| P1 | 玩家与领地规则 | 新生保护、好友附近出生、共享经验距离、领地数量/尺寸/过期/离线保护。给出单位和特殊值含义。 |
| P1 | 跨平台与管理 | 当前 Crossplay 固定关闭。开放时联动玩家数、EOS 制裁等目标模板约束，并验证客户端加入；不直接照搬旧版本教程的全部限制。 |
| P1 | 自动备份依赖 | 现有 Backups=true、BackupRequiresStop=true，未声明控制台能力。先实现并验证内部保存指令与完成确认，再设计在线自动备份；停服备份和游戏自动保存不是同一功能。 |
| P2 | 自定义地图、保留席位、管理文件、规则解析 | 按需求增加，保留路径隔离和权限边界，不将 68 项全部平铺。 |

性能参数的可用边界：模板视距 6–12；僵尸数量设置对性能有明显影响；较大的动态网格缓存提高 RAM 使用。数值输入给单位、范围和默认值，但只有游戏明确给出的范围才设为硬限制。面板 maxPlayers=1–64 是项目校验范围，不是本次验证的承载能力。

**不能作为普通清理选项：**`SaveDataLimit` 触及配额可能强制重置已存区块，`MaxChunkAge` 会按游戏日重置满足条件的区块。两者默认 -1；若开放应放入明确的区块重置设置，解释后果并确认，不能宣传成无损释放空间。

**不宜直接开放：**用户数据路径、宿主机路径、公开 Telnet 管理端口/密码；Windows 专用终端窗口；未测试的底层网络协议选项。控制台应由面板使用内部通道，不要求玩家为它开放公网管理端口。

## 页面安排建议

使用四个紧凑分组或二级标签：**房间、地图、玩法、性能与高级**；保持相同内容宽度和表单控件高度，不用折叠手风琴。

- 房间：名称、密码、人数、可见性；地区/语言紧邻，说明仅保留容易误解的含义。
- 地图：地图选择；RWG 时才出现种子与尺寸一行。已建实例用清晰的锁定状态，不以禁用的大片输入框填满页面。
- 玩法：沙盒代码与获取入口优先；通过目标版本验证后再加入少量常用玩法预设。详细规则编辑使用独立面板，避免长列表撑开详情页。
- 性能与高级：紧凑两列表单；动态网格关闭时隐藏依赖项；跨平台配置显示联动约束。常用性能上限和危险区块重置规则明确分开。
- 每项仅一处说明，避免标题、徽标、按钮重复表达状态。保留中英文文案，技术标识用于辅助信息。

## 验证范围

完成静态代码核对、um773 缓存镜像的只读模板及地图目录核对、官方公告与第三方工具第一手页面/作者源码检索。没有修改源码、生产配置或启动运行中的游戏实例，没有发布版本；本次不需要运行 Go/前端构建测试。

未验证：每张地图启动、4K 的引擎行为、Unicode 存档端到端、第三方生成代码逐项生效、全部预设与跨平台客户端加入。后续实现应增加地图条件与版本范围的后端测试、种子只生成一次的前端测试、既有存档保护回归、实际规则生效与备份恢复集成验证。

## 附录：目标模板全部 68 个生效参数

“继承”指模板值仍参与运行配置，当前没有对应面板字段；不等于参数缺失。下面默认值是游戏原始模板值，不是面板覆盖值。面板覆盖示例：PlayerKillingMode=0、ServerDescription=Managed by GamePanel Lite、TelnetEnabled=false；完整映射以提供方为准。来源为上文 SHA-256 标识的游戏 XML。

| 参数 | 游戏模板默认 | 面板现状 | 建议 |
| --- | --- | --- | --- |
| `ServerName` | `My Game Host` | 可编辑 | 保留默认 / 按需高级配置 |
| `ServerDescription` | `A 7 Days to Die server` | 固定托管 | P1 房间介绍 |
| `ServerWebsiteURL` | `` | 继承 | P2 网站 |
| `ServerPassword` | `` | 可编辑 | 保留默认 / 按需高级配置 |
| `ServerLoginConfirmationText` | `` | 继承 | P1 入服须知 |
| `Region` | `NorthAmericaEast` | 继承 | P1 地区 |
| `Language` | `English` | 继承 | P1 列表语言 |
| `ServerPort` | `26900` | 通用端口 | 保留默认 / 按需高级配置 |
| `ServerVisibility` | `2` | 可编辑 | P0 区分列表可见性与密码 |
| `ServerDisabledNetworkProtocols` | `SteamNetworking` | 固定托管 | 保留默认 / 按需高级配置 |
| `ServerMaxWorldTransferSpeedKiBs` | `512` | 继承 | P1 下载地图限速 |
| `ServerMaxPlayerCount` | `8` | 可编辑 | 保留默认 / 按需高级配置 |
| `ServerReservedSlots` | `0` | 继承 | 保留默认 / 按需高级配置 |
| `ServerReservedSlotsPermission` | `100` | 继承 | 保留默认 / 按需高级配置 |
| `ServerAdminSlots` | `0` | 继承 | 保留默认 / 按需高级配置 |
| `ServerAdminSlotsPermission` | `0` | 继承 | 保留默认 / 按需高级配置 |
| `WebDashboardEnabled` | `false` | 固定托管 | 保留默认 / 按需高级配置 |
| `WebDashboardPort` | `8080` | 继承 | 保留默认 / 按需高级配置 |
| `WebDashboardUrl` | `` | 继承 | 保留默认 / 按需高级配置 |
| `EnableMapRendering` | `false` | 继承 | 保留默认 / 按需高级配置 |
| `TelnetEnabled` | `true` | 固定托管 | 内部管理通道；暂不开放公网 |
| `TelnetPort` | `8081` | 继承 | 保留默认 / 按需高级配置 |
| `TelnetPassword` | `` | 继承 | 内部管理凭据 |
| `TelnetFailedLoginLimit` | `10` | 继承 | 保留默认 / 按需高级配置 |
| `TelnetFailedLoginsBlocktime` | `10` | 继承 | 保留默认 / 按需高级配置 |
| `TerminalWindowEnabled` | `true` | 继承 | 不展示（Windows 专用） |
| `AdminFileName` | `serveradmin.xml` | 继承 | 保留默认 / 按需高级配置 |
| `ServerAllowCrossplay` | `false` | 固定托管 | P1 联动约束后开放 |
| `EACEnabled` | `true` | 可编辑 | 保留默认 / 按需高级配置 |
| `IgnoreEOSSanctions` | `false` | 继承 | 保留默认 / 按需高级配置 |
| `HideCommandExecutionLog` | `0` | 继承 | 保留默认 / 按需高级配置 |
| `MaxUncoveredMapChunksPerPlayer` | `131072` | 继承 | 保留默认 / 按需高级配置 |
| `PersistentPlayerProfiles` | `false` | 继承 | 保留默认 / 按需高级配置 |
| `MaxChunkAge` | `-1` | 继承 | P2 危险：区块重置 |
| `SaveDataLimit` | `-1` | 继承 | P2 危险：可能重置区块 |
| `GameWorld` | `Navezgane` | 可编辑 | P0 添加预生成地图 |
| `WorldGenSeed` | `MyGame` | 可编辑 | P0 仅 RWG；新实例随机生成一次 |
| `WorldGenSize` | `6144` | 可编辑 | P0 6K / 8K / 10K |
| `GameName` | `MyGame` | 可编辑 | P0 存档内部标识 |
| `GameMode` | `GameModeSurvival` | 继承 | 保持当前生存模式 |
| `PlayerSafeZoneLevel` | `5` | 继承 | P1 玩家规则 |
| `PlayerSafeZoneHours` | `5` | 继承 | P1 玩家规则 |
| `BuildCreate` | `false` | 继承 | P1 玩家规则 |
| `BedrollDeadZoneSize` | `15` | 继承 | P1 玩家规则 |
| `BedrollExpiryTime` | `45` | 继承 | P1 玩家规则 |
| `AllowSpawnNearFriend` | `2` | 继承 | P1 玩家规则 |
| `CameraRestrictionMode` | `0` | 继承 | P1 玩家规则 |
| `MaxSpawnedZombies` | `64` | 继承 | P1 性能 |
| `MaxSpawnedAnimals` | `50` | 继承 | P1 性能 |
| `ServerMaxAllowedViewDistance` | `12` | 继承 | P1 性能 |
| `MaxQueuedMeshLayers` | `1000` | 继承 | P2 性能 |
| `PartySharedKillRange` | `100` | 继承 | P1 玩家规则 |
| `PlayerKillingMode` | `3` | 可编辑 | 保留默认 / 按需高级配置 |
| `LandClaimCount` | `5` | 继承 | P1 领地保护 |
| `LandClaimSize` | `41` | 继承 | P1 领地保护 |
| `LandClaimDeadZone` | `30` | 继承 | P1 领地保护 |
| `LandClaimExpiryTime` | `7` | 继承 | P1 领地保护 |
| `LandClaimDecayMode` | `0` | 继承 | P1 领地保护 |
| `LandClaimOnlineDurabilityModifier` | `4` | 继承 | P1 领地保护 |
| `LandClaimOfflineDurabilityModifier` | `4` | 继承 | P1 领地保护 |
| `LandClaimOfflineDelay` | `0` | 继承 | P1 领地保护 |
| `DynamicMeshEnabled` | `true` | 继承 | P1 性能 |
| `DynamicMeshLandClaimOnly` | `true` | 继承 | P1 性能 |
| `DynamicMeshLandClaimBuffer` | `3` | 继承 | P2 性能 |
| `DynamicMeshMaxItemCache` | `3` | 继承 | P2 性能 |
| `TwitchServerPermission` | `90` | 继承 | 保留默认 / 按需高级配置 |
| `TwitchBloodMoonAllowed` | `false` | 继承 | 保留默认 / 按需高级配置 |
| `SandboxCode` | `AAAJABJACJADJARFBNC` | 可编辑 | P0 获取入口与版本提示 |

补充：面板另行托管 `UserDataFolder=/data/userdata`，该项在官方模板中被注释。
