# GamePanel Lite

GamePanel Lite 是一个轻量、现代、可自托管的游戏服务器管理面板。它面向个人玩家、朋友小队、社区服主和私服管理员，让你用一个网页完成服务器创建、运行、日志、世界、备份和模组管理。

[官网/体验地址](https://dev.gamepanel.site)

## 为什么选择 GamePanel Lite

开游戏服务器不应该依赖一堆命令、散落的配置文件和手动整理目录。GamePanel Lite 把常用的服务器管理流程整理成清晰的网页操作，让你更快开服，也更容易维护。

- 适合自托管：部署在你自己的服务器上，数据保存在本机。
- 上手简单：打开网页即可创建、启动、停止和查看服务器。
- 面向玩家：围绕世界、备份、模组、日志和连接信息设计。
- 轻量运行：不需要复杂平台，也不绑定云厂商。
- 持续扩展：从 Terraria 起步，逐步扩展更多游戏和模组生态。

## 核心功能

- 多服务器实例管理
- 启动、停止、重启服务器
- 实时状态、日志和控制台
- 世界文件管理、导入、备份和恢复
- 模组发现、推荐和安装
- 每个服务器独立数据目录
- 适合个人、小团队和社区服使用

## 快速开始

准备一台已经安装 Docker 的服务器，然后运行：

```bash
git clone https://github.com/smartcat999/game-panel-lite.git && cd game-panel-lite && sh scripts/install.sh
```

启动完成后访问：

```text
http://服务器IP:3001
```

如果你已经把域名解析到服务器，可以开启 HTTPS：

```bash
sudo sh scripts/setup-https.sh your-domain.com your-email@example.com
```

## 数据保存

GamePanel Lite 默认把数据保存在安装目录下的 `data/`，包括服务器实例、世界、备份、数据库和模组文件。迁移或备份时，优先保存这个目录。

## 当前支持

GamePanel Lite 正在快速开发中，当前重点完善 Terraria / tModLoader 服务器管理，同时已经开始支持 Don't Starve Together 和 Palworld 的模组发现与管理流程。

## 项目状态

这是一个早期但可试用的开源项目。欢迎用于个人服务器、小团队联机和社区服管理场景，也欢迎反馈真实开服流程中的问题。
