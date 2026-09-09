# 区域运维访问边界

平台控制台中的租户资源和区域运维是两个管理边界。租户资源请求读取全局数据库；节点、心跳、容量预留和区域任务明细读取所属区域数据库。全局数据库只保存区域目录和异步状态摘要，不保存节点明细副本。

## 调用链

1. 浏览器使用普通平台会话请求 `GET /api/regions/{regionId}/nodes` 或 `GET /api/regions/{regionId}/deployments`。
2. 全局 Web API 先验证账号为平台管理员，再确认 Region 存在于全局目录。
3. `regionopsclient.Directory` 根据受信配置选择区域运维端点，通过 TLS 1.3 双向认证请求 `GET /internal/operations/nodes`。
4. `region-operations` 从自身绑定的区域数据库读取节点配置或 Deployment，并按页面内 ID 批量读取心跳、有效预留、待授权任务或实际 Allocation。
5. 区域服务在 Go 中组合有界分页结果。SQL 不使用 JOIN；响应不包含租户配置、节点凭证、宿主机路径或日志内容。

节点 Agent 继续连接 `region-control`，使用节点证书身份。平台控制面连接 `region-operations`，使用独立的全局控制面客户端证书。两个入口不共享调用方身份表，防止节点证书取得运维读取权限。

## 区域服务

每个 Region 使用自己的数据库连接和服务端证书启动一个进程：

```bash
GAMEPANEL_REGIONAL_DATABASE_URL='postgres://...' \
go run ./apps/api/cmd/region-operations \
  -region region-east \
  -listen 127.0.0.1:9443 \
  -certificate /run/secrets/region-operations.crt \
  -key /run/secrets/region-operations.key \
  -client-ca /run/secrets/global-control-client-ca.crt \
  -global-identities config/global-control-identities.example.json
```

生产部署应由内部负载均衡或服务网格暴露该端点。浏览器和普通租户不能直接访问它。

## 全局路由

复制 `config/region-operations.example.json` 到部署私有配置目录，替换证书路径和每个 Region 的内部 HTTPS 端点，然后设置：

```bash
GAMEPANEL_REGION_OPERATIONS_CONFIG=/run/gamepanel/region-operations.json
```

配置文件只保存端点和证书文件路径，不保存私钥内容。未配置某个 Region、连接失败、证书不受信或响应身份不匹配时，全局 API 返回不可用；不会改读旧 `compute_nodes`，也不会用异步摘要伪造节点列表。区域状态摘要与节点明细链路相互独立，因此明细连接故障不会抹去最近一次状态投影。

区域部署响应携带全局 `organizationId`、`serverId`、Placement epoch 和配置／意图版本，以及区域自己的调度状态和实际 Node ID。平台界面用 `serverId` 返回全局逻辑实例详情；Region 不读取或修改实例名称、用户配置与订单。

当前端点只提供节点、部署及容量／任务摘要。节点配置变更、日志和告警仍需后续区域运维用例；这些操作必须继续使用区域数据库和独立权限边界。
