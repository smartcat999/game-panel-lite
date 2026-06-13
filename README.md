# GamePanel Lite

GamePanel Lite is a lightweight self-hosted game server management panel. V1 focuses only on Terraria Vanilla and Terraria tModLoader while keeping the provider architecture ready for future Steam games.

## Requirements

- Go 1.25+
- Node.js 20+
- pnpm 9+
- Docker for real server runtime verification

## Local Development

Copy local environment defaults:

```bash
cp .env.example .env
```

Run the Go API:

```bash
go run ./apps/api/cmd/server
```

Run the web app:

```bash
pnpm --filter @gamepanel-lite/web dev
```

Useful checks:

```bash
gofmt -w apps/api
go test ./...
go vet ./...
pnpm lint
pnpm typecheck
pnpm test
pnpm build
```

## Workspace Layout

- `apps/api` - Go, chi, SQLite, provider and runtime backend.
- `apps/web` - Next.js, React, TypeScript, Tailwind CSS frontend.
- `packages/contracts` - OpenAPI contract.
- `packages/shared` - shared TypeScript schemas used by the frontend.

Default local ports:

- Web app: `http://localhost:3000`
- API: `http://localhost:4000`

## V1 Scope

V1 is intentionally limited to Terraria. It includes provider boundaries for Vanilla and tModLoader, SQLite persistence, server configuration rendering, isolated instance data directories, and a Docker runtime adapter boundary.

Do not add auth, billing, cloud provisioning, OAuth, RBAC, Kubernetes, or plugin marketplace features in V1.
