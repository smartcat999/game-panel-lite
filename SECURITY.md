# 安全策略 (Security Policy)

## 受支持的版本

| 版本 | 支持状态 |
| :--- | :--- |
| `v2.0.x` (`v2` 分支) | 正常维护（接收安全修复） |
| `v0.x`（旧版 Python 架构） | 已停止维护，建议升级迁移至 v2 |

---

## 自托管生产部署安全建议

1. **内部端口隔离**：从 `v2.0.0` 起，[`docker-compose.yaml`](./docker-compose.yaml) 中 PostgreSQL、Redis、NATS、SeaweedFS、Prometheus、API Server 与 Node Agent 的端口默认绑定在 `127.0.0.1`。如无需外部直连调试，请保持 `GP_BIND_IP=127.0.0.1`，切勿将无密码的 Redis 或数据库直接暴露在 `0.0.0.0`。
2. **修改默认密钥**：使用 `./install.sh` 会自动生成强随机密钥；若使用手动 `cp .env.example .env`，部署前请务必替换 `GP_BOOTSTRAP_PASSWORD`、`POSTGRES_PASSWORD`、`GP_AUTH_JWT_SECRET`、`GP_AGENT_TOKEN` 与 `GP_REGISTRY_WORKER_TOKEN`。
3. **启用 HTTPS**：在公网服务器上使用时，建议在 Nginx / Caddy 反向代理层开启 TLS/HTTPS（详见 [HTTPS 与运维指南](./docs/deployment-and-https.md)），防止管理员登录凭据与终端会话在公网明文传输。

---

## 漏洞反馈

如果你发现了安全漏洞（例如越权访问、容器逃逸风险、签名校验绕过、敏感信息泄露等），**请勿直接公开提交 Issue**，建议通过以下方式私密反馈：

- 使用 GitHub 仓库页面 `Security` -> `Advisories` -> `Report a vulnerability` 功能提交私密报告；
- 我们会在收到报告后尽快确认、评估并发布修复版本。
