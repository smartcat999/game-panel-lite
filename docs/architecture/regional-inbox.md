# 区域通知接收与持久任务

本批实现独立 PostgreSQL 区域 schema、固定区域身份、Inbox 和获取修订任务。它是接收端的持久化基础，尚未接入 RabbitMQ 消费循环、授权修订服务、Deployment 或 Node 调度。

## 状态与权限

全局 `RevisionAvailable` 事件只有身份和版本号。`RecordRevisionNotification` 校验 schema 1、非空身份、正版本及目标 Region，然后在一个事务内保存通知和 `awaiting_revision` 任务。此状态表示需要取得经授权的不可变修订；不代表权益已经验证、区域已经接受部署或允许运行。

该入口只供可信区域消息入口调用，不能直接暴露给租户。后续消费者须检查 Broker 身份／权限、消息头 ID 与事件 ID 一致性、JSON 与大小限制；持久化事务成功后才确认通知消费。后续任务处理器需获取受保护修订，验证组织归属、配置、placementEpoch、specGeneration、意图版本、权益及授权期限，之后才可以原子创建部署与执行任务。全局断连时无法完成验证的新任务保持等待，不能据此自行授予执行权。

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

这不是两个独立 PostgreSQL 服务，也不等于两个完整 Region 部署。Broker 消费确认、授权获取、Deployment、任务租约／重试／死信、结果 Outbox、断连恢复和端到端交付均仍需实现与验证。
