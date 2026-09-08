# SaaS 与后端改造验收清单

## 2026-09-08 用户确认的交付优先级调整

- 存档与备份接入方向调整为自建、兼容 S3 API 的对象存储，先完成同 Region 使用；实现边界与剩余工作见 [区域对象存储方案](../architecture/regional-object-storage.md)。

- 核心交付优先：租户隔离、创建实例、Region 调度 Node、可靠执行与状态回报、启停重启删除、日志控制台、同 Region 存档与备份恢复；随后接通商业闭环及部署验收。
- 跨 Region 迁移降为可选后续能力，允许不支持，不再阻塞核心交付。多 Region 独立部署、多个 Node 管理与故障隔离继续保留。未实现迁移时不得暴露可成功迁移的入口。
- 暂停跨 Region 文件下载接口及其独立服务建设。本轮未提交接口已从工作区移出，保留临时补丁 `/tmp/gamepanel-deferred-asset-download.patch`；此前资产目录、授权和票据提交保留，不继续扩展成核心链路前置条件。
- 下一实现检查点是全局实例应用入口与区域 Deployment／节点任务的接线。先跑通无外部资产引用的新实例，再验证同 Region 存档恢复；不能以空资产路径替代存档功能验收。

### 备份恢复存储边界

- 新增接收 `io.ReaderAt` 的归档恢复入口，本地文件入口复用；对象存储下载完成并校验后可调用同一 ZIP 校验、兼容性及回滚流程，不要求写入旧备份文件命名目录。
- 独立暂存区快照执行 `go test ./...`（含架构检查）、`go vet ./...`、备份包 race 全部通过。补充独立归档成功恢复、兼容性拒绝、截断与配置提交失败回滚用例。日志：`/tmp/gamepanel-backup-source-all.log`、`/tmp/gamepanel-backup-source-vet.log`、`/tmp/gamepanel-backup-source-race.log`。
- 本批未改 SQL／前端，无 OSS 集成成功声明；S3 适配器与持久任务接线、解压配额、崩溃恢复仍待实现。

### S3 兼容备份存储适配器

- 新增消费方 `backup.ArchiveStore` 与 `StoredArchive`，区分资产版本、StorageID、对象键和后端版本。`s3archive` 使用锁定并 vendor 的官方 AWS Go v2 S3 SDK，SDK 依赖由架构检查限制在适配器／组合根。
- 显式 HTTPS endpoint、CA、签名区域、凭证提供器及有界单次 PUT；先校验归档大小与 SHA-256，条件写入防覆盖，精确版本读取，拒绝错误版本／大小与重定向，限制整个下载时长。读取流仍需消费方校验完整摘要后才可恢复；存储引用本身不是权限证明。
- 独立快照全量 Go（含架构）、vet、S3／备份 race 通过，日志 `/tmp/gamepanel-s3-archive-all.log`、`/tmp/gamepanel-s3-archive-vet.log`、`/tmp/gamepanel-s3-archive-race.log`。真实 SDK 对接 TLS HTTP 协议夹具，验证正常上传读取、条件冲突、错误源摘要／长度、本地输入拒绝、响应版本／长度错误、重定向与阻塞流超时。未使用实际自建对象存储，不声称服务集成完成。
- 第一方差异空白检查通过；vendor 中上游 CHANGELOG／LICENSE 等文件的末尾空行保留原样。无 SQL、前端或历史迁移改动，其他草稿保留。实际对象服务验证、持久上传发布／重试对账、同 Region 恢复接线及配额仍待完成。

### 真实本地对象服务与归档恢复验证

- 新增显式开关启用的 Docker 集成测试，仅启动隔离的本机 TLS MinIO，自动生成临时证书和凭证、启用专用 bucket 版本并清理容器。真实 SDK 上传 ZIP 后，经本地摘要校验及备份兼容性检查恢复测试文件；验证条件上传冲突、同键新版本出现后仍能恢复指定旧版本、不存在版本、错误凭证与服务端摘要错误拒绝。
- 固定测试镜像及摘要、运行方法、证据范围记录在区域对象存储方案。归档源是测试文件，不证明运行中游戏快照一致性；未验证跨主机、HA 或断电持久性，不代表用户备份 API 已切换。
- 独立快照全量 Go／架构、vet、包含真实 MinIO 的 S3／备份 race 全部通过。日志 `/tmp/gamepanel-s3-real-all.log`、`/tmp/gamepanel-s3-real-vet.log`、`/tmp/gamepanel-s3-real-race.log`，首次详细运行 `/tmp/gamepanel-s3-real-integration.log`。无业务 SQL 或前端变更。
- 复核发现现有 `createBackup` 仍在 HTTP 内同步读取本地目录，`listBackups` 仍依赖本机 Stat；下一步需接持久备份记录和区域异步任务，不能仅注入 S3 Adapter 就宣称区域备份已完成。

### 上传结果未知的内容对账

- `ArchiveStore.ResolveUpload` 按任务专属对象键和预期资产摘要／大小执行完整 GET 校验，找回同一响应中的存储版本。缺失、不同内容、截断或超时不返回可发布引用。接口要求调用方提供已授权任务的专属键，不把引用视为权限或任务完成证明。
- 真实 MinIO 测试在丢弃上传回执、重新打开适配器后找回相同版本；同键外部覆盖不同内容后对账拒绝，而已知旧版本仍可恢复。HTTP 异常测试覆盖相同长度错误内容、截断、阻塞超时和缺失，失败均返回空引用。
- 独立快照全量 Go／架构、vet、真实 MinIO＋备份 race 通过。日志 `/tmp/gamepanel-s3-recovery-all.log`、`/tmp/gamepanel-s3-recovery-vet.log`、`/tmp/gamepanel-s3-recovery-race.log`。未新增 SQL 或修改前端。
- 持久任务还需原子保存对象键、预期摘要与领取状态，并在对账后复核授权及 CAS 发布；当前只完成存储层恢复能力。区域 Deployment／节点执行绑定与用户备份入口接线仍未完成。

### Region 上传任务与结果 Outbox

- 按用户确认，备份任务归全局控制面，Region 维护内部执行任务并异步回传。新增区域迁移 004，保存控制面操作／下发事件、部署／节点／快照与对象身份；完全相同计划可重放，同操作或同对象键不能另建冲突记录。登记接口仅供受信协调器在授权并准备一致归档后调用。
- 单表 SKIP LOCKED 领取，数据库时间和独立令牌约束完成／重试；比对完整计划，旧领取、错快照、跨租户回执均拒绝；损坏记录隔离。上传完成状态与 `backup.archive.uploaded` 结果 Outbox 同事务提交，仍不等于控制面备份发布。
- 真实 PostgreSQL 覆盖 8 个并发领取唯一、过期拒绝、重开 Store 重新领取、持久重试、完成后重放不重置、错误记录隔离及 Outbox 故障回滚。独立快照全量 Go／架构、vet、Store＋backup＋S3 的 PostgreSQL／MinIO race 通过；日志 `/tmp/gamepanel-archive-jobs-all.log`、`/tmp/gamepanel-archive-jobs-vet.log`、`/tmp/gamepanel-archive-jobs-integration.log`。仅新增迁移，无 JOIN，历史迁移及其他草稿保留。
- 本批尚未实现全局备份任务／下发事件接入、结果 Outbox 发布与确认、控制面结果 Inbox／状态更新、Node 快照授权和完整 Worker 接线。这些继续作为核心验收项，不能以区域上传表替代控制面用户任务。

### 控制面备份请求事务

- 全局迁移 020／SQLite 版本 8 新增用户备份任务及独立请求 Outbox。`RequestGlobalBackup` 在租户权限锁内校验幂等，对归属实例加锁并读取配置、意图及 Placement 版本，原子写入备份 Operation、任务和待下发命令。无需读取本机目录或等待区域执行。
- 备份命令仅含全局身份、版本和实例／存档范围，不携带 Node 或路径；原配置通知 Outbox 不混入备份命令。相同请求重试返回原事件和任务当前状态，不重置运行中状态；跨租户、未知实例、非法范围及幂等参数变化拒绝。
- SQLite／真实 PostgreSQL 覆盖 8 个并发相同请求唯一、授权与跨租户拒绝、命令身份、队列分离及 Outbox 故障时 Operation／任务回滚。修正旧版本升级夹具以移除新增表，历史迁移不变。独立快照全量 Go／架构、vet、Store＋backup PostgreSQL race 通过，日志 `/tmp/gamepanel-global-backup-all.log`、`/tmp/gamepanel-global-backup-vet.log`、`/tmp/gamepanel-global-backup-integration.log`。
- 这完成控制面持久受理边界，尚未开放公共 HTTP；专用 Outbox 发布、Region 备份 Inbox、执行绑定与结果回传去重仍待接通。用户侧完整 OSS 备份仍未完成。所有业务查询为单表，其他草稿保留。

### 2026-09-09 备份请求发布到区域 broker

- `outbox-publisher` 增加明确的 `backup-requests` 流，默认保持 revisions；备份专用 Outbox 通过相同 Dispatcher／RabbitMQ Publisher 执行有界领取、发布确认和持久重试。表名仅由 Store 的私有固定映射选择，外部不能传任意表名。部署必须为两类流使用独立队列与消费者。
- 抽取两类 Outbox 共用的 SQL 领取实现，新增 SQLite 先取得写锁再读取候选的处理；8 路并发测试暴露并修复 deferred read-to-write 升级锁竞争，PostgreSQL 仍为 SKIP LOCKED。过期令牌不能确认，未确认发布延迟重试，已确认备份不再领取且不修改配置通知记录。
- SQLite／真实 PostgreSQL／RabbitMQ 验证备份命令进入专用队列及完整事件身份。独立快照全量 Go／架构、vet、Store＋RabbitMQ 的真实服务 race 回归通过；日志 `/tmp/gamepanel-backup-dispatch-all.log`、`/tmp/gamepanel-backup-dispatch-vet.log`、`/tmp/gamepanel-backup-dispatch-integration.log`。
- 本批未更改历史迁移或前端。Broker 已确认不等于 Region 已接收落库；Region 备份 Inbox、执行与控制面回传仍未接通，继续作为核心工作。

### 2026-09-09 Region 备份 Inbox 与消费确认

- 新增备份专用消息入口和区域迁移 005；Region receiver 支持 `backup-requests` 流。入口限制体积和媒体类型，拒绝重复／未知字段、尾随内容、信封身份／目标区域不匹配及未知版本。持久接收后仍为 `awaiting_authority`，不能视为开始执行或已获得一致快照。
- Inbox 事件身份与稳定操作身份分别去重，备份 ID 唯一归属操作；同操作新信封不重复建任务，冲突内容不残留 Inbox。事件收件与待执行请求原子提交，数据库失败时不确认消费，保留 broker 重试。
- 真实 PostgreSQL 验证 8 路并发重复接收、操作／备份 ID 冲突、错误 Region、请求落库故障回滚与恢复。真实 RabbitMQ 双次投递得到两次 ACK、一个待执行请求，提交后队列不残留。独立快照全量 Go／架构、vet、Store／backupingress／RabbitMQ 真实服务 race 通过；日志 `/tmp/gamepanel-backup-ingress-all.log`、`/tmp/gamepanel-backup-ingress-vet.log`、`/tmp/gamepanel-backup-ingress-integration.log`。
- 只新增迁移，无 JOIN，其他草稿和前端保留。尚未完成待执行请求的当前授权校验、Node 快照执行、上传任务接线及结果回传控制面；也未用此模块级集成替代用户侧完整备份验收。


### 2026-09-09 Region 备份结果可靠发布

- 新增区域迁移 006，为结果 Outbox 增加领取租约、持久重试与发布确认字段；历史迁移不变。复用有界领取与确认实现，来源 Region 固定绑定区域数据库身份，错误 Region 不能领取。
- `outbox-publisher -stream backup-results` 使用区域数据库和控制面结果 broker，独立队列发布完整上传结果。Broker 确认后才记录发布完成；租约过期后的旧令牌不能确认，发布失败持久延后。区域 uploaded 与已发布事件均不等于全局用户任务成功。
- 真实 PostgreSQL／RabbitMQ 验证并发唯一领取、过期拒绝、失败重试、完整结果信封及确认后不再领取。独立快照全量 Go（含架构）及 vet 通过，日志 `/tmp/gamepanel-backup-results-all.log`、`/tmp/gamepanel-backup-results-vet.log`。首次集成暴露旧死信测试夹具未等待发布确认的竞态，修复夹具后 Store／RabbitMQ race 通过，日志 `/tmp/gamepanel-backup-results-integration-fixed.log`，受影响包 vet 日志 `/tmp/gamepanel-backup-results-vet-fixed.log` 为空。
- 无 JOIN，未改前端或无关草稿。控制面结果 Inbox／状态与资产元数据原子更新、当前执行授权、Node 一致快照和完整 Worker 接线仍待完成；未以消息发布测试替代用户侧备份闭环验收。完整六阶段 Goal 保持进行中。

### 2026-09-09 控制面备份结果接收契约

- 在备份消息入口模块新增 ResultIngress，由组合根提供可信来源 Region 和 ResultInbox；消息正文不能选择来源身份。校验信封 ID、来源 Region、版本、上传计划及存储回执的租户／摘要／对象一致性，持久化错误向上传播，不能提前确认消费。
- 嵌套结果 JSON 拒绝各层重复字段（含大小写别名）、未知字段、数组、尾随内容和超限输入；归档结果 Validate 只证明内部一致，不代表节点执行授权或原任务匹配。
- 独立暂存快照全量 Go（含架构）、vet、backupingress／backup race 通过，日志 `/tmp/gamepanel-result-ingress-all.log`、`/tmp/gamepanel-result-ingress-vet.log`、`/tmp/gamepanel-result-ingress-race.log`。覆盖错误来源／信封、跨租户回执、对象或摘要冲突、嵌套歧义、存储失败传播及重试。无 SQL 或 MQ Adapter 改动，本批未重跑外部服务。
- ResultInbox 的真实全局事务实现及消费者入口仍待接入；需关联原任务、去重并原子保存资产与状态，保留取消／失败终态。Node 授权与一致快照执行仍未完成。未将校验入口作为备份闭环验收，完整六阶段 Goal 保持进行中。

### 2026-09-09 全局备份结果原子登记

- 按简化结构新增一张结果表，同时保存操作级去重与对象回执；全局迁移 021／SQLite 版本 9，历史迁移不变。RecordBackupResult 在任务锁内核对来源、原请求事件、租户、实例、Placement epoch 与操作修订，拒绝冲突重放。
- 复用已有资产登记，结果回执、精确摘要元数据、任务和 Operation 成功状态同事务提交；取消／失败任务收到迟到结果只记 discarded，不复活任务、不发布资产。任务锁先于 Operation 锁，无 JOIN，不增加工作流框架或服务层。
- 独立快照全量 Go（含架构）及 vet 通过，日志 `/tmp/gamepanel-global-results-all.log`、`/tmp/gamepanel-global-results-vet.log`。真实 PostgreSQL 与 SQLite race 通过，最终日志 `/tmp/gamepanel-global-results-final-race.log`；覆盖 8 路并发、重复／冲突结果、身份错误、结果写入故障后资产和状态回滚、精确回执及摘要保存、取消／失败终态保留。
- 修正测试夹具复用 GORM 非空主键导致额外过滤的问题；旧 SQLite 升级夹具补充新表清理。只选择本批迁移初始化代码暂存，保留积分／OAuth 等无关草稿。生产结果消费者、用户备份 API、Node 授权和一致快照执行尚未接通，六阶段 Goal 保持进行中。

### 2026-09-09 全局备份结果 MQ 消费接线

