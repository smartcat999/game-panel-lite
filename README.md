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
git clone https://github.com/smartcat999/game-panel-lite.git && cd game-panel-lite && sh scripts/install.sh
```

Then open:

```text
http://YOUR_SERVER_IP:3001
```

If you have a domain pointed at the server, enable HTTPS:

```bash
sudo sh scripts/setup-https.sh your-domain.com your-email@example.com
```

## Data Location

GamePanel Lite stores its data in the `data/` directory inside the install folder. This includes the local database, server instances, worlds, backups, and mod files.

## Current Status

GamePanel Lite is in active development and ready for early self-hosted use. The current focus is Terraria / tModLoader server management, with ongoing work for Don't Starve Together and Palworld mod workflows.
