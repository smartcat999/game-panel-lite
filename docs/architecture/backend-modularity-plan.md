# 后端模块化与插件扩展方案

日期：2026-09-07。状态：M0 依赖基线与 M1 已实施，M2 进行中。完整剩余范围见 [验收清单](../goals/SAAS_REFACTOR_PROGRESS.md)。

## 首批落地情况

- `internal/architecture/boundaries_test.go` 在 Go 测试中解析生产源码导入，限制 domain、具体 Provider、Docker SDK 和 GORM 的依赖方向。原九条存量导入例外已全部移除；新增根级 workload、worker 与 Docker adapter 也受导入规则约束；这不是完整的所有模块隔离证明。
- `.github/workflows/backend.yml` 对 PR 和 main/feat/v1-full-run 推送执行 Go 测试、vet、Provider/Runtime race 检查。当前只完成本地验证，远端工作流尚未运行。
- Provider 新增 CatalogMetadata，元信息与默认排序由各游戏的 catalog.go 提供；NewRegistry 返回错误以拒绝重复或空 ID。应用入口负责处理错误，测试使用包内构造辅助函数。
- 通用契约删除 ConfigText，文件全部通过 Options.Files 传递。Terraria Provider 拥有 serverconfig.txt 文件名，运行时不再为其他游戏创建无关配置文件。未注册游戏不再以写死的 planned 条目出现在目录。
- 当前保留 HTTP 契约及已注册游戏的展示内容、顺序。后续 M2 已加入 gameconfig 与 modruntime 应用模块，预览/恢复解析、世界候选路径和模组清单迁回 Provider；完整模组/世界编排仍在迁移。数据库迁移、RPC 插件、安全沙箱未完成。

本地复验：`go test ./...`、`go vet ./...`、`go test -race ./apps/api/internal/provider/... ./apps/api/internal/runtime/...`、`pnpm typecheck`、`pnpm build`。工作区全目录 lint 的原有脚本错误详见 V1_PROGRESS；不要为本次改造删除或改写用户录屏文件。

本文补充 [ToC SaaS 方案](toc-saas-platform-plan.md)。目标是让新游戏、新执行后端和商业能力有明确归属，使业务变更集中在少量模块中，并通过 CI 验证依赖约束。

## 核心决策

采用模块化单体、显式构造依赖、小接口、静态编译的受信插件。API、Controller、Agent 可分别构建部署；共享协议必须位于三个程序都能合法导入的位置。

不采用通用插件框架包办所有业务，不为每个结构体创建 interface，不引入反射依赖注入或全局 Service Locator。只有存在实际变化或外部交互的接口才增加 Adapter。

首期“插件化”表示新增 Go 包、注册实现、重新构建发布，不表示租户上传二进制、运行时热加载或不受信代码执行。将来确需独立发布外部插件时，再评估版本化 RPC 插件协议。

## 官方案例与取舍