- 新增薄组合入口 global-receiver，直接复用 RabbitMQ Consumer、ResultIngress 和全局 Store；来源 Region 与专用队列由部署配置绑定，使用全局数据库，支持事务超时、会话重连和退出清理。未引入通用工作流框架或新的业务服务层。
- 真实 broker 集成通过已持久受理的全局任务生成结果夹具，确认发布两次相同通知，并向真实结果事务注入首次写入失败。验证消费者重试、两次 ACK 写入、唯一 published 回执、任务 succeeded 和队列无残留。测试使用上传结果夹具，不冒充真实 Node 快照／OSS 上传的完整端到端证据。
- 独立快照全量 Go（含新入口编译和架构）及 vet 通过，日志 `/tmp/gamepanel-global-receiver-all.log`、`/tmp/gamepanel-global-receiver-vet.log`；真实 PostgreSQL／RabbitMQ Store 与 MQ race 回归通过，日志 `/tmp/gamepanel-global-receiver-integration.log`。无 SQL 或历史迁移改动，其他草稿保留。
- 已记录消费者运行与队列来源权限要求。用户备份 API、Region 当前执行授权、Node 一致快照和上传 Worker 接线仍需推进；六阶段 Goal 保持进行中。

### 2026-09-09 区域备份请求关联上传生命周期

- 复核发现上传持久记录尚未关联接收请求。本批直接在已有 Store 事务核对原请求事件、操作、租户、实例、Region 和 Placement epoch；无原请求、拒绝请求或内容冲突均回滚，不留下孤立上传。
- 受信准备登记与请求 preparing 同事务；上传完成时请求 uploaded、回执及结果 Outbox 同事务。固定上传→请求锁顺序，并在全部取锁后读取数据库时间校验租约，避免等待请求锁后过期完成。重复完成计划保留终态，不新增表、模块或 SQL JOIN。
- 真实 PostgreSQL race 验证缺失／冲突／拒绝请求、并发领取、失败回滚时请求仍为 preparing、成功后原子 uploaded、完成重放和坏任务隔离；日志 `/tmp/gamepanel-request-upload-integration.log`。独立快照全量 Go（含架构）、vet 通过，日志 `/tmp/gamepanel-request-upload-all.log`、`/tmp/gamepanel-request-upload-vet.log`。未改 MQ 或历史迁移，其他草稿保留。
- 调用点复核确认授权协调器和 Agent 备份快照任务尚未接入；本批关联与状态推进仅为持久记账，不能当作当前执行授权、游戏快照一致性或生产备份闭环证明。继续推进核心执行链路，六阶段 Goal 保持进行中。

### 2026-09-09 全局备份请求当前状态检查

- 新增 CheckRegionalBackup，在同一只读快照核对可信 Region、完整原任务、Operation、当前实例修订／配置代数／意图版本及 Placement。已删除、结束、取消或归属变化的请求不可用；单表按 ID 读取，无 JOIN，无新表。
- 在现有 global-control 挂载 mTLS POST /internal/region/backups/check，复用严格请求解码；拒绝伪造 Header、跨 Region、额外查询、重复字段、超限和错误媒体类型。成功仅 204／no-store，不返回配置或执行票据。依赖检查仅增加 controlapi 对备份契约和解码模块的明确允许。
- 独立快照全量 Go（含架构）、vet 通过，日志 `/tmp/gamepanel-backup-check-all.log`、`/tmp/gamepanel-backup-check-vet.log`。真实 PostgreSQL Store 与真实 mTLS 接口 race 分别通过，日志 `/tmp/gamepanel-backup-check-integration.log`；覆盖任务／操作终态、配置与意图变化、Region／epoch 变化，以及 HTTP 身份和错误隔离。mTLS 接口测试使用检查器夹具，不冒充已接入 Node 的端到端执行验证。
- 检查结果仅代表一个时点的意图有效，不是执行租约、停服证据或一致快照。区域客户端、有限期授权、Agent 快照任务仍待接入；保留其他草稿，六阶段 Goal 保持进行中。

### 2026-09-09 区域备份检查客户端与真实往返

- 在已有 controlclient 增加 CheckBackup，复用 mTLS、受信 origin、超时和禁止重定向的传输；本地拒绝错 Region／无效请求，每次发送原请求，不缓存成功。仅 204 通过，区分请求不可用、权限失败、通信错误，并保留调用方取消原因。无新表、框架或部署入口。
- 客户端 HTTPS 测试覆盖状态映射、200 不能冒充 204、重定向不跟随、每次实际请求、错区域本地拒绝及超时取消。扩展现有 region-fetcher 集成：真实 PostgreSQL 全局备份受理后，经实际 mTLS Handler／Store 检查成功；篡改请求拒绝，服务不可用返回失败，恢复后重新检查，真实意图版本更新后旧请求不可用。
- 独立快照全量 Go（含架构）、vet 通过，日志 `/tmp/gamepanel-backup-client-all.log`、`/tmp/gamepanel-backup-client-vet.log`；真实 PostgreSQL+mTLS、controlclient／controlapi race 通过，日志 `/tmp/gamepanel-backup-client-integration.log`。本批仅扩展控制客户端的明确契约依赖，保留其他草稿。
- 备份协调器、有限期执行授权及 Node 一致快照尚未接入；实时检查不等同执行租约或离线授权。完整六阶段 Goal 保持进行中。

### 2026-09-09 不可变归档上传 Worker

- 新增 UploadWorker，组合既有持久任务、OSS 与 assetfiles 的精确版本读取接口；先核验远端对象恢复回执，再按需打开本地不可变归档上传，复核计划与回执后调用完成事务。失败持久延后，完成记录不确定时不报告成功。StorageID 由组合根绑定适配器，任务不提供路径、URL 或凭证。
- 提取 StoredArchive.ValidateFor 供结果事件与 Worker 共用，避免回执一致性规则重复。Worker 不执行游戏启停或快照生成，不把上传领取视为 Node 授权；未新增 SQL、框架或部署入口。
- 本地真实 assetfiles 与任务／OSS 夹具验证首次上传、完成失败后本地不可用仍恢复、错误回执／存储拒绝、超时及敏感后端错误隔离。真实 TLS MinIO 组合验证 Worker 写入后模拟完成记录丢失，关闭本地适配器后恢复有效版本回执；任务存储在此测试中仍为明确夹具，不能冒充进程强杀或完整业务链路。
- 独立快照全量 Go（含架构）、vet 通过，日志 `/tmp/gamepanel-upload-worker-all.log`、`/tmp/gamepanel-upload-worker-vet.log`；backup／s3archive race 含真实 MinIO 通过，日志 `/tmp/gamepanel-upload-worker-integration.log`。生产上传 Worker 入口、授权协调器与 Node 一致快照仍需接通，完整六阶段 Goal 保持进行中。

### 2026-09-09 区域上传 Worker 组合入口

- 新增 region-upload-worker 薄入口，组合区域 Store、已准备 assetfiles 目录和 S3 Adapter；凭证仅由环境变量注入，CA／endpoint／StorageID 由部署配置给出。明确传输、任务、租约时间关系、重试和轮询上限；退出取消任务并关闭资源，日志不输出驱动或 OSS 错误细节。
- 真实 PostgreSQL＋TLS S3 协议夹具验证显式迁移、原请求登记、已准备归档读取、实际上传字节、原子 uploaded／结果 Outbox 和取消退出。首次测试漏掉区域迁移而失败，补齐测试初始化后通过；没有让业务入口自动迁移数据库。
- 独立快照全量 Go（含架构）、vet 通过，日志 `/tmp/gamepanel-upload-entry-all.log`、`/tmp/gamepanel-upload-entry-vet.log`；修正后的入口／backup race 通过，日志 `/tmp/gamepanel-upload-entry-integration-fixed.log`。S3 服务在本批为协议夹具，真实 MinIO Adapter／Worker 证据沿用上一批，未冒充实际 OSS 部署。无 SQL 或历史迁移改动，其他草稿保留。
- 文档说明显式迁移与运行、区域统一目标 StorageID、归档目录必须实际可访问；多个后端尚不能抢同批任务，跨 Node 归档交付不可由路径配置推定完成。授权协调器、Node 一致快照与归档准备链路仍待接入，完整六阶段 Goal 保持进行中。

### 2026-09-09 可取消归档读取与目录约束

- 检查现有归档实现发现不接收 Context。本批新增 CreateContext／CreateSubtreeContext，旧接口兼容；目录遍历、分块读取与成功返回前检查取消，并在取消时关闭源文件。手动备份和存档快照 HTTP 调用传递请求 Context，不再因调用方已取消仍无条件继续复制。
- 源文件通过 os.Root 读取，拒绝符号链接、非普通文件并复核实际打开文件类型；失败返回空路径并清理半成品。该保护要求调用方稳定源目录，尚不代表任务互斥、游戏停服或物理隔离。
- 测试覆盖已取消调用不产生输出、复制中取消后不继续消费全部源、符号链接失败后清理半成品；原有归档内容、子树路径和恢复回归保留。独立快照全量 Go（含架构）、vet、backup／HTTP race 通过，日志 `/tmp/gamepanel-archive-context-all.log`、`/tmp/gamepanel-archive-context-vet.log`、`/tmp/gamepanel-archive-context-race.log`。
- 无 SQL、迁移或前端变更，未重跑外部服务。归档取消仅为协作式保护；区域授权协调器、互斥 Node 快照、稳定归档交付与用户异步 API 仍未接通。保留其他草稿，完整六阶段 Goal 保持进行中。

### 2026-09-09 Runtime 停止数据读取与生命周期互斥

- 复用共享 Docker Adapter 现有实例文件锁，新增 ReadStoppedData；锁内重新校验当前容器完整身份与明确停止状态，拒绝运行／重启／暂停／dead／缺失状态及未完成文件恢复。仅在回调内提供实例目录 fs.FS，完成后再次检查状态，不暴露宿主机路径。
- 两个 Adapter 实例的协议测试验证读取期间 Start 等待同一把锁，回调成功或失败后释放；覆盖运行数据拒绝、缺失状态、旧 assignment、读取期间状态变化、父目录逃逸拒绝。现有 Runtime／Worker／Agent race 回归通过，日志 `/tmp/gamepanel-stopped-data-race.log`。
- 独立快照全量 Go（含架构）、vet 通过，日志 `/tmp/gamepanel-stopped-data-all.log`、`/tmp/gamepanel-stopped-data-vet.log`。补充真实 Docker race 集成通过，日志 `/tmp/gamepanel-stopped-data-docker.log`：一次性 Alpine 容器运行中拒绝读取，停止后读取实际配置文件，结束删除容器；不以此代替真实游戏保存语义或跨主机验收。
- 无新表、SQL、迁移或业务框架，其他草稿保留。该锁仅覆盖协作进程，不防外部宿主机／Docker 写入；调用方仍须持有有效执行授权，在回调内暂存并在成功返回后发布。Agent 快照任务、共享归档写入和区域授权协调器仍待接入，完整六阶段 Goal 保持进行中。

### 2026-09-09 API／Agent 可共享的受限 ZIP 写入

- 提取 internal/archive.Write，输入为 Context、输出流、fs.FS、子树和元数据，只依赖标准库；API Service 删除重复遍历／压缩逻辑并调用共享实现，保留落盘和失败清理。Metadata 改为共享类型别名，字段、版本、子树路径和恢复兼容性不变。
- 迁移分块取消用例，新增共享写入的元数据／子树、非法路径、特殊文件、保留文件名、取消及运行时 fs.FS→ZIP 组合验证。复核 fs.WalkDir 对根路径的行为后，显式保留 API 子树根符号链接拒绝并补回归，未默默扩大读取范围。
- 最终独立快照全量 Go（含架构）、vet 通过，日志 `/tmp/gamepanel-shared-archive-final-all.log`、`/tmp/gamepanel-shared-archive-final-vet.log`；shared archive／Runtime／API backup／Agent race 通过 `/tmp/gamepanel-shared-archive-race.log`，子树修正后 backup race 通过 `/tmp/gamepanel-shared-archive-final-race.log`。无 SQL、迁移或前端改动，未重复外部服务测试。
- Agent 可使用共享包而无需导入 API 内部代码，但实际快照任务、授权协调器、暂存与交付尚未接通；Runtime 组合仍为协议测试，不作为真实游戏一致性证据。保留其他草稿，完整六阶段 Goal 保持进行中。

### Agent 停止快照暂存（已实现，任务接线未完成）

- Agent 内部复用 Runtime.ReadStoppedData 和共享 archive.Write，在有截止时间的 Context 中生成私有临时 ZIP；限制压缩后大小，返回只读句柄、长度及 SHA-256，调用方关闭时删除文件。
- 执行前、取得生命周期锁后及读取完成后调用任务授权检查；授权拒绝、运行状态变化、超限和取消均不返回成功归档并清理文件。检查回调必须绑定实际任务与部署，当前尚未接入真实授权来源，不能把此辅助函数当成执行授权。
- 独立暂存区全量 Go 测试（含架构检查）、vet、Agent／archive／Docker Runtime race 通过。日志 `/tmp/gamepanel-agent-snapshot-all.log`、`/tmp/gamepanel-agent-snapshot-vet.log`、`/tmp/gamepanel-agent-snapshot-race.log`。新增测试使用 Runtime 夹具；未声称完整 Node 备份或真实游戏验收。
- 沿用现有模块，无新增服务、SQL 或前端修改；后续仍需接通区域授权、Agent 任务领取与归档交付。

### 区域备份准备任务领取（已实现，Node 执行接线待做）

- 区域迁移 007 在现有备份请求表增加领取令牌、数据库时间期限、重试时间与次数。SKIP LOCKED 单条领取；损坏命令隔离为 rejected，避免阻塞其他请求。领取不授予 Node 执行权限。
- PrepareArchiveUpload 首次提交必须关联完整原始请求及有效准备领取，锁后以数据库时间检查过期；请求状态和上传计划原子提交。完全相同的已提交计划允许响应丢失后的重放，完成后的请求不能被准备重试重新入队。
- UploadTasks 移除上传 Worker 不使用的准备方法，由 PreparationTasks 持有领取、重试和计划提交；复用原表及上传事务，不增加服务。所有查询无 JOIN，历史迁移文件未修改。
- 最终隔离快照全量 Go（含架构）、vet、Store／区域上传入口／backup 的真实 PostgreSQL race 通过：`/tmp/gamepanel-preparation-final-all.log`、`/tmp/gamepanel-preparation-final-vet.log`、`/tmp/gamepanel-preparation-final-integration.log`。覆盖并发唯一、旧令牌／错区域／篡改请求、重开 Store、延后重试、过期计划回滚及故障注入。首轮临时库密码配置与既有无密码测试角色不匹配，重建本机测试库后通过。
- 区域 Deployment／Node 授权来源、准备协调器、Agent 任务接口及归档交付仍未接通；本批不能作为完整备份或进程强杀验收。

### 区域 Deployment 与资产准备原子交接

- 区域迁移 008 新增独立 Deployment 表，以逻辑实例与 Placement epoch 唯一绑定，保存租户、期望配置版本、受保护快照所在操作、意图版本及待授权状态。区域身份继续由数据库单例约束；不复制全局用户表或旧 game_servers 整表。
- CompleteAssets 在同一事务写入／更新待授权部署与资产完成状态，并在取得部署锁后重新检查任务租约。配置 generation 和 intent version 分别单调前进，重复通知保留部署 ID 与原配置来源；已知过期配置不新建部署，同版本冲突或租户冲突整体回滚。
- 最终独立快照全量 Go（含架构）、vet、真实 PostgreSQL Store race 通过；前一轮区域模块／region-fetcher race 也通过。日志 `/tmp/gamepanel-deployments-final-all.log`、`/tmp/gamepanel-deployments-final-vet.log`、`/tmp/gamepanel-deployments-final-integration.log`、`/tmp/gamepanel-deployments-integration.log`。验证并发唯一、乱序版本、租户／版本冲突、过期回滚、故障注入及现有 AssetWorker 实际文件准备后的部署交接。无前端修改，无 JOIN，历史迁移未变。
- 尚无 Node 分配或执行权限，不能用待授权 Deployment、资产完成或 Placement epoch 作为运行授权／源隔离证据。调度、授权、Node 任务及全局结果投影仍待接通。

### 区域节点配置与运维入口

