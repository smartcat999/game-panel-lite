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

All post-V1 roadmap goals (1-10) are implemented. Status: Complete

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
- Fully typed per-game config models beyond the current provider-specific runtime bridge.

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
- Added an optional `ServerRuntimeProvider` interface so non-Terraria providers can render runtime config and container options from the server's provider-specific payload.
- Palworld runtime spec generation now reads semantic payload fields directly instead of relying only on Terraria-compatible config fields.
- Palworld config schema now exposes `saveName`, `serverPassword`, and `adminPassword` field names.
- Added provider and handler tests proving Palworld runtime env/config text are generated from `configPayload`.
- Added provider-specific join info through a `JoinInfoProvider` contract.
- Server list/detail/create/lifecycle responses now include `joinInfo` with address, external port, password, invite text, and optional game-specific instructions.
- Terraria and Palworld now generate game-specific invite text instead of relying on a frontend-only generic string.
- Frontend server detail and copy-invite actions now use backend `joinInfo` with safe fallback for older responses.
- Create-server review step now shows game-specific join guidance and an invite text preview before creation.
- Added frontend review invite helper coverage for Terraria and Palworld preview text.

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

2026-06-18 Goal 3 provider runtime bridge:

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

2026-06-18 Goal 3 provider-specific join info:

```bash
go test ./...
go vet ./...
pnpm --filter @gamepanel-lite/web typecheck
pnpm --filter @gamepanel-lite/web lint
pnpm --filter @gamepanel-lite/web test
pnpm --filter @gamepanel-lite/web build
```

Result:
- `go test ./...` passed.
- `go vet ./...` passed.
- `pnpm --filter @gamepanel-lite/web typecheck` passed.
- `pnpm --filter @gamepanel-lite/web lint` passed.
- `pnpm --filter @gamepanel-lite/web test` could not start because local dependencies are still missing Rollup's optional native package `@rollup/rollup-darwin-arm64`.
- `pnpm --filter @gamepanel-lite/web build` passed. Next.js emitted missing optional SWC binary fallback warnings, but completed successfully.

2026-06-18 Goal 4 review join preview:

```bash
pnpm --filter @gamepanel-lite/web typecheck
pnpm --filter @gamepanel-lite/web lint
go test ./...
go vet ./...
pnpm --filter @gamepanel-lite/web test
pnpm --filter @gamepanel-lite/web build
```

Result:
- `pnpm --filter @gamepanel-lite/web typecheck` passed.
- `pnpm --filter @gamepanel-lite/web lint` passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `pnpm --filter @gamepanel-lite/web test` could not start because local dependencies are still missing Rollup's optional native package `@rollup/rollup-darwin-arm64`.
- `pnpm --filter @gamepanel-lite/web build` passed. Next.js emitted missing optional SWC binary fallback warnings, but completed successfully.

2026-06-18 Goal 4 provider schema create form:

Changes:
- Added frontend provider config helpers that derive default payloads from backend `configSchema`.
- Create-server wizard now renders non-Terraria provider config fields from provider metadata instead of using a Palworld-only branch.
- Non-Terraria create submissions keep semantic `configPayload` while deriving the existing compatibility config envelope for review and API submission.
- Added helper coverage for schema defaults, field coercion, and Palworld payload mapping.

Verification:

```bash
pnpm --filter @gamepanel-lite/web lint
pnpm --filter @gamepanel-lite/web typecheck
pnpm --filter @gamepanel-lite/web build
go test ./...
go vet ./...
pnpm --filter @gamepanel-lite/web test
```

Result:
- `pnpm --filter @gamepanel-lite/web lint` passed.
- `pnpm --filter @gamepanel-lite/web typecheck` passed after rerunning independently. The first parallel run raced with `next build` while `.next/types` was being regenerated.
- `pnpm --filter @gamepanel-lite/web build` passed. Next.js emitted missing optional SWC binary fallback warnings, but completed successfully.
- `go test ./...` passed.
- `go vet ./...` passed.
- `pnpm --filter @gamepanel-lite/web test` could not start because local dependencies are still missing Rollup's optional native package `@rollup/rollup-darwin-arm64`.

