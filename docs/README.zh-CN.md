<h1 align="center">GamePanel Lite</h1>

<p align="center">在一个自托管 Docker 面板中运行 Terraria、饥荒、幻兽帕鲁和我的世界服务器。</p>

<p align="center">
  <a href="https://dev.gamepanel.site">在线体验</a> ·
  <a href="#安装">安装</a> ·
  <a href="../README.md">English</a> ·
  <a href="https://github.com/smartcat999/game-panel-lite/releases">版本说明</a>
</p>

<p align="center">
  <a href="https://github.com/smartcat999/game-panel-lite/releases"><img alt="最新版本" src="https://img.shields.io/github/v/tag/smartcat999/game-panel-lite?sort=semver&label=release"></a>
  <a href="../LICENSE"><img alt="开源许可证" src="https://img.shields.io/github/license/smartcat999/game-panel-lite"></a>
  <img alt="Docker" src="https://img.shields.io/badge/运行环境-Docker-2496ED?logo=docker&logoColor=white">
</p>

<p align="center">
  <img alt="GamePanel Lite 产品预览" src="../apps/web/public/official/gamepanel-demo.gif">
  <br>
  <sub>仪表盘 · 创建向导 · 模组 · 预设 · 监控 · HTTPS</sub>
</p>

> GamePanel Lite 正在持续开发，请为重要存档和备份保留外部副本。

## 可以做什么

- 创建、启动、停止、重启相互隔离的服务器，并查看日志和控制台。
- 管理游戏参数、资源、端口、世界和备份。
- 发现创意工坊模组、创建模组包并管理 tModLoader ModConfig。
- 在界面中查看资源监控并更新面板或支持的游戏文件。

| 游戏 | 模式 |
| --- | --- |
| Terraria | 原版、tModLoader |
| 饥荒联机版 | 地面、可选洞穴分片 |
| 幻兽帕鲁 | 专用服务器 |
| 我的世界 | Java 版 |

## 安装

环境要求：Linux `amd64`、Docker Engine 和 Docker Compose 插件。安装器会下载最新稳定 Tag，不会安装开发分支。

```bash
curl -fsSL https://raw.githubusercontent.com/smartcat999/game-panel-lite/main/scripts/install-online.sh | sh
```

安装器会依次询问：

1. 安装目录，默认使用上次成功位置，首次安装为 `~/gamepanel-lite`。
2. 控制平面镜像区域：Docker Hub 或阿里云。

中国大陆无人值守安装：

```bash
curl -fsSL https://raw.githubusercontent.com/smartcat999/game-panel-lite/main/scripts/install-online.sh \
  | GAMEPANEL_IMAGE_REGION=cn sh
```

## 使用

1. 打开 `http://服务器IP:3001`，创建本地管理员。
2. 在**游戏库**安装运行镜像。
3. 点击**创建服务器**，按向导完成配置。
4. 根据服务器详情页显示的协议和端口，配置云防火墙与系统防火墙。
5. 配置 HTTPS 时，先把域名解析到服务器并开放 TCP `80`、`443`，然后进入**设置 → 访问与 HTTPS**。
6. 在**设置 → 更新与维护**完成面板更新和恢复操作。

<table>
  <tr>
    <td align="center"><strong>引导式创建服务器</strong><br><img alt="创建服务器向导" src="../apps/web/public/official/interface-servers.png"></td>
    <td align="center"><strong>创意工坊与模组包</strong><br><img alt="创意工坊发现与模组库" src="../apps/web/public/official/interface-mods.png"></td>
  </tr>
</table>

<details>
<summary>命令行恢复</summary>

```bash
cd <安装目录>
sudo sh scripts/manage.sh status
sudo sh scripts/manage.sh update
sudo sh scripts/manage.sh start
sudo sh scripts/manage.sh stop
```

</details>

## 问题反馈

- [提交缺陷](https://github.com/smartcat999/game-panel-lite/issues/new?template=bug_report.yml)
- [建议功能](https://github.com/smartcat999/game-panel-lite/issues/new?template=feature_request.yml)
- 提交前请先检查[已有 Issue](https://github.com/smartcat999/game-panel-lite/issues)，避免重复。

## 开源许可证

[Apache License 2.0](../LICENSE)
