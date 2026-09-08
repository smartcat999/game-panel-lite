# 区域通知接收与持久任务

已实现独立 PostgreSQL 区域 schema、固定区域身份、Inbox、获取修订任务及 RabbitMQ 手动确认接收进程。授权修订服务、Deployment 和 Node 调度尚未接入。

## 状态与权限

全局 `RevisionAvailable` 事件只有身份和版本号。`RecordRevisionNotification` 校验 schema 1、非空身份、正版本及目标 Region，然后在一个事务内保存通知和 `awaiting_revision` 任务。此状态表示需要取得经授权的不可变修订；不代表权益已经验证、区域已经接受部署或允许运行。

该入口只供可信区域消息入口调用，不能直接暴露给租户。接收器使用区域专用 Broker 凭证，检查消息头 ID 与事件 ID 一致性、内容类型、大小、schema、Region、重复 JSON 字段及尾随内容；持久化事务成功后才确认通知消费。Broker ACL 必须限制租户写入控制队列，消息字段不能替代服务身份。后续任务处理器需获取受保护修订，验证组织归属、配置、placementEpoch、specGeneration、意图版本、权益及授权期限，之后才可以原子创建部署与执行任务。全局断连时无法完成验证的新任务保持等待，不能据此自行授予执行权。

## 原子性和去重

- Inbox 以 eventId 唯一，保存完整类型化事件的规范化摘要。相同 ID 不同内容拒绝。
- 获取修订任务以 operationId 唯一，摘要不包含外层 eventId。修复投递使用新 eventId 时，相同操作内容不会重复建任务；修改修订或其他身份则拒绝并回滚新 Inbox 行。
- Inbox 与任务写入在同一事务中。任务写入失败不能留下“已收件但无任务”的去重记录。
- 多个不同操作即使乱序到达也只是持久通知；部署版本取舍由后续授权／版本检查负责。当前代码不把通知顺序当作配置权威顺序。
- 不自动清理 Inbox，也不把重复消息重新置为待执行；保留窗口、墓碑及显式重放恢复策略仍需完成。

所有 SQL 都是单表按 ID 操作，不使用 JOIN。当前实现逐条事务接纳，并无高吞吐压测证据。

## 区域数据库入口

`RegionalStore` 与全局 `Store` 为不同类型，区域 schema 使用独立迁移目录项和校验和身份，不创建用户、订单或全局逻辑实例表。普通打开只读校验版本和固定 Region；迁移以事务绑定身份，不能改绑已登记的区域。全局 Store／迁移器不能自动接管这个 schema。

运维通过外部环境提供该 Region 的 `GAMEPANEL_REGIONAL_DATABASE_URL`，然后运行：

```sh
go run ./apps/api/cmd/region-migrate -region region-a
```

每个生产 Region 使用自己的数据库和运行凭证，不能指向全局业务库。运行收件账号仅需 schema USAGE，迁移 ledger／区域身份 SELECT，Inbox／修订任务 SELECT 和 INSERT；不能修改区域身份或 schema。迁移账号与运行账号分离，任务处理器所需权限后续单独定义。

## 证据范围

真实 PostgreSQL 测试创建两个隔离 schema，验证区域身份拒绝、全局／区域 schema 拒绝混用、8 方重复去重、事件／操作内容冲突、任务失败整体回滚、未知 schema 拒绝，以及最小权限收件角色可写通知但不能改区域身份。

这不是两个独立 PostgreSQL 服务，也不等于两个完整 Region 部署。授权获取、Deployment、任务租约／重试、结果 Outbox、跨主机断连恢复和完整业务交付均仍需实现与验证。

## 手动确认接收进程

配置 `GAMEPANEL_REGIONAL_DATABASE_URL` 和 `GAMEPANEL_RABBITMQ_URL` 后运行：

```sh
go run ./apps/api/cmd/region-receiver -region region-a -queue gamepanel.region-a.intents -dead-letter-queue gamepanel.region-a.parked
```

接收进程使用预取 1，按事务超时调用 Ingress。临时错误在当前消息上按配置间隔重试，不先 ACK，也不立即循环 requeue。确定的格式错误和身份内容冲突通过 Reject(false) 交给死信机制；其余消息仅在区域事务成功后 ACK。连接关闭或取消造成 ACK 不确定时可能重投，由 Inbox／操作去重吸收。返回计数只表示 ACK 已写入客户端连接，不代表 Broker 对 ACK 的再次确认。

