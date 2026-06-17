# GamePanel Lite Post-V1 Progress

## Current Objective

Build the post-V1 product roadmap from `docs/product/product-roadmap.md` and `docs/goals/POST_V1_PRODUCT_DEVELOPMENT_PLAN.md`.

The work starts with user-facing product functionality rather than V1 UI polish:

1. Local admin account and login.
2. Multi-game provider foundation.
3. Palworld provider.
4. Game-specific create server flow.
5. Don't Starve Together provider.
6. Minecraft Java provider.
7. Cross-game save management.
8. Player management by provider capability.
9. Friend invite flows.
10. Game library and version selection.

## Active Goal

Goal 1: Local Admin Account and Login

Status: Implemented, pending review in the running app

## Goal 1 Scope

- First-run admin setup.
- Login.
- Logout.
- Change password.
- Session persistence.
- API route protection for all non-health routes.
- Frontend setup/login screens and route guard.

## Goal 1 Progress

Completed:
- Created the post-V1 product development plan.
- Added milestone grouping and dependency notes to the development plan.
- Created this post-V1 progress tracker.
- Added backend admin account and session persistence.
- Added PBKDF2-SHA256 password hashing and session cookies.
- Added first-run setup, login, logout, current-account, and password-change API routes.
- Protected non-health API routes after the first admin account exists.
- Added frontend first-run setup and login gate before rendering the app shell.
- Added logout in the local profile menu.
- Added Settings password change form.
- Updated server log SSE connections to send auth cookies.

In progress:
- Manual browser verification against a running app.

Not started:
- Goal 2 multi-game provider foundation.

## Verification Log

2026-06-18:

```bash
go test ./...
pnpm --filter @gamepanel-lite/web typecheck
pnpm --filter @gamepanel-lite/web build
pnpm --filter @gamepanel-lite/web lint
```

Result:
- All commands passed.
- `next build` emitted missing optional SWC binary fallback warnings, but completed successfully.

## Known Limitations

- Only one local administrator account is supported.
- No RBAC, OAuth, SaaS account system, or multi-user management is planned for this phase.
- If no admin account exists, backend API routes remain open so a fresh instance can bootstrap; the frontend still forces setup before rendering the app.

## Next Work

Run the app locally, verify the setup/login/logout/password-change flow in browser, then begin Goal 2 multi-game provider foundation.
