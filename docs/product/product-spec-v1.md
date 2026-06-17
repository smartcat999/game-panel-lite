# GamePanel Lite V1 Product Spec

## Goal

GamePanel Lite V1 is a lightweight self-hosted management panel for Terraria servers.

## Scope

V1 supports:
- Terraria Vanilla servers.
- Terraria tModLoader servers.
- Multiple server instances.
- Custom Terraria config and presets.
- Start, stop, restart, delete, and list server flows.
- SSE log streaming.
- World import, assignment, duplication, and download.
- Backup creation, list, download, delete, and restore with running-server guardrails.
- tModLoader mod upload, list, and delete.
- Modern dark dashboard UI.

V1 excludes auth, billing, cloud providers, SaaS tenancy, OAuth, RBAC, Kubernetes, and plugin marketplaces.

## User Experience

The UI follows the reference image in `docs/ui-reference/gamepanel-lite-v1-ui-reference.png`: dark charcoal surfaces, lightweight cards, green primary actions, purple tModLoader labels, gold backup and warning states, and restrained Terraria-inspired accents.

Primary flows:
- Open dashboard and scan server health.
- Create a Vanilla or tModLoader Terraria server from a preset.
- Copy join information.
- Manage worlds, backups, and mod files.
- Inspect logs and console output.

## Safety

The backend validates upload extensions and prevents path traversal. Runtime code owns Docker details; Terraria providers own Terraria-specific config and image choices.
