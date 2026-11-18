# Changelog

All notable changes to **GamePanel Lite** are documented in this file.

## [v2.0.0] - 2026-11-18

**GamePanel Lite v2** 是一次面向生产稳定性的架构重塑：全面从旧版 Python/FastAPI 重构为 **Go 1.26 声明式控制平面 + Next.js 15 控制台**。

### Architecture & Core
- **全新 Go 1.26 + Next.js 15 架构**：`apiserver`、`controller-manager`、`node-agent`、`registry-worker` 与 `web` 五大服务协同，基于 PostgreSQL + Redis + NATS + SeaweedFS 提供轻量可靠的状态协调与对象存储。
- **开箱即用一键部署**：新增 [install.sh](./install.sh)、[.env.example](./.env.example) 与全预构建公共阿里云 ACR 镜像支持，内部组件默认安全绑定 `127.0.0.1`。
- **同源网络与大文件代理**：`web` 层内置 API / WebSocket 终端与日志 / `/files` 签名票据 / SeaweedFS S3 同源反向代理，仅暴露单一 Web 端口即可满足复杂反向代理与移动端访问。

### Games, Mods & Operations
- **13 款开箱即用联机游戏 + 自定义容器**：内置 Minecraft (Paper)、幻兽帕鲁、泰拉瑞亚原版/tModLoader、僵尸毁灭工程、饥荒联机版、七日杀、英灵神殿、护核纪元、幸福工厂、求生之路 2、CS2、腐蚀 Rust 及自定义容器镜像模板。
- **Steam 创意工坊 & Modrinth 直装**：支持有 Key 分类搜索浏览，并在未配置 Steam Web API Key 时自动收起无效控件、支持直接粘贴创意工坊单品或合集链接免 Key 解析安装。
- **饥荒双分片安全备份**：在线备份《饥荒联机版》前向运行中的 `Master` 与 `Caves` 双分片注入带唯一 Token 的 `c_save()` 并确认双分片均落盘后再归档，保障主世界与洞穴存档一致性。
- **移动端深度适配**：修复实例设置页头部及「邀请好友」弹窗在手机窄屏下的溢出、重叠与端口截断问题。
