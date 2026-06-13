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
