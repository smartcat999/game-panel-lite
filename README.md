<h1 align="center">🎮 GamePanel Lite</h1>

<p align="center">
  <strong>Next-Generation, Lightweight Self-Hosted Game Server Management Platform.</strong><br>
  Deploy and manage Terraria, Don't Starve Together, Palworld, and Minecraft servers across distributed nodes with zero hassle.
</p>

<p align="center">
  <a href="https://dev.gamepanel.site">🌐 Live Demo</a> ·
  <a href="#-quick-start">⚡ Quick Start</a> ·
  <a href="docs/USER_GUIDE.md">📖 User Guide</a> ·
  <a href="docs/README.zh-CN.md">🇨🇳 简体中文</a> ·
  <a href="https://github.com/smartcat999/game-panel-lite/releases">📦 Releases</a>
</p>

<p align="center">
  <a href="https://github.com/smartcat999/game-panel-lite/releases"><img alt="Latest Release" src="https://img.shields.io/github/v/tag/smartcat999/game-panel-lite?sort=semver&label=Release&color=10b981"></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/github/license/smartcat999/game-panel-lite?color=38bdf8"></a>
  <img alt="Go Version" src="https://img.shields.io/badge/Backend-Go_1.24-00ADD8?logo=go&logoColor=white">
  <img alt="Next.js" src="https://img.shields.io/badge/Frontend-Next.js_15-black?logo=next.js&logoColor=white">
  <img alt="Docker" src="https://img.shields.io/badge/Runtime-Docker-2496ED?logo=docker&logoColor=white">
</p>

<p align="center">
  <img alt="GamePanel Lite Product Preview" src="apps/web/public/official/gamepanel-demo.gif" width="900" style="border-radius: 12px; box-shadow: 0 20px 40px rgba(0,0,0,0.5);">
</p>

---

## ✨ Key Features

- **🎮 Native Multi-Game Engine**: Deep lifecycle and configuration management for **Terraria** (Vanilla/tModLoader), **Don't Starve Together** (multi-shard forest/caves), **Palworld**, and **Minecraft Java Edition**.
- **🌐 Distributed Multi-Node Fleet**: Seamlessly manage game servers across multiple physical machines or cloud instances from a single master dashboard with real-time network topology visualization.
- **📦 Integrated Steam Workshop & Mod Hub**: Search, install, configure dependencies, and batch-toggle mods directly through a unified modern UI.
- **⏱️ Time-Machine Snapshots & Rollback**: One-click world snapshots, backup downloads, and instant disaster recovery without touching the CLI.
- **📊 Production-Grade Observability**: Native Prometheus metrics, host & container resource tracking (CPU, RAM, Network I/O), and cluster event streaming.
- **💻 Interactive Web Console & Real-time Logs**: Low-latency SSE logs, terminal command shortcuts, and integrated player lobby management.
- **🛡️ Container Isolation & Security**: Each game server runs in an isolated Docker container with strict CPU/memory limits and dedicated volume mounts.

---

## 🎮 Supported Games

| Game | Engine / Runtime | Mod Support | World Snapshots | Visual Configuration |
| :--- | :--- | :--- | :--- | :--- |
| **Terraria** | Vanilla / tModLoader | ✅ Steam Workshop & .tmod | ✅ Time Machine | ✅ Visual Rules Editor |
| **Don't Starve Together** | Multi-shard (Forest + Caves) | ✅ Workshop Mods & Setup | ✅ Full Cluster Backup | ✅ Shard Configs & Multipliers |
| **Palworld** | Unreal Engine 5 Dedicated | ✅ Config & Savegame | ✅ World Snapshots | ✅ World Settings & Multipliers |
| **Minecraft Java** | Paper / Fabric / Vanilla | ✅ Plugins & DataPacks | ✅ World Backup | ✅ server.properties Editor |

---

## ⚡ Quick Start

### One-line Automated Installation

Requires Linux (`amd64`), Docker Engine 24+, and Docker Compose v2+.

```bash
curl -fsSL https://raw.githubusercontent.com/smartcat999/game-panel-lite/main/scripts/install-online.sh | sh
```

Once installed, open `http://YOUR_SERVER_IP:3001` in your browser.

---

## 🏗️ Architecture

```
[ Web Dashboard (Next.js 15 + Tailwind) ] 
                 │ (REST API / SSE Streams)
[ Master Controller (Golang Chi + SQLite) ] ── [ Prometheus Observability ]
                 │ 
   ┌─────────────┴─────────────────────────────┐ (WebSocket Reverse Tunnel)
   ▼                                           ▼
[ Local Host Daemon ]                [ Worker Node Agent ]
   ├── Terraria Container (Docker)      ├── Palworld Container (Docker)
   └── DST Shard Container (Docker)     └── Minecraft Container (Docker)
```

---

## 📖 Documentation

- [User Guide (Illustrated)](docs/USER_GUIDE.md)
- [简体中文用户图文指引](docs/USER_GUIDE.zh-CN.md)
- [Architecture & Development Plan](docs/goals/V1_PLAN.md)

---

## 🤝 Feedback & Community

- [Report a Bug](https://github.com/smartcat999/game-panel-lite/issues/new?template=bug_report.yml)
- [Request a Feature](https://github.com/smartcat999/game-panel-lite/issues/new?template=feature_request.yml)
- [Join Discussions](https://github.com/smartcat999/game-panel-lite/issues)

---

## 📄 License

Licensed under the [Apache License 2.0](LICENSE).