- 区域迁移 009 在独立区域库保存节点管理配置，含身份、名称、架构、CPU／内存容量上限、调度开关及版本。创建／更新使用唯一键与版本 CAS，并发修改不能覆盖旧观察；配置不代替心跳或资源预留。
- 新增 region-node 组合入口，读取区域数据库环境变量，支持完整配置更新及有界 ID 游标分页；Region 不匹配在开库时拒绝。默认调度开关关闭，不自动迁移旧全局节点，也不把配置成功当成节点上线。
- 独立快照全量 Go（含架构）、vet、真实 PostgreSQL Store／regional race 通过；实际运行 region-migrate 和 region-node 验证创建、查询、更新、关闭调度开关、旧版本及错区域拒绝。日志 `/tmp/gamepanel-regional-nodes-all.log`、`/tmp/gamepanel-regional-nodes-vet.log`、`/tmp/gamepanel-regional-nodes-integration.log`，命令结果 `/tmp/gamepanel-regional-nodes-cli-{create,update,list}.json`。证据不包含节点上线或真实调度。
- 无 JOIN，无历史迁移修改，无前端改动。后续继续接区域节点身份／心跳、调度事务及 Agent 执行授权。

### 区域节点身份、会话与心跳入口

- 新增迁移 010，在独立观察表保存区域签发的会话 epoch、心跳序号、运行架构、就绪标志和数据库接收时间。旧会话／倒序序号／同序号冲突拒绝；相同重试不延长在线时间；会话旋转清空旧新鲜度，不修改节点管理配置。
- 新增 region-control 组合入口与 nodeapi 消费接口，复用 serviceauth 的实际证书链校验，独立 URI SAN→Node 白名单绑定固定 Region 数据库。身份不从请求体或 Header 获取，未知节点不能靠心跳自动登记；严格有界 JSON 输入，错误不返回数据库细节。
- 最终独立快照全量 Go（含架构）及 vet 通过；真实 PostgreSQL Store 与 mTLS HTTP 边界 race 通过，region-control 实际监听＋客户端证书＋心跳落库＋正常退出的组合 race 通过。日志 `/tmp/gamepanel-node-heartbeat-final-all.log`、`/tmp/gamepanel-node-heartbeat-final-vet.log`、`/tmp/gamepanel-node-heartbeat-integration.log`、`/tmp/gamepanel-node-heartbeat-entry.log`。测试使用临时证书和本机数据库，未运行实际 Agent 或游戏进程。
- 会话不是运行授权或 fencing，实际 Agent 未切换新入口。后续仍需节点客户端、心跳新鲜度策略、区域容量预留和任务调度；无 JOIN，无历史迁移修改，无前端改动。

新总 Goal 的逐阶段验收要求见 [六阶段验收矩阵](SAAS_SIX_PHASE_ACCEPTANCE.md)，本文件继续保留局部实现与测试记录。

目标：完成本任务方案中的所有改造。此清单记录当前证据，不以已通过的局部测试替代整体完成。总体状态：进行中。

依据：`docs/architecture/backend-modularity-plan.md` 与 `docs/architecture/toc-saas-platform-plan.md`。原有自托管 V1 完成记录不代表 SaaS 已完成。

| 要求 | 当前状态 | 证据或后续验证 |
| --- | --- | --- |
| M0 依赖检查与 CI | 部分完成 | AST 规则覆盖新增根级共享模块；gofmt 门禁和独立 Agent 构建已加入；完整模块方向与远端 CI 仍待验收 |
| M1 插件注册、目录、通用文件集合 | 完成当前批次 | edfb004a；注册/目录/文件传递测试 |
| 配置预览、预设、恢复解析归属 Provider | 本地验证通过 | gameconfig 用例、Provider 能力、兼容/恢复失败/路径逃逸测试；配置输入归一化、校验与摘要已统一到 gameconfig，嵌套输入隔离；预设按 schema 脱敏嵌套密码字段，创建/更新落库前及历史读取响应均覆盖，旧数据库原文清理仍待迁移 |
| 世界文件定位归属 Provider | 部分完成 | WorldFilesProvider；世界上传格式与其他存档操作仍需迁移 |
| 模组清单与文件布局归属 Provider | 本地验证通过 | ModManifestProvider、ModFilesProvider、共享 modruntime 用例 |
| 模组来源能力与注册校验 | 部分完成 | ModSupportProvider；支持上传与 Workshop 的判断已改为能力声明；同名依赖查找与分配已限制在同一 Provider；未知 Provider 不回退 tModLoader；名称/依赖元数据已集中到 modcatalog，依赖图遍历已集中到 modruntime；运行文件安装/删除已集中到 modruntime 并使用 os.Root；上传扩展名/辅助缓存文件由 Provider 声明并注入缓存服务；tMod 元数据解析已迁回 Terraria Provider，经 ModInspector 调用；严格包验证、元数据写入与完整安装事务仍待实现 |
| 工作区模组库持久化 | 部分完成 | ModFile/ModPack 归属与 revision、PostgreSQL 005 迁移；成员 SQL 读取、事务写权限/CAS、同空间同 Provider 引用及删除竞争约束通过 SQLite/PostgreSQL/race；旧全局查询排除工作区记录，预设非法引用返回 400；客户上传 API 与文件目录已接入；上传来源与规划器依赖查询已按空间接入，Workshop 缓存及完整安装写入事务尚未接入，引用关系表与容量验证待完成 |
| 工作区模组库文件存储 | 部分完成 | mod.Service 按空间/Provider/模组 ID 隔离，os.Root 句柄读写、不可覆盖发布、限额/取消清理；同名、符号链接、跨进程发布和复制/清理回归通过；已接入上传用例及确认拒绝回收/不确定提交保留；无主文件自动回收/存储容量与多节点验证未完成 |
| 工作区模组上传应用用例 | 部分完成 | modlibrary 组合权限、Provider 元数据检查、文件发布和提交结果；新增认证列表/原始流上传 API、可配置体积限制；提交时撤权及失败回收、未知结果保留上传 ID 已验证；旧库写接口管理员限制与成员列表隔离通过；前端上传已接入；完整包验证、自动对账/回收、限流与配额未完成 |
| 工作区模组安装来源与引用校验 | 本地验证通过，安装链路未完成 | 规划器按实例快照、工作区与 Provider 解析全部根引用后才复制；依赖库查询按工作区过滤，上传来源使用隔离文件句柄，上传模组名称和传递依赖保留；创建、配置保存及旧整行保存检查同空间引用，库删除检查实例直接引用，共用空间锁；双工作区/旧全局库同名文件、跨 Provider、过期快照、缺失私有依赖及 SQLite/PostgreSQL 引用删除竞争通过；检查不是执行租约，依赖失败不回滚已安装项，Workshop 元数据/缓存、完整 HTTP 安装用例与客户安装界面仍待迁移，引用关系仍为 JSON 扫描 |
| 客户模组安装请求应用用例 | 本地验证通过，交付未完成 | modlibrary.Installer 组合账号可见实例/库、写权限、Provider 上传能力和事务提交；新增 POST /api/servers/{id}/mods/installation-requests，仅支持本地已停服实例与同空间上传库来源，返回 202 requested 表示保存意图，下一次启动由规划器执行；事务内复核成员、来源 revision、完整实例配置/状态/节点快照，只写模组引用和 generation；重复来源不增加版本，过期请求返回 409，HTTP 不复制文件；SQLite/PostgreSQL 撤权/状态与来源变化/并发提交、HTTP 双租户授权及请求后真实文件规划、定向 race 通过；OpenAPI 已记录响应语义；客户请求界面已接入；远端文件分发、安装结果反馈、删除/取消、Workshop 及跨进程执行租约仍待完成 |
| 客户模组安装请求界面 | 本地验证通过，安装结果闭环未完成 | 独立 WorkspaceModInstallation 选择上传库模组及同空间同 Provider 实例，详情快照提供 generation；修复实例列表/详情转换丢失 organizationId；停服检查、重复引用禁用、切换模组清空目标、202 请求回执、409/未知结果提示及刷新后再操作；API 核对回执归属/版本/来源，不自动重试；相关列表/详情缓存失效；129 个前端测试及模拟双空间浏览器验证通过，桌面与 390px 布局已检查；当前仅保存下一次启动的安装意图，真实生产启动/安装完成反馈、远端节点、分页容量、跨标签页即时撤权仍待验收 |
| 工作区模组上传界面 | 本地验证通过 | Provider 目录公开用户上传扩展名；显式空间/Provider 选择、原始文件请求、逐文件结果、归属展示、503 上传 ID 保留；122 个前端测试及模拟两空间浏览器验证通过；工作区安装请求已接入，删除/在线导入/包编辑仍未开放，生产与跨标签页成员变化同步待验收 |
| HTTP 与生命周期仅依赖应用用例 | 未完成 | 具体 Provider 导入已消除；JSON 模组配置目录由 Provider 能力声明，格式/大小/版本/文件 IO 已迁入 modruntime，Handler 移除游戏 ID 与固定目录判断，新增自定义插件及拒绝后文件不变测试；多个 Handler 仍持有 Store/Runtime 并含编排逻辑 |
| 插件版本、配置版本与完整能力校验 | 部分完成 | Provider 声明插件/配置版本；Registry 校验声明，创建保存配置版本，执行前拒绝不兼容版本；通用编辑/预览和目标实例恢复前已检查版本；新备份持久化来源版本并在恢复前校验；ZIP 已内嵌来源并在解压前校验；其他配置变更路径、自动迁移及能力矩阵仍待实现 |
| M3 Agent RuntimeAdapter 与共享执行协议 | 本地验证通过 | internal/workload、worker、runtime/docker；导入例外归零；真实一次性 Docker 容器的资源/端口/控制台/生命周期验证通过；容器安全基准已强制注入 no-new-privileges 与危险权能剔除（SYS_ADMIN/NET_ADMIN/SYS_RAWIO/SYS_MODULE/SYS_PTRACE/SYS_BOOT，且严格保留 SETUID/CHOWN 确保 SteamCMD 与游戏内自更新可用）；分布式租约与强 fencing 仍待完成 |
| 注册个人空间与用户空间读取 | 本地验证通过 | 注册事务创建账号、空间、owner 成员与现有 starter 配额；用户接口在 SQL 中按当前成员关系过滤，撤权立即影响后续查询；SQLite 与 PostgreSQL 回滚/越权测试通过；历史账号迁移、租户资源链路及付费权益未完成 |
| 密码变更与会话撤销 | 本地验证通过 | 改密/重置与旧会话撤销原子提交，当前浏览器改密替换会话；登录落库再次核对密码并与轮换串行；角色更新不覆盖密码；邮箱恢复、限流、MFA 等尚未完成 |
| 账号切换前端缓存隔离 | 本地验证通过 | 外层认证查询与账号/平台权限业务缓存分离，退出清理旧组件及提示上下文；lib/api.ts 的 401 通知刷新认证；118 个前端测试及模拟双账号延迟响应、退出重登浏览器验证通过；独立监控/SSE 通知、跨标签页即时同步与同账号空间成员变化同步未完成 |
| 身份/租户/持久化模块所有权 | 未完成 | 待配合 PostgreSQL 与租户迁移实现；不是简单拆文件 |
| PostgreSQL 与显式数据库迁移 | 部分完成 | 可配置 PostgreSQL 驱动与连接池；真实 PostgreSQL 独立 schema 的建表/JSON/事务/查询已验证；PostgreSQL 已使用显式 SQL/校验和账本/事务锁，失败 DDL 回滚已验证；迁移命令与 API 只读 schema 校验已分离，无 DDL 运行角色已验证；旧库接管、SQLite 数据搬迁及完整升级回滚仍待演练 |
| 全链路租户授权、RLS、配额并发 | 部分完成 | 实例 ID 路由统一成员校验（含独立监控模块）、列表/分页 SQL 过滤、新建实例空间归属已实现；平台汇总监控限制为管理员；全局备份列表/读取按实例成员过滤，恢复/删除检查写权限；世界存档保存独立空间归属，未分配上传目录隔离，分配校验来源与目标同空间及写权限；实例 watch/日志 SSE 每 5 秒复核会话/角色/成员关系，取消阻塞读取；实例来源模组分配检查来源读取和目标写权限，禁止跨空间复制；模组库已增加工作区归属、隔离文件和上传 API，Workshop/依赖安装等剩余链路仍未完成；预设已增加空间归属及 revision，列表/详情按成员 SQL 过滤，写入事务复核权限并拒绝旧版本，混合批量删除按条授权；历史空归属仅管理员可见，客户模组引用暂拒绝；创建向导已支持多空间选择并将相同空间传给服务器/预设，浏览器验证通过，其他资产空间流程与历史接管未完成；活动记录持久保存空间归属，列表/监控/SSE 快照过滤后限量，保留已删除实例历史；创建/配置更新按空间行锁事务预留实例数、CPU 和内存，停服保留预留，额度下调同锁检查；空间列表已移除默认空间自动创建和历史实例自动接管，读取不会改变资源归属； 需双租户接口/文件/SSE/后台任务测试及竞争测试 |
| API/Controller/网关分离、可靠任务和调度 | 部分完成 | Controller 运行结果通过专用条件更新接口保存，配置、节点或空间变更后拒绝旧结果；禁止整行覆盖与插入已删除实例；玩家日志同步使用专用人数写入接口，状态或归属变化时丢弃旧人数结果，不再整行保存配置；启动/停止/重启/删除使用专用条件写入，提交复核成员关系和配置/状态快照；删除校验使用同一快照，拒绝取消待删除意图；SQLite/PostgreSQL 回归与 race 检查通过；删除清理限定配置根目录并校验文件/世界归属，文件失败保留元数据，Controller 重试已验证；远端任务发布已核对实例快照并串行化，拒绝同版本不同身份/内容；节点报告在事务内复核任务归属与版本，并发低版本不覆盖高版本；同版本报告使用拉取时观察令牌条件提交，成功后轮换令牌，过期报告返回 409；Worker mutation 绑定已检查的完整容器 ID，拒绝其他 UID，创建后和报告前复核身份；替换竞争与真实 Docker 生命周期已验证；同节点 Create 持跨进程实例文件锁，锁内复核容器不存在，拉取镜像后才写配置；独立进程互斥与重复创建不改文件已验证；配置暂存替换与权限回滚、Docker 创建失败结果确认及不确定时保留恢复材料已实现；Start/Stop/Remove 已与 Create 共用本地实例锁，启动检查未完成配置恢复，停止/删除保留恢复材料；跨进程文件删除隔离、多进程租约、幂等、资源预留与失联隔离仍待实现和演练 |
| 支付、订单、权益、续费与补偿 | 未完成 | 首发渠道依赖业务选择，需沙箱集成和对账验收 |
| 存档一致性备份、恢复及灾备 | 未完成 SaaS 验收 | 单机恢复已完整暂存并在返回错误时回滚文件；配置解析/保存返回失败也会回滚文件；崩溃恢复、数据库提交不确定性、区域恢复、RPO/RTO 与双写隔离仍待验证 |
| 远端文件制品协议与运行时准备 | 部分完成 | workload.Artifact 使用不含 URL/宿主路径的 ID、相对目标、SHA-256 与长度；共享 Runtime 支持带单文件/总量/数量上限的流式来源，精确长度与摘要核对、源关闭/取消错误、配置和二进制共同暂存，明确创建失败回滚、不确定结果保留恢复材料；未配置来源或不支持的 Runtime 在替换旧容器前拒绝 running 任务，停止/删除不依赖文件源；Agent 拉取对含制品的 running 任务检查 artifacts-v1 声明，旧客户端返回 409；二进制、长度/摘要错误、路径/大小写冲突、符号链接、关闭/取消失败、回滚与模拟 Docker 创建检查及定向 race 已通过；控制面上传来源清单已接入构建/发布；生产 Agent 下载与限额配置见下行，远端请求已按节点能力开放，见下行；不是分布式租约或崩溃恢复保证 |
| 制品预取与节点下载授权 | 部分完成 | Worker 在移除旧容器前预取全部文件，并在预取后重新观察容器归属和代次；准备后的 Runtime 只读本次缓存字节，缓存绑定 UID/节点/实例/版本/描述符，完成或失败关闭句柄并清理；失败的第二个文件、下载期间容器归属变化、当前代次不重复下载及二进制缓存复用已验证；新增节点 GET 制品接口，经 modlibrary.Delivery 在打开前后复核当前任务、实例、空间、Provider、来源长度/摘要及调用节点持有未过期的有效执行租约（holderId/fence），发送前再次验证节点令牌；HTTP 未授权/过期/文件缺失/无有效租约及真实二进制下载、打开后令牌轮换拒绝并关闭句柄、SQLite/PostgreSQL 授权与定向 race 回归通过；真实多节点交付、持久缓存回收、传输限流仍待完成，远端请求已按节点能力开放，见下行；请求级复核不代表传输中立即撤权或跨进程隔离 |
| 生产 Agent 制品下载接入 | 本地验证通过 | 从配置控制面派生节点授权请求，拒绝重定向和异常长度/ETag/编码；运行时复核真实字节摘要；启动解析数量/单文件/总量限额，初始化成功才声明 artifacts-v1；HTTP 下载、取消、配置、能力及真实 Runtime 预取测试通过；部署说明已更新，远端安装全链路仍未完成 |
| 控制面远端上传制品清单 | 本地验证通过 | 远端规划只读同工作区上传来源及传递依赖，不创建控制面实例目录/安装记录；Provider 定义目标路径和可选启用清单，生成固定排序的 ID/长度/摘要引用，拒绝模糊身份、缺失依赖、错误元数据和路径冲突；已验证构建、发布、节点授权闭环及停止/删除不依赖模组源；无模组 Provider 兼容性覆盖；跨主机容器实际运行与清单版本升级策略待完成 |
| 制品来源版本与引用生命周期 | 部分完成 | 制品描述符固定元数据 revision，规划器携带版本，发布在工作区锁内复核来源/版本/长度/摘要，下载复核版本；持久化任务中的传递依赖阻止原地更新/删除，期望状态变更不会提前释放；清单替换后释放，发布与删除竞争只允许一方成功，SQLite/PostgreSQL 与 race 已验证；任务制品引用已改为独立表/索引，名称依赖更新策略及旧节点执行租约仍待完成 |
| 持久化制品引用索引 | 本地验证通过 | workload_artifact_references 以任务/制品为主键、工作区/制品为查询索引；与发布同事务去重批量写入，删除任务同事务清理；PostgreSQL 006 与 SQLite 一次性迁移回填旧清单，保留无实例记录的来源引用，重复启动不重新扫描；SQLite 查询计划命中索引、无效 JSON 回滚、PostgreSQL 005→006 回填与引用竞争/race 已验证；根引用、预设、包仍需规范化，尚未进行大租户压测；升级需停止旧写入进程后迁移，未实现新旧版本混跑兼容 |
| Agent 心跳与执行调度 | 本地验证通过 | 心跳与任务使用独立串行循环，慢任务不创建重叠执行；退出取消两路并等待清理后再关闭 Runtime，取消后不启动剩余任务；通过真实心跳序列化和任务拉取、阻塞 Runtime 的集成测试与 race 验证；不代表节点失联隔离、分布式租约或多节点容量验收 |
| 客户远端安装请求与节点能力 | 部分完成 | Agent 注册/心跳声明运行时能力，控制面保存认可能力，旧/降级客户端不声明即清空；PostgreSQL 007 与 SQLite 字段迁移；在线且 45 秒内心跳的 artifacts-v1 节点可接收停服实例安装意图，事务内锁节点复核能力，旧/离线/过期/未来心跳拒绝；前端允许选择远端目标并提示提交时核验，仍区分 202 requested 与实际安装；HTTP 及应用回归、SQLite/PostgreSQL/race、真实双 Agent 能力声明通过；Playwright 模拟 API 检查远端选择、能力提示、202 请求回执和重复禁用通过，修正列表旧本地限定文案；实际游戏安装结果与跨主机失联行为待验收 |
| 节点注册/心跳条件写入 | 本地验证通过 | 专用注册和心跳更新字段，按节点 ID/令牌/心跳时间提交，禁止整行 Save 或插入已删除节点；旧上报不覆盖管理名称/地址/令牌，不回退新能力；写入失败返回错误，成功后记录心跳指标；SQLite/PostgreSQL 令牌轮换竞争、删除保护、时间与字段所有权回归通过；不代表旧节点执行隔离 |
| 节点管理配置写入与状态检查 | 本地验证通过 | 管理编辑使用明确字段补丁，仅写入请求提供的配置，不覆盖 Agent 心跳/能力/令牌，删除后返回 404；不同字段并发修改互不覆盖，同字段仍为最后写入优先；检查接口只读最近状态与上报延迟，移除固定延迟和伪造心跳；SQLite/PostgreSQL/race 的并发配置与上报、令牌保留、删除保护及 HTTP 离线状态回归通过；未实现主动网络探测或配置版本冲突提示 |
| 双 Agent 制品交付集成 | 同机真实 Docker 验证通过 | 两个独立生产 Agent、两个工作区/数据根目录，经真实注册/任务/下载/回报接口和生产规划发布链路，在 Alpine 探针容器内读到各自制品；容器身份与跨节点下载拒绝已验证，进程/容器清理完成；暴露并修复隧道空/错误响应紧循环，退避/取消 race 回归通过；同一 Docker daemon，未证明跨主机网络、真实游戏兼容性、容灾或流量容量 |
| 执行租约持久化 | 部分完成 | 独立领取/续租/释放接口，PostgreSQL 008 与 SQLite 建表；事务内令牌及实例/任务版本复核，以数据库时钟判断期限；同实例竞争只有一个领取者，续租不复活过期记录或缩短已授权窗口，旧 fence 不能释放新租约；任务删除及跨节点重建保留递增 fence；SQLite/PostgreSQL/race 验证通过；Agent 协议及协作式执行前校验见下行；Docker 端 fencing、失联停机、时钟跳变与恢复策略待完成 |
| Agent 执行租约协议与操作复核 | 部分完成 | 节点授权领取/续租/释放接口及 OpenAPI；生产 Agent 必须取得身份/代次匹配租约，使用请求发起时刻计算本地期限，每次容器变更和制品准备前续租复核；预取后的 Runtime 保留授权检查，撤销后仍可清理；错误/取消/不确定操作不提前释放；任务拉取强制 execution-lease-v1，旧 API 无租约路径不回退执行；HTTP、Agent、共享 Worker/race 与真实双 Agent Docker 制品交付通过；状态与下载绑定见下行；本地 Worker、旧命令通道和失联隔离仍待完成，不构成强 fencing |
| Agent 状态回报与制品下载绑定租约 | 本地验证通过，强隔离未完成 | 回报与下载均携带持有者/fence，提交事务内复核节点令牌、实例/任务当前代次和有效租约，同时保留 observationToken CAS；不续租、不接受已释放/过期/旧持有者回报及下载；Agent 先确认状态提交再释放，提交不确定保留租约；SQLite/PostgreSQL/race 的回报/下载与释放竞争、过期和版本拒绝、HTTP 旧协议拒绝及真实双 Agent Docker 交付通过；Docker 端 fencing 仍待完成 |
| 租约领取时状态令牌快照 | 本地验证通过 | 领取事务内读取最新 observationToken，Agent 使用领取快照替换可能过期的任务拉取令牌；续租不替换执行令牌，保留回报 CAS，避免前一个持有者提交后引起无效执行和租约等待；SQLite/Agent/HTTP 定向 race、全量 Go 与前端检查通过；PostgreSQL 在已提交基线加本批修改的临时副本通过；2026-09-08 验证时当前工作区未提交调度模型缺少 unschedulable 列迁移，当时当前工作区 PG 集成失败，后续 011 已修复，见数据库迁移收敛行；租约快照本身无需数据库迁移 |
| 数据库迁移收敛（010–012） | 本地验证通过 | 保留已有 010 积分/OAuth 建表顺序；011 补齐节点 unschedulable，旧节点默认可调度、配置保留、重复升级不清除禁止调度；012 将 OAuth 表重命名为 GORM 实际使用的 o_auth_identities，保留历史账号与唯一约束；真实 PostgreSQL 升级/race、受限角色读写和全量 Go 通过；不代表 OAuth 线上授权或积分账务验收完成 |
| 强隔离、监控、区域容量扩展 | 部分完成 | 容器层已强制启用 no-new-privileges 并剔除 SYS_ADMIN、NET_ADMIN、SYS_RAWIO 等高危能力，同时保护 SteamCMD/游戏内自更新与权限切换能力；生产仍需跨节点网络策略、监控与区域容量扩展 |
| P0/P4 成本、规模与业务闭环 | 未完成 | 容量数字为目标，未进行真实游戏负载/多区域/支付上线验证 |

