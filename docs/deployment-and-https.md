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

1. **游戏世界与服务端缓存目录**：默认 `/var/lib/gamepanel/instances`（当与同机其他面板共存时，`install.sh` 会自动设为 `/var/lib/gamepanel-lite/instances`），**务必以旧机 `.env` 中实际的 `GP_AGENT_WORK_DIR` 为准**。
2. **持久化数据库与对象存储**：
   - `postgres_data`：账号、权限、服务器声明配置、定时任务与备份元数据记录
   - `seaweedfs_data`（默认单机部署内置 SeaweedFS）或外部 S3：面板内创建的游戏在线备份压缩包与镜像站已同步的制品 Blob

> **重要提醒**：请务必在旧机与原数据卷保留至新机**恢复验收通过**后再清理；仅迁移数据库而遗漏 `seaweedfs_data` 会导致面板上能看到历史备份记录但无法下载或恢复压缩包。

### 默认单机部署（内置 SeaweedFS）迁移步骤

1. 在面板确认当前没有正在执行的备份、恢复或镜像同步任务后，先停止写入组件，再停止 SeaweedFS，确保对象存储卷静默落盘：
   ```bash
   docker compose stop apiserver controller-manager registry-worker node-agent
   docker compose stop seaweedfs
   ```
2. 导出数据库，确认 `.env` 中的 `GP_AGENT_WORK_DIR` 并打包实例目录，同时解析并导出 SeaweedFS 实际挂载的数据卷（如修改过 `deploy/seaweedfs/s3.json` 也请一并打包）：
   ```bash
   docker compose exec postgres pg_dump -U gamepanel gamepanel > gamepanel-db.sql

   WORK_DIR="$(grep -E '^GP_AGENT_WORK_DIR=' .env | cut -d= -f2-)"
   WORK_DIR="${WORK_DIR:-/var/lib/gamepanel/instances}"
   tar -czf instances-backup.tar.gz -C "$(dirname "$WORK_DIR")" "$(basename "$WORK_DIR")"

   SW_VOL="$(docker inspect -f '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Name}}{{end}}{{end}}' gamepanel-lite-seaweedfs)"
   docker run --rm -v "${SW_VOL}:/data:ro" -v "$PWD":/backup alpine tar -czf /backup/seaweedfs-data.tar.gz -C /data .
   ```
3. 将 `.env`、自定义 `deploy/seaweedfs/s3.json`（若有）、`gamepanel-db.sql`、`instances-backup.tar.gz`、`seaweedfs-data.tar.gz` 传输到新服务器后，按顺序恢复：
   ```bash
   # 1) 还原实例工作目录（与 .env 中 GP_AGENT_WORK_DIR 一致）
   WORK_DIR="$(grep -E '^GP_AGENT_WORK_DIR=' .env | cut -d= -f2-)"
   WORK_DIR="${WORK_DIR:-/var/lib/gamepanel/instances}"
   mkdir -p "$(dirname "$WORK_DIR")"
   tar -xzf instances-backup.tar.gz -C "$(dirname "$WORK_DIR")"

   # 2) 初始化 Compose 目标卷后，先停止 seaweedfs 并恢复 seaweedfs_data
   docker compose up --no-start seaweedfs postgres
   NEW_SW_VOL="$(docker inspect -f '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Name}}{{end}}{{end}}' gamepanel-lite-seaweedfs)"
   docker run --rm -v "${NEW_SW_VOL}:/data" -v "$PWD":/backup alpine sh -c 'rm -rf /data/* && tar -xzf /backup/seaweedfs-data.tar.gz -C /data'

   # 3) 启动 postgres 导入数据库并核对 S3 桶名与凭据（GP_S3_BUCKET / GP_S3_ACCESS_KEY / GP_S3_SECRET_KEY / s3.json）
   docker compose up -d postgres
   cat gamepanel-db.sql | docker compose exec -T postgres psql -U gamepanel -d gamepanel

   # 4) 启动全部服务
   docker compose up -d
   ```
4. **恢复验收**：从新机面板挑选一条**迁移前已有的历史备份记录**执行下载校验并测试恢复到测试实例，确认能正常读取且游戏世界内容一致后，再下线旧机或清理旧卷。

### 外部 S3 对象存储部署迁移说明

- **沿用原外部 S3 桶**：新机 `.env` 保持相同的 `GP_S3_ENDPOINT`、`GP_S3_REGION`、`GP_S3_BUCKET` 及访问凭据（`GP_S3_ACCESS_KEY` / `GP_S3_SECRET_KEY`），迁移前核对新机到外部 S3 的网络连通性与读写权限，仅需迁移 `.env`、数据库（`pg_dump`）与 `GP_AGENT_WORK_DIR`。
- **更换新的 S3 桶**：必须将原桶内全部对象**按原有对象键（Object Key）原样同步**到新桶后，再切换新机 `.env` 中的 `GP_S3_*` 配置并启动面板。
