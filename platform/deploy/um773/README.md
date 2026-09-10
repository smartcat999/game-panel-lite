# um773 production deployment

This Compose project runs a fresh v2 control plane, Region plane, node agent, PostgreSQL databases, NATS JetStream, web console, and workload egress firewall. Workload containers retain internet access for game updates and mod downloads, while RFC1918 and link-local management destinations are rejected in `DOCKER-USER`.

## Candidate and cutover

1. Build the three release images from the same Git commit: `gamepanel-platform-backend`, `gamepanel-platform-web`, and `gamepanel/tmodloader`.
2. Store generated secrets only in `/opt/gamepanel-platform-v2/.env` with mode `0600`.
3. Start the candidate on port 3006 and wait for every long-running service to become healthy.
4. Verify password login and forced password replacement, Terraria and tModLoader creation, start/stop/restart, console, instance-scoped logs, backup, and restore.
5. Verify the candidate in a real browser at the desktop viewport.
6. Stop the old deployment, change `GAMEPANEL_PUBLIC_PORT` to `3005`, and recreate only the v2 nginx service.
7. Repeat health and browser checks on port 3005. Only then remove the old containers; keep their deployment files and data until the rollback window closes.

nginx resolves the `web` and `control-plane` service names through Docker DNS on each validity window. Release verification must recreate both upstream services without recreating nginx, then prove `/health` and `/login` still succeed; this catches stale container-address regressions during rolling replacement.

## Rollback

1. Stop `gamepanel-platform-v2` nginx, without deleting v2 volumes or `/var/lib/gamepanel-v2`.
2. Restart the recorded previous release from `/opt/gamepanel-platform/releases/<previous-tag>/platform/deploy/um773/compose.yaml` with its original `GAMEPANEL_RELEASE_TAG` and `GAMEPANEL_PUBLIC_PORT=3005`.
3. Verify its health and login page.

The cutover does not import any old business records or game data. Rolling back changes traffic ownership only; it does not rewrite either deployment's database or instance directories.

The 2026-09-11 replacement retained previous release `1e030957` and removed only its Compose containers and network. Its volumes, release directory, and data remain recoverable during the rollback window.
