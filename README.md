<h1 align="center">GamePanel Lite</h1>

<p align="center">Run Terraria, DST, Palworld, and Minecraft servers from one self-hosted Docker panel.</p>

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
  <img alt="GamePanel Lite product preview" src="apps/web/public/official/gamepanel-demo.gif">
  <br>
  <sub>Dashboard · Guided setup · Mods · Presets · Monitoring · HTTPS</sub>
</p>

> GamePanel Lite is under active development. Keep an external copy of important saves and backups.

## What you can do

- Create, start, stop, restart, inspect logs, and use the console for isolated servers.
- Configure game settings, resources, ports, worlds, and backups.
- Discover Workshop mods, build mod packs, and manage tModLoader ModConfig files.
- Monitor resources and update the panel or supported game files from the UI.

| Game | Modes |
| --- | --- |
| Terraria | Vanilla, tModLoader |
| Don't Starve Together | Master, optional Caves shard |
| Palworld | Dedicated server |
| Minecraft | Java Edition |

## Install

Requirements: Linux `amd64`, Docker Engine, and the Docker Compose plugin. The installer downloads the latest stable Tag, not the development branch.

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
4. Open the game port shown on the server details page in the cloud and host firewalls.
5. For HTTPS, point a domain to the server, allow TCP ports `80` and `443`, then open **Settings → Access & HTTPS**.
6. Use **Settings → Updates & Maintenance** for panel updates and recovery.

<table>
  <tr>
    <td align="center"><strong>Guided server creation</strong><br><img alt="Server creation wizard" src="apps/web/public/official/interface-servers.png"></td>
    <td align="center"><strong>Workshop mods and mod packs</strong><br><img alt="Workshop discovery and mod library" src="apps/web/public/official/interface-mods.png"></td>
  </tr>
</table>

<details>
<summary>Command-line recovery</summary>

```bash
cd <installation-directory>
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
