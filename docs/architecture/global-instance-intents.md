# 全局实例意图的持久化基础

2026-09-08。本批实现逻辑模型和数据库事务，尚未切换 HTTP 创建入口，也未实现独立 Global／Region 数据库或 MQ 消费。目标边界见 [蓝图](architecture-target-blueprint.md)。

## 已实现的数据所有权

`instances` 包仅依赖标准库，定义全局逻辑身份、配置修订、部署归属、操作及消息身份。它不依赖旧 `domain.GameServer`、GORM、Provider 实现或 Runtime。

| 表 | 内容 | 当前规则 |
| --- | --- | --- |
| `logical_servers` | 组织、名称、当前修订、specGeneration、用户期望状态、intentVersion | 不存实际 NodeID、容器和路径 |
| `server_revisions` | 不可变规格、Provider／游戏版本、配置 schema、受保护配置、版本化资产引用 | 数据库触发器禁止 UPDATE／DELETE；每实例 generation 唯一 |
| `server_placements` | 目标 Region、placementEpoch | 修改配置不会修改区域或 epoch |
| `server_operations` | 操作身份、租户、目标实例／修订、幂等键及请求摘要 | 幂等范围为组织＋操作类型＋键；同键不同参数拒绝 |
| `server_outbox` | 操作的持久交付记录及目标区域 | 与实例／修订事务提交；每操作唯一记录；尚无发布器 |

配置修订不包含区域任务 generation、执行租约、运行路径、Env／Cmd 或实际容器状态。规格中的资产引用必须给出版本；这只校验引用形状，资产存在、内容摘要和租户授权仍需应用用例及资产模块验证。

`ProtectedConfiguration` 接收保护器生成的 keyId 和不透明密文。当前结构检查不能证明输入已正确加密，实际配置校验、保护器、密钥管理和区域解密授权仍待实现。不要把此内部接口直接暴露成允许客户端自称“已加密”的公共 API。

Outbox 仅包含 schemaVersion、eventId、operationId、organizationId、serverId、revisionId、regionId、placementEpoch 和 specGeneration；不包含配置文档。将来的接收方必须经过授权获取并持久化不可变修订，不能仅凭事件字段授权执行。全局断连期间尚未取得修订的事件不能被当作区域已接纳任务。

## 事务接口

- `CreateGlobalServer`：校验结构、锁定组织及成员写权限、检查新旧实例总配额，原子写入实例／初始修订／Placement／Operation／Outbox。空 actor 不代表管理员，不允许绕过权限。
- `ReviseGlobalServer`：同一组织锁内验证成员、幂等键及 expectedGeneration，追加修订，以 CAS 推进当前指针，同时写 Operation／Outbox。失败回滚修订和指针；不覆盖用户停止意图或区域归属。
- 重放请求返回原操作的修订与当前实例／Placement；重放旧创建不会把当前修订指针回滚。旧操作的成功返回也不表示当前游戏进程已经运行。

两条接口均为内部持久化 Adapter。Provider、Region 可售性、受保护配置及资产授权须由后续应用用例验证；配额及成员权限仍在事务中重查。Node 准入与物理预留属于区域，不在这里执行。

## 迁移与兼容范围

PostgreSQL 显式迁移 015 创建上述表及不可变触发器；SQLite 使用一次性版本 3，在一个事务内建表和登记版本。此迁移不自动复制旧 `game_servers`，也不创建区域 Deployment。旧数据继续由旧路径管理，不能同时把同一个实例导入两套权威写入路径。

过渡期间，旧实例和新逻辑实例共用组织锁与配额统计；新逻辑实例暂时只计逻辑预留，不把 pending 意图计作实际运行。未知／缺失修订资源拒绝准入。删除／停服后释放语义仍待区域生命周期落实。

升级应停止旧写入进程，并以匹配 schema 的版本启动。旧进程不知道新逻辑表的配额，不支持与新写入流程混跑。新建表不等于完成了区域拆库；导入、对账、物理拆分和恢复门槛仍按 [旧数据迁移说明](regional-data-migration.md) 执行。

## 验证与后续要求

SQLite／PostgreSQL 测试覆盖并发重复创建、同键改参数、创建末步失败回滚、数据库级修订不可变、修订末步失败回滚、同版本竞争、原版本保留、跨租户拒绝、新旧配额共同限制，以及 schema 重放。SQLite 另覆盖建表失败回滚；当前证据不代表跨主机部署或真实开服。

下一步接入应用用例和受保护配置、全局 API 操作查询、Region 注册及严格节点授权，再实现区域 Deployment／Inbox／持久任务和实际 MQ 交付。现有旧 API、Controller 和 Worker 尚未消费这些新记录。
