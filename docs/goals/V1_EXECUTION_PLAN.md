# GamePanel Lite V1 Execution Plan

## Overview

Execute all phases continuously in one run.

The backend must be Go.
The frontend must be Next.js.
The runtime must be Docker.
The database must be SQLite.
The UI must follow docs/ui-reference/gamepanel-lite-v1-ui-reference.png.

## Phase 0: Repository and Environment Check

Tasks:
1. Check current repository structure.
2. Create branch feat/v1-full-run if not already on it.
3. Check:
   - git status
   - go version
   - node version
   - pnpm version
   - docker version
4. Create docs/goals/V1_PROGRESS.md.
5. Record detected environment and any missing tools.
6. Continue even if Docker is unavailable, but implement Docker runtime code.

Commit:
- docs: initialize v1 execution progress

## Phase 1: Monorepo Foundation

Tasks:
1. Initialize monorepo.
2. Create apps/web with Next.js + TypeScript + Tailwind.
3. Create apps/api with Go module.
4. Create packages/contracts for OpenAPI.
5. Create backend structure:

apps/api/
  cmd/server/main.go
  internal/
    app/
    config/
    domain/
    http/
    middleware/
    store/
    service/
    provider/
      terraria/
    runtime/
      docker/
    backup/
    world/
    mod/
    logs/
  migrations/
  data/

6. Implement chi HTTP server.
7. Add:
   - GET /healthz
   - GET /api/version
8. Add domain models:
   - GameServerInstance
   - TerrariaConfig
   - Backup
   - World
   - ModFile
   - ActivityEvent
9. Add core interfaces:
   - GameProvider
   - RuntimeAdapter
   - BackupService
   - WorldService
10. Add MockRuntimeAdapter.
11. Add provider registry.
12. Add empty TerrariaVanillaProvider.
13. Add empty TerrariaTModLoaderProvider.
14. Initialize SQLite store.
15. Add initial migration.
16. Add README local development section.
17. Add .env.example.
18. Add basic tests.

Checks:
- gofmt
- go test ./...
- go vet ./...
- pnpm lint
- pnpm typecheck
- pnpm build

Commit:
- feat: initialize go backend monorepo foundation

## Phase 2: V1 Mock UI

Tasks:
1. Build frontend routes:
   - /dashboard
   - /servers
   - /servers/new
   - /servers/[id]
   - /worlds
   - /backups
   - /settings
2. Use mock data only.
3. Build reusable components:
   - AppSidebar
   - TopBar
   - PageHeader
   - StatsCard
   - ActiveServerCard
   - ServerCard
   - ServerStatusBadge
   - ServerModeBadge
   - JoinInfoCard
   - ServerActions
   - CreateServerWizard
   - GameStep
   - ModeStep
   - PresetStep
   - ConfigStep
   - WorldModsStep
   - ReviewStep
   - WorldCard
   - BackupTable
   - RestoreBackupDialog
   - EmptyState
   - ConfirmDialog
   - CopyButton
   - TerminalLog
4. UI must match the reference image direction.
5. Join Server / Copy Invite must be prominent.
6. Server status must be obvious.
7. Create Server Wizard must include:
   - Game
   - Mode
   - Preset
   - Config
   - World / Mods
   - Review
8. Vanilla server hides Mods tab.
9. tModLoader server shows Mods tab.
10. Add polished loading, empty, and error states.

Checks:
- go test ./...
- go vet ./...
- pnpm lint
- pnpm typecheck
- pnpm build

Commit:
- feat: build v1 mock ui

## Phase 3: Terraria Config and Presets

Tasks:
1. Implement Go TerrariaConfig model and validation.
2. Support:
   - ServerName
   - WorldName
   - WorldSize: small / medium / large
   - Difficulty: classic / expert / master / journey
   - MaxPlayers
   - Port
   - Password
   - MOTD
   - Seed
   - Secure
   - Language
   - AutoCreate
3. Implement serverconfig.txt renderer.
4. Implement presets:
   - Friends Casual
   - Expert Adventure
   - Master Challenge
   - Building World
   - Modded Starter
5. Add APIs:
   - GET /api/terraria/presets
   - POST /api/terraria/config/preview
6. Connect Create Server Wizard config preview to API.
7. Update OpenAPI contract.
8. Add tests:
   - preset validation
   - renderer snapshot
   - invalid port
   - invalid max players
   - invalid world name

Checks:
- gofmt
- go test ./...
- go vet ./...
- pnpm lint
- pnpm typecheck
- pnpm build

Commit:
- feat: add terraria config renderer

## Phase 4: Go Docker Runtime and Server Management

