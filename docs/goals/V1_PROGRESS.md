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
- Updated server detail copy buttons to write to the clipboard and disabled console command input because V1 only exposes SSE logs, not command submission.

Known issues:
- Server detail logs remain mock-rendered in the UI even though the backend exposes an SSE logs endpoint.
- Console command submission is intentionally not implemented because there is no V1 backend command endpoint.

Checks:
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go test ./...`: passed
- `GOCACHE=/Users/pengwu/Desktop/Projects/go-project/game-panel-lite/.cache/go-build go vet ./...`: passed
- `pnpm lint`: passed
- `pnpm typecheck`: passed
- `pnpm test`: passed
- `pnpm build`: passed after stopping an old Next dev process that was concurrently writing `.next`.
- Browser smoke: Dashboard, Servers, Worlds, Backups, and Mods rendered with dark styles loaded; Servers status filter interaction passed.

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
