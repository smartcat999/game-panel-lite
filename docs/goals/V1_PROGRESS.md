# GamePanel Lite V1 Progress

## Phase 0: Repository and Environment Check

Status: Completed

Completed:
- Checked repository structure before implementation.
- Switched work branch to `feat/v1-full-run`.
- Read `AGENTS.md`, `docs/goals/V1_EXECUTION_PLAN.md`, and the V1 UI reference image.
- Confirmed the current project is an early monorepo with root workspace files, `packages/shared`, docs, and no committed `apps/api` or `apps/web` yet.
- Detected that the existing README still describes an old TypeScript Fastify/Prisma backend direction; Phase 1 will replace it with the required Go backend direction.

Environment:
- Go: `go version go1.25.11 darwin/arm64`
- Node.js: `v20.19.0`
- pnpm: `9.15.4`
- Docker CLI: `Docker version 29.4.0, build 9d7ad9f`
- Current branch: `feat/v1-full-run`

Checks:
- No buildable Go backend or Next.js app existed at Phase 0.
- Tool availability was checked directly.

Known issues:
- Docker daemon availability has not been verified yet; Phase 4 will expose Docker runtime status in the API and UI.
- Playwright is not configured yet.
- The root lockfile currently contains dependencies for an old `apps/api` TypeScript backend importer, but V1 implementation must use Go for the backend.

Next:
- Phase 1: create the Go API foundation, Next.js app foundation, OpenAPI contract, initial tests, and updated local development docs.

## Phase 1: Monorepo Foundation

Status: Completed

Completed:
- Added root Go module for the backend packages under `apps/api`.
- Added chi HTTP server entrypoint with graceful shutdown.
- Added config loading, domain models, provider registry, Terraria provider shells, mock runtime adapter, SQLite store initialization, migration seed file, and basic HTTP endpoints.
- Added OpenAPI contract package with the first health/version/Terraria config endpoints.
- Added initial `apps/web` Next.js, TypeScript, Tailwind app foundation.
- Replaced old README Fastify/Prisma references with the required Go backend development flow.

Checks:
- `gofmt -w apps/api`: passed
- `go test ./...`: passed
- `go vet ./...`: passed
- `pnpm lint`: passed
- `pnpm typecheck`: passed
- `pnpm test`: passed
- `pnpm build`: passed

Known issues:
- Docker runtime is still a mock adapter in Phase 1 by design.
- Server management, worlds, backups, and mod management APIs are added in later phases.

Next:
- Phase 2: build the dark gamer-friendly mock UI routes and reusable frontend components from the reference image.

## Phase 2: V1 Mock UI

Status: Completed

Completed:
- Added the full dark dashboard shell with sidebar, top search, Docker status, and create-server action.
- Added mock data for servers, worlds, backups, activity, and logs.
- Added reusable UI primitives and feature components for server cards, status badges, mode badges, server actions, page headers, and the create-server wizard.
- Added routes for dashboard, servers, create server, server detail, worlds, backups, mods, activity, and settings.
- Added TanStack Query provider and Framer Motion transitions for the wizard.
- Vanilla mode hides the modded preset and tModLoader mode exposes mod upload copy in the wizard.

Checks:
- `go test ./...`: passed
- `go vet ./...`: passed
- `pnpm lint`: passed
- `pnpm test`: passed
- `pnpm build`: passed
- `pnpm typecheck`: passed after rerunning serially because a parallel `next build` temporarily rebuilt `.next/types`.

Known issues:
- Pages use mock data only in Phase 2.
- Copy, start, stop, restart, import, backup, and upload actions are visual only until later API phases.

Next:
- Phase 3: implement Terraria config validation, presets, serverconfig rendering, OpenAPI updates, and connect the wizard preview to the API.

## Phase 3: Terraria Config and Presets

Status: Completed

Completed:
- Terraria config validation, presets, and `serverconfig.txt` rendering are implemented in Go under the Terraria provider package.
- Added API endpoints for listing presets and previewing rendered server config.
- Expanded OpenAPI schema for the config preview request and response.
- Connected the Create Server wizard Config step to the preview API with TanStack Query mutation state.

Checks:
- `gofmt -w apps/api`: passed
- `go test ./...`: passed
- `go vet ./...`: passed
- `pnpm lint`: passed
- `pnpm test`: passed
- `pnpm build`: passed
- `pnpm typecheck`: passed

Known issues:
- The preview uses the Friends Casual preset payload in the UI until the full create-server form state is wired in Phase 4.

Next:
- Phase 4: implement Docker runtime adapter, server management APIs, SSE logs, and replace key frontend mock server flows with real API calls where practical.

## Phase 4: Go Docker Runtime and Server Management

Status: Completed

Completed:
- Added Docker SDK runtime adapter under `internal/runtime/docker`.
- Added runtime status, server list/create/detail/start/stop/restart/delete, and SSE log endpoints.
- Server creation renders provider config, creates an isolated `data/instances/{instanceId}` directory, and asks the runtime adapter to create the container.
- Docker details remain inside `RuntimeAdapter`; Terraria image/config choices remain in providers.
- Servers page and Settings page now use TanStack Query to call the Go API with mock fallback when the API is not running.

Checks:
- `gofmt -w apps/api`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed
- `pnpm lint`: passed
- `pnpm test`: passed
- `pnpm build`: passed
- `pnpm typecheck`: passed

Known issues:
- Docker image pull/container create requires a running Docker daemon and network access to image registries.
- The UI still uses mock detail data for individual server detail pages until the remaining API flows are connected.
- Go checks used a repo-local `GOCACHE` because the sandbox cannot write to the default user Go build cache.

Next:
- Phase 5: implement world import/management and backup create/restore/download/delete flows with path traversal protection.

## Phase 5: Worlds and Backups

Status: Completed

Completed:
- Added shared file safety helpers for file-name validation and root-contained path joins.
- Added world import service with `.wld` extension validation and per-instance storage under `data/worlds/{instanceId}`.
- Added backup service that creates zip archives under `data/backups/{instanceId}`.
- Added world and backup store methods and HTTP routes for list/import/assign/duplicate/download/delete, backup creation, restore, download, and delete.
- Added tests for traversal protection, world upload validation, and backup creation.

Checks:
- `gofmt -w apps/api`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed
- `pnpm lint`: passed
- `pnpm test`: passed
- `pnpm build`: passed
- `pnpm typecheck`: passed

Known issues:
- Backup restore requires the server to be stopped before replacing files.

Next:
- Phase 6: implement tModLoader provider completion and mod upload/manage APIs.

## Phase 6: tModLoader

