# 独立控制面进程镜像

`docker/control-plane/Dockerfile` 是所有薄组合入口共用的一份参数化构建规则。构建时必须用 `GAMEPANEL_ENTRY` 选择白名单中的一个入口，每个产物只包含对应的一个静态 Go 二进制；运行时不能通过参数切换成其他职责。

全局入口可分别构建为：

- `global-control`
- `global-receiver`
- `order-maintainer`
- `outbox-publisher`
- `region-status-receiver`

每个 Region 独立启动需要的入口：

- `region-migrate`
- `region-receiver`
- `region-fetcher`
- `region-scheduler`
- `region-control`
- `region-operations`
- `region-status-publisher`
- `region-upload-worker`
- `region-node-access`
- `region-node`
- `region-repair-manifests`

例如构建区域调度器：

```bash
docker buildx build --load \
  --build-arg GAMEPANEL_ENTRY=region-scheduler \
  -f docker/control-plane/Dockerfile \
  -t gamepanel-region-scheduler:local .
```

同一入口镜像可以按 digest 发布到多个 Region。不同职责使用不同镜像标签和摘要，避免把十余个 Go 二进制的重复运行库一起分发。部署配置必须分别挂载 Region 数据库 DSN、服务证书、客户端证书、CA、身份映射、Provider 目录和配置密钥；不能把一个 Region 的数据库凭证或节点信任表复用到另一区域。`region-control` 同时拥有面向 Node 的服务端身份和面向全局控制面的 Region 客户端身份，这两套信任域不能共用证书或 CA；完整参数见 [Region 到 Node 的执行授权](regional-node-execution.md)。迁移入口应由发布任务单独运行，不能由每个长期服务并发执行。镜像默认使用非 root UID/GID `65532`，挂载目录必须显式授予所需读写权限。

该构建规则解决独立入口缺少可部署镜像的问题。完整生产编排、Secret 管理、两个 Region 的跨主机运行和滚动升级恢复仍需单独验收；旧 `compose.yaml` 仍是自托管兼容部署，不代表 SaaS 拓扑。
