# GamePanel Lite V1 Master Goal

## Product

GamePanel Lite is a modern lightweight self-hosted game server management panel.

V1 focuses only on Terraria, including:

- Terraria Vanilla server
- Terraria tModLoader server
- Multiple server instances
- Custom server configuration
- Quick presets
- Server start / stop / restart
- Logs and console
- World import / backup / restore / migration
- Easy join information display: IP, port, password
- Modern young gamer-friendly UI

GitHub repo: smartcat999/game-panel-lite  
Local path: /Users/pengwu/Desktop/Projects/go-project/game-panel-lite

## Tech Stack

Frontend:

- Next.js
- React
- TypeScript
- Tailwind CSS
- shadcn/ui
- lucide-react
- TanStack Query
- Framer Motion for subtle transitions only

Backend:

- Golang
- chi router
- SQLite
- GORM or sqlc
- Docker SDK for Go
- Server-Sent Events for logs/status streaming
- Go standard slog logger
- go-playground/validator
- OpenAPI contract for frontend/backend API typing

Runtime:

- Docker
- One game server instance maps to one container
- Each server instance has isolated data directory

Testing:

- Go unit tests for backend
- Vitest for frontend utilities
- Playwright for UI flows

## Architecture

Use a modular backend architecture:

- GameProvider interface
- TerrariaVanillaProvider
- TerrariaTModLoaderProvider
- RuntimeAdapter interface
- DockerRuntimeAdapter
- BackupService
- WorldService
- ConfigRenderer
- LogStreamService

Future games should be added through new providers, not by rewriting runtime or UI.

## Backend Directory

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

## Frontend Directory

apps/web/
  app/
    dashboard/
    servers/
    worlds/
    backups/
    settings/
  components/
  lib/
  hooks/
  api/

## V1 Pages

1. Dashboard
2. Servers
3. Create Server Wizard
4. Server Detail
5. Worlds
6. Backups
7. Settings

## Core Backend APIs

Servers:

- GET /api/servers
- POST /api/servers
- GET /api/servers/{id}
- POST /api/servers/{id}/start
- POST /api/servers/{id}/stop
- POST /api/servers/{id}/restart
- DELETE /api/servers/{id}
- GET /api/servers/{id}/logs

Worlds:

- GET /api/worlds
- POST /api/worlds/import
- POST /api/worlds/{id}/assign
- GET /api/worlds/{id}/download
- POST /api/worlds/{id}/duplicate
- DELETE /api/worlds/{id}

Backups:

- GET /api/backups
- POST /api/servers/{id}/backups
- POST /api/backups/{id}/restore
- GET /api/backups/{id}/download
- DELETE /api/backups/{id}

Mods:

- GET /api/servers/{id}/mods
- POST /api/servers/{id}/mods/upload
- DELETE /api/servers/{id}/mods/{modId}

Settings:

- GET /api/runtime/docker
- GET /api/settings
- PUT /api/settings

## Acceptance Criteria

V1 is complete when:

1. User can create a Terraria Vanilla server from UI.
2. User can create a Terraria tModLoader server from UI.
3. User can start / stop / restart a server.
4. User can view server detail.
5. User can view logs through SSE.
6. User can copy join info.
7. User can import a .wld world.
8. User can create a backup.
9. User can restore a backup.
10. User can manage multiple servers with different ports.
11. UI matches the provided modern dark GamePanel Lite style.
12. Backend is written in Go.
13. Code is modular enough to add future game providers.

## Out of Scope for V1

Do not implement:

- cloud provider provisioning
- billing
- multi-tenant SaaS
- public user accounts
- OAuth login
- Kubernetes deployment
- all Steam games
- mobile app
- plugin marketplace
