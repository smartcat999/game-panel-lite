## Problem and result / 问题与结果

<!-- Describe the concrete trigger and resulting behavior. / 描述触发问题和最终行为。 -->

Closes #<!-- one implementation issue number / 一个实现需求编号 -->
Parent module / 模块父需求: #

## Main baseline and scope / 主分支基线与范围

- Branch created from fetched main SHA / 创建分支所用最新 main SHA:
- Latest main SHA checked before merge / 合并前核对的最新 main SHA:
- Alignment method / 对齐方式: merge / rebase / already current
- Owning module and allowed write scopes / 所属模块与允许修改范围:

<!-- Use an independent branch/worktree for this issue. Never force-push shared history. / 使用独立分支或工作树，不强推共享历史。 -->

## Validation / 验证

<!-- List only checks relevant to the change. Separate actual results, skips, and missing environments. Include CI or evidence links. / 仅列适用检查，区分通过、跳过与缺少环境，附 CI 或证据链接。 -->

| Check / 检查 | Result / 结果 | Evidence or limitation / 证据或限制 |
| --- | --- | --- |
| | | |

## Compatibility and recovery / 兼容与恢复

<!-- API group/version and noun resource paths, old-client compatibility, schema rollout, and rollback where relevant. Write N/A when not applicable. / 按需描述 API 分组版本、名词路径、旧客户端兼容、数据库迁移与回滚，无关项填不适用。 -->

## Review / 评审

- [ ] This PR implements one issue within its declared write scopes. / 本 PR 只实现一个需求，遵守允许修改范围。
- [ ] The branch includes the latest main and affected checks pass after alignment. / 分支包含最新 main，对齐后相关检查通过。
- [ ] Module/data ownership and typed route-filter authorization remain intact. / 保持模块与数据权威及过滤器授权。
- [ ] New API paths contain resource nouns; action types belong in request data. / 新 API 路径使用资源名，动作类型放请求数据。
- [ ] No secrets, fabricated telemetry, or unverified production claims are introduced. / 未引入秘密、伪造指标或未验证的生产声明。
- [ ] Skips, remaining limitations, and applicable migration/rollback evidence are explicit. / 已明确跳过项、限制和适用迁移回滚证据。

<!-- Merge only after review and successful checks. Deployment requires its own authorized environment scope. / 评审与检查通过后合并，部署另按授权环境范围执行。 -->
