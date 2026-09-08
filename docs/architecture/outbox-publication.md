# 全局 Outbox 分发

已增加可恢复的发布状态、分发用例、RabbitMQ 发布适配器及独立发布进程。Region Inbox 和区域持久任务尚未接入；发布确认不改变业务 Operation 的成功条件。

`delivery` 定义自身消费的 `Outbox` 和 `Publisher` 接口，仅依赖标准库；Store 实现持久化接口，实际 Broker Adapter 由后续组合根注入。`Publisher.Publish` 只有在获得持久发布确认并确认可路由后才能返回 nil，超时属于不确定结果。

PostgreSQL 016／SQLite 版本 4 扩展原有 `server_outbox`，保留事件 ID、Payload 和创建事务。增加领取令牌、租约到期时间、下一次尝试时间、尝试次数及发布确认时间。每次领取按明确 Region 查询，最多 100 条；待发布索引与筛选一致，无 JOIN。时间来自数据库而非分发主机。

PostgreSQL 使用行锁和 SKIP LOCKED 支持同区域多个分发器；SQLite 并发写仍受单写者限制，事务冲突返回错误，由调用方稍后重试，不能据此宣称 SQLite 适合大规模托管部署。

分发流程：

1. 短事务领取一批到期事件，写随机令牌、租约及尝试次数。
2. 事务提交后逐条调用 Broker，每条调用有超时；批次和超时须小于租约预算。数据库耗时也可能导致租约过期，过期后允许重复投递，不能强行确认旧领取。
3. Broker 确认后，按事件 ID 和当前未过期令牌更新发布状态；此更新不把业务操作置为成功。
4. 发布失败或确认丢失时，在当前租约下记录下一次尝试时间。若进程退出、数据库回写失败或上下文取消，租约到期后由其他分发器重新领取同一事件 ID。

完成／重试操作先锁定当前记录，再取数据库时间，防止使用等待锁之前的时间确认过期租约。旧令牌不能覆盖新的领取；发布成功但数据库确认失败仍可能重发，接收端必须按稳定 ID 幂等。

当前 `RunOnce` 只处理有界批次；独立进程按配置间隔轮询，RabbitMQ 适配器重建失败连接。部署级进程监督、退避抖动、重试上限与死信审计、积压告警、Inbox／任务原子提交和对账尚待接入。固定重试间隔由配置传入，不将这一基础实现当作全部可靠交付验收。

测试使用替身 Publisher 复现确认丢失，SQLite／PostgreSQL 验证持久状态与租约；PostgreSQL 另验证四方并发领取。替身测试不能证明 Broker 持久化或跨 Region 交付能力。

## RabbitMQ 发布进程

适配器使用官方 `amqp091-go v1.10.0`，依赖随仓库 vendor 保存。每个实例只接受配置 Region 的事件，使用专用连接和持久 quorum 队列，持久消息经默认交换器定向发布；mandatory 返回优先于确认。健康连接复用，失败、取消或确认不确定时关闭底层连接，下次重建。连接、握手、声明、写入和等待确认由整体超时约束；取消回调退出后才允许下一次调用复用状态。

同一适配器串行发布；并行吞吐由多个发布进程／适配器及 PostgreSQL 领取实现。此实现尚无吞吐容量证据。队列声明需要目标队列的配置权限，已有不匹配队列会失败；生产 Broker 权限、队列策略、保留窗口和副本部署须由区域运维配置。

先用现有迁移命令将全局 PostgreSQL 升级至匹配 schema。通过外部环境提供 `GAMEPANEL_DATABASE_URL` 与该区域的 `GAMEPANEL_RABBITMQ_URL`，凭证不写入仓库或命令行参数，然后运行：

```sh
go run ./apps/api/cmd/outbox-publisher -region region-a -queue gamepanel.region-a.intents
```

Region 与队列名称须替换成已登记部署配置。可配置批次、发布超时、领取租约、重试间隔、轮询间隔、载荷上限及数据库连接数；SIGTERM／中断停止轮询并关闭连接。该进程仅读取全局 Outbox 并更新发布状态，不运行游戏或处理 Region 接纳。

真实 Broker 测试通过 `GAMEPANEL_TEST_RABBITMQ_URL` 启用，使用随机临时队列并清理，验证持久消息、稳定 ID、区域拒绝、连接复用、无法路由和断连恢复；另以本机无响应端点验证握手超时。单节点 Broker 的这些测试不证明多副本故障恢复、进程重启后端到端交付或跨区域高可用。

## 实际 Broker Adapter 的实现依据

2026-09-08 核对 RabbitMQ 官方文档：未路由的消息也可能得到 Publisher Confirm；因此 Adapter 必须同时处理 mandatory 返回与确认，不能仅凭确认成功清除 Outbox。持久消息需配合持久队列，消费者确认与发布确认属于不同阶段。[RabbitMQ 确认语义](https://www.rabbitmq.com/docs/confirms)

RabbitMQ 官方 Go 客户端提供 DeferredConfirmation 和 WaitContext，但 PublishWithContext 在底层 I/O 开始后不能靠 context 中断写入。实现 Adapter 时必须让连接 I/O 本身有界，并在超时后的不确定状态重建发布通道，不能仅给调用套一个 context 就宣称满足分发器超时契约。[amqp091-go API](https://pkg.go.dev/github.com/rabbitmq/amqp091-go)
