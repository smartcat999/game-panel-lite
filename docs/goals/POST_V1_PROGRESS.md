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

Goal 3: Palworld Provider

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
- Palworld provider runtime verification against a real Docker daemon.

Not started:
- Full runtime spec model that no longer maps non-Terraria providers through the compatibility config fields.

## Goal 3 Scope

- Create Palworld server.
- Start, stop, restart, delete through the shared server lifecycle.
- Show join information using the external host port.
- Keep save data isolated under the server instance directory.
- Palworld-specific creation form with the most important fields first.

## Goal 3 Progress

Completed:
- Added `palworld` provider key and registered a Palworld provider in the backend registry.
- Palworld now appears as an available game in the game catalog.
- Added Palworld provider metadata, capabilities, schema, default config, validation, and runtime options.
- Added Palworld Docker runtime options using UDP 8211 and an isolated `/palworld` data mount.
- Extended Docker runtime port mapping to support provider-selected TCP or UDP protocol.
- Create-server API now normalizes runtime config by provider instead of forcing Terraria's internal port.
- Create-server wizard can select Palworld and shows a simplified Palworld config form.
- Create-server flow now sends the selected provider key explicitly instead of deriving it only from Terraria mode.
- Added backend tests for Palworld catalog, create/start runtime spec, provider validation, and UDP mapping.
- Added provider-specific config payload persistence so Palworld can store semantic fields such as `saveName`, `serverPassword`, and `adminPassword`.
- Server list/detail responses now expose hydrated `configPayload` while retaining the existing `config` field for compatibility.
- Create-server and config-update APIs now decode provider-specific config payloads before mapping them to runtime config.
- Frontend create flow now sends semantic Palworld config payloads while keeping Terraria creation unchanged.
- Added backend coverage for Palworld config payload creation and update.

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

2026-06-18 Goal 3 Palworld provider slice:

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

2026-06-18 Goal 3 provider-specific config payload:

```bash
go test ./...
go vet ./...
pnpm --filter @gamepanel-lite/web typecheck
pnpm --filter @gamepanel-lite/web lint
pnpm --filter @gamepanel-lite/web build
```

Result:
- `go test ./...` passed.
- `go vet ./...` passed.
- `pnpm --filter @gamepanel-lite/web typecheck` passed.
- `pnpm --filter @gamepanel-lite/web lint` passed.
- `pnpm --filter @gamepanel-lite/web build` passed. Next.js emitted missing optional SWC binary fallback warnings, but completed successfully.

## Known Limitations

- Only one local administrator account is supported.
- No RBAC, OAuth, SaaS account system, or multi-user management is planned for this phase.
- If no admin account exists, backend API routes remain open so a fresh instance can bootstrap; the frontend still forces setup before rendering the app.
- Palworld uses a first-pass provider implementation. It can be selected and created, and its API payload now uses Palworld-specific config fields. Runtime rendering still maps through compatibility config fields until the runtime spec model is fully generalized.
- Palworld runtime uses the `thijsvanloef/palworld-server-docker:latest` image tag for the first slice. Pinning to a curated version list remains follow-up work.
- Palworld has automated create/runtime spec coverage, but still needs manual Docker start verification on a host that can pull and run the image.

## Next Work

Manually verify Palworld start with Docker, then upgrade the server config envelope so non-Terraria providers no longer reuse Terraria field names internally.