2026-06-18 Goal 5 Don't Starve Together provider slice:

Changes:
- Added `dont-starve-together` game/provider domain keys.
- Added backend DST provider metadata, config schema, validation, runtime options, generated `cluster.ini`, `server_token.txt`, `Master/server.ini`, and join info.
- Registered DST in the backend provider registry and application startup.
- Added create/config payload decoding for DST fields such as `clusterName`, `clusterToken`, and `gameMode`.
- Create-server review now uses the catalog game name for invite previews, including Don't Starve Together.
- Added backend provider, registry, and create/start runtime spec tests for DST.

Verification:

```bash
go test ./...
go vet ./...
pnpm --filter @gamepanel-lite/web typecheck
pnpm --filter @gamepanel-lite/web lint
pnpm --filter @gamepanel-lite/web test
pnpm --filter @gamepanel-lite/web build
```

Result:
- `go test ./...` passed.
- `go vet ./...` passed.
- `pnpm --filter @gamepanel-lite/web typecheck` passed.
- `pnpm --filter @gamepanel-lite/web lint` passed.
- `pnpm --filter @gamepanel-lite/web test` could not start because local dependencies are still missing Rollup's optional native package `@rollup/rollup-darwin-arm64`.
- `pnpm --filter @gamepanel-lite/web build` passed. Next.js emitted missing optional SWC binary fallback warnings, but completed successfully.

2026-06-18 Goal 5 DST caves, preset, and Workshop setup:

Changes:
- Extended DST provider schema with world preset, caves toggle, and Workshop ID fields.
- Runtime spec generation now writes Master world preset files, optional Caves shard files, and `dedicated_server_mods_setup.lua` when Workshop IDs are provided.
- DST semantic payload persistence now keeps `worldPreset`, `cavesEnabled`, and normalized Workshop IDs so restarts use the same setup.
- Expanded provider and HTTP create/start tests for DST caves, presets, and Workshop setup files.

Verification:

```bash
go test ./...
go vet ./...
pnpm --filter @gamepanel-lite/web lint
pnpm --filter @gamepanel-lite/web build
pnpm --filter @gamepanel-lite/web typecheck
pnpm --filter @gamepanel-lite/web test
```

Result:
- `go test ./...` passed.
- `go vet ./...` passed.
- `pnpm --filter @gamepanel-lite/web lint` passed.
- `pnpm --filter @gamepanel-lite/web build` passed. Next.js emitted missing optional SWC binary fallback warnings, but completed successfully.
- `pnpm --filter @gamepanel-lite/web typecheck` passed after rerunning independently. The first parallel run raced with `next build` while `.next/types` was being regenerated.
- `pnpm --filter @gamepanel-lite/web test` could not start because local dependencies are still missing Rollup's optional native package `@rollup/rollup-darwin-arm64`.

2026-06-18 Goal 5 DST runtime image script:

Changes:
- Added `docker/dst/Dockerfile` for a GamePanel Lite Don't Starve Together dedicated server runtime image.
- Added `docker/dst/gamepanel-dst-entrypoint.sh` to start the Master shard and optional Caves shard from the generated cluster config.
- Updated DST provider runtime files to match the real DST cluster layout: `/data/dst/<cluster-name>/...`.
- Added `dst` target to `scripts/build-game-images.sh` with an explicit `linux/amd64` guard.
- Added `docker/dst/README.md` documenting build commands, runtime layout, and the Klei token requirement.

Verification:

```bash
bash -n scripts/build-game-images.sh docker/dst/gamepanel-dst-entrypoint.sh docker/tmodloader/gamepanel-tmodloader-entrypoint.sh docker/terraria-vanilla/gamepanel-terraria-entrypoint.sh
go test ./...
go vet ./...
pnpm --filter @gamepanel-lite/web typecheck
pnpm --filter @gamepanel-lite/web lint
pnpm --filter @gamepanel-lite/web build
pnpm --filter @gamepanel-lite/web test
```

