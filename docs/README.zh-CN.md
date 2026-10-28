<h1 align="center">🎮 GamePanel Lite</h1>

<p align="center">
  <strong>新一代轻量级自托管分布式游戏服务器管理面板。</strong><br>
  在一处部署与管理泰拉瑞亚、饥荒联机版、幻兽帕鲁、我的世界、七日杀和僵尸毁灭工程服务器，支持多计算节点。
</p>

<p align="center">
  <a href="https://dev.gamepanel.site">🌐 在线体验 Demo</a> ·
  <a href="#-极速安装">⚡ 极速安装</a> ·
  <a href="USER_GUIDE.zh-CN.md">📖 图文使用指南</a> ·
  <a href="../README.md">🇺🇸 English</a> ·
  <a href="https://github.com/smartcat999/game-panel-lite/releases">📦 版本发布</a>
</p>

<p align="center">
  <a href="https://github.com/smartcat999/game-panel-lite/releases"><img alt="最新版本" src="https://img.shields.io/github/v/tag/smartcat999/game-panel-lite?sort=semver&label=Release&color=10b981"></a>
  <a href="../LICENSE"><img alt="开源许可证" src="https://img.shields.io/github/license/smartcat999/game-panel-lite?color=38bdf8"></a>
  <img alt="Go Version" src="https://img.shields.io/badge/后端-Go_1.24-00ADD8?logo=go&logoColor=white">
  <img alt="Next.js" src="https://img.shields.io/badge/前端-Next.js_15-black?logo=next.js&logoColor=white">
  <img alt="Docker" src="https://img.shields.io/badge/运行环境-Docker-2496ED?logo=docker&logoColor=white">
</p>

<p align="center">
  <a href="../apps/web/public/official/gamepanel-demo-zh-CN.mp4"><img alt="GamePanel Lite 产品预览" src="../apps/web/public/official/gamepanel-demo-zh-CN.gif" width="900" style="border-radius: 12px; box-shadow: 0 20px 40px rgba(0,0,0,0.5);"></a>
</p>

---

## ✨ 核心亮点

- **🎮 多游戏原生适配**：管理 **泰拉瑞亚**（原版 / tModLoader）、**饥荒联机版**（地上 + 洞穴）、**幻兽帕鲁**、**我的世界 Java 版**、**七日杀** 和 **僵尸毁灭工程**（42.21 原版）的生命周期与配置。
- **🌐 分布式多节点与集群舰队**：支持跨多台物理机/云服务器调度游戏容器，主控面板集成实时网络与流量拓扑透视。
- **📦 一体化创意工坊与模组中心**：直接在 Web 界面检索 Steam Workshop 模组、配置依赖、并进行可视化一键启用/禁用。
- **⏱️ 时光机快照与秒级存档回档**：一键保存当前世界快照，支持备份即时下载与灾难秒级回档还原。
- **📊 工业级可观测性监控大盘**：原生集成 Prometheus，实时监控主机与容器级 CPU、内存、网络 I/O 及集群事件流。
- **💻 交互式 Web 控制台与日志流**：低延迟 SSE 实时日志推流，内置快捷控制台命令与开黑大厅邀请机制。
- **👥 分级本地协作权限**：管理员、运维成员和只读访客拥有不同的导航与操作范围，后端对每次请求独立校验，适用于一套自托管面板内的多人协作。
- **🛡️ 纯 Docker 容器隔离与安全**：每个游戏服务器均运行在独立的 Docker 沙箱内，拥有严格的资源限制与持久化卷隔离。

---

## 🎮 游戏支持矩阵

