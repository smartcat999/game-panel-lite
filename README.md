<h1 align="center">GamePanel Lite</h1>

<p align="center">A lightweight, self-hosted panel for running game servers with Docker.</p>

<p align="center">
  <a href="https://dev.gamepanel.site">Live demo</a> ·
  <a href="#install">Install</a> ·
  <a href="docs/README.zh-CN.md">简体中文</a> ·
  <a href="https://github.com/smartcat999/game-panel-lite/releases">Releases</a>
</p>

<p align="center">
  <a href="https://github.com/smartcat999/game-panel-lite/releases"><img alt="Latest release" src="https://img.shields.io/github/v/tag/smartcat999/game-panel-lite?sort=semver&label=release"></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/github/license/smartcat999/game-panel-lite"></a>
  <img alt="Docker" src="https://img.shields.io/badge/runtime-Docker-2496ED?logo=docker&logoColor=white">
</p>

<p align="center">
  <img alt="GamePanel Lite dashboard" src="apps/web/public/official/interface-dashboard.png">
</p>

## What you can do

- Create and manage multiple isolated game servers.
- Start, stop, restart, inspect logs, and use the server console.
- Configure game settings, CPU, memory, ports, worlds, and backups.
- Discover Workshop mods, build mod packs, and manage tModLoader ModConfig files.
- Monitor host and container resources.
- Update the panel, runtime images, and supported game files from the UI.

| Game | Modes |
| --- | --- |
| Terraria | Vanilla, tModLoader |
| Don't Starve Together | Master, optional Caves shard |
| Palworld | Dedicated server |
| Minecraft | Java Edition |

## Install

Requirements: Linux `amd64`, Docker Engine, and the Docker Compose plugin.

```bash
curl -fsSL https://raw.githubusercontent.com/smartcat999/game-panel-lite/main/scripts/install-online.sh | sh
```

The installer asks for:

1. Installation directory, defaulting to the last successful location or `~/gamepanel-lite`.
2. Control-plane image region: Docker Hub or Alibaba Cloud.

For unattended China Mainland installation:

```bash
curl -fsSL https://raw.githubusercontent.com/smartcat999/game-panel-lite/main/scripts/install-online.sh \
  | GAMEPANEL_IMAGE_REGION=cn sh
```

## Use

1. Open `http://YOUR_SERVER_IP:3001` and create the local administrator.
2. Install a runtime image from **Game Library**.
3. Select **Create server** and follow the guided setup.
4. For HTTPS, point a domain to the server, allow TCP ports `80` and `443`, then open **Settings → Access & HTTPS**.
5. Use **Settings → Updates & Maintenance** for panel updates and recovery.

<table>
  <tr>
    <td><img alt="Server creation wizard" src="apps/web/public/official/interface-servers.png"></td>
    <td><img alt="Workshop discovery and mod library" src="apps/web/public/official/interface-mods.png"></td>
  </tr>
</table>

<details>
<summary>Command-line recovery</summary>

```bash
cd ~/gamepanel-lite
sudo sh scripts/manage.sh status
sudo sh scripts/manage.sh update
sudo sh scripts/manage.sh start
sudo sh scripts/manage.sh stop
```

</details>

## Feedback

- [Report a bug](https://github.com/smartcat999/game-panel-lite/issues/new?template=bug_report.yml)
- [Request a feature](https://github.com/smartcat999/game-panel-lite/issues/new?template=feature_request.yml)
- Check [existing issues](https://github.com/smartcat999/game-panel-lite/issues) before submitting a duplicate.

## License

[Apache License 2.0](LICENSE)