远端制品交付的下一步还必须完成：验证生产 Agent 与控制面在真实多节点环境中的上传制品交付闭环，补齐清单升级行为与根引用/预设/包的索引迁移，继续验收客户远端安装结果反馈。当前预取与请求授权仍不是分布式执行租约，需继续完成旧节点隔离、崩溃缓存回收和多节点部署验证。

执行顺序保持原方案：先完成模块与能力接口迁移，补充分布式身份/存储/执行契约，再验证 SaaS 的商业与规模要求。每个独立批次通过适用测试后提交。只有全部要求取得与其范围匹配的证据，才能关闭任务目标。

### 2026-09-08 积分一致性修复（当前工作区）

- 积分充值/扣减在读取余额之前锁定租户行，沿用配额分配的锁顺序；扣减检查实际更新行数，充值拒绝 int64 溢出，流水 ID 使用完整 UUID，事务失败不返回未提交流水。
- 新增 `CreateChargedGameServer`，将成员权限、配额检查、实例创建、扣款及流水放在同一事务；HTTP 创建路径已改用此操作。
- SQLite 和真实 PostgreSQL 共用并发用例：20 次充值无丢失、30 次竞争扣款仅 20 次成功、流水总额与余额一致；余额不足回滚实例，越权/配额拒绝不扣款，溢出不写流水。
- 验证：`go test ./...`、`go vet ./...` 通过；临时 PostgreSQL 的 `TestPostgresIntegration` 在 `-race` 下通过。首次全量检查受沙箱 Go 缓存访问限制，获准后重跑通过；临时数据库已删除。前端本轮未修改，未重复运行前端检查。
- 本批依赖当前未提交积分模型及接口草稿，尚未单独提交。仍须收敛费用规则、支付/充值请求幂等、开户赠送流水、账务对账及真实支付流程；不能据此宣称商业化完成。

### 2026-09-08 失联恢复前置条件（当前工作区）

- 移除当前草稿中“心跳超时即删除 assignment 并报告 failover/evicted”的路径；失联超过恢复阈值时记录 `RecoveryReady=False / FencingAndCheckpointRequired`，保留源节点、代次及 assignment UID，同时将运行状态标记为未知。
- 查明原自动迁移测试的假 Store 会保存 NodeID，而真实 `SaveReconciledGameServer` 只更新观察状态，原测试不能证明真实迁移已生效。
- 覆盖有/无替代节点、重复协调、不得发出虚假迁移成功事件；新增真实 SQLite Store 回归验证恢复前置条件持久化及 assignment 身份保留。
- 此修改是恢复状态机的第一步，并不完成跨节点恢复。基础设施 fencing 证据、最终存档传输与目标恢复、手动迁移/排空的统一事务约束仍待实现；现有自动重新分配能力不可宣称可用。
- 本轮验证：`go test ./apps/api/internal/server -count=1`、`go test ./...`、`go vet ./...` 均通过；未修改前端，未重复前端检查。修改仍在工作区，与当前调度草稿一起待收敛提交。

### 2026-09-08 迁移分配事务

- `MigrateGameServer` 将实例条件更新与源 assignment（含制品引用）删除放进同一个数据库事务；按实例→任务顺序加锁，拒绝身份、归属及旧版本不匹配的迁移。
- 当前工作区手动迁移和排空调用方不再提前删除任务。该调用方修改仍属于未提交调度草稿，独立提交只包含 Store 操作及回归测试。
- SQLite/PostgreSQL 共用用例通过：过期请求保留源任务、故障注入使任务删除失败时节点变更完整回滚、8 个竞争请求只有 1 个成功、旧节点控制器无法重新发布退役任务。
- 当前工作区 `go test ./...`、`go vet ./...` 通过，真实临时 PostgreSQL 集成 `-race` 通过。前端本轮未修改。
- 这只完成数据库事务一致性，尚未完成 fencing、存档传输、容量预留或跨主机迁移验收。
- 待提交索引的独立源码快照也通过 SQLite 专项和真实 PostgreSQL `-race` 集成验证，不依赖工作区的积分、OAuth、区域或调度草稿。

### 2026-09-08 Provider 节点要求与调度边界

- 新增可选 `NodeRequirementsProvider` 和 Registry 查询契约；DST 在自身 Provider 内声明 amd64，Registry 返回架构切片副本，未知 Provider 返回错误。
- 当前调度草稿必须注入 Provider 要求解析器；移除调度器直接判断 DST 的代码，自动选择与显式选节点共用规则。有架构要求时未知架构不再被当成兼容；平台解析只读取 Agent OSInfo 首个 token，避免发行版描述文字干扰。
- 覆盖第三方名称的 ARM Provider、DST、未知 Provider、未知平台及平台别名；无需修改调度器即可改变新 Provider 的要求。
- 全量检查暴露分页测试与 10ms 后台协调器的可重复竞争。分页测试改为不启动协调器，仍验证真实 HTTP/Store 分页和排序；其他生命周期测试保持原行为。分页测试连续 10 次通过，随后 `go test ./...`、`go vet ./...` 通过。
- Provider 契约和分页测试修复独立收敛提交；调度器及构造调用方仍属于工作区调度批次。前端和数据库本轮未修改，未重复其专项检查。
- 限制：OSInfo 是 Agent 平台报告，尚非 Docker daemon 的可信架构能力；缺失平台会阻止有架构限制的游戏调度，仍需补齐本地/远端运行时能力上报及镜像兼容验证。容量、端口原子预留和完整恢复流程仍未完成。
- 待提交索引的隔离源码快照中 Provider 包测试和分页测试（10 次）均通过。

### 2026-09-08 Docker daemon 架构上报

- Docker Adapter 的 Info 返回结构化 daemon 信息。Agent 注册、每次心跳上报独立 `runtimeArchitecture`；探测失败不保留旧架构，也不回退到 Agent GOARCH。
- API 规范化架构别名，使用现有凭证/时间条件更新节点字段；旧 Agent 省略字段会清空旧证据。迁移 013 添加非空字段，旧节点默认未知，重复迁移保留后来上报的值。
- 本地 DockerMonitor 提供最近 30 秒内成功探测的架构；当前调度草稿中本地节点使用该探测，远端节点使用 runtimeArchitecture，已移除 OSInfo 推断。
- 验证：全量 Go 测试及 vet 通过，真实 PostgreSQL `-race` 升级/节点报告测试通过，OpenAPI YAML 解析通过。双真实 Agent + 同机 Docker daemon 交付集成通过，并比较持久化架构与实际 daemon Info。覆盖架构与 Agent OSInfo 不一致、失败清空、未知节点拒绝、过期本地探测拒绝等情况。
- 架构上报本身不是镜像平台或仿真运行能力证明。异构跨主机真实游戏、镜像 manifest、CPU/内存容量预留和故障恢复仍需验收。调度器及其组装调用方继续保留在未提交功能批次中。
- 独立提交快照首次全量检查发现启动前模组测试也受到后台协调器竞争影响（删除返回生命周期 409）。测试改用已有的无协调器 fixture，保留生产冲突检查和其他生命周期集成测试。
- 修复后模组专项连续 10 次通过；当前工作区全量测试/vet，以及待提交索引隔离快照的全量测试和真实 PostgreSQL `-race` 均通过。临时数据库及隔离源码快照在验证结束后清理；前端本轮未修改。

### 2026-09-08 节点容量和主端口原子预留

