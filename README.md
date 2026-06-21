# GamePanel Lite

GamePanel Lite 是一个轻量级自托管游戏服务器管理面板，让玩家和私服管理员用网页管理游戏服务器。

官网/体验地址：[https://dev.gamepanel.site](https://dev.gamepanel.site)

## 它能做什么

- 创建和管理多个游戏服务器
- 启动、停止、重启服务器
- 查看服务器状态、日志和控制台
- 管理世界文件、备份和恢复
- 发现和安装模组
- 每个服务器都有独立数据目录，互不影响

## 一句话安装

在已经安装 Docker 的服务器上运行：

```bash
git clone https://github.com/smartcat999/game-panel-lite.git && cd game-panel-lite && sh scripts/install.sh
```

安装完成后打开：

```text
http://服务器IP:3001
```

如果你已经把域名解析到服务器，可以开启 HTTPS：

```bash
sudo sh scripts/setup-https.sh your-domain.com your-email@example.com
```

## 数据保存在哪里

GamePanel Lite 会把数据保存在安装目录下的 `data/`，包括服务器实例、世界、备份、数据库和模组文件。

## 当前状态

项目正在快速开发中，适合个人玩家、小团队和私服管理场景试用。
