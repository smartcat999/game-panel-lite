# 全局 Region 目录

目录是全局控制面的区域身份与创建可用性元数据，独立于节点列表、区域数据库、资源容量和监控。资源部署层级仍是 Region → Node；注册目录不会创建基础设施或区域服务凭证。

`regions.Entry` 包含稳定 ID、展示名称、acceptingCreates 与单调版本。ID 使用小写 ASCII 字母、数字、短横线和下划线，最多 128 字节。注册默认关闭创建；重复 ID 返回冲突，不覆盖已有名字或可用性。不提供删除入口，避免抹除仍被逻辑部署归属引用的身份。

Store 的 RegisterRegion／SetRegionAcceptingCreates 仅供受信运维组合入口调用，尚未开放给租户 HTTP。修改开放状态必须携带所见目录版本，以单表 CAS 更新；同版本并发决定只有一个成功，过期请求不能重新开放已关闭区域。关闭目录不停止现有运行，也不代表区域或节点已隔离。

GetRegion 按 ID 查询；ListRegions 使用 ID 游标及 1–100 条上限，不查节点、容量或监控，不使用 JOIN。展示名称和目录状态不是可用容量承诺，实际调度准入仍属于 Region。

PostgreSQL 迁移 017／SQLite 版本 5 新增 global_regions，不从旧节点数据推断或自动注册区域。已有逻辑实例的 Region 字符串暂时保留，不自动改写；后续受信准入必须连接目录并处理历史区域登记。区域 schema 不创建该表。

受保护创建 `CreateEncryptedGlobalServer` 在权限和既有幂等操作检查之后、配额检查及加密写入之前，按 ID 单表读取目录。新操作只接受已登记且 acceptingCreates 为真的区域；检查和实例／Operation／Outbox 写入属于同一事务。PostgreSQL 使用共享行锁，允许不同租户并发创建，同时让关闭目录的 UPDATE 等待已准入事务结束；SQLite 由已有写事务串行化。关闭事务完成后，新创建被拒绝，原操作重放仍先按既有授权规则返回。

旧 `CreateGlobalServer` 保留内部不透明配置兼容路径，尚不经过目录准入，不可直接作为新的公共创建入口。acceptingCreates 目前只约束新实例创建，配置修订、扩容及 Provider 可售策略需各自定义准入；资产版本授权、目录运维入口、权限与审计、公共 HTTP 接入仍待完成。目录开放不构成执行授权。
