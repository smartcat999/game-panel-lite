# Region 存档与备份对象存储

2026-09-08：用户确认优先核心功能，跨 Region 迁移允许不支持；存档与备份考虑接入自建 OSS。本方案采用兼容 S3 API 的可替换适配器，不要求独立文件下载服务。

## 资源归属与执行边界

用户备份任务归控制面管理。控制面校验租户与实例并事务写入任务／Outbox，MQ 下发到 Region；Region Inbox 去重后生成内部执行任务。Region 将执行状态和结果 Outbox 同事务保存，MQ 回传，控制面去重并复核自身任务后更新用户状态及发布备份资产。这是双向异步链路，不能让控制面等待文件上传，也不能让 Region 的上传状态直接成为用户备份成功状态。

- Global 管理租户、逻辑实例、资产版本、摘要和区域归属；不保存部署机绝对路径，不中转文件内容。
- Region 管理 StorageID 对应的受信 endpoint、bucket、凭证引用与存储策略。StorageID 不等于 bucket；禁止用户指定 endpoint 或凭证。每个 Region 可以配置独立对象存储；是否共享底层集群是部署选择，不能因此取消区域权限与配额隔离。
- Node 游戏进程使用本地数据目录。Provider 定义需要保存的文件，执行编排保证停服或提供经过验证的一致快照；不能边复制变化文件边宣称备份一致。挂载对象存储不作为默认运行方式。
- 备份模块负责归档格式、兼容性、安全解压和恢复回滚；S3 SDK 只进入具体存储适配器，由组合根注入。HTTP 只调用用例，不实现上传、重试或对象路径拼接。

## 同 Region 备份与恢复链路

备份任务领取后，先取得实例操作权限并生成一致归档，在本地计算全文件 SHA-256 和大小，上传到服务器生成的唯一对象键。上传成功并验证后，才发布可恢复的备份记录。失败或结果未知不能发布为成功；任务重试复核已上传对象。后台回收无引用对象需有宽限期，不能清理仍被任务或备份引用的对象。

恢复任务按租户、实例、备份 ID 读取已发布记录，复核区域、对象身份和版本。下载到受控暂存目录，限制大小、时间和磁盘用量，验证完整 SHA-256 后，在实例停止且独占操作期间调用现有恢复流程。兼容性检查先于文件替换，配置提交失败回滚。只有恢复与配置提交成功后，才能按用户期望状态启动。

数据库保存不透明对象键、StorageID，以及后端支持时返回的对象 VersionID；业务资产版本与对象存储 VersionID 分开。不能通过列举 bucket 作为租户备份清单，不能用 latest 对象替代用户选择的不可变备份。查询仍按 ID／有界批量查询后在 Go 中组合，禁止 JOIN。