Status: Completed

Completed:
- Added tModLoader mod service with validation for `.tmod`, `install.txt`, and `enabled.json`.
- Added per-instance mod storage under `data/mods/{instanceId}`.
- Added mod store methods and HTTP routes for list, upload, and delete.
- Upload API rejects mod uploads for Vanilla servers.
- Existing UI keeps Mods tab visible only for tModLoader server detail and uses purple tModLoader badges.
- Added mod validation tests.

Checks:
- `gofmt -w apps/api`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed
- `pnpm lint`: passed
- `pnpm test`: passed
- `pnpm build`: passed
- `pnpm typecheck`: passed

Known issues:
- Mod dependency resolution and tModLoader workshop sync are outside V1.

Next:
- Phase 7: polish docs, add V1 checklist and architecture docs, add Playwright smoke coverage where available, and run final full checks.

## Phase 7: V1 Polish, Docs, and E2E

Status: Completed

Completed:
- Expanded README with quick start, requirements, usage, Docker notes, safety notes, known limitations, and roadmap.
- Added `docs/product/product-spec-v1.md`.
- Added `docs/architecture/architecture-v1.md`.
- Added `docs/goals/V1_CHECKLIST.md`.
- Finalized `.env.example` with API base URL.
- Checked Playwright availability; no project Playwright config or script exists yet.

Checks:
- `gofmt -w apps/api`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed
- `pnpm lint`: passed
- `pnpm test`: passed
- `pnpm build`: passed
- `pnpm typecheck`: passed
- Playwright: not run, no project Playwright config or script exists yet.

Known issues:
- Playwright e2e tests were not run because the project does not have Playwright configured yet.

Post-audit gap closing:
- Added actual backup restore extraction with zip-slip protection and running-server guardrails.
- Added world assign and duplicate endpoints.
- Wired the create-server wizard custom config fields to preview and `POST /api/servers`.
- Added browser confirmations for destructive server stop and restart actions.

Final status:
- FULL V1 RUN completed through Phase 7 plus post-audit gap closing.
- Real Docker runtime behavior still needs manual verification with Docker daemon and Terraria image pulls.

## Post-V1 Local Testing Update: Chinese UI and Docker Socket

Status: Completed

Completed:
- Added lightweight Chinese/English frontend copy support for the app shell and Settings page.
- Defaulted the frontend locale to Chinese and added a header language toggle.
- Stabilized header control widths so Chinese/English switching does not shift the surrounding layout.
- Reserved the browser scrollbar gutter and prevented the primary create-server CTA from wrapping between locales.
- Added `GAMEPANEL_DOCKER_HOST` backend config for Docker socket/host selection.
- Docker status responses now include the configured host so Settings can show the active value.
- Documented common Docker socket examples in README and `.env.example`.

Known issues:
- Docker availability depends on the local daemon and the socket configured in `GAMEPANEL_DOCKER_HOST`.
- Full app-wide localization is not complete yet; this update covers the main shell and runtime settings needed for local testing.

Checks:
- `gofmt -w apps/api`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed
- `pnpm lint`: passed
- `pnpm test`: passed
- `pnpm build`: passed
- `pnpm typecheck`: passed after rerunning serially because a parallel `next build` temporarily regenerated `.next/types`.

## Post-V1 UI Action Wiring Update

Status: Completed

Completed:
- Wired Dashboard quick actions to real navigation for server creation, world import, and backup management.

- Added real Servers page search and filter behavior.
- Wired Worlds page import, duplicate, download, and delete actions to existing API endpoints.
- Wired Backups page server selection, create, restore, download, and delete actions to existing API endpoints.
- Wired Mods page tModLoader server selection, upload, list, and delete actions to existing API endpoints.
- Updated server detail copy buttons to write to the clipboard and temporarily disabled console command input because V1 only exposed SSE logs at that point. Superseded by the later Runtime Container Lifecycle And Console update.

Known issues:
- Server detail logs remain mock-rendered in the UI even though the backend exposes an SSE logs endpoint.
- Console command submission was not implemented at that point because there was no backend command endpoint. Superseded by the later Runtime Container Lifecycle And Console update.

Checks:
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed
- `pnpm lint`: passed
- `pnpm typecheck`: passed
- `pnpm test`: passed
- `pnpm build`: passed after stopping an old Next dev process that was concurrently writing `.next`.
- Browser smoke: Dashboard, Servers, Worlds, Backups, and Mods rendered with dark styles loaded; Servers status filter interaction passed.

## Post-V1 Server Detail Completion Update

Status: Completed

Completed:
- Rebuilt the server detail page as a real tabbed workspace instead of static tab labels.
- Added current-server Overview, Console, Logs, Config, Worlds, Backups, and tModLoader Mods tabs.
- Connected Console command sending to the backend command API and kept SSE logs visible in Console/Logs.
- Connected Config to the stored Terraria config and `serverconfig.txt` preview API.
- Connected Worlds tab to import `.wld` files for the current server and download existing worlds.
- Connected Backups tab to create current-server backups, download backups, and restore with confirmation.
- Connected Mods tab to upload supported tModLoader files and delete mod files with confirmation.
- Preserved backend-returned Terraria config in frontend server mapping instead of dropping it.
- Disabled frontend TypeScript incremental build cache to avoid stale `.next/types` failures after local dev/e2e/build switching.
- Added Playwright coverage for the detail page tab interactions and key actions.

Checks:
- `pnpm lint`: passed
- `pnpm typecheck`: passed
- `pnpm test`: passed
- `pnpm e2e`: passed
- `pnpm build`: passed
- `go test ./...`: passed
- `go vet ./...`: passed

Known issues:
- Manual runtime verification with a real Terraria container and live player join flow is still needed.

## Post-V1 Activity Events Update

Status: Completed

Completed:
- Added backend activity storage methods and `GET /api/activity`.
- Recorded activity events for server lifecycle, world import/assign/duplicate/migrate/delete, backup create/restore/migrate/delete, and mod upload/delete actions.
- Wired Dashboard recent activity to real API data.
- Rebuilt the Activity page as a real event list with loading, empty, and API error states.
- Updated OpenAPI with the activity endpoint and `ActivityEvent` schema.
- Removed the unused frontend mock data file so new UI work cannot accidentally reintroduce mock server/world/backup/activity data.

Checks:
- `gofmt -w apps/api/internal/store/store.go apps/api/internal/http/handler.go apps/api/internal/http/handler_test.go`: passed
- `go test ./...`: passed
- `go vet ./...`: passed
- `pnpm lint`: passed
- `pnpm typecheck`: passed
- `pnpm build`: passed
- `pnpm e2e`: passed

