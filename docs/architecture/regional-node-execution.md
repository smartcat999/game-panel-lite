# Region 到 Node 的执行授权

这条链路把全局业务意图、Region 资源决策和 Node 容器操作分成三个边界。它只覆盖期望状态为 `running` 的无外部资产实例，当前不是完整生命周期实现。

## 职责

| 模块 | 拥有的数据与决定 | 不负责 |
| --- | --- | --- |
| 全局控制面 | 租户、逻辑实例、不可变配置修订、Placement、意图版本和当前运行权益 | Node 选择、容器操作、Region 原始监控 |
| Region Scheduler | 本 Region 的 Deployment、节点策略、候选计算、容量与端口预留、待授权 Node 任务 | 签发长期运行权限、保存明文配置、直接操作容器 |
| Region Control | 认证 Node，重新读取全局当前修订与权益，短时解密并渲染 Workload，签发和续期有限租约 | 修改全局实例或商业数据、替 Node 执行 Docker 操作 |
| Node Agent | 在有效 Region 租约下调用 RuntimeAdapter，回报实际状态 | 自行选择 Region／Node、解释租户权益、持久保存全局密钥 |

`apps/api/internal/regional` 是执行授权模块。它定义消费侧 Interface，并协调当前全局快照、执行策略、渲染器和 Region 持久化端口。HTTP、PostgreSQL、全局 mTLS Client、配置密钥和具体 Provider 都是 Adapter 或组合根依赖。架构测试禁止 `nodeapi`、`regional` 和共享 worker 反向依赖具体 Provider、Store 或 Docker SDK。

## 授权流程

1. Scheduler 完成当前权益与节点策略准入，在一个 Region 数据库事务中预留计算、端口并登记元数据任务。任务不含可执行配置。
2. Agent 使用所属 Node 的 mTLS 身份和当前 session epoch 领取任务。
3. Region Control 按任务 ID 分别读取任务、Allocation、Deployment 和配置快照，在 Go 中组合，不使用 SQL JOIN。
4. Region Control 通过独立的 Region 客户端证书读取全局当前修订和运行权益。Placement、generation、intent、期望状态、配置及资产任一变化都会拒绝旧任务。
5. 校验通过后，Region Control 只在内存中解密并渲染 Workload，再在 Region 数据库事务中签发有限期租约。配置明文和 Workload Spec 不写入 Region 数据库。
6. Agent 在每次 Runtime 变更前续租。续租会重复第 4 步，并在数据库事务内确认 Node session、运行能力及心跳新鲜度。
7. Agent 回报带 holder 和单调 fence 的观测。Region 以 observation token 做 CAS；同内容重放幂等，冲突重放和旧 fence 被拒绝。
8. 保存观测的同一 Region 事务写入逐实例状态 Outbox。发布进程经 RabbitMQ 确认后，全局接收端按 Placement、generation、intent 和 fence 投影；匹配当前操作的成功运行观测会把 Operation 标为成功。
9. 只有当前租约已保存成功的 `running` 观测后，Agent 才能释放租约并把任务标为成功。

租约到期只撤销后续变更权限，不代表旧容器已经停止。Region 不会因控制面暂时不可用而伪造 stopped 状态或提前释放容量。物理隔离、停服和删除仍需要后续生命周期状态机处理。

## 运行配置

`region-control` 除 Node 服务端证书和身份映射外，还需要一套独立的全局客户端身份：

```sh
GAMEPANEL_REGIONAL_DATABASE_URL="$REGIONAL_DATABASE_URL" \
region-control \
  -region "$REGION_ID" \
  -listen 0.0.0.0:8443 \
  -certificate "$NODE_SERVER_CERT" \
  -key "$NODE_SERVER_KEY" \
  -client-ca "$NODE_CLIENT_CA" \
  -node-identities "$NODE_IDENTITIES" \
  -global-control-endpoint "$GLOBAL_CONTROL_ORIGIN" \
  -global-client-certificate "$REGION_GLOBAL_CLIENT_CERT" \
  -global-client-key "$REGION_GLOBAL_CLIENT_KEY" \
  -global-server-ca "$GLOBAL_SERVER_CA" \
  -configuration-keys "$CONFIGURATION_KEYRING" \
  -provider-catalog "$PROVIDER_CATALOG" \
  -execution-lease 2m \
  -max-heartbeat-age 30s
```

Node Agent 的 Region 模式增加 `AGENT_EXECUTION_POLL_INTERVAL`。Agent 心跳间隔必须明显小于 `region-control -max-heartbeat-age`；Scheduler 和 Region Control 应使用一致的心跳年龄策略。

## 已验证与未完成

真实 PostgreSQL 16 race 测试覆盖并发领取只能一个成功、进程重开后续租、过期租约增加 fence、过期心跳拒绝、观测 CAS、幂等重放和成功完成。真实 TLS 测试覆盖 Node session、心跳、领取、每次变更续租、Runtime 创建／启动、观测与释放。

当前还缺启停、重启、删除、资源释放、逐实例状态的用户 API／前端接线、外部资产准备，以及真实游戏容器和多主机故障验收。这里的测试不能替代这些完成条件。
