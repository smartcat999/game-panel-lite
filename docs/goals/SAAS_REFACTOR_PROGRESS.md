# SaaS 与后端改造验收清单

目标：完成本任务方案中的所有改造。此清单记录当前证据，不以已通过的局部测试替代整体完成。总体状态：进行中。

依据：`docs/architecture/backend-modularity-plan.md` 与 `docs/architecture/toc-saas-platform-plan.md`。原有自托管 V1 完成记录不代表 SaaS 已完成。

| 要求 | 当前状态 | 证据或后续验证 |
| --- | --- | --- |
| M0 依赖检查与 CI | 部分完成 | AST 规则覆盖新增根级共享模块；gofmt 门禁和独立 Agent 构建已加入；完整模块方向与远端 CI 仍待验收 |
| M1 插件注册、目录、通用文件集合 | 完成当前批次 | edfb004a；注册/目录/文件传递测试 |
| 配置预览、预设、恢复解析归属 Provider | 本地验证通过 | gameconfig 用例、Provider 能力、兼容/恢复失败/路径逃逸测试 |
| 世界文件定位归属 Provider | 部分完成 | WorldFilesProvider；世界上传格式与其他存档操作仍需迁移 |
| 模组清单与文件布局归属 Provider | 本地验证通过 | ModManifestProvider、ModFilesProvider、共享 modruntime 用例 |
| 模组来源能力与注册校验 | 部分完成 | ModSupportProvider；支持上传与 Workshop 的判断已改为能力声明；同名依赖查找与分配已限制在同一 Provider；未知 Provider 不回退 tModLoader；名称/依赖元数据已集中到 modcatalog，依赖图遍历已集中到 modruntime；运行文件安装/删除已集中到 modruntime 并使用 os.Root；上传扩展名/辅助缓存文件由 Provider 声明并注入缓存服务；tMod 元数据解析已迁回 Terraria Provider，经 ModInspector 调用；严格包验证、元数据写入与完整安装事务仍待实现 |
| HTTP 与生命周期仅依赖应用用例 | 未完成 | 具体 Provider 导入已消除，但多个 Handler 仍持有 Store/Runtime 并含编排逻辑 |
| 插件版本、配置版本与完整能力校验 | 部分完成 | Provider 声明插件/配置版本；Registry 校验声明，创建保存配置版本，执行前拒绝不兼容版本；通用编辑/预览和目标实例恢复前已检查版本；新备份持久化来源版本并在恢复前校验；ZIP 已内嵌来源并在解压前校验；其他配置变更路径、自动迁移及能力矩阵仍待实现 |
| M3 Agent RuntimeAdapter 与共享执行协议 | 本地验证通过 | internal/workload、worker、runtime/docker；导入例外归零；真实一次性 Docker 容器的资源/端口/控制台/生命周期验证通过；分布式租约与隔离不在本批验收内 |
| 身份/租户/持久化模块所有权 | 未完成 | 待配合 PostgreSQL 与租户迁移实现；不是简单拆文件 |
| PostgreSQL 与显式数据库迁移 | 部分完成 | 可配置 PostgreSQL 驱动与连接池；真实 PostgreSQL 独立 schema 的建表/JSON/事务/查询已验证；仍需显式迁移、数据搬迁及回滚演练 |
| 全链路租户授权、RLS、配额并发 | 未完成 | 需双租户接口/文件/SSE/后台任务测试及竞争测试 |
| API/Controller/网关分离、可靠任务和调度 | 未完成 | 需多进程租约、幂等、资源预留与失联隔离演练 |
| 支付、订单、权益、续费与补偿 | 未完成 | 首发渠道依赖业务选择，需沙箱集成和对账验收 |
| 存档一致性备份、恢复及灾备 | 未完成 SaaS 验收 | 单机恢复已完整暂存并在返回错误时回滚文件；配置解析/保存返回失败也会回滚文件；崩溃恢复、数据库提交不确定性、区域恢复、RPO/RTO 与双写隔离仍待验证 |
| 强隔离、监控、区域容量扩展 | 未完成 | 需部署环境、隔离实现和真实运行负载 |
| P0/P4 成本、规模与业务闭环 | 未完成 | 容量数字为目标，未进行真实游戏负载/多区域/支付上线验证 |

执行顺序保持原方案：先完成模块与能力接口迁移，补充分布式身份/存储/执行契约，再验证 SaaS 的商业与规模要求。每个独立批次通过适用测试后提交。只有全部要求取得与其范围匹配的证据，才能关闭任务目标。