## Post-V1 Docker Host Selection Update

Status: Completed

Completed:
- Added `GET /api/runtime/docker/hosts` to return the current Docker host and common local candidates.
- Candidate scanning includes `GAMEPANEL_DOCKER_HOST`, `DOCKER_HOST`, Docker Engine default, Docker Desktop user socket, Colima, Rancher Desktop, OrbStack, and local TCP daemon examples.
- Settings page now supports scanning candidates, selecting a detected host, entering a custom host, and copying a backend restart command.
- Documented that applying a new Docker host requires restarting the Go backend with `GAMEPANEL_DOCKER_HOST`.

Known issues:
- Docker host selection is not hot-applied to the running backend process; this is intentional because the Docker SDK client is created from backend process configuration.

Checks:
- `gofmt -w apps/api/internal/config/config.go apps/api/internal/config/config_test.go apps/api/internal/http/handler.go`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed
- `pnpm lint`: passed
- `pnpm typecheck`: passed
- `pnpm test`: passed
- `pnpm build`: passed
- Runtime verification: `GET /api/runtime/docker/hosts` returned Docker Desktop and OrbStack candidates; Settings page rendered scanner controls.

## Post-V1 Mock Data Removal Update

Status: Completed

Completed:
- Removed frontend fallback from API data pages to mock data when the API returns an empty list.
- Dashboard now uses real server and backup queries for stats and active server cards.
- Servers, Worlds, and Backups now show real API data, API errors, loading, or empty states instead of simulated rows.
- Activity and server detail logs now show explicit V1 data-source limitations instead of simulated activity/log lines.
- Removed stale `apiMock*` copy keys so new UI code cannot accidentally reintroduce mock fallback messaging.

Known issues:
- Activity list and server detail logs still need real API/SSE UI wiring in a later pass.

Checks:
- `rg -n "mock-data|apiMock|showing mock|模拟|mock" apps/web`: no matches
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed
- `pnpm lint`: passed
- `pnpm typecheck`: passed
- `pnpm test`: passed
- `pnpm build`: passed after stopping the 3004 dev server to avoid `.next` conflicts.
- Browser verification: Dashboard, Servers, Worlds, Backups, and Activity rendered real empty states with no mock names or mock text.

## Post-V1 Docker Host Reconnect Update

Status: Completed

Completed:
- Added a switchable Go RuntimeAdapter so the Docker SDK host can be changed from the running API process.
- Added `POST /api/runtime/docker/host` with Docker host scheme validation for `unix://`, `tcp://`, and `npipe://`.
- Reworked Settings from a large candidate-card scanner into a compact candidate select, custom host input, and `Apply and reconnect` action.
- Updated the top-bar Docker badge to reflect real runtime availability instead of showing a hardcoded online state.
- Removed the copy-restart-command workflow from the Settings page; process restarts still use `GAMEPANEL_DOCKER_HOST` as the persisted source of truth.

Known issues:
- The hot-applied Docker host is in-memory only. To keep it after restarting the Go API, set `GAMEPANEL_DOCKER_HOST` in the shell or local environment.

Checks:
- `gofmt -w apps/api/internal/app/app.go apps/api/internal/http/handler.go apps/api/internal/runtime/switchable.go apps/api/internal/runtime/switchable_test.go`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed
- `go build ./...`: passed after rerunning outside the sandbox so Go could write its module/build cache.
- `pnpm lint`: passed
- `pnpm typecheck`: passed
- `pnpm test`: passed
- `pnpm build`: passed after clearing stale `.next` output from dev/build switching.
- Runtime verification: `GET /api/runtime/docker/hosts` returned Docker Desktop and OrbStack candidates; `POST /api/runtime/docker/host` hot-applied the selected Docker host and returned runtime status.
- Browser verification: Settings rendered with dark styles, compact Docker Host controls, no copy-restart command, and a Docker badge that showed unavailable when the daemon could not be reached.

## Post-V1 Settings Docker Consolidation Update

Status: Completed

Completed:
- Consolidated Settings Docker information from separate runtime, socket, and scanner cards into one Docker Runtime card.
- Kept the Docker status, current host, candidate scan, custom host input, and reconnect action in a single compact workflow.
- Left Data Directories as the only separate Settings card so the page no longer repeats Docker-specific blocks.
- Verified the merged card renders without the old `Docker Socket / Host` or `Docker Host Scanner` headings.

Known issues:
- The selected Docker host can still be a long socket path; it is wrapped in the summary line to prevent horizontal page overflow.

Checks:
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed
- `go build ./...`: passed
- `pnpm lint`: passed
- `pnpm typecheck`: passed
- `pnpm test`: passed
- `pnpm build`: passed after clearing stale `.next` output from dev/build switching.
- Browser verification: Settings rendered one Docker Runtime card plus one Data Directories card; old `Docker Socket / Host` and `Docker Host Scanner` headings were absent.

## Post-V1 Servers CTA Cleanup Update

Status: Completed

Completed:
- Removed the duplicate page-level Create Server action from the Servers page.
- Kept the global top-bar Create Server button as the single primary creation entry point.

Known issues:
- None for this UI cleanup.

Checks:
- `pnpm lint`: passed
- `pnpm typecheck`: passed
- `pnpm test`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed
- `go build ./...`: passed
- `pnpm build`: passed after clearing stale `.next` output from dev/build switching.

## Post-V1 Create Server Navigation Update

Status: Completed

Completed:
- Prefetched the create-server route from the app shell and on hover of the global create button.
- Added immediate button feedback (`Opening...` / `打开中...`) after clicking Create Server so slow first-load compiles are visible to the user.
- Added a Cancel action to the create-server wizard header that returns to the Servers page.

Known issues:
- In local Next.js dev mode, the first visit to `/servers/new` can still take longer while the route compiles. The button now provides immediate feedback during that delay.

Checks:
- `pnpm lint`: passed
- `pnpm typecheck`: passed
- `pnpm test`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed
- `go build ./...`: passed
- `pnpm build`: passed after clearing stale `.next` output from dev/build switching.
- Browser verification: Create Server link count was 1, click showed immediate `打开中...`, `/servers/new` rendered `取消创建`, and Cancel returned to `/servers`.

## Post-V1 Create Success Redirect Update

Status: Completed

Completed:
- Changed the create-server mutation to invalidate the server list cache after success.
- Seeded the new server detail query cache with the created server response.
- Redirected successful creates from the wizard review step to `/servers/{id}`.

Known issues:
- None for this flow cleanup.

