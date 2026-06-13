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
- World import/list/download/delete APIs.
- Backup create/list/download/delete and restore acknowledgement APIs.
- tModLoader mod list/upload/delete APIs.
- Next.js dashboard UI.
- Dark gamer-friendly visual direction based on the reference image.
- TanStack Query and Framer Motion usage in the frontend.
- README, product spec, architecture doc, and progress log.

## Needs Manual Verification

- Docker daemon lifecycle against real Terraria images.
- Actual server join behavior from a Terraria client.
- Backup restore extraction after running-server guardrails are added.
- Playwright e2e flows after Playwright is configured.

## Out of Scope for V1

- Authentication.
- Billing.
- Cloud provider provisioning.
- SaaS tenancy.
- OAuth.
- RBAC.
- Kubernetes.
- Plugin marketplace.
