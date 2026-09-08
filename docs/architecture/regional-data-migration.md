# 全局／区域拆库的旧数据迁移

2026-09-08。当前实现到第一步「只读归属预检」。后续配置版本回填、区域部署导入、资产副本校验和生产切换仍待实现。该工具不修改旧库，也不表示独立 Region 已可上线。

## 归属预检

对与当前二进制 schema 一致的 PostgreSQL 执行：

```sh
# GAMEPANEL_DATABASE_URL 由部署环境提供，不写进配置仓库或命令历史。
go run ./apps/api/cmd/regional-migration-audit -timeout 1m > ownership-audit.json
```

部署时建议构建二进制直接运行。退出码：0 为本次归属检查没有问题；2 为报告包含待处理项；1 为连接、schema、超时或输出失败。`go run` 本身可能将子进程退出码转换成 1，应读取其错误提示或直接使用二进制。

审计账号只需 schema USAGE，以及以下 SELECT 权限：

- `gamepanel_schema_migrations`：版本与校验和。
- `organizations(id)`。
- `compute_nodes(id, region)`。
- `game_servers(id, organization_id, node_id)`。
- `workload_assignments(id, server_id, node_id)`。

不需要 DDL、INSERT、UPDATE、DELETE，也不需要读取配置、状态 JSON、节点令牌或密码。PostgreSQL 使用只读 REPEATABLE READ 事务，归属检查来自同一数据库快照；查询按 ID 排序。报告含内部资源 ID，应作为运维资料保存。

报告列出源实例、节点、执行任务数量和可解析的部署归属，同时保留全部发现的问题：

| 问题 | 含义与处置 |
| --- | --- |
| `organization_unresolved` | 逻辑实例没有可验证的组织；显式确认归属，不能自动加入任意用户空间 |
| `node_ambiguous` | 空节点可能是旧本地实例，也可能是等待调度；必须区分后再回填 |
| `node_missing` | 引用的节点不存在；保留运行、存档和端口预留证据，不能直接丢弃 |
| `region_required` / `region_unresolved` | 区域为空或不可解析；不默认到香港或其他区域 |
| `region_not_canonical` | 区域标识有首尾空白；先显式统一节点归属 |
| `server_missing` | 执行任务没有对应逻辑实例；可能仍有实际容器，必须清理或恢复追踪 |
| `placement_diverged` | 任务与实例指向不同节点；可能是待退役源部署，不证明源进程已停止 |

`placements[].nodeId` 只记录旧的实际分配，不据此生成用户严格绑定节点的要求。即使某个实例能解析归属，只要全局报告仍有问题，迁移就不能自动忽略其旧任务。

## 后续迁移顺序与门槛

1. 明确组织和 Region／Node 映射，保留迁移清单与源库备份；预检不能覆盖游戏进程、物理存档和备份的真实性检查。
2. 将逻辑身份、用户配置及意图转换为全局实例和不可变 ServerRevision；旧运行路径、容器 ID、节点任务不能进入全局配置。秘密须转换成受控引用或密文，不能通过消息明文复制。
3. 建立 Placement 和独立 placementEpoch；保留旧 specGeneration 和任务 generation 的映射，不把这三个版本号混用。历史实际节点不能变成用户指定节点策略。
4. 为每个区域导入 Deployment、执行任务、预留、观察结果及资产副本。旧的源部署不能因为当前 NodeID 已变化就跳过；无法证明停止的容器仍需保留追踪与容量。
5. 停止旧写入进程后生成最终一致性清单，验证数量、内容摘要、租户归属、版本和副本完整性；重复导入必须幂等，冲突必须拒绝。预检快照不代替这一步。
6. 通过隔离环境中的恢复与回滚演练，再切换全局／区域读写入口。目标已产生新写入后，不能直接把过期源库或存档重新启用。

当前预检不检查资产内容、配置合法性、组织成员权限、端口冲突、Region 注册授权、全局／区域新表一致性，也不执行导入。这些要求继续保留在六阶段验收矩阵中。