- 租户实例创建在权限/租户配额锁之后获取目标节点行锁，重新核算 CPU、内存和主端口，再在同一事务落库。未分配节点的实例保留待调度状态。
- 租户资源/网络修改、租户 Store 保存及迁移目标节点接入相同检查；迁移容量不足不修改原节点或删除源任务。节点禁止调度、容量未知、实例或既有实例无有限资源限制时拒绝新预留。
- 以已分配实例记录作为预留依据，包含已请求删除但尚未移除记录的实例，避免提前释放。主端口检查保守地跨协议互斥。
- 跨 8 个租户的 SQLite/PostgreSQL 共用测试验证容量仅接纳 2 个、同一主端口仅接纳 1 个；扩容/迁移失败保留旧值，扩容与新建争抢剩余 CPU 仅一个成功。真实 PostgreSQL `-race` 通过。
- 现有声明式创建测试夹具原本未上报任何节点容量，现明确设置 CPU/内存后继续验证创建不依赖控制平面本地磁盘。当前工作区全量测试和 vet 通过。
- 尚未完成：Provider 附加端口预留、待调度实例首次分配节点的原子提交、无租户旧写入口收敛、节点容量缩减/重报与既有分配协调，以及迁移 fencing/存档传输。不能将本批等同于完整防超卖验收。
- 独立快照验证发现权限测试依赖不存在的默认本地节点；补齐有限容量的节点夹具后权限专项通过。生产路径保持目标节点不存在时拒绝预留，不通过放宽检查修复测试。
- 独立快照还暴露运行态模组策略测试的后台协调竞争（删除阶段收到生命周期 409）；该策略测试改用固定运行态 fixture，其他生命周期测试继续运行协调器。此调整不放宽生产中的生命周期冲突检查。
- 最终验证：运行态模组策略测试连续 10 次、当前工作区权限专项，以及待提交索引隔离快照的 `go test ./...`、`go vet ./...`、真实 PostgreSQL `-race` 全部通过。临时数据库和隔离源码在验证后清理。前端本轮未修改。

### 2026-09-08 待调度实例首次分配

- 新增 `AssignPendingGameServer`：检查原实例未分配且希望运行、限制仅增加一次 generation，目标节点容量检查与 NodeID/spec 条件更新在同一事务完成，拒绝同时改资源等其他意图。
- 首次分配拒绝已有 workload assignment 或历史 execution lease 的记录；即使旧租约已过期，也不能据此断言旧游戏进程已隔离。拒绝时回滚节点和代次，保留恢复证据。
- 控制器改为调用原子首次分配入口，成功后才发布新节点任务；不再尝试用仅保存运行状态的接口写 NodeID。并发失败不会继续使用未提交的内存分配。
- 本批同时收敛此前失联恢复前置条件修改及测试：保留源 assignment，记录 FencingAndCheckpointRequired；配置命名改为 recovery timeout，移除未实现的自动驱逐开关。完整故障恢复仍未完成。
- SQLite/PostgreSQL 共用测试验证 8 个待调度实例只接受 2 个、失败不改代次、旧请求/意图篡改/旧任务/历史租约拒绝。真实 Store 的 4 个控制器竞争一份 CPU，仅一实例获得节点且发布对应代次任务；race 检查通过。
- 当前工作区全量 Go 测试/vet、PostgreSQL 集成 race、控制器专项 race 和恢复专项通过；前端本轮未修改。具体调度器及 app/HTTP 组装仍在功能草稿中，待附加端口和其他入口收敛。
- 待提交索引的隔离源码快照全量 Go 测试、vet、真实 PostgreSQL `-race` 全部通过。临时数据库与隔离源码在检查结束后清理；控制器首次分配和恢复前置条件随本批提交。

### 2026-09-08 主端口与附加端口解释统一

- 新增共享 `ResolvePortBindings`，由任务发布前校验、Agent Docker Adapter 和本地 Docker Adapter 使用。规范默认 host port/protocol、范围、完全重复映射去重、同一 host/protocol 冲突，以及同一容器端口的多个主机映射。
- 修复 Provider 附加端口在未指定 host port 时错误地从 0 计算偏移的问题；现在从有效主端口计算，拒绝溢出。修复本地 Adapter 后一条映射覆盖同一容器端口前一条映射的问题。
- 本地 Adapter 在任何删除旧容器/文件写入/镜像准备之前检查网络；移除本次替换后废弃的端口解释函数，测试覆盖实际使用的路径。
- 覆盖默认映射、TCP/UDP 同数值端口、重复映射、多主机映射、非法端口、冲突端口、偏移溢出、发布失败无任务落库。双真实 Agent Docker 交付集成通过。
- 持久化附加端口预留、历史已绑定端口在新代次交付前的保留与安全释放仍待实现；不能把本批解释规则统一视为跨实例端口防超卖完成。
- 补齐 Worker 重建前网络预检：非法新网络不得先删除已有容器；停止和删除仍可清理坏配置实例。任务发布仅对运行意图执行网络校验，避免阻断清理。新增回归验证无运行时调用且旧容器保留，以及后续停止/删除成功。
- 当前工作区 `go test ./...`、`go vet ./...` 通过。新增测试曾缺少导入，补齐后全量重跑通过。数据库 schema 未改，本批未重跑 PostgreSQL 专项；任务发布的拒绝落库断言在 SQLite 测试中验证。真实 Docker 集成验证来自双 Agent、同一 daemon，不代表跨主机端口预留。
- 待提交索引的独立源码快照全量 Go 测试和 vet 通过；检查结束后清理隔离源码。前端本轮未修改。

### 2026-09-08 持久化主/附加端口预留

- 节点端口池锁串行化当前预留写入；主端口分配与运行任务的全部网络绑定检查同一持久化账本，事务失败回滚新增占用。历史任务替换、删除不会提前清除占用。
- PostgreSQL 014、SQLite 一次性版本 2 回填实例与任务的主/附加端口，保留历史冲突的所有实例；升级需停止旧写入进程。
- 当前有效租约、状态令牌和删除任务共同授权端口释放；无错误、无运行时 ID 的 missing 回报才释放当前节点预留。仍未完成迁移源节点隔离、旧配置端口回收和孤儿清理。
- SQLite/PG 共用测试覆盖 8 个发布者争用同一附加端口、失败更新回滚、主端口不得抢占旧附加端口、任务删除保留占用、旧持有者/令牌/失败删除/矛盾运行时信息不得释放。
- 端口持久化留在 Store 以保持事务一致性，游戏解释留在 Provider/workload；调度与事务分配的容量规则统一为下一批架构收敛项。

### 2026-09-08 Global / Region / Node 设计收敛

- 用户确认全局逻辑实例和配置、Region 部署与执行、Node 运行时的资源分层；普通用户选区域和规格，专属用户及运维可严格指定授权节点。
- 新增领域术语、ADR-0001 和目标架构蓝图，记录 Outbox/Inbox、配置/部署双版本、停服释放与存档保留、停服迁移和断连授权契约；旧方案增加优先级链接。
- 本轮仅文档，不改业务代码、不提交此前暂存批次。目标拓扑、状态机和验收矩阵不代表已实现；MQ 选型、价格和具体时间参数仍待专项确定。

### 2026-09-08 资源层级简化

- 用户确认资源部署层级仅保留 Region → Node；全局控制面负责逻辑资源与商业管理，不作为资源树父资源。
- 撤回默认 Cell、cellId 及区域内 Cell 调度，统一目标蓝图、领域术语、ADR 和早期规模路线。节点池仅表达分类、授权及调度条件。
- 本轮仅更新文档，未修改业务代码或提交暂存批次；区域自治仍是待实现目标。

### 2026-09-08 六阶段执行启动与端口批次提交

- 建立覆盖全部六阶段的验收矩阵，分类现有工作区草稿；不以局部测试替代区域、商业或容量目标。
- `64811a9f` 提交主/附加端口持久预留与删除确认释放。独立索引快照的 `go test ./...`、`go vet ./...` 和真实 PostgreSQL `TestPostgresIntegration -race` 通过，包含历史端口回填与受限运行账号验证。
- 首次独立全量检查暴露 Minecraft 白名单测试固定运行态与后台协调器竞争；改用已有无协调器夹具后，该用例连续 10 次通过，全量重跑通过。未放宽生产状态检查。
- 日志位于 `/tmp/gamepanel-port-index-tests.log`、`/tmp/gamepanel-port-index-vet.log`、`/tmp/gamepanel-port-index-pg.log`。同机双 Agent Docker 交付的此前证据仍只覆盖探针交付，本轮不冒充跨主机或真实游戏验证。

### 2026-09-08 共享 CPU / 内存准入规则

- 新增无数据库、HTTP、Runtime 依赖的 `scheduling.CheckCapacity`，供候选过滤与 Store 事务准入共用。事务锁及持久预留保持原有顺序；只有明确释放的资源才能从输入预留中移除。
- 拒绝未知容量、无上限规格、NaN/Infinity 和超额申请；逐项比较后扣减，防止内存累加溢出。纯规则覆盖精确满配、剩余容量及非法资源；依赖门禁禁止引入持久化和传输适配器。
- Scheduler 草稿补齐 CPU 校验、删除中占用及有效主端口回退检查。两个 HTTP 调度测试原先省略资源上限，补齐规格后通过；不放宽生产准入规则。
- 工作区 `go test ./...`、`go vet ./...`、真实 PostgreSQL `TestPostgresIntegration -race` 通过，日志为 `/tmp/gamepanel-capacity-all.log`、`/tmp/gamepanel-capacity-vet.log`、`/tmp/gamepanel-capacity-pg.log`。独立容量核心快照全量 Go 与 vet 通过，日志为 `/tmp/gamepanel-capacity-index-all.log`、`/tmp/gamepanel-capacity-index-vet.log`。
- 本批独立提交仅含容量核心、Store 接入及依赖门禁；Scheduler、HTTP、商业和界面草稿仍未整体提交。附加端口候选查询、严格区域/节点授权和停止后计算容量释放尚未完成。
- 独立快照真实 PostgreSQL `TestPostgresIntegration -race` 随后通过，日志 `/tmp/gamepanel-capacity-index-pg.log`；验证范围包括 Store 事务准入，并不包含尚未提交的 Scheduler 接线。

### 2026-09-08 区域拆库的只读归属预检

- 审查确认旧 `GameServer` 仍混合逻辑配置、NodeID 和运行状态，空 NodeID 又兼有旧本地实例与未调度实例的歧义；部分区域查询把空值当作香港。迁移不能直接沿用这些默认值。
- 新增 `regional-migration-audit` 命令和 Store 一致快照审计。报告实例/节点/任务数量、可解析的租户与区域归属，以及空区域、孤儿引用、历史任务与当前 NodeID 不一致等问题；不自动修复，也不把实际分配转成用户严格绑定要求。
- PostgreSQL 使用只读 REPEATABLE READ，仅读取归属列。真实 PostgreSQL 测试验证列级 SELECT 账号可完成审计，而写入和读取节点令牌均被数据库拒绝；SQLite/PG 共用用例验证歧义报告、确定排序、重复读取不改源数据，以及非法配置 JSON 不被读取或输出。
- 工作区全量 Go、vet 和 PostgreSQL race 通过。独立命令行在临时库上验证：空库退出 0，含未归属实例退出 2，并输出机器可读报告。报告位于 `/tmp/gamepanel-regional-audit-cli-clean.json` 和 `/tmp/gamepanel-regional-audit-cli-issues.json`。
- [迁移说明](../architecture/regional-data-migration.md) 明确权限、报告含义与后续回填／资产验证／切换门槛。全局实例、不可变修订、Placement、区域 Deployment 和实际导入仍待实现；本批不等于模型拆分或独立 Region 验收完成。
- 独立提交快照的 `go test ./...`、`go vet ./...`、`TestPostgresIntegration` 和 `TestPostgresRegionalMigrationAuditReadOnly` race 测试全部通过；日志 `/tmp/gamepanel-regional-audit-index-all.log`、`/tmp/gamepanel-regional-audit-index-vet.log`、`/tmp/gamepanel-regional-audit-index-pg.log`。未变更前端，保留其他草稿及用户媒体文件。

### 2026-09-08 全局实例模型与事务意图

- 新增标准库依赖的 `instances` 模块，分离逻辑身份、不可变配置修订、Region Placement、Operation 及交付事件身份；模型不包含实际 NodeID、容器、宿主机路径或运行观察。
- PostgreSQL 015／SQLite 版本 3 新建逻辑表及修订不可变触发器。创建原子写入实例、修订、归属、操作和 Outbox；修订更新追加版本并 CAS 推进指针，不修改区域 epoch、用户期望状态或 intentVersion。
- 幂等范围为组织＋操作类型＋键；同键不同参数拒绝。组织与成员权限在事务中重查，空 actor 无管理员绕过。过渡期新旧实例共用租户锁、配额准入和用量统计，未知修订资源拒绝准入。
- 测试覆盖 8 个并发重复创建、Outbox 失败整体回滚、修订末步失败回滚、数据库 UPDATE／DELETE／SQLite REPLACE 拒绝、同版本 8 方竞争仅一个成功、原版本保留、旧创建重放不回滚当前指针、新旧配额共用、跨租户拒绝和 SQLite 迁移失败回滚。
- 工作区全量 Go／vet 通过，真实 PostgreSQL race 验证通过；随后新增的跨租户与 SQLite 回滚用例专项通过。最终独立快照验证结果另行登记，不能以先前快照代替。
- [模型与事务说明](../architecture/global-instance-intents.md) 明确暂未接入 HTTP、MQ 发布器、Region 接收端及实际配置保护器。新表尚在同一数据库，不迁移旧实例；全局应用用例、区域 Deployment、Inbox、实际交付与数据导入继续待实现。
- 最终独立索引快照的全量 Go（含架构依赖门禁）、vet、真实 PostgreSQL `TestPostgresIntegration`／`TestPostgresRegionalMigrationAuditReadOnly -race` 全部通过，覆盖本批最终测试。日志 `/tmp/gamepanel-global-model-index-all.log`、`/tmp/gamepanel-global-model-index-vet.log`、`/tmp/gamepanel-global-model-index-pg.log`。本轮未修改前端；其他草稿和用户文件未纳入提交。

### 2026-09-08 用户新增 SQL 约束

- 用户要求所有 SQL 禁止连表 JOIN，采用 ID／批量查询和代码组合；已记录为 [SQL 访问边界](../architecture/sql-access-policy.md)，后续六阶段工作必须遵循。
- 已替换三个在线 JOIN：全局配额的修订资源读取、实例成员角色、用户组织查询。批量关联验证 ID 与资源归属，缺失修订不释放配额；只读快照保持多次读取的一致性。全量 Go／vet 正在验证。
- 历史迁移中的 JOIN 和关联子查询仍待替换，不能宣称所有 SQL 已符合。PostgreSQL 历史脚本有校验和，后续需保留升级兼容并替换实际执行路径，不能直接改旧脚本导致已有库启动失败。
- 组织查询的旧辅助方法还被世界、预设、模组库和活动记录使用；首次编译发现这些调用方后，已同步改用物化组织 ID，并在外层只读快照中执行资源查询。随后全量 Go／vet 通过，日志 `/tmp/gamepanel-nojoin-all.log`、`/tmp/gamepanel-nojoin-vet.log`。这些旧列表的大组织集合分批分页还需继续优化，不能以现有小规模测试宣称容量验收。
- 大组织集合改用单个 JSON ID 参数，避免超过数据库参数数量限制，并保持整体排序／LIMIT；不关联其他业务表。新增 501 个组织的测试检查跨批次完整性、全局最新记录截断和外部用户隔离。首次夹具遗漏成员主键，补齐唯一 ID 后重跑全量和 PostgreSQL；最终结果另行登记。
- 最终工作区及独立索引快照的全量 Go、vet、真实 PostgreSQL `TestPostgresIntegration -race` 均通过，包含 501 组织用例。独立日志 `/tmp/gamepanel-nojoin-index-all.log`、`/tmp/gamepanel-nojoin-index-vet.log`、`/tmp/gamepanel-nojoin-index-pg.log`。目前在线 `.Joins` 调用清零，历史回填 JOIN 和其他关联子查询尚未清零，总 Goal 继续进行。

### 2026-09-08 实例与备份读取改用 ID 集合