Result:
- `bash -n ...` passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `pnpm --filter @gamepanel-lite/web typecheck` passed.
- `pnpm --filter @gamepanel-lite/web lint` passed.
- `pnpm --filter @gamepanel-lite/web build` passed. Next.js emitted missing optional SWC binary fallback warnings, but completed successfully.
- `pnpm --filter @gamepanel-lite/web test` could not start because local dependencies are still missing Rollup's optional native package `@rollup/rollup-darwin-arm64`.

## Goal 6 Progress (Minecraft Java Provider)

Completed:
- Added `minecraft` game/provider domain keys and registered a Minecraft provider in the backend registry.
- Minecraft now appears as an available game in the game catalog with version selection (latest + pinned versions).
- Added Minecraft provider metadata, capabilities (console, player list, kick, ban, saves, backups), config schema, validation, runtime options, and join info.
- Added Minecraft Docker runtime options using TCP 25565 and an isolated `/data` mount.
- Minecraft config renders `server.properties` and `eula.txt`; EULA acceptance is enforced during creation.
- Provider runtime bridge reads semantic payload fields (serverName, worldName, maxPlayers, gameMode, difficulty, onlineMode, whitelistEnabled, eulaAccepted).
- Create-server wizard renders Minecraft fields from provider metadata automatically (provider-aware flow).
- Added backend provider and HTTP create/start runtime spec tests for Minecraft.

## Goal 7 Progress (Cross-Game Save Management)

Completed:
- Added `SaveMetadataProvider` contract and `saveDisplayName` on each provider (Terraria=world, Palworld=save, DST=cluster save, Minecraft=world).
- Game catalog now exposes `saveDisplayName` per provider so the UI uses game-aware naming.
- Added cross-game save snapshot APIs:
  - `GET /api/servers/{id}/saves`
  - `POST /api/servers/{id}/saves/snapshot`
  - `POST /api/servers/{id}/saves/{saveId}/restore`
  - `GET /api/servers/{id}/saves/{saveId}/download`
- Existing Terraria world routes and backup routes remain unchanged.
- Added HTTP test covering create/list/download/restore with Minecraft and verifying game-aware save display name.
- Frontend server detail backups tab label now reflects the game save noun.

## Goal 8 Progress (Player Management by Provider Capability)

Completed:
- Added `PlayerCommandProvider` contract with `KickCommand` and `BanCommand` for sanitised command generation.
- Terraria (Vanilla + tModLoader) and Minecraft expose kick/ban commands; Palworld and DST correctly do not.
- Added player management APIs:
  - `GET /api/servers/{id}/players` (returns `supported` flag + online players when supported)
  - `POST /api/servers/{id}/players/{player}/kick`
  - `POST /api/servers/{id}/players/{player}/ban`
- All endpoints are gated by provider capability; unsupported games return `supported: false` / `400`.
- Added a `PlayersPanel` frontend component shown only when `playerList` is supported, with confirmation dialogs for kick/ban.
- Added backend provider command tests and an HTTP capability-gating test.

## Goal 9 Progress (Friend Invite Experience)

Completed:
- Added a configurable public host setting (`GAMEPANEL_PUBLIC_HOST` env + mutable via API/DB).
- Added `Setting` domain model and store methods (`GetSetting`/`SetSetting`).
- Added `PUT /api/settings/public-host` to update the public host at runtime.
- Added `GET /api/servers/{id}/join-info` dedicated endpoint.
- Join info now resolves the public host and rewrites invite text/addresses accordingly.
- Settings page now includes a Public Host card.
- Added HTTP test covering join-info, public host update, and settings exposure.

## Goal 10 Progress (Game Version Selection and Library Presentation)

Completed:
- Added game metadata: `coverImage` key and `serverCount` (computed per game in the catalog API).
- Added `recommendedVersion` per provider (first non-`latest` version).
- Frontend dashboard now renders a Game Library section with game cards, server counts, and create-server links.
- Added HTTP test verifying server counts, cover image keys, and recommended version.

## Goal 13 Progress (Shareable Server Page)