Checks:
- `pnpm lint`: passed
- `pnpm typecheck`: passed
- `pnpm test`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed
- `go build ./...`: passed
- `pnpm build`: passed after clearing stale `.next` output from dev/build switching.
- Verification note: did not create another real server/container during this check; redirect is implemented in the create mutation success handler with cache invalidation and `router.push(/servers/{id})`.

## V1 Completion Gap Closure Update

Status: Completed

Completed:
- Wired the Server Detail log panel to the backend `GET /api/servers/{id}/logs` SSE stream with connection, waiting, and unavailable states.
- Kept console command input disabled because V1 has no backend command endpoint.
- Added `GET /api/settings` and `PUT /api/settings` for safe local settings access; updates are limited to Docker Host and reuse the RuntimeAdapter hot-swap path.
- Added dedicated world and backup migration APIs plus Worlds/Backups page controls for copying assets to a selected server instance.
- Updated the OpenAPI contract for Docker status/hosts/hot-apply, Settings, Worlds, Backups, Mods, server logs, and Terraria config preview.
- Updated README and V1 checklist to reflect live SSE logs and the remaining manual verification boundaries.

Known issues:
- Docker image pull, container lifecycle, and Terraria client join still require manual verification against a running Docker daemon and real Terraria images.
- Playwright is not installed in the current workspace, so E2E browser test execution still requires adding the dependency and browser binaries.

Checks:
- `gofmt -w apps/api/internal/world/service.go apps/api/internal/world/service_test.go apps/api/internal/backup/service.go apps/api/internal/backup/service_test.go apps/api/internal/http/handler.go`: passed
- `go test ./...`: passed
- `go vet ./...`: passed
- `go build ./...`: passed
- `pnpm --filter @gamepanel-lite/web lint`: passed
- `pnpm --filter @gamepanel-lite/web typecheck`: passed
- `pnpm --filter @gamepanel-lite/web build`: passed
- `pnpm test`: passed
- Browser/API verification: pending in this environment because binding the Go API to port 4000 was blocked by sandbox permissions, and the escalation request was rejected by the current usage limit.

## Post-V1 Server Action Dialog Update

Status: Completed

Completed:
- Replaced native browser `confirm`/`alert` flows for server stop, restart, and delete actions with an in-app dark confirmation dialog.
- Added action-specific confirmation copy, cancel/close controls, Escape and backdrop dismissal, and disabled controls while an action is running.
- Kept server start as a direct action and moved action failures into inline panel-styled error text instead of browser alerts.
- Added Chinese and English i18n strings for the new confirmation dialog and working state.

Known issues:
- The destructive stop/restart/delete action itself was not executed during UI verification to avoid changing a running local server without explicit confirmation.
- Browser validation was partially blocked after production build cleanup because the local Next.js dev server needed a restart, and the desktop escalation request to stop the stale 3004 process was rejected by the environment usage limit. The code-level checks and production build passed.
- The repository still has unrelated local API/runtime/settings changes in the working tree; they were left untouched.

Checks:
- `pnpm lint`: passed
- `pnpm typecheck`: passed
- `pnpm test`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go build ./...`: passed with a non-fatal sandbox warning when Go attempted to write its global module stat cache.
- `pnpm build`: passed

## Post-V1 Server Detail Tabs Layout Update

Status: Completed

Completed:
- Fixed the server detail tab bar so active/focused tab styling is not clipped by the horizontal scroll container.
- Added safe top/bottom padding around the tab rail.
- Changed the focus ring to render inset and added an explicit active border/shadow, so Chinese tab labels display fully without visual cropping.

Known issues:
- The tab rail still scrolls horizontally on narrow widths by design; this keeps all V1 tabs accessible without wrapping into multiple rows.

Checks:
- `pnpm lint`: passed
- `pnpm typecheck`: passed
- `pnpm test`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go build ./...`: passed with a non-fatal sandbox warning when Go attempted to write its global module stat cache.
- `pnpm build`: passed after clearing stale `.next` build output from a previous dev/build conflict.

## Post-V1 Server Detail Information Deduplication Update

Status: Completed

Completed:
- Reduced repeated information on the server detail page.
- Changed the page header summary to show the world name instead of repeating player, port, and version metrics already available elsewhere.
- Changed the Overview tab to focus on resource entry points (worlds, backups, mods) instead of repeating connection and configuration values.
- Kept connection values in the Join Server card and moved version into Server Info with the rest of the configuration details.

Known issues:
- The Overview tab remains intentionally lightweight; detailed management is still split into the dedicated Console, Logs, Config, Worlds, Backups, and Mods tabs.

Checks:
- `pnpm lint`: passed
- `pnpm typecheck`: passed
- `pnpm test`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go build ./...`: passed with a non-fatal sandbox warning when Go attempted to write its global module stat cache.
- `pnpm build`: passed

## Post-V1 Create Wizard Upload Execution Update

Status: Completed

Completed:
- Changed the Create Server wizard World / Mods step to retain the selected `File` objects, not just display file names.
- After creating a server, the wizard now imports the selected `.wld` world file into that new server.
- For tModLoader servers, the wizard now uploads selected `.tmod`, `install.txt`, or `enabled.json` files into the new server.
- The wizard invalidates server, world, and mod query caches before redirecting to the new server detail page.
- Updated the upload note copy to reflect that imports/uploads happen automatically after creation.
- Added Playwright coverage proving create-server, world import, and mod upload requests all fire from the wizard flow.

Checks:
- `pnpm lint`: passed
- `pnpm typecheck`: passed
- `pnpm test`: passed
- `pnpm e2e`: passed
- `pnpm build`: passed
- `go test ./...`: passed
- `go vet ./...`: passed

## Post-V1 Server Game Art Update

Status: Completed

Completed:
- Replaced the blank gradient server-card thumbnail with a reusable `ServerGameArt` component.
- Server cards now show the existing Terraria cover image for V1 Terraria servers.
- tModLoader servers receive a small purple modded badge overlay, while vanilla servers keep the clean Terraria cover.
- Kept the mapping in the frontend presentation layer so future game support can extend the visual map without changing runtime logic.

Known issues:
- Browser visual verification was limited because the current Chrome tab intermittently stayed on a stale route while the Next dev server returned 200 responses. The implementation was still validated by lint, typecheck, tests, and production build.
- The frontend could not load real server data during the visual pass until the Go API was restarted; after restart, Chrome still showed stale dashboard content in that tab. No backend code was changed for this update.

Checks:
- `pnpm lint`: passed
- `pnpm typecheck`: passed
- `pnpm test`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go build ./...`: passed with a non-fatal sandbox warning when Go attempted to write its global module stat cache.
- `pnpm build`: passed

