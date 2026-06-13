# GamePanel Lite V1 Architecture

## Overview

GamePanel Lite is a monorepo with a Go backend and Next.js frontend.

## Backend

The backend lives under `apps/api` and uses:
- Go
- chi router
- SQLite via GORM
- Docker SDK for Go
- `slog`
- SSE for logs

Key packages:
- `internal/domain`: core models.
- `internal/provider`: provider interface and registry.
- `internal/provider/terraria`: Vanilla and tModLoader provider logic.
- `internal/runtime`: runtime adapter interface and mock adapter.
- `internal/runtime/docker`: Docker SDK implementation.
- `internal/store`: SQLite persistence.
- `internal/world`, `internal/backup`, `internal/mod`: file-domain services.
- `internal/safety`: path and file-name validation.

## Frontend

The frontend lives under `apps/web` and uses:
- Next.js App Router
- React
- TypeScript
- Tailwind CSS
- lucide-react
- TanStack Query
- Framer Motion for subtle wizard transitions

The UI is a product dashboard, not a landing page. API-backed pages use mock fallback when the Go API is unavailable.

## Data Layout

Configured by `GAMEPANEL_DATA_DIR`:
- `instances/{instanceId}` for runtime server data.
- `worlds/{instanceId}` for imported `.wld` files.
- `backups/{instanceId}` for zip backups.
- `mods/{instanceId}` for tModLoader files.

## Extension Model

Future games should add new `GameProvider` implementations. Runtime behavior remains behind `RuntimeAdapter`; providers render game-specific config and supply runtime specs.