发布端和接收端声明相同持久 quorum 源队列、显式持久隔离队列、可配置 delivery-limit、reject-publish overflow，以及 at-least-once 死信策略。隔离队列不自动消费、不设置 TTL，并禁用二次重投上限；审计重放和保留／告警策略仍待实现。

RabbitMQ 4 默认重投上限可能在未配置死信时丢弃消息；至少一次死信需 quorum 源队列、reject-publish 和死信目标配合，不能只配置一个普通 DLX。[重投上限说明](https://www.rabbitmq.com/blog/2024/08/28/quorum-queues-in-4.0)、[至少一次死信说明](https://www.rabbitmq.com/blog/2022/03/29/at-least-once-dead-lettering)

旧队列参数不兼容时拒绝声明，保留旧消息，不删除或自动重建。升级需显式规划队列迁移，保留原事件 ID，并确认目标持久化后才能确认源消息；当前没有自动队列迁移器。队列配置权限和 Broker 端消息大小限制须由部署配置提供，应用载荷检查发生在收到消息后。

真实 Broker 测试覆盖 ACK 前取消重投、临时失败后成功确认、坏消息隔离、拒绝／重投上限死信及旧队列内容保留；同时启用 PostgreSQL 和 RabbitMQ 的测试覆盖真实重复通知原子落库并发送两个 ACK，只生成一个等待修订任务。目标隔离队列失联、Broker 多副本故障及进程级端到端重启尚未验证。

## 全局修订读取边界

`Store.GetRegionalRevision` 提供内部只读 Adapter。调用方传入的 authenticatedRegion 必须来自经过验证的服务身份，不能照抄租户或消息字段；`global-control` 入口通过 mTLS 证书 URI SAN 与运维提供的 Region 映射建立该身份。

读取在同一快照中逐表按 ID 校验原始 Outbox 内容、实例租户、当前 Region／placementEpoch 和不可变修订。不存在的事件、篡改的身份、旧部署归属和已删除实例统一拒绝，失败不返回部分修订。读取不要求 Outbox 的发布标志已更新，避免发布确认丢失期间阻止合法重复通知处理。

返回值包含通知对应的历史修订，以及读取时的最新 specGeneration、desiredState 和 intentVersion。延迟通知不能把旧配置伪装成当前版本，也不能把最新停止意图改回运行。该快照可能在返回后过时；它不是执行授权，不代替权益、配置密文验证、授权有效期或源部署隔离。区域远程客户端、materializer 及有限期授权仍待实现。

## 控制面 mTLS 修订接口

`global-control` 是独立内部入口，通过已迁移的 `GAMEPANEL_DATABASE_URL` 连接全局库。运维提供服务端证书／私钥、区域客户端 CA 和 JSON 格式的 URI SAN → Region 映射文件；例如映射条目 `"spiffe://gamepanel.example/region/region-a": "region-a"`。该 URI 仅作为精确身份标识，当前没有 SPIFFE 自动签发或工作负载证明实现。

```sh
go run ./apps/api/cmd/global-control \
  -certificate /run/secrets/control.crt -key /run/secrets/control.key \
  -client-ca /run/secrets/region-ca.crt \
  -region-identities /run/config/region-identities.json
```

默认监听 `127.0.0.1:8443`，通过 `-listen` 显式配置部署地址。TLS 最低 1.3，强制验证客户端证书链；叶证书必须恰有一个 URI SAN 且已登记。请求时再次检查已验证链的有效期，避免复用连接绕过证书到期。身份不接受请求头或自报 Region。信任根与映射启动时加载，撤销／轮换需要更新配置并重启实例关闭旧连接；证书签发、短期轮换及自动撤销仍需部署系统提供。

`POST /internal/region/revisions/resolve` 接收 RevisionAvailable JSON，按证书归属调用只读查询。请求有大小和时间限制，拒绝未知／重复字段、尾随内容及跨区域请求；响应禁止缓存，数据库内部错误不返回给调用方。HTTP 成功只表示修订读取成功。配置保护器、Region 客户端、执行授权和任务推进尚未接入。

测试使用内存临时 CA 和真实 TLS 连接，覆盖有效身份、过期／缺失证书、未登记身份、跨区域及请求头伪造，并检查载荷限制、重复字段和错误响应。此证据不代表生产证书生命周期或完整跨 Region 交付已经验收。