| 游戏 | 运行引擎 / 模式 | 模组支持 | 世界时光机快照 | 可视化游戏规则配置 |
| :--- | :--- | :--- | :--- | :--- |
| **泰拉瑞亚 (Terraria)** | 原版 / tModLoader | ✅ Steam 创意工坊 & .tmod | ✅ 时光机快照回档 | ✅ 可视化世界与游戏规则 |
| **饥荒联机版 (DST)** | 多分片集群 (地面 + 洞穴) | ✅ Workshop 模组与自动配置 | ✅ 完整集群存档备份 | ✅ 世界倍率与分片配置 |
| **幻兽帕鲁 (Palworld)** | 虚幻引擎 5 专用服 | ✅ 配置文件与存档管理 | ✅ 世界时光机快照 | ✅ 帕鲁捕捉与经验倍率 |
| **我的世界 (Minecraft)** | Paper / Fabric / 原版 | ✅ 插件与数据包管理 | ✅ 世界完整备份 | ✅ server.properties 编辑器 |
| **七日杀 (7 Days to Die)** | 原版专用服务器 | 暂无模组服管理 | 停服后备份存档 | 原生服务器分组、沙盒规则与代码 |
| **僵尸毁灭工程 (Project Zomboid)** | 42.21 原版专用服务器 | 首版仅支持原版 | 停服后完整备份 Zomboid | 原生服务器 / 沙盒分组及五个官方预设 |

### 新增生存游戏支持

- **七日杀**：可配置游戏世界、随机地图种子、服务器访问、高级设置和多人规则；沙盒选项按游戏内分类编辑，也可使用沙盒规则代码。游戏文件更新与运行镜像版本分开处理，匹配客户端时需检查实际游戏 Build。
- **僵尸毁灭工程 42.21**：支持搜索原生服务器与沙盒选项、应用五个经核验的官方预设，统一备份存档、配置和账号数据库。应用预设只替换沙盒规则，保留服务器选项；应用配置和重启会保留已有世界的原生身份。首版使用原版地图和默认的 11 个出生区域，暂不提供 Workshop / 模组服管理。
- **网络与存储**：PZ 在容器内使用 UDP 16261/16262；公网访问需映射对应的两个主机端口，FRP 和云防火墙需另行配置。两款新游戏均要求停服后创建一致的备份。游戏程序放在共享镜像层中，各实例单独保存存档数据。

以上支持包含在 **v0.2.19** 中；下方安装脚本跟随已发布版本的元数据。客户端需匹配服务端版本，安装较大的游戏运行镜像前请检查剩余磁盘空间。

---

## ⚡ 极速安装

### 一键自动化安装脚本

需满足系统环境：Linux (`amd64`)、Docker Engine 24+ 及 Docker Compose v2+。

```bash
curl -fsSL https://raw.githubusercontent.com/smartcat999/game-panel-lite/main/scripts/install-online.sh | sh
```

安装完成后，在浏览器中打开 `http://你的服务器IP:3001` 即可访问。

---

## 🏗️ 系统架构设计

```
[ Web 控制台 (Next.js 15 + Tailwind) ] 
                 │ (REST API / SSE 实时推流)
[ 主控调度端 (Golang Chi + SQLite) ] ── [ Prometheus 可观测性系统 ]
                 │ 
   ┌─────────────┴─────────────────────────────┐ (WebSocket 安全反向流隧道)
   ▼                                           ▼
[ 本地宿主机 Daemon ]                 [ 边缘 Worker Agent 节点 ]
   ├── 泰拉瑞亚 容器 (Docker)            ├── 幻兽帕鲁 容器 (Docker)
   └── 饥荒联机版 分片容器 (Docker)       └── 我的世界 容器 (Docker)
```

---

## 📖 文档导航

- [完整图文用户指南 (中文)](USER_GUIDE.zh-CN.md)
- [User Guide (English)](USER_GUIDE.md)
- [系统架构与开发演进规划](goals/V1_PLAN.md)

---

## 🤝 社区交流与问题反馈

- [提交 Bug 缺陷](https://github.com/smartcat999/game-panel-lite/issues/new?template=bug_report.yml)
- [提出新功能建议](https://github.com/smartcat999/game-panel-lite/issues/new?template=feature_request.yml)
- [参与社区 Issue 讨论](https://github.com/smartcat999/game-panel-lite/issues)

---

## 📄 开源许可证

本项目基于 [Apache License 2.0](../LICENSE) 协议开源。
