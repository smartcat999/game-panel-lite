# 部署进阶、HTTPS 反向代理与升级迁移指南

本文档适用于已完成基础部署，并计划在生产环境中启用 **域名 + HTTPS**、执行 **版本升级** 或进行 **数据备份/跨机迁移** 的自托管用户。

---

## 1. HTTPS 反向代理配置（必需支持 WebSocket）

GamePanel Lite v2 的 `web` 容器（默认监听 `3000` 端口）内置了同源反向代理，会自动将 API、WebSocket 终端控制台/日志流、大文件传输与 S3 备份流量转发到内部后端服务，因此**只需要反向代理 `web` 的 `3000` 端口**，无需将 `8080`、`8082`、`8333` 等内部端口暴露到公网。

> **关键注意事项**：
> 1. 控制台交互终端（`/apis/.../exec`）与实时日志（`/apis/.../log`）依赖 **WebSocket**，务必透传 `Upgrade` 与 `Connection` 请求头。
> 2. 在线上传世界存档或备份包需放宽请求体大小限制（建议 `client_max_body_size 512m;` 或不限制），并调大长连接读写超时，避免大文件传输中途断开。

### Nginx 示例

```nginx
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

server {
    listen 80;
    server_name panel.example.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name panel.example.com;

    ssl_certificate     /etc/letsencrypt/live/panel.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/panel.example.com/privkey.pem;

    # 允许上传较大的地图存档或备份文件
    client_max_body_size 512m;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;

        # WebSocket 支持（控制台终端与实时日志）
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;

        # 透传真实 Host 与协议，确保邀请链接、直连代理地址识别正确
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
        proxy_buffering off;
    }
}
```

### Caddy 示例（自动申请 HTTPS 证书）

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:3000
}
```

> 使用宝塔或 1Panel 配置反向代理时，除了填入目标 URL `http://127.0.0.1:3000`，请在后台确认已开启 **WebSocket 支持** 并将上传限制调整为 `512M` 以上。

---

## 2. 端口绑定与公网地址说明

- **默认安全绑定**：从 `v2.0.0` 起，[docker-compose.yaml](../docker-compose.yaml) 中 PostgreSQL、Redis、NATS、SeaweedFS、Prometheus、API Server 与 Node Agent 端口均默认绑定 `127.0.0.1`，仅 `web (3000)` 默认监听 `0.0.0.0`。若已配置 Nginx/Caddy 反代，可在 `.env` 中进一步设置 `GP_WEB_BIND_IP=127.0.0.1` 并关闭防火墙的 `3000/tcp`。
- **好友联机显示域名地址**：如希望「邀请好友」卡片展示你的域名而不是公网 IP，可在 `.env` 中设置：
  ```dotenv
  GP_AGENT_EXTERNAL_IP=play.example.com
  ```
  修改后执行 `docker compose up -d node-agent` 即可生效。

---

## 3. 版本升级

GamePanel Lite 的游戏存档与数据库全部持久化在宿主机目录和 Docker Named Volume 中，升级镜像不会丢失数据：

```bash
cd /path/to/game-panel-lite

# 1. 更新编排代码
git pull --ff-only origin v2

# 2. 拉取最新预构建镜像
docker compose pull

# 3. 滚动重建容器（运行中的游戏服务器容器不会中断）
docker compose up -d
```

---

## 4. 数据备份与迁移

面板核心数据分布在两部分：

1. **游戏世界与服务端缓存目录**（默认宿主机 `/var/lib/gamepanel/instances`，可在 `.env` 中通过 `GP_AGENT_WORK_DIR` 查看）
2. **Docker 持久化卷**：
   - `postgres_data`：账号、权限、服务器声明配置、定时任务记录
   - `seaweedfs_data`：面板内创建的游戏在线备份压缩包（S3 对象存储）

### 迁移步骤建议

1. 在旧服务器停止面板服务（确保数据静默落盘）：
   ```bash
   docker compose stop apiserver controller-manager node-agent
   ```
2. 打包 `.env`、`/var/lib/gamepanel/instances`，并通过 `pg_dump` 导出数据库：
   ```bash
   docker compose exec postgres pg_dump -U gamepanel gamepanel > gamepanel-db.sql
   tar -czf instances-backup.tar.gz -C /var/lib/gamepanel instances
   ```
3. 将 `.env`、`gamepanel-db.sql`、`instances-backup.tar.gz` 传输到新服务器后，先解压 `/var/lib/gamepanel/instances`，启动 `postgres` 并导入 SQL，再执行 `docker compose up -d` 启动全部服务。