## V1 HTTP Integration Coverage Update

Status: Completed

Completed:
- Added HTTP integration coverage for `GET /api/settings` and `PUT /api/settings` using the real chi router, temporary SQLite store, provider registry, Docker monitor, and mock runtime adapter.
- Added HTTP integration coverage for `POST /api/worlds/{id}/migrate`, proving the route creates a target-server world record and copies the `.wld` file into the target instance directory.
- Added HTTP integration coverage for `POST /api/backups/{id}/migrate`, proving the route creates a target-server backup record and copies the backup archive into the target instance directory.

Known issues:
- Playwright remains unavailable in the current workspace; browser E2E still needs dependency/browser installation.
- Real Docker image pulls, container lifecycle, SSE against real container logs, and Terraria client join remain manual verification items for a Docker-enabled environment.

Checks:
- `gofmt -w apps/api/internal/http/handler_test.go`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./apps/api/internal/http ./apps/api/internal/world ./apps/api/internal/backup`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go build ./...`: passed with a non-fatal sandbox warning when Go attempted to write its global module stat cache.
- `pnpm --filter @gamepanel-lite/web lint`: passed
- `pnpm --filter @gamepanel-lite/web typecheck`: passed
- `pnpm --filter @gamepanel-lite/web build`: passed
- `pnpm test`: passed

## V1 Server Runtime HTTP Smoke Update

Status: Completed

Completed:
- Added HTTP smoke coverage for `POST /api/servers`, `POST /api/servers/{id}/start`, `POST /api/servers/{id}/restart`, `POST /api/servers/{id}/stop`, `GET /api/servers/{id}/logs`, and `DELETE /api/servers/{id}`.
- The server lifecycle smoke test uses the real chi router and mock runtime adapter, proving the V1 server APIs create records, transition statuses, return SSE log events, and delete records through the HTTP layer.
- Added HTTP smoke coverage for tModLoader mod upload/list/delete using multipart `.tmod` upload and the real mod storage path.

Known issues:
- These smoke tests prove the API layer and mock runtime contract. Real Docker image pulls, real container logs, and Terraria client join still require a Docker-enabled manual verification environment.
- Playwright remains unavailable in the current workspace; browser E2E still needs dependency/browser installation.

Checks:
- `gofmt -w apps/api/internal/http/handler_test.go`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./apps/api/internal/http`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go build ./...`: passed with a non-fatal sandbox warning when Go attempted to write its global module stat cache.
- `pnpm --filter @gamepanel-lite/web lint`: passed
- `pnpm --filter @gamepanel-lite/web typecheck`: passed
- `pnpm --filter @gamepanel-lite/web build`: passed
- `pnpm test`: passed

## V1 World And Backup HTTP Smoke Update

Status: Completed

Completed:
- Added HTTP smoke coverage for `POST /api/worlds/import`, `GET /api/worlds`, `GET /api/worlds/{id}/download`, `POST /api/worlds/{id}/duplicate`, and `DELETE /api/worlds/{id}`.
- The world smoke test uses multipart `.wld` upload, verifies the stored file, downloads the file content, duplicates it, and verifies deletion removes the copied file.
- Added HTTP smoke coverage for `POST /api/servers/{id}/backups`, `GET /api/backups`, `GET /api/backups/{id}/download`, `POST /api/backups/{id}/restore`, and `DELETE /api/backups/{id}`.
- The backup smoke test creates a stopped server with data, creates an archive, verifies listing and download, mutates server data, restores the backup, and verifies deletion removes the archive.

Known issues:
- HTTP smoke coverage still uses the mock runtime adapter where Docker is involved. Real Docker image pulls, container lifecycle, real container logs, and Terraria client join remain manual verification items.
- Playwright remains unavailable in the current workspace; browser E2E still needs dependency/browser installation and test specs before it can be reported as complete.
- `go build ./...` exits successfully, but the sandbox still reports a non-fatal warning when Go tries to write the global module stat cache outside the workspace.

Checks:
- `gofmt -w apps/api/internal/http/handler_test.go`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./apps/api/internal/http`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go build ./...`: passed with a non-fatal sandbox warning when Go attempted to write its global module stat cache.
- `pnpm --filter @gamepanel-lite/web lint`: passed
- `pnpm --filter @gamepanel-lite/web typecheck`: passed
- `pnpm --filter @gamepanel-lite/web build`: passed
- `pnpm test`: passed

## V1 Playwright And Real Docker Verification Update

Status: Partially Completed

Completed:
- Added `@playwright/test`, root `pnpm e2e` script, `playwright.config.ts`, and `e2e/v1-smoke.spec.ts`.
- Playwright smoke coverage verifies the Chinese app shell, Terraria official cover rendering, user avatar rendering, Docker scan feedback, and create-server wizard mode/preset selected states.
- Fixed Docker image pulls by reading and closing the pull stream before `ContainerCreate`.
- Fixed real Docker SSE logs by demultiplexing Docker stdout/stderr streams before scanning and emitting SSE lines.
- Added provider-owned runtime options so Terraria providers can supply image-specific env, command args, mounts, and small data files without moving Terraria logic into `RuntimeAdapter`.
- Verified real Vanilla Terraria on OrbStack Docker through the Go API:
  - `GET /healthz`: returned 200.
  - `GET /api/runtime/docker`: returned available with the OrbStack Docker socket.
  - `POST /api/servers`: pulled/created `ryshe/terraria:latest` and returned a non-empty container ID.
  - `POST /api/servers/{id}/start`: returned running.
  - Docker logs showed world generation, `Listening on port 17777`, and `Server started`.
  - `GET /api/servers/{id}/logs`: returned clean text/event-stream log lines without Docker multiplex control bytes.
  - `nc -vz 127.0.0.1 17777`: succeeded.
  - `DELETE /api/servers/{id}` removed the temporary container; follow-up `docker ps -a` showed no leftover `gamepanel-` container.
- Verified real tModLoader image pull/create/start path enough to prove V1 config propagation:
  - `POST /api/servers` created `jacobsmile/tmodloader1.4:latest` with a non-empty container ID.
  - `POST /api/servers/{id}/start` returned running.
  - Container logs showed `TMOD_WORLDNAME=V1TmodWorld`, `TMOD_MAXPLAYERS=4`, `TMOD_WORLDSIZE=1`, `TMOD_SECURE=1`, and automatic world creation intent.
  - Temporary tModLoader container was deleted after verification.

