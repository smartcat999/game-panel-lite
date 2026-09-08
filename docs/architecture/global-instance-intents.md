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
| `server_outbox` | 操作的持久交付记录及目标区域 | 与实例／修订事务提交；每操作唯一记录；已补充分发租约与发布状态，实际 MQ 尚未接入 |

配置修订不包含区域任务 generation、执行租约、运行路径、Env／Cmd 或实际容器状态。规格中的资产引用必须给出版本；这只校验引用形状，资产存在、内容摘要和租户授权仍需应用用例及资产模块验证。

`ProtectedConfiguration` 接收保护器生成的 keyId 和不透明密文。当前结构检查不能证明输入已正确加密；`configprotection` 已实现下述认证加密 Adapter，但尚未接入事务写入／区域执行用例。实际 Provider 配置校验、密钥管理和区域解密授权仍待实现。不要把此内部接口直接暴露成允许客户端自称“已加密”的公共 API。

## 配置保护 Adapter 与接入约束

`configprotection.New` 接收外部 AES-256 密钥集合、活动 keyId 和明文字节上限，初始化后不持有调用方可变密钥切片。Seal 使用活动密钥，Open 可使用保留的旧密钥，便于新修订换钥而旧修订继续可读。不存在默认密钥、落盘配置或网络密钥分发。

使用 Go 标准库 `cipher.NewGCMWithRandomNonce`，密文首字节为格式版本 1，其后为标准库产生的随机 nonce、密文及认证标签。认证附加数据绑定用途、格式版本、keyId、租户、逻辑实例、修订 ID、specGeneration、Provider 和配置 schema；修改任一身份或密文都会拒绝，失败不返回部分明文。Region 不参与不可变修订身份绑定，因此迁移不会要求修改历史修订，但能否在目标 Region 解密仍需独立授权。

Go 要求每个密钥最多加密 `2^32` 条消息以控制随机 nonce 碰撞风险。跨进程密钥使用计数、轮换、撤销、备份与托管密钥服务尚未实现；当前 Adapter 不声明生产密钥生命周期已经安全验收。[标准库契约](https://pkg.go.dev/crypto/cipher#NewGCMWithRandomNonce)

当前 Store 的幂等摘要包含密文，而实例／修订 ID 在事务内生成，不能在原接口外简单随机加密后重试。后续应用写入用例必须先验证 Provider 配置与资产／区域授权，对规范化业务输入建立稳定且受保护的幂等摘要，在租户事务内核对原操作；仅新操作分配实例／修订 ID 并加密，重放返回已保存结果。不得用固定 nonce 或改写历史密文维持幂等。摘要密钥轮换和旧操作兼容也必须有明确持久化版本方案。

目前测试证明加密往返、逐字节篡改拒绝、身份／keyId 替换拒绝、随机输出、密钥集合复制、旧密文换钥读取与并发安全；不能以此证明 Provider 输入已经合规、配置写入已受保护或 Region 已获执行权限。

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

后续已增加 [Outbox 分发基础](outbox-publication.md)。仍需接入应用用例和受保护配置、全局 API 操作查询、Region 注册及严格节点授权、区域 Deployment／Inbox／持久任务和实际 MQ 交付。现有旧 API、Controller 和 Worker 尚未消费这些新记录。