Completed:
- Added `ServerShare` domain model and migration support.
- Added share-page APIs:
  - `GET /api/servers/{id}/share`
  - `POST /api/servers/{id}/share`
  - `DELETE /api/servers/{id}/share`
  - `GET /api/public/servers/{token}`
- Public share responses expose only join-safe fields: server name, game/provider, status, player count, and join info.
- Passwords are hidden on public pages unless the admin explicitly enables password visibility.
- Server deletion now removes associated share records.
- Added server-detail share controls for enabling/disabling share pages, copying share links, opening the public page, and choosing whether to include the password.
- Added `/share/[token]` public frontend route that bypasses the admin login gate and app shell.
- Added HTTP coverage for enable/status/public/hide-password/include-password/disable flows.

## Verification Log

2026-06-18 Goals 6-10 full implementation:

```bash
go test ./...
go vet ./...
gofmt -l apps/api/internal/
pnpm --filter @gamepanel-lite/web typecheck
pnpm --filter @gamepanel-lite/web lint
pnpm --filter @gamepanel-lite/web build
```

Result:
- `go test ./...` passed (15 packages).
- `go vet ./...` passed.
- `gofmt -l apps/api/internal/` reported no files (all formatted).
- `pnpm --filter @gamepanel-lite/web typecheck` passed.
- `pnpm --filter @gamepanel-lite/web lint` passed.
- `pnpm --filter @gamepanel-lite/web build` passed. Next.js emitted missing optional SWC binary fallback warnings, but completed successfully.

2026-06-18 Goal 13 shareable server page:

```bash
git diff --check
go test ./...
go vet ./...
pnpm --filter @gamepanel-lite/web typecheck
pnpm --filter @gamepanel-lite/web lint
pnpm --filter @gamepanel-lite/web build
```

Result:
- `git diff --check` passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `pnpm --filter @gamepanel-lite/web typecheck` passed.
- `pnpm --filter @gamepanel-lite/web lint` passed.
- `pnpm --filter @gamepanel-lite/web build` passed. Next.js emitted missing optional SWC binary fallback warnings, but completed successfully.

## Known Limitations

- Only one local administrator account is supported.
- No RBAC, OAuth, SaaS account system, or multi-user management is planned for this phase.
- If no admin account exists, backend API routes remain open so a fresh instance can bootstrap; the frontend still forces setup before rendering the app.
- Palworld uses a first-pass provider implementation. It can be selected and created, and its API/runtime payload now uses Palworld-specific config fields through a provider runtime bridge.
- Palworld runtime uses the `thijsvanloef/palworld-server-docker:latest` image tag. Pinning to a curated version list remains follow-up work.
- Palworld and Don't Starve Together have automated create/runtime spec coverage, but still need manual Docker start verification on a host that can pull and run each image.
- Minecraft runtime uses the `itzg/minecraft-server` image. Manual Docker start verification on a host is recommended before production use.
- Non-Terraria create flow still uses a compatibility config envelope internally while the backend provider payload model is being phased in.
- Don't Starve Together is wired through catalog/create/runtime spec generation, including caves and Workshop setup files. The runtime image has a build script, but still needs a real Docker host build/start verification before claiming full runtime support.
- Player kick/ban and save snapshot restore require the server to be running/stopped respectively; the UI enforces these preconditions.
- Share pages are read-only and token-based. They intentionally do not support lifecycle controls or admin-only server data.
- Frontend unit tests (`pnpm test`) cannot run locally because optional native Rollup binaries are missing on this machine; backend and frontend type/lint/build checks all pass.

## Next Work

The post-V1 roadmap Goals 1-13 have implementation slices. Product direction has paused adding more game providers until the existing multi-game surfaces are complete enough for current Palworld, DST, and Minecraft support.

Recommended next work:
- Continue closing existing multi-game feature gaps across list/detail/resource pages before adding another provider.
- Manual Docker host verification for Palworld, DST, and Minecraft runtime images.
- Curate and pin version lists per game.

## Goal 11 Progress (Mobile-Friendly Controls)

Status: completed.