- 实例列表／分页／详情移除权限 EXISTS，备份移除实例关联子查询，当前实例活动先读取归属再查询历史；在同一只读快照内组合，保持过滤后排序和截断。
- 复用安全列标识的 ID 集合过滤器；大集合用单个参数，501 组织测试新增实例第二页与备份归属断言。NULL 租户字段保留为空指针，避免误当作空字符串匹配。
- 首次全量回归暴露自动版本检查测试在最后一次活动写入前清理数据库，随后空 logger 崩溃。测试清理改为释放适配器阻塞并等待已有 Worker；该用例连续 10 次及全量 Go／vet 重跑通过，未放宽生产状态判断。
- 本批仍未清理节点制品授权、关联删除、区域过滤草稿及历史迁移执行路径；六阶段 Goal 不变。最终独立快照验证结果待登记。
- 最终独立索引快照的全量 Go、vet、真实 PostgreSQL `TestPostgresIntegration -race` 均通过，包含新增分页／备份／NULL 归属测试。日志 `/tmp/gamepanel-idqueries-index-all.log`、`/tmp/gamepanel-idqueries-index-vet.log`、`/tmp/gamepanel-idqueries-index-pg.log`。其他草稿及用户媒体文件未纳入提交。

### 2026-09-08 制品授权、引用删除与 SQLite 回填

- 节点制品授权改为独立读取组织 ID，保留执行前新鲜租约和版本复核；新增组织删除后拒绝读取测试。
- 任务与引用删除改为事务内锁定并物化 ID 后分批删除；注入删除失败验证引用回滚，验证重复删除及无关任务保留，SQLite 和 PostgreSQL 共用用例。
- SQLite 制品引用迁移按任务 ID 分页、批量读取制品归属，在 Go 中组合并去重；非法清单回滚，不输出清单内容。PostgreSQL 历史脚本尚未替换。
- 全量测试暴露手动调谐测试同时启动后台控制器的竞争，改用已有无后台夹具；连续 10 次与全量回归通过。
- 工作区及独立暂存快照的全量 Go（含架构门禁）、vet、真实 PostgreSQL `TestPostgresIntegration -race` 全部通过。独立日志 `/tmp/gamepanel-artifact-ids-index-all.log`、`/tmp/gamepanel-artifact-ids-index-vet.log`、`/tmp/gamepanel-artifact-ids-index-pg.log`。前端未变更；其他草稿未纳入本批。

### 2026-09-08 端口回填与 PostgreSQL 兼容执行

- SQLite 端口迁移使用按 ID 分页的 Go 回填，保留冲突和旧任务全部占用者；批量写入并由唯一键去重，不选择胜出者，也不释放资源。
- PostgreSQL 006／014 匹配完整历史身份后采用等价 DDL 和 Go 回填；原 SQL 字节、历史校验和及迁移事务保持。重复／并发升级和篡改校验和拒绝由集成测试验证。
- 新增 501 任务跨批次、重复附加端口、第二批非法端口整体回滚、NULL 占用者拒绝及重复执行用例。严格拒绝非法清单，不把无法解释的历史绑定转换成可用容量。
- 初次工作区全量和 PG 运行受沙箱本地端口限制，获准使用临时端口后重跑通过。最终独立暂存快照全量 Go（含架构检查）、vet 和真实 PostgreSQL race 全部通过；日志 `/tmp/gamepanel-port-backfill-index-all.log`、`/tmp/gamepanel-port-backfill-index-vet.log`、`/tmp/gamepanel-port-backfill-index-pg.log`。
- 归属回填、区域过滤和完整 SQL 门禁仍待收敛；不代表全部 SQL 或六阶段 Goal 完成。

### 2026-09-08 归属回填与明确区域过滤

- 世界／活动记录回填改为每页 500 个目标，批量查询实例 ID 与归属，单表 CASE 每批最多 256 行更新；已有归属不覆盖、孤儿不认领。SQLite 两类回填共用事务，PostgreSQL 002／003 采用同一逻辑且保留历史校验和。
- 区域列表筛选物化节点 ID，空区域不再默认香港；管理员分页与计数也使用只读快照。新增测试检查明确区域、空节点集合和无筛选时历史数据保留。
- 新增 501 个不同实例归属的跨批次测试，同时验证重复回填与原归属保留。首次夹具向非空组织字段写 NULL，按真实约束修正为未归属空串；孤儿继续保持未归属。
- 工作区及最终独立索引快照的全量 Go（含架构门禁）、vet、真实 PostgreSQL `TestPostgresIntegration -race` 全部通过。独立日志 `/tmp/gamepanel-owner-region-index-all.log`、`/tmp/gamepanel-owner-region-index-vet.log`、`/tmp/gamepanel-owner-region-index-pg.log`。新批次测试覆盖 SQLite，PostgreSQL 集成验证既有升级数据、并发／重复迁移及校验和拒绝。
- Store 生产 Go 源码扫描无显式 JOIN 或跨表过滤子查询；历史 SQL 的替代执行需继续由生成 SQL 门禁保护。无 JOIN 不等于性能验收，执行计划和高流量测量仍属未完成工作。其他商业、调度、界面草稿和媒体文件保留。

### 2026-09-08 全局 Outbox 持久领取与分发用例

- PostgreSQL 016／SQLite 版本 4 增加分发令牌、数据库时间租约、重试时间、尝试次数和发布确认时间，保留原事件 ID 与业务事务。按 Region 有界领取，无 JOIN；PostgreSQL 使用 SKIP LOCKED，SQLite 冲突按事务失败返回。
- 新增标准库依赖的 `delivery` 用例及接口边界，Broker I/O 不持有数据库事务。发布确认后才更新分发状态，业务 Operation 仍 pending；失败／确认丢失／租约过期后以同一事件 ID 重发，旧令牌不能回写。
- SQLite 与 PostgreSQL 共用测试覆盖有效租约排他、到期恢复、旧令牌拒绝、重试间隔、区域隔离、确认丢失重复和发布状态与业务状态分离。PostgreSQL 另覆盖四方并发领取；首次该夹具因默认资源配额不足失败，改为显式配置测试租户配额后通过。
- 工作区及最终独立提交快照全量 Go（含架构依赖门禁）／vet 与真实 PostgreSQL race 全部通过。独立日志 `/tmp/gamepanel-outbox-index-all.log`、`/tmp/gamepanel-outbox-index-vet.log`、`/tmp/gamepanel-outbox-index-pg.log`。前端未变更。
- [分发说明](../architecture/outbox-publication.md) 列明当前只完成持久分发基础：实际 MQ Adapter、连接监督、退避／死信、Region Inbox 与持久任务原子写入、对账和独立部署均未完成。已核对 RabbitMQ 官方路由确认与 Go 客户端 I/O 取消语义，后续 Adapter 需满足这些契约，不能用替身 Publisher 冒充 MQ 验收。

### 2026-09-08 实际 RabbitMQ 发布适配器与独立入口

- 新增 `messaging/rabbitmq`，使用官方 amqp091-go v1.10.0，并同步 vendor。每个实例绑定明确 Region／队列，持久 quorum 队列＋持久消息＋mandatory 返回检查＋发布确认；健康连接复用，失败／取消／确认不确定后关闭底层连接并重新建立。
- 单适配器只有一个在途发布，避免返回消息和确认错配。超时覆盖拨号、握手、声明、写入及等待确认；context 回调关闭底层连接，结束后才允许复用状态，不仅依靠客户端 PublishWithContext。
- 新增 `outbox-publisher` 独立进程，环境注入全局数据库与区域 Broker 凭证，显式 Region／队列，参数控制批次、租约、超时、载荷、重试／轮询和数据库连接数；信号取消停止分发，不等待游戏启动。应用用例继续只依赖消费接口，架构门禁限定 AMQP 客户端只能位于适配器。
- 实际本机 RabbitMQ 4.3.5（rabbitmq:4-alpine，镜像 ID `sha256:abb0844d027dd92d80a3a3e9189a30b4b935356f30fc1ee3975482614ee86bd9`）race 测试覆盖持久消息内容与 ID、错误区域拒绝、连接复用、删除队列后无法路由的 ACK 拒绝、重新声明和断连恢复；本机黑洞端点验证握手超时。临时队列与 Broker 清理，不连接用户生产系统。
- 工作区及最终独立暂存快照的全量 Go（含架构门禁）／vet 和实际 RabbitMQ race 全部通过。独立日志 `/tmp/gamepanel-rabbit-index-all.log`、`/tmp/gamepanel-rabbit-index-vet.log`、`/tmp/gamepanel-rabbit-index-integration.log`。第三方 vendor 原始文档自带两处空白格式提示，保留上游文件；本项目代码的 diff 空白检查通过。
- Region Inbox／任务原子提交、消费确认、版本与授权检查、死信／对账、端到端进程重启、跨区域与多副本容灾、容量验收仍未完成。单节点 Broker 测试不替代这些目标。

### 2026-09-08 独立区域库与通知 Inbox

- 新增独立 `RegionalStore` 和区域迁移身份，区域 schema 只创建固定 Region、Inbox 与获取授权修订任务，不创建用户／全局实例表；普通打开只读校验，错误区域或全局 Store／迁移器不能接管。
- `RecordRevisionNotification` 校验 schema／身份／正版本／Region，Inbox 与任务同事务写入。eventId 摘要去重与 operationId 语义去重并用；修改内容拒绝并回滚，不给已存在任务重置状态。
- 任务状态为 `awaiting_revision`，只证明通知持久化，尚未授予执行权或接受部署。后续必须取得受保护修订并检查归属、版本、意图、权益和授权期限。详见 [区域 Inbox 说明](../architecture/regional-inbox.md)。
- 真实 PostgreSQL 两个隔离 schema 验证 8 方重复去重、双重身份冲突、任务写入失败整体回滚、区域隔离、schema 混用拒绝和最小权限角色；收件角色可 INSERT／SELECT Inbox 与任务，不能修改区域身份。
- 新增 `region-migrate` 入口。实际 CLI 在临时容器专用数据库上重复迁移成功，改绑另一区域退出失败，日志 `/tmp/gamepanel-region-cli.log`、`/tmp/gamepanel-region-cli-rejected.log`。
- 工作区全量 Go／vet、区域 PostgreSQL race 通过；最终独立快照的全量 Go（含架构门禁）／vet 以及全局＋区域 PostgreSQL race 全部通过。独立日志 `/tmp/gamepanel-region-inbox-index-all.log`、`/tmp/gamepanel-region-inbox-index-vet.log`、`/tmp/gamepanel-region-inbox-index-pg.log`。临时数据库和角色均清理，其他草稿保留。
- 尚未接入 Broker 消费循环／确认、授权修订获取、Deployment、任务租约与重试、死信／对账、结果 Outbox；两个测试 schema 不代表两个完整 Region 或跨主机容灾验收。总 Goal 继续进行。

### 2026-09-08 区域消费确认与持久死信转移

- 新增 `regional.Ingress`，以消费接口连接区域 Store；校验内容类型、大小、头／体事件 ID、Region、schema、正版本、未知／重复 JSON 字段及尾随内容。确定无效或身份冲突消息交给隔离队列，不创建部署；数据库错误保持重试。
- 新增真实 RabbitMQ Consumer 和 `region-receiver` 独立入口，预取 1，区域事务成功后才手动 ACK。处理失败在当前未确认消息上限速重试；取消或断连导致 ACK 不确定时保留重投，由 Inbox 去重吸收。计数只表示 ACK 写出，不是 Broker 对 ACK 的确认。
- 官方核查发现 quorum 队列默认重投上限后可无死信丢弃消息，因此同步补齐显式持久隔离队列、可配置 delivery-limit、reject-publish 和 at-least-once 死信策略。隔离队列不自动消费／过期，并禁用二次重投上限。发布端和接收端必须配置相同拓扑。
- 新队列参数与旧队列不兼容时拒绝声明，保留旧队列及消息，不自动删除重建；已有部署需显式队列迁移。操作方式和官方依据见 [接收说明](../architecture/regional-inbox.md)。
- 真实 Broker 测试覆盖临时失败后确认、坏消息隔离、取消前已处理但未 ACK 的重投、显式拒绝／重投上限转入死信，以及旧拓扑消息保留。实际 Broker→区域 PostgreSQL 重复通知测试发送两个 ACK，仅生成一个 Inbox 和一个 `awaiting_revision` 任务。
- 工作区及最终独立提交快照的全量 Go（含架构门禁）／vet、真实 MQ race 和全局＋区域 PostgreSQL／Broker 整合 race 全部通过。独立日志 `/tmp/gamepanel-consume-index-all.log`、`/tmp/gamepanel-consume-index-vet.log`、`/tmp/gamepanel-consume-index-rabbit.log`、`/tmp/gamepanel-consume-index-bridge.log`。测试只使用本机专用临时容器，前端与其他草稿未纳入本批。
- 授权修订获取、Deployment 和执行任务、结果 Outbox、死信审计重放／告警、隔离队列不可用故障验证及跨主机容灾仍未完成；通知接收成功不能替代业务交付或六阶段验收。

### 2026-09-08 全局修订的区域归属读取边界

- 新增 `GetRegionalRevision` 内部只读 Adapter 与 `regional.RevisionSnapshot`。在一个快照中按 ID 读取并验证原始 Outbox、租户实例、当前 Region／epoch 和不可变修订，无 JOIN；失败不返回部分配置。
- 快照同时携带历史修订及读取时最新 specGeneration／desiredState／intentVersion，避免延迟通知被当作当前配置或覆盖停止意图。该数据不是执行授权，也不证明已有不透明配置已正确加密。
- SQLite／PostgreSQL 共用用例覆盖合法快照、错误或空服务区域、伪造事件／操作／租户／修订、延迟修订、更新后的停止意图、过期 epoch、区域迁移后的读取拒绝及删除后的拒绝。
- 工作区及最终独立提交快照的全量 Go（含架构门禁）／vet 与真实 PostgreSQL race 全部通过，独立日志 `/tmp/gamepanel-revision-source-index-all.log`、`/tmp/gamepanel-revision-source-index-vet.log`、`/tmp/gamepanel-revision-source-index-pg.log`。前端和其他草稿未纳入本批。
- 重新核对确认：区域服务身份认证、实际配置保护器、跨层远程读取、权益及有限期执行授权仍未实现，不能把调用方提供的区域字符串直接视为认证。下一步接入这些边界及区域任务 materializer；六阶段 Goal 不变。

### 2026-09-08 控制面 mTLS 服务身份与修订 HTTP 入口

- 新增 `serviceauth`，以经过 TLS 验证的唯一 URI SAN 和外部 allowlist 绑定 Region，拒绝自报请求头、未登记身份和过期链。配置入口提供证书／信任根，不把服务身份写死在业务代码；当前没有自动签发、热轮换或在线撤销。
- 新增消费接口驱动的 `controlapi` 及 `global-control` 独立入口，强制 mTLS，以已有只读快照 Adapter 取得修订。HTTP 和 MQ 复用事件校验；限制请求体与处理时间，拒绝重复字段与跨区读取，错误响应不暴露数据库详情，成功仍不代表执行授权。
- 本批没有新增或改变 SQL 查询；Store 仅增加领域错误映射，保留原 NotFound 兼容。模块架构门禁限制服务身份和 HTTP 层对具体存储／运行时的依赖。数据库访问约束同步进入模块化方案。
- 工作区与独立暂存快照的全量 Go（含架构门禁）／vet，以及真实本地 TLS、区域与 Store race 测试全部通过。独立日志 `/tmp/gamepanel-mtls-index-all.log`、`/tmp/gamepanel-mtls-index-vet.log`、`/tmp/gamepanel-mtls-index-race.log`。证书只在测试内存生成，没有提交真实凭证；本批尚未运行独立 CLI 与 PostgreSQL 的端到端连接测试。
- Region 远程客户端、任务 materializer、配置保护器、权益与有限期授权仍待实现。六阶段 Goal 保持进行中，生产证书生命周期与跨主机交付不以本次本地测试替代。

### 2026-09-08 区域修订 HTTPS 客户端与快照校验