Known issues:
- tModLoader real listening remains incomplete in the local arm64/OrbStack environment. The `jacobsmile/tmodloader1.4:latest` amd64 image accepted the generated env config and downloaded .NET, then exited before opening the configured TCP port.
- Actual Terraria client join still requires manual verification with a Terraria desktop client.
- `go build ./...` exits successfully, but this sandbox still reports a non-fatal warning when Go attempts to write its global module stat cache outside the workspace.
- Running `next build` concurrently with Playwright's dev server can corrupt `.next`; rerun production build after E2E or keep those commands sequential.

Checks:
- `pnpm e2e`: passed, 2 Playwright tests.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go build ./...`: passed with a non-fatal sandbox warning when Go attempted to write its global module stat cache.
- `pnpm --filter @gamepanel-lite/web lint`: passed.
- `pnpm --filter @gamepanel-lite/web typecheck`: passed.
- `pnpm --filter @gamepanel-lite/web build`: passed after rerunning sequentially after E2E.
- `pnpm test`: passed.

## V1 tModLoader Real Docker Listening Update

Status: Completed

Completed:
- Investigated the `radioactivehydra/tmodloader:latest` image and confirmed it ships an arm64-capable tModLoader runtime with the official `start-tModLoaderServer.sh` script.
- Verified the root cause of the previous tModLoader hang: the old env-only `jacobsmile/tmodloader1.4:latest` path applied values but did not produce a listening server in the local arm64/OrbStack environment.
- Switched the tModLoader provider to `radioactivehydra/tmodloader:latest`.
- Changed tModLoader runtime options to write a provider-owned `/data/serverconfig.txt` with `/data/Worlds` persistence and start the image with `-config /data/serverconfig.txt`, avoiding the interactive `Choose World` prompt.
- Added provider test coverage proving the tModLoader image, command, and generated runtime config are non-interactive.
- Verified direct Docker smoke:
  - `docker run -d --name gamepanel-tmod-config-smoke -p 17784:7777 -v /private/tmp/gamepanel-tmod-smoke:/data radioactivehydra/tmodloader:latest sh -c '${TMOD_HOMEDIR}/start-tModLoaderServer.sh -config /data/serverconfig.txt'`.
  - Logs showed automatic world generation, `Listening on port 7777`, and `Server started`.
  - `nc -vz 127.0.0.1 17784` succeeded.
  - Temporary container was removed.
- Verified full Go API and Docker RuntimeAdapter path:
  - Temporary API started on port `4012` with OrbStack Docker host.
  - `GET /healthz`: returned 200.
  - `GET /api/runtime/docker`: returned available with `unix:///Users/pengwu/.orbstack/run/docker.sock`.
  - `POST /api/servers`: created a tModLoader server with a non-empty container ID.
  - `POST /api/servers/{id}/start`: returned running.
  - Docker logs showed automatic world generation, `Listening on port 17785`, and `Server started`.
  - `GET /api/servers/{id}/logs`: returned clean text/event-stream log lines including `Listening on port 17785`.
  - `nc -vz 127.0.0.1 17785`: succeeded.
  - `DELETE /api/servers/{id}` removed the temporary server and container; follow-up `docker ps -a` showed no leftover `gamepanel-` container.

Known issues:
- Actual Terraria client join still requires manual verification with a Terraria desktop client.
- Remaining UI polish includes replacing browser-native confirmations on world, backup, and mod management pages and expanding Playwright coverage for the live API flows.

Checks:
- `gofmt -w apps/api/internal/provider/terraria/provider.go apps/api/internal/provider/terraria/config_test.go`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./apps/api/internal/provider/terraria`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go build ./...`: passed with a non-fatal sandbox warning when Go attempted to write its global module stat cache.
- `pnpm --filter @gamepanel-lite/web lint`: passed.
- `pnpm --filter @gamepanel-lite/web typecheck`: passed.
- `pnpm --filter @gamepanel-lite/web build`: passed.
- `pnpm test`: passed.
- `pnpm e2e`: passed, 2 Playwright tests.

## V1 Sidebar Navigation And E2E Stability Update

Status: Completed

Completed:
- Removed the sidebar optimistic active-path state after Playwright showed it could make the selected nav item change before the route actually changed.
- Sidebar active styling now follows the real `pathname`, so the UI no longer suggests a page switch until navigation actually completes.
- Set Playwright smoke tests to one worker and disabled dev server reuse so E2E does not accidentally run against a stale Next.js dev server.
- Increased the Settings URL assertion timeout to account for first-time Next.js route compilation in local dev mode.

Checks:
- `pnpm exec playwright test e2e/v1-smoke.spec.ts:70 --workers=1`: passed.
- `pnpm e2e`: passed, 2 Playwright tests.
- `pnpm --filter @gamepanel-lite/web lint`: passed.
- `pnpm --filter @gamepanel-lite/web typecheck`: passed.
- `pnpm test`: passed.
- `pnpm --filter @gamepanel-lite/web build`: passed.

## V1 Server Detail Resource Panels Update

Status: Completed

Completed:
- Connected the Server Detail page resource panels to real API-backed world, backup, and tModLoader mod queries.
- Server Detail now filters recent worlds and backups for the current server instance and shows recent mod files for tModLoader servers.
- Added compact empty, error, and management-link states for Worlds, Backups, and Mods instead of static placeholder copy.
- Kept the main console/log area focused on SSE logs and left command submission disabled because V1 has no backend command endpoint.

Checks:
- `pnpm --filter @gamepanel-lite/web lint`: passed.
- `pnpm --filter @gamepanel-lite/web typecheck`: passed.
- `pnpm test`: passed.
- `pnpm --filter @gamepanel-lite/web build`: passed.
- `pnpm e2e`: passed, 2 Playwright tests.

## V1 Management Confirmation Dialog Update

Status: Completed

Completed:
- Added a reusable dark `ConfirmDialog` component for management pages.
- Replaced browser-native `confirm` and `alert` flows on Worlds, Backups, and Mods with in-app confirmation dialogs and inline error feedback.
- World deletion, backup restore, backup deletion, and mod deletion now use consistent modal confirmation with Escape/backdrop dismissal.
- Upload, duplicate, migrate, restore, delete, and mod upload errors now render as panel-styled page feedback instead of browser alerts.
- Removed the README roadmap item for replacing remaining browser-native confirmations.

