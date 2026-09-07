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

节点报告提交时锁定当前 assignment 行，再复核 UID、节点、实例和报告版本。任务已经撤销/改派或报告版本越界返回冲突。多个报告在此锁下比较已保存 generation，低版本不能覆盖高版本，避免先查询再保存造成版本倒退或重复插入。相同 generation 的报告仍按提交顺序保存；ObservedAt 不是可信的分布式顺序号。

SQLite 与真实 PostgreSQL 测试覆盖八个发布者竞争一个身份、相同版本内容不一致、旧实例快照、八个版本的并发报告、节点切换后的旧 UID、任务与实例删除后拒绝重新发布/报告。此约束仅保证数据库发布与报告提交，不会停止失联节点上的旧容器；执行租约、fencing、报告顺序号、并发任务删除和节点接管协议仍待实现，不能据此宣称安全自动故障转移。