- 新增 `controlclient`，组合根注入固定 Region、HTTPS 地址、客户端证书／服务端 CA、超时及载荷上限。验证服务端证书和主机名，禁用重定向与环境代理，限制响应读取；不在任务错误中暴露 URL 或服务端错误正文。
- `RevisionSnapshot.ValidateFor` 校验完整事件、实例／修订身份、代数、正意图版本、允许的期望状态及规格。历史配置与较新的停止意图可共存，读取失败不返回部分数据。没有新增 SQL，也没有把快照变成执行授权。
- 真实 mTLS 覆盖客户端→现有控制面 Handler 往返；HTTPS 测试覆盖篡改修订、旧代数、非法状态、尾随／超大响应、404／403／503、重定向拒绝、超时与 context 取消。
- 初次超时夹具因未消费请求体阻塞在测试服务 Close；对已确认的测试进程取 goroutine 堆栈定位后，修复读取与明确清理出口。修复后工作区相关 race 全部通过；独立提交快照全量 Go（含架构门禁）／vet／相关 race 全部通过，日志 `/tmp/gamepanel-controlclient-index-all.log`、`/tmp/gamepanel-controlclient-index-vet.log`、`/tmp/gamepanel-controlclient-index-race.log`。本批未改 SQL，未重跑外部 PostgreSQL／Broker 集成，前端及其他草稿未纳入提交。
- 客户端尚未接入区域持久任务进程；任务领取／恢复、快照持久化、配置保护器和有限期执行授权继续推进。此批不代表两个完整 Region、游戏交付、生产证书生命周期或容量验收完成。

### 2026-09-08 区域修订任务领取与持久快照

- 区域迁移 002 增加数据库时间租约、唯一领取令牌、尝试次数、重试时间和快照；保留历史迁移字节。单表 SKIP LOCKED 领取，落库先锁任务再核对时间、令牌和完整事件。
- 新增消费接口驱动的 `regional.Fetcher`，网络请求在事务外，失败延迟重试，成功原子保存快照和 `revision_fetched` 状态；该状态不授予执行权，也不覆盖全局最新配置或区域运行状态。
- 专用真实 PostgreSQL race 验证 8 方并发唯一领取、租约到期、新连接重新领取、旧令牌拒绝、持久重试间隔、失败再成功、停止意图保留及重复通知不重置。独立快照全量 Go（含架构检查）／vet 与全局＋区域 PostgreSQL race 全部通过，日志 `/tmp/gamepanel-revision-tasks-index-all.log`、`/tmp/gamepanel-revision-tasks-index-vet.log`、`/tmp/gamepanel-revision-tasks-index-pg-fixed.log`。
- 额外全局集成首次失败在旧夹具的无密码运行角色：临时数据库默认密码认证拒绝连接。确认服务端日志后，仅重建本批专用本机容器以匹配夹具认证条件，重跑通过；未修改项目生产认证配置。测试容器随后清理。
- 独立 Fetcher 入口、配置保护器、授权有效期、Deployment／执行任务及结果回传仍待实现；本次不代表游戏交付、跨主机恢复或容量验收。其他草稿与前端未纳入本批。

### 2026-09-08 独立区域获取入口与数据库／mTLS 组合验证

- 新增 `region-fetcher`，从外部配置读取区域数据库、服务证书与控制面地址，组合真实 Store、HTTPS Client 和 Fetcher；单并发持续处理、无任务／失败限速等待，任务与 HTTP 超时小于租约，信号取消关闭连接。
- 组合测试使用真实全局和区域 PostgreSQL schema，创建逻辑实例并取得 Outbox 通知，通过真实 mTLS 修订接口读取并保存快照。控制面先返回失败，确认持久重试时间后停止／重新启动循环，再验证成功落库和退出。
- 通知在测试中直接进入 Inbox，未经过 MQ；重新启动的是入口运行循环，不是 OS 进程。配置仍为测试用不透明载荷，不能冒充已实现配置加密。工作区组合 race 与独立提交快照全量 Go（含架构检查）／vet／真实 PostgreSQL+mTLS 组合 race 全部通过，独立日志 `/tmp/gamepanel-fetcher-cli-index-all.log`、`/tmp/gamepanel-fetcher-cli-index-vet.log`、`/tmp/gamepanel-fetcher-cli-index-integration.log`。临时测试证书、schema 和专用容器清理。
- 配置保护器、有限期执行授权、Deployment／调度／执行和结果回传继续推进。其他草稿与前端未纳入本批；总 Goal 不变。

### 2026-09-08 配置认证加密 Adapter 与身份绑定

- 新增 `ConfigurationBinding` 与 `configprotection`，外部注入 AES-256 密钥，使用标准库随机 nonce GCM，绑定租户／实例／修订／代数／Provider／schema、用途和 keyId。密文版本、大小、身份与认证标签失败统一拒绝，不返回部分明文。
- 活动密钥加密新内容，保留密钥读取历史密文；初始化复制密钥状态，支持并发调用。标准库每密钥最多 `2^32` 次加密的跨进程计数／轮换约束须由后续密钥管理落实，未宣称已具备生产 KMS 能力。
- 定向 race 覆盖往返、逐字节篡改、身份及 keyId 替换、随机输出、密钥状态复制、轮换和并发。独立快照全量 Go（含架构检查）／vet／相关 race 全部通过，日志 `/tmp/gamepanel-protection-index-all.log`、`/tmp/gamepanel-protection-index-vet.log`、`/tmp/gamepanel-protection-index-race.log`。本批未改数据库／MQ 路径，未重跑外部集成；未提交密钥或明文配置。
- 已确认现有 Store 用密文做幂等摘要且在事务内分配 ID，不能直接接上随机加密。后续需调整受信写入用例与稳定幂等摘要版本；当前 Adapter 尚未用于业务修订，旧不透明测试载荷不是已加密证据。没有新增 SQL，其他草稿保留。

### 2026-09-08 受保护创建事务与版本化幂等摘要

- 新增 `CreateEncryptedGlobalServer`，共享原创建的成员锁、配额及事务持久化路径，调用方单独提供规范化配置字节和消费接口实现。新操作分配实例／修订 ID 后才认证加密，重放不再次加密；加密失败不写实例或 Operation。
- `Fingerprinter` 使用独立外部 HMAC-SHA256 密钥，摘要携带 h1 版本和 keyId，按原记录密钥验证重放，支持保留旧密钥的轮换。旧 64 位摘要继续由旧入口使用，不隐式升级或混用；业务配置不写入请求摘要或 Outbox。
- SQLite／PostgreSQL race 已验证真实密文落库并解密、随机加密下重放身份稳定、摘要换钥、不同配置冲突、无权重试拒绝、加密失败无残留及敏感字节不落明文。独立快照全量 Go（含架构检查）／vet／SQLite 与 PostgreSQL race 全部通过，日志 `/tmp/gamepanel-encrypted-create-index-all.log`、`/tmp/gamepanel-encrypted-create-index-vet.log`、`/tmp/gamepanel-encrypted-create-index-pg.log`。专用临时容器与快照清理，未提交密钥。
- 这是内部受信创建边界，公共应用用例、Provider／资产／Region 校验及配置更新路径尚未切换。有限期授权与区域解密继续推进；未新增 JOIN，其他草稿保留。

### 2026-09-08 受保护配置更新与真实密文跨层获取

- 新增 `ReviseEncryptedGlobalServer`，共享原修订事务，权限／幂等／代数／配额检查后才为新修订加密。更新摘要显式包含 revise 类型，保留创建 h1 摘要兼容；只推进配置指针，不覆盖停止意图或部署归属。
- SQLite race 覆盖更新密文解密、旧身份拒绝、摘要换钥重放不重新加密、不同配置冲突、无权重放、旧代数拒绝、加密失败不推进指针、较新更新后旧操作重放不回退。
- 区域获取组合测试改用真实受保护创建；全局库密文经 mTLS 读取进入区域库，测试侧按原修订身份可解密且快照不含明文。密钥仅由测试创建，不是区域密钥分发／解密授权实现。工作区与独立快照的相关 SQLite／PostgreSQL／mTLS race，以及独立全量 Go（含架构检查）／vet 全部通过。独立日志 `/tmp/gamepanel-encrypted-revise-index-all.log`、`/tmp/gamepanel-encrypted-revise-index-vet.log`、`/tmp/gamepanel-encrypted-revise-index-integration.log`；专用临时数据库与快照清理。
- 公共应用用例、Provider／资产／Region 准入、密钥生命周期、有限期执行授权及 Deployment／运行链路仍待完成。旧创建／修订 API 未切换，本批未新增生产 SQL，其他草稿保留。

### 2026-09-08 Provider 显式全局配置与稳定归一化

- 新增 `LogicalConfigProvider` 可选能力和 `gameconfig.LogicalNormalizer`：使用只读 Registry，核对 Provider／已声明游戏版本／正 schema，限制载荷大小，拒绝非对象、重复字段、尾随内容和非法 UTF-8，输出稳定 JSON，不依赖 Store 或 Runtime。
- Terraria Vanilla／tModLoader 明确允许游戏用户配置字段，拒绝未知字段、null、非法值和配置行注入；全局配置不接收或保存端口／节点／宿主机路径。游戏规则位于 Provider，通用模块不硬编码游戏。
- 定向 race 验证两种 Provider 的稳定字节、密码和玩家设置保留、执行字段与非法输入拒绝、未知版本／旧 schema 别名拒绝。区域获取组合测试切换为真实 Terraria 归一化配置，随后加密、全局写入、mTLS 获取及区域落库。工作区相关 race 与独立全量 Go（含架构检查）／vet／真实 Provider+PostgreSQL+mTLS 组合 race 全部通过。独立日志 `/tmp/gamepanel-logical-config-index-all.log`、`/tmp/gamepanel-logical-config-index-vet.log`、`/tmp/gamepanel-logical-config-index-integration.log`；专用测试容器与快照清理。
- 公共应用用例、Region／资产准入、其他 Provider 的全局配置能力及有限期执行授权仍待实现；校验已声明游戏版本不等于制品摘要固定或真实游戏交付。未修改生产 SQL，其他草稿保留。

### 2026-09-08 实例应用编排与受保护 Writer 组合

- 新增 `instanceapp.Service`，通过消费接口编排准入、Provider 规范化、受保护创建／更新，不依赖数据库或 HTTP。拒绝客户端自报密文；临时规范化字节在同步写入后清除。架构门禁保护导入边界。
- 新增 `EncryptedIntentWriter` 在组合阶段绑定真实 Store 与密钥实现；数据库原有权限、配额、版本事务复核继续保留。缺少 Writer／Normalizer／Admission 时不能构造应用服务，不提供默认准入放行。
- 测试使用真实 Terraria Provider、AES/HMAC 与 SQLite，验证拒绝 Region／资产／客户端密文／非法配置、规范化后重试身份稳定、受保护更新及调用方输入不被清除。Admission 是明确的测试夹具，不是生产准入。工作区定向 race 与独立全量 Go（含架构检查）／vet／应用组合 race 全部通过，独立日志 `/tmp/gamepanel-instanceapp-index-all.log`、`/tmp/gamepanel-instanceapp-index-vet.log`、`/tmp/gamepanel-instanceapp-index-race.log`。本批未重跑外部 PostgreSQL/MQ，独立临时快照清理。
- 真实 Region／资产准入及事务一致性尚未实现；正式 HTTP 接入前还需区分旧操作重放与新操作可售性检查。公共 API 未切换，完整六阶段 Goal 不变；本批无生产 SQL 改动，其他草稿保留。

### 2026-09-08 授权重放与新操作准入分离

- Writer 增加授权重放查询，复用原创建／更新的摘要编码，保持历史 h1 字节兼容。事务内复核成员写权限，单表按租户／操作类型／幂等键读取并验证；同键不同参数拒绝，缺失原操作返回未命中。
- 应用服务规范化后先查重放，仅新操作执行 Region／资产准入。已持久化操作不被新操作下架规则阻止，但原 Operation 仍可能 pending；不把重放成功当成运行完成。后续写事务继续复核权限与幂等，早期查询不授予永久权限。
- 真实 SQLite 应用测试覆盖创建／更新重放、下架后原操作返回、不同参数冲突、新操作仍拒绝、成员撤销后重放拒绝。Store 共用测试覆盖较新修订后的原操作读取、跨用户拒绝、摘要冲突与未命中。工作区 race 与独立全量 Go（含架构检查）／vet／SQLite+PostgreSQL 应用及重放 race 全部通过。独立日志 `/tmp/gamepanel-replay-admission-index-all.log`、`/tmp/gamepanel-replay-admission-index-vet.log`、`/tmp/gamepanel-replay-admission-index-integration.log`；专用临时数据库与快照清理。
- 真实 Region／资产目录与事务内准入仍待实现，历史 Provider／schema 移除时的规范化兼容尚未解决；公共 API 未切换，六阶段 Goal 保持进行中。新增查询无 JOIN，其他草稿保留。

### 2026-09-08 全局 Region 目录持久化

- 新增独立 `regions.Entry` 与 global_regions 表，PostgreSQL 017／SQLite 版本 5；只管理区域 ID、名称、创建开放状态和版本，不推断节点、不创建资源／监控系统。注册默认关闭，不覆盖重复 ID。
- 更新采用版本 CAS，分页按 ID 游标有界读取，SQL 均为单表。SQLite 测试覆盖重复注册、稳定分页、过期更新拒绝、关闭及重复迁移保留状态；PostgreSQL 同时验证同版本并发只有一个成功。独立全量 Go（含架构检查）／vet／SQLite 及真实全局＋区域 PostgreSQL race 全部通过。独立日志 `/tmp/gamepanel-region-directory-index-all-fixed.log`、`/tmp/gamepanel-region-directory-index-vet.log`、`/tmp/gamepanel-region-directory-index-integration.log`；专用容器与快照清理。
- 首次独立全量检查发现旧制品升级夹具删除全部 SQLite 迁移记录但遗漏新目录表；已补齐夹具的旧 schema 恢复，生产迁移仍严格拒绝未登记却已存在的表，不用 IF NOT EXISTS 掩盖不一致。
- 目录仍为受信内部 Store 接口，运维入口、目录权限／审计、Region／资产事务准入及历史区域登记还需继续接入。关闭目录不代表已有实例停止或资源可释放。其他草稿保留，详情见 [目录说明](../architecture/global-region-directory.md)。

### 2026-09-08 受保护创建的 Region 事务准入

- 新受保护创建在权限／幂等重放之后，按 ID 单表检查已登记区域的创建开放状态，再继续配额、加密及实例／Operation／Outbox 原子写入。PostgreSQL 共享行锁与目录关闭 UPDATE 串行化，SQLite 复用已有写事务；没有新增 JOIN 或关联子查询。
- SQLite／PostgreSQL 验证未登记或已关闭区域拒绝新操作、已关闭区域的旧操作仍可授权重放。真实 PostgreSQL 阻塞加密夹具验证目录关闭等待在途事务、提交后关闭成功及关闭后的创建拒绝。首次检查的超时错误断言受驱动合并错误影响，已改为核对上下文超时后通过。
- 独立快照的根模块全量 Go（含架构门禁）／vet、API 全量 Go／vet、SQLite＋PostgreSQL＋mTLS 区域组合 race 均通过。证据日志 `/tmp/gamepanel-region-admission-index-root-all.log`、`/tmp/gamepanel-region-admission-index-root-vet.log`、`/tmp/gamepanel-region-admission-index-integration-fixed.log`；未更改前端或历史迁移。
- 旧不透明 CreateGlobalServer 仍为内部兼容路径，不可直接作为新公共创建入口。资产授权、修订／扩容准入、目录运维入口、HTTP 接入、有限期执行授权及完整六阶段验收仍待完成；禁止 JOIN 不代表数据库性能已经通过容量验收。其他草稿保留。

### 2026-09-08 已发布资产目录与批量事务准入

- 新增独立 assets 契约、全局资产身份及不可变版本目录，PostgreSQL 018／SQLite 版本 6。固定组织归属、版本摘要和大小；相同元数据重复发布安全，不同归属或内容不能覆盖。数据库禁止更新／删除，SQLite 另防止 REPLACE 更换内容。旧文件记录不自动冒充经过内容验证的全局版本。
- 受保护创建／配置更新在既有权限、幂等及事务边界内验证所属组织和精确资产版本，每 100 个引用分两次单表查询。真实 SQLite／PostgreSQL 测试对 101 个引用观察到四次查询，无 JOIN；同时覆盖缺失资产、缺失版本、跨组织引用拒绝，真实加密修订保留引用及不可变约束。目录登记是受信入口，尚未接入内容上传／校验或公共发布 API。
- 独立快照全量 Go（含架构门禁）／vet，以及资产、Store、应用服务、PostgreSQL＋mTLS 区域组合 race 全部通过，日志 `/tmp/gamepanel-global-assets-index-all.log`、`/tmp/gamepanel-global-assets-index-vet.log`、`/tmp/gamepanel-global-assets-index-integration.log`。历史迁移未修改；旧 SQLite 升级夹具补齐新表清理，其他草稿与前端保留。
- 资产副本、内容验证回报、共享／撤销、保留期 GC、有限期执行授权及公共入口仍需完成。查询次数证据只覆盖本批准入，不能作为全库 SQL 门禁或容量验收。完整六阶段 Goal 保持进行中，见 [资产目录边界](../architecture/global-assets.md)。

