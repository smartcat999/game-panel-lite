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
| 模组来源能力与注册校验 | 部分完成 | ModSupportProvider；支持上传与 Workshop 的判断已改为能力声明；同名依赖查找与分配已限制在同一 Provider；未知 Provider 不回退 tModLoader；名称/依赖元数据已集中到 modcatalog，依赖图遍历已集中到 modruntime；上传解析、元数据写入与安装事务仍待集中 |
| HTTP 与生命周期仅依赖应用用例 | 未完成 | 具体 Provider 导入已消除，但多个 Handler 仍持有 Store/Runtime 并含编排逻辑 |
| 插件版本、配置版本与完整能力校验 | 未完成 | 已校验部分能力；版本演进协议和其他能力矩阵待实现 |
| M3 Agent RuntimeAdapter 与共享执行协议 | 本地验证通过 | internal/workload、worker、runtime/docker；导入例外归零；真实一次性 Docker 容器的资源/端口/控制台/生命周期验证通过；分布式租约与隔离不在本批验收内 |
| 身份/租户/持久化模块所有权 | 未完成 | 待配合 PostgreSQL 与租户迁移实现；不是简单拆文件 |
| PostgreSQL 与显式数据库迁移 | 未完成 | 需真实数据库迁移与回滚演练 |
| 全链路租户授权、RLS、配额并发 | 未完成 | 需双租户接口/文件/SSE/后台任务测试及竞争测试 |
| API/Controller/网关分离、可靠任务和调度 | 未完成 | 需多进程租约、幂等、资源预留与失联隔离演练 |
| 支付、订单、权益、续费与补偿 | 未完成 | 首发渠道依赖业务选择，需沙箱集成和对账验收 |
| 存档一致性备份、恢复及灾备 | 未完成 SaaS 验收 | 现有单机备份不能证明区域恢复、RPO/RTO 或双写隔离 |
| 强隔离、监控、区域容量扩展 | 未完成 | 需部署环境、隔离实现和真实运行负载 |
| P0/P4 成本、规模与业务闭环 | 未完成 | 容量数字为目标，未进行真实游戏负载/多区域/支付上线验证 |

执行顺序保持原方案：先完成模块与能力接口迁移，补充分布式身份/存储/执行契约，再验证 SaaS 的商业与规模要求。每个独立批次通过适用测试后提交。只有全部要求取得与其范围匹配的证据，才能关闭任务目标。
