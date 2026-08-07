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
# Default: installs to ~/gamepanel-lite
curl -fsSL https://raw.githubusercontent.com/smartcat999/game-panel-lite/main/scripts/install-online.sh | sh

# Or specify a custom install path:
curl -fsSL https://raw.githubusercontent.com/smartcat999/game-panel-lite/main/scripts/install-online.sh | sh -s -- /opt/gamepanel-lite
```

Then open:

```text
http://YOUR_SERVER_IP:3001
```

If you have a domain pointed at the server, enable HTTPS:

```bash
cd ~/gamepanel-lite && sudo sh scripts/setup-https.sh your-domain.com your-email@example.com
```

On a systemd host, HTTPS setup installs a persistent daily certificate-renewal timer. Certbot only replaces a certificate after it enters the renewal window; the check itself is safe to run every day. Existing HTTPS installs can enable the timer with:

```bash
cd ~/gamepanel-lite
sudo sh scripts/install-https-renewal-timer.sh
sudo systemctl status gamepanel-lite-https-renewal.timer
```

Run a renewal check immediately or inspect its logs with:

```bash
sudo systemctl start gamepanel-lite-https-renewal.service
sudo journalctl -u gamepanel-lite-https-renewal.service
```

The Certbot Compose service is isolated behind the `certificate` profile, so ordinary control-plane starts no longer create a stopped Certbot container.

## Remote Server Operations

Update the control-plane images and recreate only changed services:

```bash
cd ~/gamepanel-lite
sudo sh scripts/manage.sh update
```

Use `start`, `stop`, or `status` in place of `update` for routine operations. The script automatically selects HTTP or HTTPS mode. It manages the panel control plane only; game containers remain under GamePanel Lite and are not recreated by these commands.

Production uses `compose.prod.yaml`. The HTTPS override replaces the same `nginx` service, so only one reverse proxy owns ports 80 and 443. Do not add the development `compose.yaml` to production commands.

## Data Location

GamePanel Lite stores its data in the `data/` directory inside the install folder. This includes the local database, server instances, worlds, backups, and mod files.

## Current Status

GamePanel Lite is in active development and ready for early self-hosted use. The current focus is Terraria / tModLoader server management, with ongoing work for Don't Starve Together and Palworld mod workflows.