Tasks:
1. Implement DockerRuntimeAdapter using Docker SDK for Go.
2. Support:
   - check Docker availability
   - create container
   - start container
   - stop container
   - restart container
   - remove container
   - inspect status
   - stream logs
   - map ports
   - mount instance data directory
3. Implement TerrariaVanillaProvider.
4. Implement server APIs:
   - GET /api/servers
   - POST /api/servers
   - GET /api/servers/{id}
   - POST /api/servers/{id}/start
   - POST /api/servers/{id}/stop
   - POST /api/servers/{id}/restart
   - DELETE /api/servers/{id}
   - GET /api/servers/{id}/logs
5. Logs should use SSE.
6. Replace frontend mock server calls with real API where possible.
7. Settings page shows Docker status.
8. Multiple servers must support different ports.
9. Data dirs must be isolated by instanceId.

Safety:
- Do not put Terraria logic inside DockerRuntimeAdapter.
- Do not put Docker details inside UI.
- Show clear UI errors when Docker is unavailable.

Checks:
- gofmt
- go test ./...
- go vet ./...
- pnpm lint
- pnpm typecheck
- pnpm build

Commit:
- feat: add go docker runtime server management

## Phase 5: Worlds and Backups

Tasks:
1. Implement WorldService:
   - upload .wld
   - list worlds
   - assign world to server
   - download world
   - duplicate world
   - delete world
   - migrate world to another server
2. Implement BackupService:
   - create manual backup
   - list backups
   - restore backup
   - download backup
   - migrate backup
   - delete backup
3. Use:
   - data/worlds/{instanceId}
   - data/backups/{instanceId}
4. Add APIs:
   - GET /api/worlds
   - POST /api/worlds/import
   - POST /api/worlds/{id}/assign
   - GET /api/worlds/{id}/download
   - POST /api/worlds/{id}/duplicate
   - DELETE /api/worlds/{id}
   - GET /api/backups
   - POST /api/servers/{id}/backups
   - POST /api/backups/{id}/restore
   - GET /api/backups/{id}/download
   - DELETE /api/backups/{id}
5. Connect Worlds and Backups pages to API.
6. Connect Server Detail Worlds / Backups tabs to API.
7. Add tests:
   - world upload validation
   - backup create
   - backup restore
   - path traversal prevention

Safety:
- Validate file extensions.
- Prevent path traversal.
- Confirm destructive actions.
- Warn before restore if server is running.

Checks:
- gofmt
- go test ./...
- go vet ./...
- pnpm lint
- pnpm typecheck
- pnpm build

Commit:
- feat: add go world and backup management

## Phase 6: tModLoader

Tasks:
1. Implement TerrariaTModLoaderProvider.
2. Support creating tModLoader server.
3. Support upload:
   - .tmod files
   - install.txt
   - enabled.json
4. Use:
   - data/mods/{instanceId}
5. Add APIs:
   - GET /api/servers/{id}/mods
   - POST /api/servers/{id}/mods/upload
   - DELETE /api/servers/{id}/mods/{modId}
6. Server Detail shows Mods tab for tModLoader only.
7. Vanilla server does not show Mods tab.
8. Modded Starter preset must work.
9. tModLoader badge must use purple style.
10. Add tests:
   - provider registry
   - tModLoader file validation
   - mod upload validation
   - create tModLoader server test

Checks:
- gofmt
- go test ./...
- go vet ./...
- pnpm lint
- pnpm typecheck
- pnpm build

Commit:
- feat: complete go terraria v1

## Phase 7: V1 Polish, Docs, and E2E

Tasks:
1. Add or finalize Playwright tests:
   - open dashboard
   - create Vanilla server
   - create tModLoader server
   - copy join info
   - create backup
   - open server detail logs
2. Complete README:
   - Quick Start
   - Requirements
   - Go backend dev
   - Web frontend dev
   - Docker requirement
   - Terraria Vanilla usage
   - tModLoader usage
   - World import
   - Backup restore
   - Troubleshooting
   - Known limitations
   - Roadmap
3. Add docs:
   - docs/product/product-spec-v1.md
   - docs/architecture/architecture-v1.md
   - docs/goals/V1_CHECKLIST.md
4. Finalize .env.example.
5. Final full check.

Checks:
- gofmt
- go test ./...
- go vet ./...
- pnpm lint
- pnpm typecheck
- pnpm build
- Playwright tests if configured

Commit:
- docs: polish v1 documentation

## Final Output

At the end, report:
1. Summary
2. Commits created
3. How to run locally
4. Test results
5. Known limitations
6. What is actually working
7. What still needs manual verification
