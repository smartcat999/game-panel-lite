# GamePanel Lite

GamePanel Lite is a lightweight, self-hosted game server panel for players, friend groups, and community server owners. It gives you a clean web interface for creating servers, starting and stopping instances, checking logs, managing worlds, backing up data, and discovering mods.

[Live demo](https://dev.gamepanel.site) · [中文文档](docs/README.zh-CN.md)

![GamePanel Lite dashboard](docs/assets/dashboard.png)

## Why GamePanel Lite

Running a game server should not mean juggling shell commands, scattered config files, and manual folders. GamePanel Lite brings the everyday server workflow into one focused dashboard.

- Self-hosted: run it on your own machine or VPS.
- Player-friendly: built around servers, worlds, backups, logs, join info, and mods.
- Lightweight: no cloud account, no billing system, no SaaS lock-in.
- Multi-instance: keep each server isolated with its own data directory.
- Extensible: starts with Terraria-focused workflows and expands toward more game providers.

## What You Can Do

- Create and manage multiple game servers
- Start, stop, and restart server instances
- View server status, logs, and console output
- Import, back up, restore, and manage worlds
- Discover recommended mods and add them to servers
- Keep server files, worlds, backups, and mods organized in one place

## Quick Start

Run this on a server with Docker installed:

```bash
# Default: installs to ~/gamepanel-lite
curl -fsSL https://raw.githubusercontent.com/smartcat999/game-panel-lite/main/scripts/install-online.sh | sh

# Or specify a custom install path:
curl -fsSL https://raw.githubusercontent.com/smartcat999/game-panel-lite/main/scripts/install-online.sh | sh -s -- /opt/gamepanel-lite
```

Then open:

```text
http://YOUR_SERVER_IP:3001
```

If you have a domain pointed at the server, enable HTTPS:

```bash
cd ~/gamepanel-lite && sudo sh scripts/setup-https.sh your-domain.com your-email@example.com
```

On a systemd host, HTTPS setup installs a persistent daily certificate-renewal timer. Certbot only replaces a certificate after it enters the renewal window; the check itself is safe to run every day. Existing HTTPS installs can enable the timer with:

```bash
cd ~/gamepanel-lite
sudo sh scripts/install-https-renewal-timer.sh
sudo systemctl status gamepanel-lite-https-renewal.timer
```

Run a renewal check immediately or inspect its logs with:

```bash
sudo systemctl start gamepanel-lite-https-renewal.service
sudo journalctl -u gamepanel-lite-https-renewal.service
```

The Certbot Compose service is isolated behind the `certificate` profile, so ordinary control-plane starts no longer create a stopped Certbot container.

## Remote Server Operations

Update the control-plane images and recreate only changed services:

```bash
cd ~/gamepanel-lite
sudo sh scripts/manage.sh update
```

Use `start`, `stop`, or `status` in place of `update` for routine operations. The script automatically selects HTTP or HTTPS mode. It manages the panel control plane only; game containers remain under GamePanel Lite and are not recreated by these commands.

Production uses `compose.prod.yaml`. The HTTPS override replaces the same `nginx` service, so only one reverse proxy owns ports 80 and 443. Do not add the development `compose.yaml` to production commands.

## Data Location

GamePanel Lite stores its data in the `data/` directory inside the install folder. This includes the local database, server instances, worlds, backups, and mod files.

## Current Status

GamePanel Lite is in active development and ready for early self-hosted use. The current focus is Terraria / tModLoader server management, with ongoing work for Don't Starve Together and Palworld mod workflows.

服务器环境ubuntu 24.04 LTS

这个面板在tmodloader的开服过程中有几个问题：


## 1. `concreteVersions` 过滤 `latest`（catalog.go）

`latest` 作为版本号被硬编码过滤掉，导致无法用 `latest` tag 做自动更新镜像。

**改法**：去掉 `|| strings.EqualFold(version, "latest")`，或只在 `ImageFor` 里做 fallback 而不在 `VersionList` 里过滤。

## 2. Docker `not found` 错误未映射为 `ErrWorkloadNotFound`（adapter.go）

手动删容器后 reconciler 不重建，因为 Docker 返回的 `No such container` 没被包装成 `ErrWorkloadNotFound`。

**改法**：`inspectContainerState` 里用 `client.IsErrNotFound(err)` 判断，返回 `ErrWorkloadNotFound`。

## 3. `pull_policy: always` 覆盖本地构建（compose.prod.yaml）

每次 `docker compose up` 都从 registry 拉，覆盖本地构建的镜像。

**改法**：`pull_policy: missing`，或用环境变量控制。

这个需要考虑是否实现，我认为需要保留这个方式，只是更新频率会有问题，以及拿数据会不怎么方便。

## 4. 缺少 `ModConfigs` 挂载（provider.go）

tModLoader 的 mod 配置文件在容器重建时丢失。

**改法**：已修，加 `"ModConfigs:/home/container/ModConfigs"`。

## 5. tModLoader 版本硬编码（Dockerfile）

`ARG TML_VERSION=v2026.04.3.0` 写死，每次发新版要手动改。

**改法**：构建脚本从 GitHub API 获取最新版号作为 build arg。

## 6. `providers.json` 版本列表静态

新增版本需要手动编辑 JSON。

**改法**：支持 `latest` tag（已修），配合镜像 tag 策略做到"选 latest 永远拿最新"。

## 7. Dockerfile 依赖网络下载（tModLoader + steamcmd）

构建时从 GitHub/Steam CDN 下载，墙内必挂。

**改法**：提供 `--build-arg` 切换下载/Copy 模式，或拆成 `Dockerfile` + `Dockerfile.offline`。

## 8. entrypoint 路径硬编码 `server/`

GitHub 下载的 tModLoader 解压到根目录，但 entrypoint 写死了 `./server/start-tModLoaderServer.sh`。

**改法**：已修，改为 `./start-tModLoaderServer.sh`。更好的做法是自动检测。

## 9. steamcmd workshop sync 无限阻塞

墙内 steamcmd 连 Steam CDN 极慢/超时，卡住整个容器启动。

**改法**：给 steamcmd 加 `timeout`，超时后跳过 workshop sync 继续启动，或把 workshop sync 移到后台异步执行。



longrunSigmads

longrunSigma
