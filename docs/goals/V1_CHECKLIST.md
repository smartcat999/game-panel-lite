# GamePanel Lite V1 Checklist

## Completed

- Go backend foundation.
- chi HTTP server.
- SQLite/GORM persistence.
- Provider registry.
- Terraria Vanilla provider.
- Terraria tModLoader provider.
- Terraria presets and `serverconfig.txt` renderer.
- Docker SDK runtime adapter.
- Server list/create/detail/start/stop/restart/delete APIs.
- SSE logs endpoint.
- Server detail live log UI wired to the SSE endpoint.
- Server detail console command submission wired to a running runtime container.
- Start/restart recreates a missing runtime container from persisted server config and the existing data directory.
- World import/list/download/delete/migrate APIs.
- Backup create/list/download/delete and restore APIs with running-server guardrails.
- Backup migrate API.
- tModLoader mod list/upload/delete APIs.
- Local settings read/update API for safe Docker Host settings.
- OpenAPI contract updated for runtime, settings, server, world, backup, mod, and config endpoints.
- Next.js dashboard UI.
- Dark gamer-friendly visual direction based on the reference image.
- TanStack Query and Framer Motion usage in the frontend.
- Playwright E2E smoke coverage for the Chinese app shell, Docker scan feedback, game cover/avatar rendering, and create-server wizard selection states.
- Real Docker daemon status verification on OrbStack.
- Real Vanilla Terraria Docker lifecycle verification through the Go API: create, start, clean SSE logs, auto-create world, listen on the configured port, TCP port probe, and delete cleanup.
- Real tModLoader Docker lifecycle verification through the Go API: create, start, clean SSE logs, auto-create world from `/data/serverconfig.txt`, listen on the configured port, TCP port probe, and delete cleanup.
- README, product spec, architecture doc, and progress log.

## Needs Manual Verification

- Actual server join behavior from a Terraria client. Follow `docs/goals/V1_MANUAL_VERIFICATION.md`.

## Out of Scope for V1

- Authentication.
- Billing.
- Cloud provider provisioning.
- SaaS tenancy.
- OAuth.
- RBAC.
- Kubernetes.
- Plugin marketplace.