| 一手参考 | 借鉴 | 本项目选择 |
| --- | --- | --- |
| [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) | 消费方定义所需接口、错误与 context 习惯 | 小接口由调用模块拥有；实现返回具体类型，不做全套 Java 式分层 |
| [Caddy 扩展机制](https://caddyserver.com/docs/extending-caddy) | 模块标识、注册、配置和生命周期 | 借鉴描述与验证契约，使用显式注册，避免依靠 init 的隐式副作用 |
| [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin) | 通过 RPC 连接独立插件进程 | 作为独立发布插件的后续选项，首期不用额外 RPC；进程隔离不等于安全沙箱 |

以上是参考事实与本项目的设计选择，不声称这些项目完整采用本文目录或规则。更详细的原始资料见 [参考笔记](go-plugin-reference-notes.md)。

## 模块职责与接口

| Module | 拥有的规则与数据 | 不得承担 |
| --- | --- | --- |
| identity | 用户、凭证、Session | 游戏、套餐权益 |
| tenancy | 组织、Membership、组织权限 | 平台机器操作 |
| billing | 订单、支付、订阅、权益、对账 | 直接启动容器 |
| server | 实例 spec/status、操作意图、生命周期 | 游戏命令、Docker SDK、支付渠道规则 |
| scheduling | 节点选择、容量与端口预留、租约 | 游戏配置渲染 |
| assets | 世界/备份元数据、对象归属、恢复编排 | 直接识别游戏存档格式 |
| provider | 游戏配置、文件格式、命令、存档和模组规则 | 读取用户数据库、订单、任意平台密钥 |
| runtime | 工作负载创建/观察/停止、执行与 IO | 按游戏 ID 分支 |

跨模块通过公开用例、值类型或持久事件交互，禁止直接写其他模块的表。初期允许共用 PostgreSQL；模块所有权不意味着每模块独立数据库。

跨模块一致性必须具体设计：权益校验、配额与实例创建由明确的应用用例协调；需要原子性的预留和 outbox 放同一事务，通过用例所需的窄事务接口实现。支付外部调用不能放进数据库长事务。异步通知只用于允许最终一致的工作，不把全部函数调用替换成事件。

## 目录与依赖

目标目录示意，按迁移批次建立，不一次性搬动全仓库：

```text
apps/api/cmd/server/          # HTTP 进程入口
apps/api/cmd/controller/      # 协调进程入口
apps/api/internal/app/        # 组合根：显式接线
apps/api/internal/identity/
apps/api/internal/tenancy/
apps/api/internal/billing/
apps/api/internal/server/    # 用例、规则、消费方接口
apps/api/internal/server/internal/persistence/  # 仅 server 树可导入
apps/api/internal/scheduling/
apps/api/internal/assets/
apps/api/internal/http/       # transport，只做解码/认证映射/响应
internal/workload/            # API 与 Agent 共用的窄协议
internal/provider/            # 受信游戏插件契约及实现
internal/runtime/             # 共用执行契约及实现
apps/agent/                  # Agent 的组合根与执行循环
```

Go 的 `apps/api/internal` 不能被 `apps/agent` 合法导入；共享契约迁往根级 internal 时，必须同步检查导入可见性。禁止通过复制一套协议结构解决共享问题。

依赖规则：

- app 组合根可以引用具体 Adapter 并完成注册；普通用例只能依赖消费方接口。
- domain/规则代码不得导入 net/http、chi、GORM、Docker 或具体 Provider。
- HTTP 不导入具体游戏实现，不访问 GORM、不直接调用 Runtime；现有专用接口通过对应应用用例适配。
- Docker SDK 只由 Docker Runtime Adapter 引用；对象存储、支付 SDK 同样限制在对应 Adapter。
- 顶层 `internal` 只能防止仓库外部导入，不能阻止同层包相互导入；更细隔离使用嵌套 internal 和 CI import 规则。
- 不扩大现有 domain/models.go 为所有业务共享模型桶；逐步把账号、订单等模型迁回所有者，公开交互使用小型类型。

## 游戏插件契约

保留现有 GameProvider 和能力接口，按真实调用场景收敛，避免新造一个涵盖所有游戏功能的巨型接口。

插件描述包含稳定 ID、游戏元信息、插件版本与支持的配置 schema 版本；游戏版本、插件版本、配置版本分别管理。插件先解析/校验原始配置，再使用内部强类型 Config 生成结构化 WorkloadSpec 和相对路径文件集合。

Registry 的职责仅为注册、校验、查找和枚举：

- 重复 ID 直接返回启动错误，不能静默覆盖。
- 已声明能力必须具备对应实现；不支持的能力返回统一 Unsupported 错误。
- 元信息由插件提供；展示排序、上架状态、推荐版本属于目录策略，不依赖通用代码的游戏 switch。
- Registry 启动完成后只读；首期不支持在服务请求期间热变更插件。
- 注册只在组合根，例如显式传入 Terraria、DST 等构造结果；这里列出具体实现是合理依赖组装，不是业务硬编码。

只有多实现的实际需求才提取支付/对象存储/通知扩展接口。数据库是持久化 Adapter，租户授权与账务不作为可被插件替换的随意钩子。

若将来支持外部 RPC 插件，补齐协议协商、超时、取消、资源限制、重启策略、兼容测试和二进制来源控制。RPC 插件即使独立进程，也必须另行限制 OS 权限及网络访问。

## hard code 的处理规则

| 内容 | 应放位置 |
| --- | --- |
| 游戏配置文件名、Steam AppID、命令语法、存档格式 | 对应 Provider 的强类型常量或实现 |
| 镜像 digest、可售版本、推荐顺序、上架状态 | 版本化游戏目录；更新时校验 |
| 区域、节点池、容量、套餐价格和权益 | 明确拥有者的数据表或配置；价格保存版本快照 |
| 并发数、轮询周期、超时、退避上限 | 有默认值、有范围校验的类型化部署配置 |
| 路径隔离、权限检查、金额约束、状态机合法转换 | 核心规则代码与测试，不能通过配置关闭 |

业务逻辑不要全部改成 YAML、任意脚本或 map[string]any。固定协议值留在代码中合理，关键是归属正确；配置化同样要有 schema、默认值、验证和升级策略。

## 已观察到的优先改造点

1. `provider/provider.go` 的 Games 和 providerCatalogPriority 写死游戏目录及 Terraria 排序；迁往插件描述与目录策略。NewRegistry 当前覆盖重复 key，应增加冲突检测。
2. `runtime/runtime.go` 的转换函数识别 serverconfig.txt；`runtime/docker/adapter.go` 写同名文件；`server/workload.go` 也有兼容处理。Provider 应输出通用文件集合，再由受控文件写入模块校验路径和大小。
3. `server/mod_planner.go` 导入具体 Terraria 实现，并按多个 Provider ID 分支；将模组规则留在 Provider，公共下载、暂存、安装事务由应用用例编排。
4. 多个 HTTP 文件引用 Terraria 实现或按其 ID 分支；保持现有 HTTP 契约，逐路径迁往用例接口，不让 Handler 再补新的游戏知识。
5. `store/store.go` 集合了账号、实例、组织和节点等持久化职责；结合 PostgreSQL/租户迁移按归属拆分，避免仅把文件拆开却继续暴露一个万能 Store。

这些是抽样证据，不是完整审计结果；游戏专用兼容路由可临时保留，但需要明确迁移范围。

## Go 编程约定

- 小写、表达领域的包名；避免 utils/common/helpers 大杂烩和重复包名前缀。
- 显式构造函数注入依赖；避免全局可变单例，启动阶段验证必需依赖。
- context.Context 作为需要取消的操作首参，贯穿数据库和网络；goroutine 明确由谁启动、停止和等待，队列及并发有上限。
- 错误用 errors.Is/As 匹配，用 %w 包装上下文；业务错误在 HTTP 层集中映射状态码，不比较错误字符串，不在每层重复记录同一个错误。
- 外部 JSON 在入口校验并转成强类型；动态游戏配置只在插件契约边缘出现，插件内部及时解析。
- 日志记录 operation/assignment 等关联信息，密码与凭证禁止写日志。接口注释说明幂等性、并发性、取消和所有权，而不是重述函数名。

## 通过工具保障边界

后续实施增加 CI 架构检查：基于 `go list -json ./...` 的直接导入图与 AST 检查禁用依赖，按包维护明确允许列表；存量例外只列具体路径和迁移任务，新代码不能扩大例外。

验收用例：

- 添加测试用 Provider，只增加实现与显式注册即可出现在目录、生成 workload；不修改 Handler、Controller、Docker Adapter。新能力本身仍可能需要扩展契约及 UI，不承诺任意功能零修改。
- 所有真实 Runtime Adapter 运行共享行为契约测试，检查幂等、取消、NotFound 和状态观察；真实 Docker 集成测试覆盖假实现不能证明的行为。
- 拒绝重复插件 ID、错误配置版本、不一致能力声明；备份与模组路径遍历测试保持通过。
- 跨租户测试、任务重复与重启测试在重构前后保持通过，不能用接口抽象替代授权验证。
- CI 运行 gofmt 检查、go test ./...、go vet ./...、适用包的 race 测试及依赖规则；前后端协议变化时运行现有前端检查。

## 迁移顺序

M0：建立导入基线与行为测试，记录例外。M1：修正 Registry 元数据与重复注册，再移除通用文件名特判。M2：把模组和世界操作移入 Provider 能力与应用用例，保持 API 行为。M3：结合 SaaS P1 拆租户/持久化，再结合 P2 拆执行进程和共享协议。

每批独立验证，不进行全仓库一次性目录重排。先通过测试再删除本批造成的旧代码，不顺手改造无关模块。M0/M1 已完成首批实现；M2/M3 和更完整的能力校验、模块隔离仍按上述计划推进。

## Agent 共享执行协议落地

API 与 Agent 共用 internal/workload 的 Assignment、Spec 和 Observation，保留 CPU、内存与 TCP/UDP 端口字段，避免双方复制结构导致字段丢失。internal/worker 通过消费方 Runtime 接口协调状态；Docker SDK、日志、stdin 和文件准备集中在 internal/runtime/docker。Agent 入口显式组装，轮询和连接随进程 context 取消并等待退出。

远端 Spec.DataDir 不决定本机挂载路径；Agent 从本地 AGENT_INSTANCE_ROOT 和实例 ID 派生目录。默认 bridge 网络，按协议绑定端口并应用资源限制。部署配置与验证方式见 [Agent runtime](agent-runtime.md)。本批保留已有文件权限兼容策略，不代表不受信模组的强隔离已完成；新旧 assignment 的分布式 fencing 与调度租约仍需后续验证。

## 模组规则共享进展

HTTP 与启动规划共用 modcatalog.Identity/Dependencies，统一名称推导、持久元数据优先级及同 Provider 推荐目录回退。modruntime.ResolveDependencies 负责依赖图遍历、去重与取消；调用方提供按当前实例/Provider 查找或安装依赖的函数。该用例顺序执行，返回新分配记录，不承诺失败时回滚此前安装。上传解析、元数据写入、文件与数据库事务仍需继续迁移。架构测试禁止这两个共享模块反向依赖 HTTP、server 或具体 store/runtime。

模组运行文件统一通过 modruntime.Install/Remove 操作，由 Provider 提供相对路径，应用用例持有 os.Root 并在实例目录内完成暂存、替换和删除。安装先完成全部暂存，再逐文件 rename；读取失败不发布半写入文件，但多个 rename 与数据库写入仍不具备事务原子性。保留现有容器镜像依赖的文件权限，后续强隔离需结合运行用户与卷所有权实现。

模组二进制元数据由可选 ModInspector 能力解析，当前 tMod 实现在 Terraria Provider 内。modruntime 负责打开文件并传递 context，HTTP 仅消费通用名称、版本、加载器版本。当前上传接口保留元数据解析失败后继续上传的兼容策略；元数据提取不等于完整包验证，后续严格校验需单独验收。

## 插件与配置版本基线

ProviderCatalogMetadata 声明 PluginVersion（数字 major.minor.patch）和 ConfigVersion（正整数），独立于游戏发行版本。当前所有 Provider 以 1.0.0 / 1 建立版本基线。Registry 在启动时验证声明，新建实例保存 spec.configVersion，WorkloadBuilder 在文件或模组操作前检查配置版本。历史零值固定解释为格式 1，不能随插件升级自动变成最新版本。

当前只允许读取与 Provider 声明相同的格式；尚未实现自动迁移，也未覆盖全部配置编辑/恢复入口。未来格式升级必须增加显式迁移、事务与恢复演练，不能仅修改版本常量。插件版本目前属于内部描述，不代表外部 RPC 协议或独立发布机制已完成。

## 备份内嵌来源

新归档写入保留文件 `.gamepanel-backup.json`，包含归档格式 1、来源游戏/Provider 和配置版本。元数据最多 16 KiB，读取时拒绝重复条目、损坏字段及未知格式；恢复入口在创建目标目录前检查兼容性，元数据不会写入游戏目录。历史无元数据 ZIP 按配置格式 1 处理。

归档元数据仅描述兼容性，不证明真实性。恢复通过 os.Root 限制写入范围，文件先完整暂存再逐个替换，返回错误时可尝试回滚；尚不提供崩溃后的持久恢复。独立 ZIP 导入、完整性验证及事务恢复仍需后续验收。

恢复文件写入现已先完整暂存并校验 ZIP 数据，再逐文件替换；发生可返回的写入错误时恢复已替换原文件并移除新增文件。回滚失败则保留原文件暂存目录并在错误中报告位置。此机制不涵盖进程崩溃、数据库提交结果不确定或外部并发文件修改；失败后可能残留新建空目录。持久化恢复日志与跨文件/数据库事务仍待实现。

RestoreHooks.Commit 现将配置解析和保存纳入文件恢复完成条件；回调返回错误会触发文件回滚。回调必须在失败时保持自身状态不变，现有 gameconfig 用例遵守这一约定。进程中断及数据库提交结果不确定仍不能靠内存回调解决，需要持久化恢复协调。

## JSON 模组配置能力与应用模块

`JSONModConfigProvider` 明确声明可编辑 JSON 对象配置的相对目录，tModLoader 在自身 Provider 中返回 `ModConfigs`。其他格式不隐式当作 JSON，需基于真实需求增加对应能力。HTTP 已删除 tModLoader ID 判断、固定目录及直接文件读写，改为调用 modruntime 的 ListConfigs、ReadConfig、WriteConfig、DeleteConfig。

modruntime 统一负责配置版本检查、写操作生命周期限制、文件名及 JSON 对象校验、1 MiB 上限、暂存后 rename 和目录句柄内的读写。读取缺失目录返回空列表，不创建目录；路径组件拒绝已存在符号链接，实际操作使用 os.Root 限制到实例目录。写操作保留现有镜像所需的目录/文件权限；不以此声明完成租户进程强隔离。读取保留既有有效 JSON 的兼容行为，写入必须为 JSON 对象。

HTTP 继续承担认证路由、实例查询、维护状态检查、本进程实例锁、活动记录和错误响应；multipart 请求有整体大小上限并清理解析临时文件。应用模块接收已授权实例快照，不自行做数据库授权，因此成员关系与文件副作用的跨进程原子性、远程 Agent 文件访问仍未完成。此批不代表整个 HTTP 层已脱离 Store/Runtime。

验收：测试插件使用不同 Provider key 和嵌套目录完成配置增删读写，无需修改 Handler；路径逃逸、符号链接、非法 JSON、超限读取/上传、取消及忙碌/不兼容版本均有拒绝测试。HTTP 原有配置生命周期及新增拒绝后文件不变测试通过。架构检查禁止该 Handler 再导入 os、filepath 或 safety 承担文件操作。全量 Go 测试、vet、modruntime/Provider race、前端 typecheck/build 和受版本控制源码 lint 均通过。

## 配置输入与预设脱敏归属

创建实例、配置编辑、世界分配/快照和预设保存共用 gameconfig.Normalize、Validate、Summary。Normalize 在调用 Provider 前检查配置版本，保留原有顶层覆盖语义，并通过 JSON 复制隔离嵌套配置和 Provider 默认值；Provider 原地修改输入或返回错误时不会污染调用者的原始配置。校验保持独立，允许创建流程补入 Provider 相关值后再校验；仍有具体游戏创建编排留在 Handler，尚未完成整个创建用例迁移。

预设 PublicConfig 按 Provider ConfigSchema 的 password 字段处理嵌套路径及字面点分键，同时保留旧顶层凭证键的兼容脱敏。脱敏后不再调用 Normalize，避免默认值重新引入敏感字段。PublicPreset 分别清理 config、configPayload 和 configPayloadJSON，列表和详情接口也处理历史记录。未知 Provider 或损坏的历史 JSON 返回错误，不输出无法确认已脱敏的预设。

修复前 DST 的 identity.password / identity.clusterToken 嵌套字段未被顶层 delete 删除。现在创建/更新在持久化前脱敏，历史读取在序列化前脱敏，测试检查响应、持久化结果与非敏感字段保留。已有数据库原文及旧备份不在此批自动改写；仍需专门的数据清理迁移与历史凭证处理。预设租户归属尚未完成，不以移除凭证替代租户授权。

## 预设空间归属与写入版本

ConfigPreset 增加 organizationId 和内部 revision。客户列表/详情通过当前空间成员关系在 SQL 中过滤；历史空归属记录仅保留平台管理员/既有未初始化自托管策略访问，不自动分配。创建接收 organizationId，客户恰好属于一个空间时可省略；多空间客户必须明确选择。更新不允许移动空间。

创建、更新与删除通过独立持久化用例，在事务内锁定空间并再次验证写角色。更新/删除比较空间、ID、revision；每次更新递增 revision，拒绝并发旧编辑、旧删除以及删除后的插入式复活。批量删除对每条记录独立授权和提交，结果保留 succeeded/failed 结构。HTTP 仍持有具体 Store，后续须继续迁入完整 tenancy/preset 应用用例。

PostgreSQL 迁移 004 增加归属和 revision，SQLite 使用现有 AutoMigrate。升级应先停止旧 API 进程，执行匹配版本迁移命令，再启动新 API；旧版本运行中的进程仍缺少租户过滤，不能与新进程混跑。未执行迁移的新 PostgreSQL API 会拒绝启动。旧预设归属接管与历史敏感原文清理需显式迁移，不能猜测所有者。

全局模组和模组包尚未拥有租户归属，因此客户创建/更新预设暂不接受 modIds/modPackId 引用；管理员保留原能力。完成空间模组库后应恢复经归属验证的引用。这是过渡限制，不是模组功能已完成 SaaS 验收。前端类型与 OpenAPI 增加 organizationId；创建向导的多空间选择现已补齐，详见下节。

## 创建向导的空间选择

服务器创建向导和随附预设共用明确的 organizationId。客户通过 /api/auth/me/organizations 加载自身空间，管理员通过管理接口加载空间；查询缓存按账号区分。单空间客户首次加载自动保存该空间 ID，多空间客户必须选择，已选空间失效时不会悄悄换成列表中的其他空间。加载失败提供重试，无空间或未选择时阻止提交；管理员可显式保持不分配空间。

预设选择按当前空间过滤，从可访问的预设链接进入时选择其所属空间。手动切换空间会清理所选资源及预设保存重试状态，并停止重新应用地址栏的资源链接，防止切换被旧链接覆盖。配置草稿保留。既有预设编辑沿用后端不可变归属契约，表单不提供移动归属操作。保存包含模组引用的客户预设暂不可用时，确认页显示说明，并允许关闭保存预设继续创建。

验证：前端 114 个单元测试通过，包含创建用例传递 organizationId。Playwright CLI 隔离会话使用双空间模拟 API，验证未选空间阻止继续、单空间自动选择、空列表/失败阻止提交、两次创建请求空间一致、切换空间后重新保存预设，以及旧预设链接不覆盖手动选择。此验证未创建真实服务器，不替代后端租户授权或真实部署验收。全局顶部空间切换器与其他资产选择仍需继续统一；账号缓存隔离见下节。


## 账号会话与业务查询缓存隔离

认证 bootstrap 使用独立的外层 QueryClient，权限、用户菜单和创建向导共用同一认证查询。受保护页面以账号 ID、平台角色和显式权限集合为边界创建业务 QueryClient、组件树和 ToastProvider；边界变化或退出时卸载并清理旧缓存。旧业务回调保留旧客户端引用，不能向新账号的查询缓存或提示上下文写入结果。成功登录/注册/初始化后刷新外层认证状态，并清理认证 mutation 缓存中的提交参数。

lib/api.ts 的请求封装遇到 401 时通知认证层移除当前账号并重新查询，bootstrap 自身不重复触发通知，403 保留原有权限错误语义。认证状态另有 30 秒轮询与窗口聚焦刷新；独立 monitoring 客户端及 SSE 尚未统一此通知，因此不承诺所有请求即时检测会话撤销，也不承诺跨标签页即时同步。同一账号的空间成员关系变化不属于该缓存 key，后端授权仍是强制边界，工作区切换和撤权后的完整前端状态同步仍待实现。

验收：前端 118 个单元测试通过，新增身份 key、401 通知、bootstrap 排除与 403 行为测试。Playwright CLI 使用模拟双账号 API，延迟第二账号预设响应，验证等待期间没有第一账号预设残留，随后验证退出及重新登录。模拟监控接口按客户权限返回 403，未创建真实服务器。Go 全量测试、vet、前端 typecheck、生产 build 和受版本控制源码及本批新增文件 lint 通过；根目录 lint 包含用户未跟踪脚本，使用上述范围检查避免修改无关文件。
