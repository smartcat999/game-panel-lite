# Distributed worker reconciliation

GamePanel Lite treats a remote game server as durable desired state, not as a sequence of imperative Docker commands.

## Ownership

- HTTP handlers mutate `GameServer.Spec` only.
- The control-plane controller materializes one `WorkloadAssignment` for the selected remote node.
- The worker agent continuously compares that assignment with real Docker state and applies the smallest required change.
- The worker reports a `WorkloadObservation` after inspecting Docker again.
- The control plane derives `GameServer.Status` only from an observation matching the assignment UID, node, and generation.

Lifecycle ACKs must never set a server to running or stopped. Console input and other one-shot operations may use a separate operation queue, but they do not own lifecycle state.

## Generation and fencing

Every assignment carries the server spec generation and a placement UID. Containers are labeled with both values. A new placement receives a new UID, so a delayed observation from an older placement cannot update the current server.

An observation with an older generation leaves the server reconciling. The server is running only when the real container is running and the observed generation has caught up with the assignment generation.

## Deletion

Deletion remains asynchronous. The assignment carries desired state `deleted` until the assigned worker reports the runtime missing. Only then may the controller remove the assignment and the server resource.

## Current compatibility slice

The initial implementation keeps the existing node operation queue for console input. Legacy lifecycle tasks are rejected by the agent because lifecycle now converges through assignments. Log snapshots retry on content changes and advance their local digest only after a successful upload; an ordered, durable log cursor remains future work.

Automatic failover is intentionally excluded. An unreachable node produces unknown state; the control plane must not start the same server on another node without a fencing lease or an explicit migration procedure.

## 发布与观察结果的提交约束

Controller 只能通过 PublishWorkloadAssignment 发布任务；原始 upsert 已收为 Store 内部方法。事务对实例原始 spec、节点和空间执行条件行锁，拒绝构建期间发生变化的快照或已删除实例。相同 generation 的任务身份、目标节点、期望状态和 workload 内容必须一致；重复发布沿用原任务及删除时间。同节点升级保留 UID，节点变化必须使用更高 generation 和新的 UID。

节点报告提交时锁定当前 assignment 行，再复核 UID、节点、实例和报告版本。任务已经撤销/改派或报告版本越界返回冲突。多个报告在此锁下比较已保存 generation，低版本不能覆盖高版本，避免先查询再保存造成版本倒退或重复插入。相同 generation 的报告通过观察令牌进行条件提交；ObservedAt 不参与新旧排序。

SQLite 与真实 PostgreSQL 测试覆盖八个发布者竞争一个身份、相同版本内容不一致、旧实例快照、八个版本的并发报告、节点切换后的旧 UID、任务与实例删除后拒绝重新发布/报告。此约束仅保证数据库发布与报告提交，不会停止失联节点上的旧容器；执行租约、fencing、报告顺序号、并发任务删除和节点接管协议仍待实现，不能据此宣称安全自动故障转移。

## 观察令牌与升级

任务拉取响应包含 `observationToken`，等于当前 observation 行的 opaque ID；尚无观察结果时为空字符串。Worker 将原令牌带入报告，包括运行时失败的报告。事务锁定 assignment 后比较令牌，只有匹配当前观察结果的报告才能写入；每次写入为 observation 生成新的 UUID。重复或延迟的同版本报告返回 409；Agent 不盲目重发旧报告，下一轮拉取新令牌并重新观察运行状态。低于已保存 generation 的报告仍可被确认接收，但不修改数据。

令牌利用现有 observation 主键，不增加列或迁移；外部引用应使用 assignment UID，observation ID 不再是稳定记录标识。API 冷启动或 Agent 重启后都从数据库取当前令牌，无需依赖节点时间或本地计数器。读取令牌与任务不是同一事务快照，但提交会再次校验任务归属、版本和令牌，不匹配时拒绝。

部署顺序为先更新所有 Agent，再更新 API。新 Agent 可向旧 API 发送额外令牌字段（旧 API 忽略）；新 API 上线后，新 Agent 会读取并回传令牌。旧 Agent 缺少令牌，在已有观察结果的任务上无法持续上报。此协议仅防止旧观察结果覆盖，不能阻止旧 Agent 的运行时副作用，也不能取代执行租约或节点 fencing。

## Worker 的不可变容器目标

Worker 的 Start/Stop/Remove 接口接收 Inspect 返回的 State，Docker Adapter 只使用其中的完整 64 位十六进制容器 ID。拒绝容器名称、短 ID 和不完整的归属信息。名称仍用于发现实例；检查后执行操作不再按名称重新定位，旧容器消失时不会把操作转向后来复用名称的容器。

所有期望状态均拒绝其他 assignment UID、其他节点/实例以及更高 generation 的容器。同 UID 的旧 generation 仍可被当前任务替换。创建后的检查及最终观察也验证身份，避免把竞争者创建的容器启动或报告为自己的实例。遇到残留的其他 UID 容器将返回错误，需显式完成旧任务清理和接管协议，不能自动删除它。

验证包括替换发生在 Inspect 与 mutation 之间、创建后发现竞争容器、完整 ID 的 Docker HTTP 请求，以及真实一次性 Alpine 容器的启动/控制台/停止/删除。此保护不能阻止旧进程操作仍存在的旧容器，也不保护创建前的共享配置文件写入。执行租约、节点失联停机和文件写入 fencing 仍未完成。

## 同节点创建互斥

Docker Create 在 `.creation-locks/<server-id>.lock` 上持有操作系统排他文件锁，取得锁后再次 Inspect；只要同名容器存在，就拒绝 Create，不改配置。镜像拉取完成并确认上下文仍有效后才准备文件。锁覆盖检查、拉取、文件准备和 ContainerCreate，多个 Agent 进程共享同一数据目录时不能交错执行同一实例的创建。不同实例使用不同锁；等待支持上下文取消。

锁文件不在释放时删除，避免等待者继续锁旧 inode、后来者锁新 inode 而失去互斥。锁文件因此会留存，清理需在 Agent 停止且确认无使用者后进行；尚未实现在线锁文件回收。Linux/macOS/BSD 使用 flock，Windows 使用 LockFileEx，其他平台拒绝此创建路径。已验证 macOS 独立子进程互斥、双 Adapter 竞争与真实 Docker 生命周期；Linux Agent 交叉编译通过。Windows 整体 Agent 仍因既有 syscall.Statfs 调用不可编译，本批只检查 Windows Runtime 包的编译；不能宣称 Windows Agent 已可发布，也不能用交叉编译替代对应平台运行验证。

部署要求相关 Agent 使用同一数据目录及可靠文件锁语义。不共享目录、绕过该接口的程序、外部 Docker 管理员以及不保证锁语义的网络文件系统不在保护范围内。旧版本 Agent 不会持锁，须先停止旧进程再启动新版本；此改造不等于跨节点执行租约。配置准备自身中途失败的回滚、其他文件写入路径及跨节点 fencing 仍待完成。
