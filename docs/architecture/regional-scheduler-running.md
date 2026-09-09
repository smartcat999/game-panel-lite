# 运行区域调度进程

`apps/api/cmd/region-scheduler` 组合现有持久任务、全局 Revision HTTPS 客户端、配置保护、Provider、租户节点策略及区域原子预留。它完成资源预留并在同一事务登记待授权 Node 任务，不签发运行许可或下发可执行命令。单个进程按一次一任务运行，空闲或失败后等待配置的轮询间隔。

## 启动前

1. 使用 `region-migrate` 完成区域迁移，数据库绑定正确 Region；入口不会自动迁移。
2. 使用 `region-node` 配置节点，使用 `region-node-access` 显式配置租户节点范围。没有策略的租户默认拒绝新准入。
3. 节点通过所属区域的 `region-control` 上报当前会话心跳；仅配置节点而没有可用心跳不会被选择。
4. 全局 `global-control` 提供 mTLS Revision 接口，区域客户端证书 URI 必须映射到该 Region。
5. 准备与全局配置保护兼容的私有密钥文件及 Provider catalog。显式指定但不存在的 catalog 会导致启动失败；未指定则使用内置 catalog。

密钥文件为 JSON，密钥值是 32 字节密钥的 Base64 编码，不要把实际密钥提交仓库：

```json
{"active":"configuration-v1","keys":{"configuration-v1":"<base64-encoded-32-byte-key>"}}
```

文件读取限制为 1 MiB。历史配置使用的密钥需要保留；更换文件后重启进程才能重新加载。密钥持有不等于当前运行权限。

## 启动示例

以下环境变量指向部署环境提供的配置与私有文件；端口范围是示例，需要按区域实际端口规划设置。

```sh
go run ./apps/api/cmd/region-scheduler \
  -region "$REGION_ID" \
  -control-endpoint "$GLOBAL_CONTROL_ORIGIN" \
  -certificate "$REGION_CLIENT_CERT" \
  -key "$REGION_CLIENT_KEY" \
  -server-ca "$GLOBAL_SERVER_CA" \
  -configuration-keys "$CONFIGURATION_KEYRING" \
  -architecture amd64 \
  -first-port 32000 -last-port 32063
```

数据库 DSN 从 `GAMEPANEL_REGIONAL_DATABASE_URL` 读取。主端口窗口最多 64 个，Provider 约束与节点架构必须匹配。副本应使用一致的架构、端口窗口、catalog 和密钥配置；不同窗口或架构的任务分区尚未实现，不能将不同配置副本当作自动分片。

默认领取期限 1 分钟，单次工作 10 秒，外围任务 20 秒，失败持久退避 5 秒，空闲轮询 1 秒。可通过 `-lease`、`-request-timeout`、`-task-timeout`、`-retry-delay` 和 `-poll-interval` 修改；期限要求通过入口校验。SIGINT／SIGTERM 取消正在进行的请求并退出；未完成领取通过数据库到期回收。

## 验证范围与限制

测试通过 `GAMEPANEL_TEST_SCHEDULER_BINARY` 指向独立构建的二进制，在真实 PostgreSQL 的独立全局／区域 schema 上启动实际进程，通过真实 mTLS 全局 Handler 验证不可用时退避、恢复后预留及正常退出。使用无外部资产的新实例，节点心跳是夹具；未证明真实 Agent、游戏容器、跨主机或多 Region 完整运行。

当前入口在每次调度尝试中通过 mTLS 读取全局当前运行权益，再读取区域运维节点策略。全局查询在同一只读快照内校验租户、逻辑实例、Placement、当前意图、权益有效期和 CPU／内存额度；没有有效权益时区域不能预留节点、容量或端口。返回的权益记录不缓存，也不是 Node 执行授权。用户级指定节点权限、Node 执行授权及任务下发仍未完成。全局读取不是跨库原子承诺；`scheduling_status=reserved` 不表示容器已创建、游戏已启动或用户操作已成功。

迁移 015 会将原已预留且期望运行的部署重新排入调度恢复队列。恢复期间需要全局接口可用；原资源不会自动释放。调度完成时，`regional_node_tasks` 中应有对应的 `awaiting_authority` 元数据任务，这仍不是开服完成状态。