Checks:
- `rg -n "window\\.confirm|window\\.alert|confirm\\(|alert\\(" apps/web`: no matches.
- `pnpm --filter @gamepanel-lite/web typecheck`: passed.
- `pnpm --filter @gamepanel-lite/web lint`: passed.
- `pnpm test`: passed.
- `pnpm --filter @gamepanel-lite/web build`: passed.
- `pnpm e2e`: passed, 2 Playwright tests.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go build ./...`: passed with a non-fatal sandbox warning when Go attempted to write its global module stat cache.

## V1 Expanded Playwright Flow Coverage Update

Status: Completed

Completed:
- Expanded Playwright API mocks to include server detail, SSE log events, worlds, backups, mods, world migration, backup restore, and backup migration responses.
- Added a server detail E2E flow that verifies live log rendering, recent world/backup/mod panels, copy join info, and clipboard content.
- Added management flow coverage for world migration and backup restore confirmation, including assertions that the expected POST requests are made.
- Updated README to describe the broader E2E smoke coverage and narrowed the roadmap to optional live-API/Docker E2E coverage.

Checks:
- `pnpm e2e`: passed, 3 Playwright tests.
- `pnpm --filter @gamepanel-lite/web lint`: passed.
- `pnpm --filter @gamepanel-lite/web typecheck`: passed.
- `pnpm test`: passed.
- `pnpm --filter @gamepanel-lite/web build`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go build ./...`: passed with a non-fatal sandbox warning when Go attempted to write its global module stat cache.

## V1 Top-Bar Service Status Update

Status: Completed

Completed:
- Replaced the top-bar Docker status badge with a backend service status badge.
- The top-bar status now checks the Go API `GET /healthz` endpoint and no longer depends on Docker runtime availability.
- Removed the visible `Docker` label from the top bar; Docker status remains available in the Settings Docker Runtime card.
- Updated Settings copy so Docker status request failures are described as Docker status issues, not API availability issues.
- Added Playwright route coverage for `/healthz` and a top-bar online-state assertion.
- Added root `pnpm dev:api` and `pnpm dev:web` scripts so local API/Web startup is explicit.
- Updated README to explain that the top-bar service badge depends on the Go API process.

Known issues:
- The top bar will show unavailable until the Go API is started. Start it with `pnpm dev:api`.

Checks:
- `pnpm --filter @gamepanel-lite/web lint`: passed.
- `pnpm --filter @gamepanel-lite/web typecheck`: passed.
- `pnpm --filter @gamepanel-lite/web build`: passed after clearing a `.next` cache corrupted by an accidental concurrent build/E2E run.
- `pnpm e2e`: passed, 3 Playwright tests.

## V1 Runtime Container Lifecycle And Console Update

Status: In Progress

Completed:
- Changed the server lifecycle model so a GamePanel server record is the persistent backend object and Docker containers are runtime instances.
- `POST /api/servers` now creates the server record and isolated data directory without requiring an immediate Docker image pull/container create.
- `POST /api/servers/{id}/start` and restart now ensure a runtime container exists; if the previous container is missing, the API recreates it from the persisted provider config and the existing server data directory before starting.
- Docker runtime operations now resolve containers by the `gamepanel.instance=<serverId>` label when the stored container ID is stale.
- Docker runtime bind mounts now normalize relative data directories to absolute host paths before calling Docker, avoiding invalid local volume-name errors with the default `./data` directory.
- New Docker containers are created with stdin open for console command support.
- Added `POST /api/servers/{id}/command` to send commands to a running server container.
- Server Detail console input now sends commands to the API and shows a local command echo on success.
- SSE runtime errors are shown as console errors instead of being rendered as normal game log lines.
- Updated OpenAPI, README, and V1 checklist for the runtime-container lifecycle and console command behavior.