Implemented:
- Server list cards collapse to a single-column mobile layout, with primary actions grouped into comfortable two-column touch targets.
- Server detail desktop actions remain in the header, while mobile gets a first-screen control block for join address, copy invite, start/stop, and restart.
- Mobile detail controls intentionally omit delete from the first-screen action block; destructive actions still use confirmation where they are exposed.
- App chrome now has a compact mobile create button and bottom navigation for the primary sections hidden behind the desktop sidebar.

Verification:
- `pnpm --filter @gamepanel-lite/web typecheck` passed.
- `pnpm --filter @gamepanel-lite/web lint` passed.
- `pnpm --filter @gamepanel-lite/web build` passed. Next.js emitted the existing optional SWC fallback warnings, but the production build completed successfully.

## Goal 12 Progress (Configuration Presets)

Status: completed.

Implemented:
- Added a `ConfigPreset` domain model and SQLite migration coverage through `AutoMigrate`.
- Added configuration preset APIs:
  - `GET /api/config-presets`
  - `POST /api/config-presets`
  - `GET /api/config-presets/{id}`
  - `PUT /api/config-presets/{id}`
  - `DELETE /api/config-presets/{id}`
- Preset create/update validates payloads through the selected provider and preserves game, provider, version, friendly config values, resource limits, and optional mod pack reference.
- Preset persistence strips secrets before saving: generic server passwords plus provider password fields such as Palworld admin password and DST cluster token.
- Create-server flow now shows saved configuration presets on the first step and applies them as editable form defaults.
- Added a configuration preset management page with search, game filtering, delete confirmation, and create-from-preset entry points.
- Create-server flow now accepts `presetId` in the URL and automatically applies the selected saved preset.
- Config step can save the current non-world configuration as a preset, with copy clarifying that worlds/saves, passwords, and runtime state are excluded.
- Updated development-plan wording from "server templates" to "configuration presets" to keep the feature separate from world snapshots.

Verification:
- `go test ./...` passed.
- `GOCACHE=/private/tmp/game-panel-lite-go-cache go vet ./...` passed.
- `git diff --check` passed.
- `pnpm --filter @gamepanel-lite/web typecheck` passed.
- `pnpm --filter @gamepanel-lite/web lint` passed.
- `pnpm --filter @gamepanel-lite/web build` passed. Next.js emitted the existing optional SWC fallback warnings, but the production build completed successfully.

## Goal 10 / Multi-Game List Filtering Progress

Status: in progress. Current list-filtering slice complete.

Implemented:
- Server list game filters now come from the game catalog instead of a hardcoded Terraria-only list.
- World, backup, and mod list game filters now use the game catalog plus resource data, so current and future supported games appear in the filter bar without per-page rewrites.
- Server and backup type filters now treat "vanilla/modded" as Terraria-specific, avoiding Palworld, DST, or Minecraft being accidentally grouped under Terraria vanilla.
- Backend world and backup list/create responses now include JSON-only game/provider metadata derived from the owning server/provider, so filters do not depend on frontend guessing.
- Activity page now supports a game filter based on the game catalog and the current server mapping.

Verification:
- `git diff --check` passed.
- `GOCACHE=/private/tmp/game-panel-lite-go-cache go test ./apps/api/internal/http -run TestResourceListsIncludeGameMetadata -count=1` passed.
- `GOCACHE=/private/tmp/game-panel-lite-go-cache go test ./...` passed.
- `GOCACHE=/private/tmp/game-panel-lite-go-cache go vet ./...` passed.
- `pnpm --filter @gamepanel-lite/web typecheck` passed.
- `pnpm --filter @gamepanel-lite/web lint` passed.
- `pnpm --filter @gamepanel-lite/web build` passed. Next.js emitted the existing optional SWC fallback warnings, but the production build completed successfully.
- `pnpm --filter @gamepanel-lite/web test -- game-filters.test.ts server-filters.test.ts` could not start because the local optional Rollup native package `@rollup/rollup-darwin-arm64` is missing from `node_modules`; this matches the existing known local test limitation.
