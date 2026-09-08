# 待支付订单维护入口

`apps/api/cmd/order-maintainer` 使用全局 PostgreSQL 数据库，关闭超过付款期限且仍为 pending 的订单。它不退款、不撤销订阅、不停止实例、不释放 Region 资源。订单期限以数据库时间判定。

先运行匹配当前版本的数据库迁移，再由部署的秘密配置提供 `GAMEPANEL_DATABASE_URL`。入口只检查已有 schema，不自动迁移。需要读取订单与更新状态、取消原因／时间／操作者的数据库权限。

从仓库根目录构建和运行：

```sh
go build -o order-maintainer ./apps/api/cmd/order-maintainer
./order-maintainer -batch-size 100 -task-timeout 10s -poll-interval 5s
```

默认持续运行，每批处理结束后等待轮询间隔，包括空批、满批和失败。单批上限为 1–200 条，事务超时范围为 1ms–1m，轮询间隔范围为 1ms–1h。合理配置应结合数据库负载；这些边界不是吞吐承诺。

仅执行一次有界批次：

```sh
./order-maintainer -once -batch-size 100
```

`-once` 不会清空整个积压：一批成功即退出，失败返回非零退出码。持续模式在事务失败后等待并重试；进程收到 SIGINT／SIGTERM 时取消当前事务或等待并正常退出。数据库启动失败、无连接配置或非法参数会退出。日志不包含 DSN 或底层数据库错误。

多个 PostgreSQL 工作进程通过行锁和 SKIP LOCKED 分担到期订单。部署重启后重新查询持久 pending 状态即可继续；取消与未来支付确认必须使用相同订单行锁并检查最新状态。支付确认与该维护入口的竞争仍需在支付实现后验收。

验证范围：单元测试验证事务超时、失败等待、单批执行及退出；真实 PostgreSQL 测试调用实际二进制验证单批到期更新和批次边界。尚无生产部署、规模积压或真实支付渠道验收。