ETag 不作为全文件 SHA-256，分片上传时也不应假定它是全文件 MD5。适配器需明确验证所选自建服务支持的校验与版本语义，不能由“兼容 S3”推定全部 AWS 功能可用。参考 [S3 对象校验说明](https://docs.aws.amazon.com/AmazonS3/latest/userguide/checking-object-integrity-upload.html)。

## 当前实现与下一步

全局迁移 020／SQLite 版本 8 增加 `global_backup_tasks` 与 `backup_request_outbox`。`RequestGlobalBackup` 在租户写权限锁内检查幂等键，对目标租户实例加锁后读取当前配置版本、意图版本和 Placement；原子创建 `server_operations(kind=backup)`、用户备份任务和专用下发 Outbox。相同请求返回原命令与当前任务状态，参数冲突拒绝。命令不包含 Node、主机路径或存储凭证，Region 负责后续执行绑定。普通配置通知仍使用原 `server_outbox`，避免备份命令被旧修订消费者错误处理。此入口目前是 Store 用例边界，公共 HTTP 仍待接入。

`outbox-publisher -stream backup-requests` 选择专用备份 Outbox，默认 `-stream revisions` 保持原行为；queue 和 dead-letter-queue 必须配置为备份专用队列，不可连接现有修订消费者。两种流复用私有 SQL Outbox 实现与已确认 RabbitMQ Publisher；表名只在 Store 内固定选择，不接受外部 SQL 表名。备份发布确认、重试、过期领取和原流隔离均需验证，消息发送成功仍不代表 Region 已接收落库或完成执行。

`region-receiver -stream backup-requests` 使用备份专用接收入口。入口限制消息大小及 JSON 类型，拒绝未知／重复字段、信封 ID 不匹配及目标 Region 不匹配。区域迁移 005 增加备份 Inbox 与待执行请求；消息事件和稳定操作身份分别去重，同一备份 ID 不能属于不同操作。Inbox 与请求原子保存，失败时不确认消费，冲突内容按不可重试通知处理。落库状态为 `awaiting_authority`，配置快照、执行授权、Node 快照一致性及后续上传绑定仍需协调器完成。

区域迁移 006 为结果 Outbox 增加发布租约、重试、尝试计数和确认时间。`outbox-publisher -stream backup-results` 使用 `GAMEPANEL_REGIONAL_DATABASE_URL`，打开并验证 Region 数据库身份，再向 `GAMEPANEL_RABBITMQ_URL` 指定的控制面结果 broker 发布。此流的 `-region` 表示来源 Region；Outbox 适配器只能使用绑定身份，不接受其他 Region。来源身份由数据库配置提供，不通过业务表 JOIN 或信任结果 payload 获取。它复用发布确认与重试实现，不更新控制面用户任务。

```sh
# 数据库和 broker 连接通过部署环境注入；队列名为示例。
go run ./apps/api/cmd/outbox-publisher -stream backup-requests \
  -region east -queue gamepanel.east.backup-requests \
  -dead-letter-queue gamepanel.east.backup-requests.dead
# 在使用 east 独立数据库和 broker 凭证的 Region 进程中运行：
go run ./apps/api/cmd/region-receiver -stream backup-requests \
  -region east -queue gamepanel.east.backup-requests \
  -dead-letter-queue gamepanel.east.backup-requests.dead
# Region 发布结果；broker 凭证仅允许写入对应来源的控制面结果队列：
go run ./apps/api/cmd/outbox-publisher -stream backup-results \
  -region east -queue gamepanel.control.east.backup-results \
  -dead-letter-queue gamepanel.control.east.backup-results.dead
```

区域迁移 004 增加 `regional_archive_uploads` 和 `regional_backup_result_outbox`。上传计划绑定控制面 OperationID／请求事件、Region、实例、Deployment、Node、Placement epoch、准备好的 SnapshotID、对象键及精确资产摘要。控制面操作与存储对象绑定均有唯一约束；重放只能接受完全相同计划，不能更换节点、快照或归属。此登记接口仅供已完成授权与快照准备的受信协调器调用；目前尚未由控制面下发链路驱动。

Worker 领取使用数据库时间、单表 `FOR UPDATE SKIP LOCKED` 和独立领取令牌。完成／重试锁内复核原计划、令牌与未过期租约；损坏记录隔离，重试延迟持久保存。上传完成只转为 `uploaded` 并写入 `backup.archive.uploaded` 结果 Outbox，二者原子提交；Outbox 插入失败会回滚全部完成修改。结果 Outbox 已接入确认发布；全局 `RecordBackupResult` 已实现结果去重及任务／资产原子更新，生产消费者接线仍待完成。全部业务 SQL 为单表查询，无 JOIN。

已存在本地 ZIP 归档、兼容性检查、暂存解压和返回错误时的回滚。新增 `backup.RestoreArchiveChecked(io.ReaderAt, size, target, hooks)`，让经过验证的对象存储下载文件复用同一恢复实现；调用方持有并关闭源文件。本地文件名入口委托给此入口，不引入 SDK 或数据库依赖。

S3 适配器 `s3archive` 已实现，使用官方 AWS Go v2 SDK；消费接口定义在 `backup.ArchiveStore`。组合根显式提供 StorageID、HTTPS endpoint、签名 Region、bucket、凭证提供器、受信 CA、超时和对象大小上限；不自动读取环境代理或机器 AWS 配置。当前 bucket 名仅接受小写字母、数字与连字符，对象键接受服务器生成的小写字母、数字、连字符及下划线，均有长度限制。路径风格寻址支持自建 endpoint。

上传先校验调用方持有的不可变本地归档的完整 SHA-256 和精确大小，再携带校验头与 `If-None-Match: *` 执行单次 PUT。已有对象不能被覆盖；请求结果未知和重复键返回失败，后续持久任务必须对账，不能直接当作成功。当前仅实现配置上限内的单次 PUT，最大 5 GiB，不支持分片上传。源文件在调用期间必须保持不变，读取取消检查发生在读取边界。

`ArchiveStore.ResolveUpload` 为上传回执丢失提供对账入口：任务必须持久保留其专属对象键和预期资产摘要／大小。适配器 GET 当前对象，验证完整内容及 EOF，返回同一 GET 响应中的后端 VersionID；不能只看对象存在、HEAD 元数据或 ETag。错误内容、截断、超时和不存在对象均返回空引用。若当前对象已被外部写入不同内容，对账拒绝采用；已知旧版本仍可通过精确读取恢复。返回引用不等于任务成功，持久任务仍须复核领取权和实例权限，再事务发布备份记录。无版本后端仍有后续外部覆盖风险，恢复时必须继续校验摘要。

读取核对 StorageID、对象键和精确后端 VersionID（存在时），检查响应版本与大小。禁止重定向，整个响应流有时间上限。返回的流仍须由消费方在暂存期间校验摘要，不能直接恢复或发布为已验证资产。空 VersionID 表示后端未返回版本；仍依赖唯一键和下载摘要检查，不能推定后端启用了版本保护。

已完成真实 SDK + TLS HTTP 协议夹具验证（签名头、路径、条件上传、版本读取、冲突、无效输入、错误版本／大小、重定向及阻塞超时），并补充本机独立 MinIO 容器兼容性测试，见下文。持久对象记录、上传发布状态机、区域配额、应用任务接线、保留与回收流程，以及生产部署验收仍未完成。恢复流程目前也不提供进程崩溃回滚保证，解压总量限制仍须补齐。

SDK endpoint 与校验配置参考 [官方 endpoint 配置](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/configure-endpoints.html)和[校验配置](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/s3-checksums.html)。未以 SDK 成功响应替代目标自建服务的协议兼容性、持久性或容量证据。

接下来按核心闭环推进：区域存储配置与适配器、可靠上传及备份记录发布、同 Region 恢复与故障验证。跨 Region 对象复制和迁移不属于此轮前置条件。

## 可复现的本地对象服务验证

测试使用固定版本 MinIO 作为 S3 兼容性目标，不代表生产产品选型或生产版本推荐。该测试不会连接用户配置的远端 endpoint；启动仅绑定 `127.0.0.1` 随机端口的独立容器，临时生成 TLS 证书和凭证，创建专用 bucket 并启用版本，结束时停止并删除容器及测试目录。不挂载任何用户数据。

```sh
docker pull minio/minio:RELEASE.2025-04-22T22-12-26Z
GAMEPANEL_TEST_MINIO=1 go test -race ./apps/api/internal/s3archive -run TestMinIOArchiveIntegration -v
```

2026-09-08 本地验证通过，拉取的镜像摘要为 `sha256:a1ea29fa28355559ef137d71fc570e508a214ec84ff8083e39bc5428980b015e`。测试覆盖真实 ZIP 归档上传、条件写入冲突、后端返回版本、同键新增版本后读取旧版本、下载至本地并校验 SHA-256、元数据检查与文件恢复，以及不存在版本、错误凭证和服务端错误摘要拒绝。

归档内容为测试文件，未启动真实游戏，因此不证明运行中游戏快照一致性。它也不证明备份任务、租户 API、上传结果未知的对账、跨主机网络、断电持久性、HA 或容量要求；这些仍需各自验收。默认 `go test ./...` 跳过该 Docker 测试，必须显式设置测试开关才能得到实际对象服务证据。

## 控制面结果事务

全局迁移 021／SQLite 版本 9 仅增加 `global_backup_results` 一张表，以 OperationID 唯一保存回执并去重。接收事务锁定原任务，核对可信来源 Region、原请求事件、租户、实例、Placement epoch 及操作所属修订；相同操作的相同结果可重放，内容冲突拒绝。该校验关联原请求，不重新授予节点执行权限。

成功结果复用资产登记，在同一事务发布精确资产元数据、更新任务与 Operation，并保存完整对象回执。取消／失败任务的迟到结果记为 discarded，保留终态且不发布资产；对象的后续清理仍需保留策略。后续取消实现应与接收事务采用相同的任务→Operation 加锁顺序。Region 必须先完成授权和一致快照验证；全局受信接收不代替 Node 执行证明。当前尚未将真实结果消费者和用户 API 接入这一事务。