Checks:
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./apps/api/internal/http ./apps/api/internal/runtime ./apps/api/internal/runtime/docker`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./apps/api/internal/runtime/docker ./apps/api/internal/http ./apps/api/internal/runtime`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go build ./...`: passed with a non-fatal sandbox warning when Go attempted to write its global module stat cache.
- `pnpm --filter @gamepanel-lite/web lint`: passed.
- `pnpm --filter @gamepanel-lite/web typecheck`: passed.
- `pnpm test`: passed.
- `pnpm --filter @gamepanel-lite/web build`: passed after clearing stale `.next` output.
- `pnpm e2e`: passed, 3 Playwright tests.

## V1 Server Detail Resource Actions Update

Status: Completed

Completed:
- Added real Server Detail world actions for importing, setting the current world, downloading, and deleting current-server worlds.
- Setting the current world now goes through the Go API, requires the server to be stopped, updates the persisted server world/config, rewrites runtime config files, and clears any old container ID so the next start recreates the runtime container against the same data directory.
- Added real Server Detail backup deletion alongside create, restore, and download.
- Changed Server Detail log streaming to connect only while the Console or Logs tab is open, reducing noisy unavailable states when the user is managing other tabs.
- Added frontend API support for `POST /api/worlds/{id}/assign` and fixed imported world mapping so current-server uploads immediately show as attached to the server.
- Allowed `PUT` in the API CORS method list so cross-port Settings updates can complete preflight.
- Added HTTP coverage for assigning a world and verifying server config/container state updates.

Checks:
- `pnpm lint`: passed.
- `pnpm typecheck`: passed.
- `pnpm test`: passed.
- `pnpm build`: passed.
- `go test ./...`: passed.
- `go vet ./...`: passed.

## V1 Server Detail Config Editing Update

Status: Completed

Completed:
- Added `PUT /api/servers/{id}/config` to update persisted Terraria config while the server is stopped.
- Config updates validate through the provider, synchronize server name/world/port/player/password fields, rewrite `serverconfig.txt` and provider runtime files, and clear stale container IDs so the next start recreates the runtime container against the existing data directory.
- Added Server Detail Config tab editing with compact fields, select controls, toggles, reset, save feedback, and live serverconfig preview.
- Running servers show a stopped-required hint and disable config edits to avoid changing a live runtime container underneath the process.
- Updated frontend API client and OpenAPI contract for the config update endpoint.
- Added HTTP coverage proving config updates rewrite runtime config and clear stale container state.

Checks:
- `pnpm lint`: passed.
- `pnpm typecheck`: passed.
- `pnpm test`: passed.
- `pnpm build`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed.

## V1 Top-Bar Search Interaction Update

Status: Completed

Completed:
- Turned the top-bar server search field into a real quick-jump interaction backed by the server API.
- Search results show matching server name, world, port, and mode; clicking a result navigates directly to that server detail page.
- Pressing Enter opens the first match, or falls back to the Servers page with the search term applied.
- The Servers page now reads a `search` query parameter on the client and applies it to the existing local filter.
- Removed the unused legacy `ServerDetailView` static component so stale mock-like detail UI cannot be accidentally reintroduced.

Checks:
- `pnpm typecheck`: passed.
- `pnpm lint`: passed.
- `pnpm test`: passed.
- `pnpm build`: passed after replacing `useSearchParams` with a client-side URL read to avoid a Next.js Suspense build error.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed.

## V1 Runtime Status Synchronization Update

Status: Completed

Completed:
- `GET /api/servers` and `GET /api/servers/{id}` now refresh stored server status from the runtime adapter when a server has a container ID.
- Runtime status refresh persists changes back to SQLite so list and detail views converge on the actual container state.
- Refresh failures are logged but do not make list/detail reads fail, keeping the UI usable if Docker is temporarily unavailable.
- Servers list and server detail pages now refetch every 5 seconds so start/stop/restart or external container state changes become visible without a manual refresh.
- Added HTTP coverage proving server list and detail responses synchronize stale database status from runtime `Inspect`.

Checks:
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed.
- `pnpm typecheck`: passed.
- `pnpm lint`: passed.
- `pnpm test`: passed.
- `pnpm build`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed.

## V1 Server Detail Action Feedback Update

Status: Completed

Completed:
- Added consistent success feedback on the server detail page for console commands, world import/assignment/deletion, backup creation/restoration/deletion, and mod upload/deletion.
- Added clipboard failure handling for the join info copy controls instead of letting failed browser clipboard writes look like no-op clicks.
- Added copied-state feedback to the shared server action copy-invite button and included the server password in the copied invite text when configured.
- Added Chinese and English localization for the new detail action feedback messages.

Checks:
- `pnpm test`: passed.
- `pnpm typecheck`: passed after fixing the browser timer ref type.
- `pnpm lint`: passed.
- `pnpm build`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed.

## V1 Server Lifecycle UI Completion Update

Status: Completed

Completed:
- Exposed the existing server delete API through the shared server action UI with the existing in-app destructive confirmation dialog.
- Server start, stop, and restart now consume the returned API server payload and immediately update the server detail query cache instead of waiting for the next poll.
- Server lifecycle actions now show inline success feedback for start, stop, restart, and delete.
- Deleting a server from its detail page now returns the user to the Servers page after the API call succeeds.
- Added Chinese and English localization for lifecycle success messages.

Checks:
- `pnpm typecheck`: passed.
- `pnpm lint`: passed.
- `pnpm test`: passed.
- `pnpm build`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed.

## V1 Resource Page Feedback Update

Status: Completed

Completed:
- Added success feedback to the Worlds page for import, duplicate, migrate, and delete actions.
- Added success feedback to the Backups page for create, restore, migrate, and delete actions.
- Added success feedback to the Mods page for upload and delete actions.
- Cleared success messages whenever a resource action fails so users do not see stale positive feedback next to an error.
- Removed stale activity copy that said the V1 backend did not expose activity events after the activity API had already been implemented.

Checks:
- `pnpm typecheck`: passed.
- `pnpm lint`: passed.
- `pnpm test`: passed.
- `pnpm build`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed.

## V1 Create Wizard Config Completion Update

Status: Completed

Completed:
- Expanded the Create Server wizard config step from basic text inputs to the full V1 Terraria configuration surface.
- Added world size and difficulty selectors during server creation.
- Added numeric constraints for port and max players during server creation.
- Added secure mode and auto-create-world toggles during server creation.
- Replaced placeholder-only config inputs with labeled fields for clearer Chinese and English UI.

Checks:
- `pnpm typecheck`: passed.
- `pnpm lint`: passed.
- `pnpm test`: passed.
- `pnpm build`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed.

## V1 Dashboard Metric Accuracy Update

Status: Completed

Completed:
- Preserved raw backup byte counts in the frontend API mapper so dashboard aggregate metrics can use real data.
- Added API mapper unit coverage proving backup `sizeBytes` is retained from the Go API response.
- Changed the dashboard player capacity metric from a hardcoded `32` to the sum of configured server `maxPlayers`.
- Changed dashboard storage usage from latest-backup size to total backup storage across all loaded backups.
- Updated storage hint copy to show the number of backups represented by the metric.

Checks:
- `pnpm --filter @gamepanel-lite/web test`: failed first as expected because `sizeBytes` was missing, then passed after the mapper fix.
- `pnpm typecheck`: passed.
- `pnpm lint`: passed.
- `pnpm test`: passed.
- `pnpm build`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed.

## V1 Server Card Backup Accuracy Update

Status: Completed

Completed:
- Preserved raw backup creation timestamps in the frontend API mapper.
- Added a shared server metrics helper that attaches the newest backup time to each server card from real Backup API data.
- Added unit coverage proving the newest backup is selected by timestamp, not by implicit API order.
- Servers page now loads backups and shows real last-backup information on each server card.
- Dashboard active-server cards now use the same backup-enriched server data as the Servers page.

Checks:
- `pnpm --filter @gamepanel-lite/web test`: failed first because the helper was missing, then failed again when it relied on implicit backup order, then passed after comparing `createdAt`.
- `pnpm typecheck`: passed.
- `pnpm lint`: passed.
- `pnpm test`: passed.
- `pnpm build`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed.

## V1 Download Interaction Completion Update

Status: Completed

Completed:
- Replaced direct world and backup download links with fetch-based downloads so failed downloads stay on the current page.
- Added inline download success and error feedback on the server detail, Worlds, and Backups pages.
- Added per-resource downloading states to prevent repeated clicks while a download request is in flight.
- Preserved backend error messages from failed download endpoints instead of navigating users to a raw error response page.
- Added API helper coverage proving download failures surface the backend error.

Checks:
- `pnpm --filter @gamepanel-lite/web test -- api.test.ts`: failed first because the download helper did not exist, then passed after implementation.
- `pnpm typecheck`: passed.
- `pnpm lint`: passed.
- `pnpm test`: passed.
- `pnpm build`: passed with the existing Next.js ESLint plugin warning.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed.

## V1 Server Runtime Recovery Update

Status: Completed

Completed:
- Updated the command endpoint to recover a missing or stale runtime container for running servers before sending commands.
- Updated the server log SSE endpoint to recover and start a recreated runtime container before streaming logs for running servers.
- Reused the existing provider/runtime creation path so recovered containers mount the same isolated instance data directory and keep existing world/config data.
- Added backend coverage for the stale-container case that previously made server detail logs and console commands fail with Docker errors.

Checks:
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./apps/api/internal/http`: failed first because commands used the stale container ID, then passed after runtime recovery was added.
- `pnpm typecheck`: passed.
- `pnpm lint`: passed.
- `pnpm test`: passed.
- `pnpm build`: passed with the existing Next.js ESLint plugin warning.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed.
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed.
