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

Goal 2: Multi-Game Provider Foundation

Status: In progress

## Completed Goals

### Goal 1: Local Admin Account and Login

- First-run admin setup.
- Login.
- Logout.
- Change password.
- Session persistence.
- API route protection for all non-health routes.
- Frontend setup/login screens and route guard.

Implemented:
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

## Goal 2 Scope

- Provider capabilities.
- Generic game metadata.
- Generic server config envelope.
- Game-specific config payloads.
- Game-specific join info.

## Goal 2 Progress

Completed:
- Added domain types for game keys, provider capabilities, provider config schema, and game catalog entries.
- Extended `GameProvider` with `GameKey`, description, capabilities, and config schema metadata.
- Added provider registry game catalog generation.
- Added available Terraria catalog metadata for Vanilla and tModLoader.
- Added planned Palworld catalog stub without runtime/provider support yet.
- Added protected backend APIs:
  - `GET /api/games`
  - `GET /api/games/{gameKey}`
  - `GET /api/games/{gameKey}/versions`
- Changed server creation to persist the provider's `GameKey` instead of hardcoding `terraria`.
- Added frontend game catalog types and API client methods.
- Added backend and frontend mapper tests for the game catalog contract.
- Updated the create-server first step to render games from `GET /api/games`.
- Updated create-server version loading to use `GET /api/games/terraria/versions`.
- Shows planned games such as Palworld as roadmap-visible but not creatable yet.
- Server detail now resolves provider capabilities from `GET /api/games`.
- Server detail hides unsupported tabs for console, worlds, backups, and mods.
- Server detail resource queries only run when the provider supports the corresponding feature.
- Console quick actions hide kick and ban commands when the provider does not support them.
- Server API mapping now preserves `gameKey` and `providerKey` for frontend capability lookups.
- Create-server wizard now tracks selected game and provider catalog state explicitly.
- Create-server wizard loads versions by selected game and provider instead of hardcoding Terraria.
- Create-server wizard steps are generated from provider capabilities, so unsupported mod steps are hidden.
- Create-server review summary now uses game/provider names instead of Terraria-only copy.

In progress:
- Full game-specific create-server flow.

Not started:
- Goal 3 Palworld provider runtime implementation.

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

2026-06-18 Goal 2 foundation:

```bash
go test ./...
go vet ./...
pnpm --filter @gamepanel-lite/web typecheck
pnpm --filter @gamepanel-lite/web lint
pnpm --filter @gamepanel-lite/web build
pnpm --filter @gamepanel-lite/web test
```

Result:
- `go test ./...` passed.
- `go vet ./...` passed.
- `pnpm --filter @gamepanel-lite/web typecheck` passed after `next build` generated `.next/types`.
- `pnpm --filter @gamepanel-lite/web lint` passed.
- `pnpm --filter @gamepanel-lite/web build` passed.
- `pnpm --filter @gamepanel-lite/web test` could not start because local dependencies are missing Rollup's optional native package `@rollup/rollup-darwin-arm64`.

2026-06-18 Goal 2 capability-aware detail UI:

```bash
go test ./...
go vet ./...
pnpm --filter @gamepanel-lite/web typecheck
pnpm --filter @gamepanel-lite/web lint
pnpm --filter @gamepanel-lite/web build
pnpm --filter @gamepanel-lite/web test
```

Result:
- `go test ./...` passed.
- `go vet ./...` passed.
- `pnpm --filter @gamepanel-lite/web typecheck` passed.
- `pnpm --filter @gamepanel-lite/web lint` passed.
- `pnpm --filter @gamepanel-lite/web build` passed.
- `pnpm --filter @gamepanel-lite/web test` still could not start because local dependencies are missing Rollup's optional native package `@rollup/rollup-darwin-arm64`.

2026-06-18 Goal 2 provider-aware create wizard:

```bash
pnpm --filter @gamepanel-lite/web typecheck
pnpm --filter @gamepanel-lite/web lint
pnpm --filter @gamepanel-lite/web build
go test ./...
go vet ./...
```

Result:
- `pnpm --filter @gamepanel-lite/web typecheck` passed.
- `pnpm --filter @gamepanel-lite/web lint` passed.
- `pnpm --filter @gamepanel-lite/web build` passed. Next.js emitted missing optional SWC binary fallback warnings, but completed successfully.
- `go test ./...` passed.
- `go vet ./...` passed.

## Known Limitations

- Only one local administrator account is supported.
- No RBAC, OAuth, SaaS account system, or multi-user management is planned for this phase.
- If no admin account exists, backend API routes remain open so a fresh instance can bootstrap; the frontend still forces setup before rendering the app.
- Palworld is visible only as a planned catalog stub; it cannot be created until Goal 3.
- The create-server page now uses game/provider catalog state and provider capabilities, but the detailed config form is still implemented with Terraria fields until the first non-Terraria runtime lands.

## Next Work

Continue toward full game-specific create-server flows, then begin Goal 3 Palworld provider runtime implementation.