### 2026-09-08 授权区域修订的精确资产清单

- 区域修订读取在既有事件／实例／Placement 授权和只读快照内，按租户及精确版本解析资产元数据。复用有界单表批量查询，以映射保留引用顺序；失败不返回部分结果。历史修订保持原资产摘要，旧内部写入路径产生的跨组织或缺失引用也不能通过读取边界。
- RevisionSnapshot 增加已发布资产清单，区域客户端及持久化入口验证其与修订引用一一对应；拒绝遗漏、多余、重复、错版本、错租户、非法摘要与负大小。真实 PostgreSQL／mTLS 组合验证元数据随受保护配置跨接口并持久化到独立区域库；测试发布元数据不是内容上传或文件交付证明。
- 独立快照全量 Go（含架构门禁）／vet，以及 SQLite＋PostgreSQL、区域契约、mTLS 客户端与区域获取组合 race 全部通过。日志 `/tmp/gamepanel-asset-manifest-index-all.log`、`/tmp/gamepanel-asset-manifest-index-vet.log`、`/tmp/gamepanel-asset-manifest-index-integration.log`。未修改历史迁移、前端或无关草稿。
- 有资产引用却缺少清单的历史快照必须授权补取，尚未实现已保存任务批量补取工具；新执行链路前需协调升级。区域副本内容验证、实际分发、有限期执行授权及其余六阶段目标继续推进，完整 Goal 不变。

### 2026-09-08 历史资产清单补取工具

- 新增 region-repair-manifests 受信运维入口，默认预览，显式 apply 才将身份有效但整份资产清单缺失的 revision_fetched 任务重新排队。按 Operation ID 有界分页、单表事务锁定，不使用 JOIN；保留原事件、任务、尝试次数和旧快照，由正常 mTLS 获取成功后替换。
- 报告游标、候选与异常任务 ID，不输出配置。已物化／拒绝／等待中任务不改写；错误身份、损坏或超过 4 MiB 的快照只报告，不自动修复。错误租户／修订代数不能通过补取资格检查。支持从已报告游标继续和重复运行；结束后需从头预览覆盖并发新增记录。
- 真实 PostgreSQL 验证预览不写、两个并发修复者合计只重置两个任务、重复无重置、原快照及计数保留、异常记录和其他生命周期不变、重新领取后仍拒绝旧不完整快照且接受完整清单。独立快照全量 Go／架构／vet、Store＋regional＋区域获取组合 race 和补充契约 race 通过。日志 `/tmp/gamepanel-manifest-repair-index-all.log`、`/tmp/gamepanel-manifest-repair-index-vet.log`、`/tmp/gamepanel-manifest-repair-index-integration.log`、`/tmp/gamepanel-manifest-repair-index-contract.log`。
- CLI 已在专用临时 PostgreSQL 实际执行迁移和默认预览，返回 applied=false、scanned=0，证据 `/tmp/gamepanel-manifest-repair-cli-preview.log`；这项 CLI 探针不冒充生产历史数据升级验收。重新排队不等于授权补取完成，更不代表运行已停止。未改历史迁移、前端或无关草稿；完整六阶段 Goal 保持进行中。

### 2026-09-08 区域文件内容校验与原子发布

- 新增 assetfiles 本地 Adapter，接收已授权资产清单与源流，部署配置限定私有根目录；不依赖 Docker、HTTP 或数据库。以流式长度／SHA-256 校验写入私有临时文件，同步文件后原子重命名并同步目录，成功前不发布半成品。完整租户／资产／版本／摘要／大小参与文件定位。
- 源流读取／关闭错误、长度或摘要不符及取消均拒绝发布，取消关闭源流解除阻塞并清理本次临时文件。并发同版本写入安全；失败替换保留原文件。Open 重新校验普通文件内容，拒绝损坏和符号链接，返回只读句柄；不支持外部进程并发原地修改私有存储。
- 真实临时文件系统 race 覆盖并发、失败清理、取消、半成品不可读、原文件保留、身份隔离、损坏／符号链接、配置上限、空内容和关闭后重新打开。独立快照全量 Go／架构／vet 与定向 race 通过，日志 `/tmp/gamepanel-regional-files-index-all.log`、`/tmp/gamepanel-regional-files-index-vet.log`、`/tmp/gamepanel-regional-files-index-race.log`。本批无数据库／MQ 改动，未重跑外部集成；前端及其他草稿保留。
- 尚需接入源授权、区域任务、副本目录、总容量／背压、崩溃残留回收和 GC；文件存储成功不等于可执行。未以文件重开测试冒充进程强杀／断电或跨主机交付验收，详见 [区域资产文件](../architecture/regional-asset-files.md)。完整六阶段 Goal 保持进行中。

### 2026-09-08 修订资产准备编排与批次限制

- 新增 regional.AssetPreparer，通过消费接口组合内容源与真实文件存储。整份快照、目标 Region、文件数量、单文件及总字节限制在 I/O 前校验；扣减剩余额度避免总量溢出。每批统一截止时间，逐项获取和校验发布，最多一个活动源流。
- 源接口携带原通知与精确资产版本，要求重新验证当前访问权限；已有本地文件不绕过源授权。部分文件成功不会把整批标为成功，也不改变任务／意图／执行状态；已验证文件保留供重试及后续 GC。生产网络来源尚未实现，测试源明确为夹具。
- 真实文件 Adapter 组合验证中途摘要失败、重试成功、缓存存在时授权拒绝仍失败、数量／字节／Region／缺失清单在 I/O 前拒绝、整数溢出防护和超时解除阻塞。独立快照全量 Go／架构／vet 与 regional＋assetfiles race 通过，日志 `/tmp/gamepanel-asset-preparer-index-all.log`、`/tmp/gamepanel-asset-preparer-index-vet.log`、`/tmp/gamepanel-asset-preparer-index-race.log`。
- 本批无 SQL 或 MQ 改动，未重跑外部集成；其他草稿保留。持久准备任务、下载服务授权实现、副本目录、区域总配额及执行授权仍待接入；批次限制不作为生产容量验收，完整六阶段 Goal 保持进行中。

### 2026-09-08 持久资产准备领取与恢复

- 区域迁移 003 增加独立资产租约、重试时间、计数与 assets_prepared 状态及索引，历史迁移不变。ClaimAssets 单表 SKIP LOCKED 领取，锁内校验事件／Region／完整快照并使用数据库时间；坏记录持久延后避免占据队首，准备期间不持有数据库事务。
- AssetWorker 组合真实文件准备与持久任务。完成／重试按 token、事件、完整快照及数据库当前时间复核，过期或被替换的领取不能完成。修订重新获取清空资产 token 和时间，保留独立尝试计数。assets_prepared 不等于节点副本就绪、部署或运行授权。
- 真实 PostgreSQL＋文件 Adapter 测试覆盖并发唯一领取、过期拒绝、重连恢复、修改快照拒绝、重试延迟、源失败后重试成功、事件重放不重置完成状态及坏快照冷却。补充测试验证修订补取后旧资产 token 失效，修订／资产计数分别保持 2／5。源为明确夹具，不冒充生产下载授权。
- 独立快照全量 Go／架构／vet、Store＋regional＋mTLS 区域获取 race 及补充 PostgreSQL race 通过，日志 `/tmp/gamepanel-asset-tasks-index-all.log`、`/tmp/gamepanel-asset-tasks-index-vet.log`、`/tmp/gamepanel-asset-tasks-index-integration.log`、`/tmp/gamepanel-asset-tasks-index-refetch.log`。无 JOIN，其他草稿和前端保留。
- 生产内容源、独立资产 Worker 入口、副本／存储卷身份、容量预留、跨主机可用性与有限期执行授权仍需完成；多个不共享目录不能被当作同一可用副本。未以数据库重连替代进程强杀验收，完整六阶段 Goal 保持进行中。

### 2026-09-08 指定资产版本的区域访问校验

- 新增 ResolveRegionalAsset，与修订获取共享同一只读快照内的 Region 服务身份、原 Outbox 事件、租户实例、当前 Placement／epoch 和不可变修订校验。请求的资产 ID／版本必须被该修订引用，再按租户和精确版本查询目录；同租户未引用资产、新版本借旧修订访问均拒绝。
- 单资产访问仅读取所请求的目录项，不重复解析全部引用目录；SQL 保持单表，无 JOIN。失败返回空元数据；返回值不包含配置、文件路径或可复用下载票据，不能用一次成功解析代表永久访问权。
- SQLite／PostgreSQL 测试覆盖精确元数据、同租户未引用拒绝、伪造事件、错 Region、跨组织旧引用、缺失版本、历史资产版本保留、旧 Placement／Region 和删除实例拒绝。独立全量 Go／架构／vet 与 Store＋真实 PostgreSQL／mTLS 区域获取 race 通过，日志 `/tmp/gamepanel-asset-authorization-index-all.log`、`/tmp/gamepanel-asset-authorization-index-vet.log`、`/tmp/gamepanel-asset-authorization-index-integration.log`。
- 这是受信内部权限读取边界，生产下载 API、源副本绑定和有界传输授权仍需实现；内容不经全局控制面转发。未改历史迁移、前端或无关草稿，完整六阶段 Goal 保持进行中。

### 2026-09-08 mTLS 资产解析 API 与区域客户端

- global-control 同时挂载修订和指定资产解析路由。资产接口从已验证服务证书映射 Region，复用严格事件解码，拒绝重复／多余参数、重复事件字段、超大输入和 Region 不匹配。响应 no-store，只返回授权元数据，错误不泄露后端细节。
- 新增 controlclient.ResolveAsset，复用受信 HTTPS origin、客户端证书／CA、超时、无代理及禁止重定向策略，限制响应体并复核租户、ID、版本、摘要和大小。错误响应、尾随内容及错误身份返回空结果，不生成下载票据或文件地址。
- 真实 mTLS 验证正常往返、Header 伪造、其他 Region、重复参数／事件、超大输入、不可用与后端错误隔离；客户端验证错租户／资产／版本、超限、尾随 JSON 和重定向拒绝。PostgreSQL＋mTLS 组合直接调用真实 Store 权限解析，验证有效资产和不存在版本，并保留原修订获取回归。
- 独立快照全量 Go／架构／vet 与接口、客户端、真实 PostgreSQL 区域组合 race 通过，日志 `/tmp/gamepanel-asset-api-index-all.log`、`/tmp/gamepanel-asset-api-index-vet.log`、`/tmp/gamepanel-asset-api-index-integration.log`。未改生产 SQL、历史迁移、前端或无关草稿。
- 该 API 只提供当前时点的访问校验，源副本绑定、有界传输授权、实际下载及资产 Worker 生产入口仍需接入；不以可跨进程解析元数据代替真实文件交付验收。完整六阶段 Goal 保持进行中。

### 2026-09-08 源副本目录与版本化可用性

- PostgreSQL 019／SQLite 版本 7 新增副本目录，绑定精确资产版本、源 Region、StorageID、可用性和观测版本。同一区域存储上的同一资产版本只有一个副本身份；注册默认不可用，重复登记不覆盖观测状态，不能重绑已有 ID。StorageID 是受信存储身份，不是客户端 URL 或主机路径。
- 可用性更新要求源 Region 匹配和版本 CAS；授权来源查询在每页只读快照内复核目标 Region、租户、事件、Placement 及精确修订引用，随后按版本／可用性／ID 游标单表读取，每页最多 100 条。无 JOIN，不在控制面保存物理文件路径。
- SQLite／PostgreSQL 验证注册默认关闭、重复状态保留、存储位置唯一、缺失资产版本拒绝、跨 Region／旧观测拒绝、稳定分页、下架过滤及旧 Placement 拒绝。PostgreSQL 额外验证两个同版本观测只有一个成功。独立全量 Go／架构／vet、Store＋assets＋真实 PostgreSQL/mTLS 区域组合 race 通过，日志 `/tmp/gamepanel-replicas-index-all.log`、`/tmp/gamepanel-replicas-index-vet.log`、`/tmp/gamepanel-replicas-index-integration.log`。
- 历史迁移未改；旧 SQLite 升级夹具补齐新副本表清理，其他草稿与前端保留。目录尚未接入区域文件验证回报、真实存储身份、外部来源查询、传输票据或复制流程；可用观测不能当作实时健康或持久性证明。完整六阶段 Goal 保持进行中。

### 2026-09-08 指定源副本与目标权限联合校验

- 新增 AssetSourceSnapshot 与 ResolveRegionalAssetSource，在同一只读快照内核对目标 Region、原事件、当前 Placement、租户与精确修订引用，并要求指定副本 ID、资产版本、available 和观测版本全部匹配。返回原事件、不可变资产元数据及源 Region／StorageID，失败不返回部分结果。
- 选择其他资产的副本、未知／旧观测、下架来源和旧部署归属均拒绝；副本下架后重新开放也不能复用旧版本，需要刷新来源目录。新增查询仍为单表，无 JOIN，不长时间持有数据库事务等待传输。
- SQLite／PostgreSQL 测试覆盖正确跨 Region 来源、错误副本／资产绑定、错误目标、未知和过期观测、下架及重新开放、旧 Placement 拒绝；契约测试覆盖篡改身份、版本、可用性、存储标识与摘要。独立全量 Go／架构／vet、Store＋regional＋assets＋PostgreSQL/mTLS 组合 race 通过，日志 `/tmp/gamepanel-source-binding-index-all.log`、`/tmp/gamepanel-source-binding-index-vet.log`、`/tmp/gamepanel-source-binding-index-integration.log`。
- 联合校验结果仅为传输授权签发输入，尚未签名或绑定有效期限，源服务不能信任客户端自带该对象。实际下载票据、源服务验证、区域文件回报和资产 Worker 入口仍需接入。未改历史迁移、前端或无关草稿，完整六阶段 Goal 保持进行中。

### 2026-09-08 短期签名传输票据与源端验证

- 新增 transferauth，外部注入全局 Ed25519 私钥和源端公钥集合；构造时复制密钥状态。Issuer 每次调用真实源联合校验，签名绑定用途、版本、keyId、随机 ID、原事件、目标 Region、租户／资产摘要、源副本观测与 Region／StorageID。TTL 从查询开始计时，慢查询不延长授权。
- Verifier 绑定自身 Region／存储和经认证请求 Region，核验签名、规范 JSON、完整源绑定、当前时间及最大 TTL；拒绝过期／未来、重复签名字段、错误用途／版本和身份。允许同一授权 Region 在期限内重试，不提供单次消费或运行权限。
- 定向 race 覆盖逐字节载荷篡改、签名篡改、时间／TTL、错误目标／源、签名非法声明、密钥状态复制与重新签发再次授权。真实 SQLite／PostgreSQL 组合验证 Store 权限解析后签发与公钥验证、目录下架拒绝新票据。独立全量 Go／架构／vet 与 transferauth＋Store PostgreSQL race 通过，日志 `/tmp/gamepanel-transfer-ticket-index-all.log`、`/tmp/gamepanel-transfer-ticket-index-vet.log`、`/tmp/gamepanel-transfer-ticket-index-integration.log`。
- 尚未接入签发 HTTP、源端下载服务、整个传输期限强制关闭及部署密钥轮换／撤销流程；现存票据的全局撤销窗口由 TTL 限定，不能冒充实时撤销。无生产 SQL／迁移修改，前端和其他草稿保留。完整六阶段 Goal 保持进行中，详见 [票据约束](../architecture/asset-transfer-tickets.md)。
