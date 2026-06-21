# GamePanel Lite

GamePanel Lite 是一个轻量、现代、适合自托管的游戏服务器管理面板。

它把“创建服务器、启动停止、查看日志、管理世界、备份恢复、安装模组”放在一个简单的网页里。你不需要记一堆命令，也不用手动整理每个游戏的目录。

## 现在可以做什么

- 创建和管理多个游戏服务器
- 启动、停止、重启服务器
- 查看服务器状态、日志和控制台
- 管理世界文件、备份和恢复
- 管理 Terraria / tModLoader 模组
- 发现 Don't Starve Together Workshop 推荐模组
- 发现 Palworld `.pak` / UE4SS 文件型推荐模组
- 每个服务器都有独立数据目录，互不影响

## 一键安装

准备一台已经安装 Docker 的机器，然后在项目目录执行：

```bash
sh scripts/install.sh
```

启动完成后打开：

```text
http://localhost:3001
```

脚本会拉取正式镜像并启动服务，不会在生产环境重新构建镜像。
生产入口由 Nginx 代理到 Web 和 API，外部只需要访问一个端口。

默认数据会保存在项目目录下的 `data/`，包括数据库、服务器实例、世界、备份和模组文件。

## 常用配置

第一次运行会自动生成 `.env`。常用配置如下：

```env
GAMEPANEL_WEB_PORT=3001
GAMEPANEL_API_PORT=4000
NEXT_PUBLIC_API_BASE_URL=
```

如果你部署到服务器，并希望网页直接使用 80 端口，可以改成：

```env
GAMEPANEL_WEB_PORT=80
NEXT_PUBLIC_API_BASE_URL=
```

默认情况下，Nginx 会把同源 `/api` 代理到后端，不需要额外暴露 API 端口。然后重新启动：

```bash
docker compose -f compose.prod.yaml up -d
```

## 使用方式

1. 打开 GamePanel Lite。
2. 创建一个服务器，选择游戏和运行版本。
3. 按需要调整配置。
4. 点击启动，面板会自动准备运行环境。
5. 在服务器详情页查看日志、控制台、连接信息、世界、备份和模组。

## Palworld 模组包

Palworld 的 Nexus 模组不是 Steam Workshop 模组。GamePanel Lite 会把它们作为“文件型推荐项”展示。

如果你已经准备好 Palworld 模组文件，可以同步到本地数据目录：

```bash
docker compose --profile jobs run --rm palworld-mod-pack
```

同步后的文件会进入：

```text
data/mods/palworld-pack/
```

推荐列表里的 Palworld 模组会引用这些本地文件，并按依赖关系加入模组列表。

## 开发者入口

本地开发可以使用：

```bash
pnpm dev:api
pnpm dev:web
```

常用检查：

```bash
go test ./...
pnpm typecheck
pnpm build
```

更多架构和实现说明放在 `docs/` 目录。
