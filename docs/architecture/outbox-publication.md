# 全局 Outbox 分发

本批增加可恢复的发布状态及分发用例，尚未接入真实 Broker、独立进程或 Region Inbox。它不改变业务 Operation 的成功条件。

`delivery` 定义自身消费的 `Outbox` 和 `Publisher` 接口，仅依赖标准库；Store 实现持久化接口，实际 Broker Adapter 由后续组合根注入。`Publisher.Publish` 只有在获得持久发布确认并确认可路由后才能返回 nil，超时属于不确定结果。

PostgreSQL 016／SQLite 版本 4 扩展原有 `server_outbox`，保留事件 ID、Payload 和创建事务。增加领取令牌、租约到期时间、下一次尝试时间、尝试次数及发布确认时间。每次领取按明确 Region 查询，最多 100 条；待发布索引与筛选一致，无 JOIN。时间来自数据库而非分发主机。

PostgreSQL 使用行锁和 SKIP LOCKED 支持同区域多个分发器；SQLite 并发写仍受单写者限制，事务冲突返回错误，由调用方稍后重试，不能据此宣称 SQLite 适合大规模托管部署。

分发流程：

1. 短事务领取一批到期事件，写随机令牌、租约及尝试次数。
2. 事务提交后逐条调用 Broker，每条调用有超时；批次和超时须小于租约预算。数据库耗时也可能导致租约过期，过期后允许重复投递，不能强行确认旧领取。
3. Broker 确认后，按事件 ID 和当前未过期令牌更新发布状态；此更新不把业务操作置为成功。
4. 发布失败或确认丢失时，在当前租约下记录下一次尝试时间。若进程退出、数据库回写失败或上下文取消，租约到期后由其他分发器重新领取同一事件 ID。

完成／重试操作先锁定当前记录，再取数据库时间，防止使用等待锁之前的时间确认过期租约。旧令牌不能覆盖新的领取；发布成功但数据库确认失败仍可能重发，接收端必须按稳定 ID 幂等。

当前 `RunOnce` 只处理有界批次；轮询、进程监督、退避抖动、重试上限与死信审计、积压告警、实际 MQ、Inbox／任务原子提交和对账尚待接入。固定重试间隔由配置传入，不将这一基础实现当作全部可靠交付验收。

测试使用替身 Publisher 复现确认丢失，SQLite／PostgreSQL 验证持久状态与租约；PostgreSQL 另验证四方并发领取。替身测试不能证明 Broker 持久化或跨 Region 交付能力。

## 实际 Broker Adapter 的实现依据

2026-09-08 核对 RabbitMQ 官方文档：未路由的消息也可能得到 Publisher Confirm；因此 Adapter 必须同时处理 mandatory 返回与确认，不能仅凭确认成功清除 Outbox。持久消息需配合持久队列，消费者确认与发布确认属于不同阶段。[RabbitMQ 确认语义](https://www.rabbitmq.com/docs/confirms)

RabbitMQ 官方 Go 客户端提供 DeferredConfirmation 和 WaitContext，但 PublishWithContext 在底层 I/O 开始后不能靠 context 中断写入。实现 Adapter 时必须让连接 I/O 本身有界，并在超时后的不确定状态重建发布通道，不能仅给调用套一个 context 就宣称满足分发器超时契约。[amqp091-go API](https://pkg.go.dev/github.com/rabbitmq/amqp091-go)
