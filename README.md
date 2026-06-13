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

If your shell cannot write to the default Go build cache, use a local cache:

```bash
mkdir -p .cache/go-build
GOCACHE="$PWD/.cache/go-build" go test ./...
GOCACHE="$PWD/.cache/go-build" go vet ./...
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

## V1 Usage

1. Start the API with `go run ./apps/api/cmd/server`.
2. Start the web UI with `pnpm --filter @gamepanel-lite/web dev`.
3. Open `http://localhost:3000/dashboard`.
4. Use **Create Server** to choose Terraria, Vanilla or tModLoader, a preset, and config values.
5. Use **Preview serverconfig.txt** to render the Go backend config output.
6. Use the Servers page to view API-backed servers when the Go backend is running; mock data is shown when it is not.
7. Use Worlds to import `.wld` files, Backups to manage zip backups, and Mods for tModLoader-only `.tmod`, `install.txt`, and `enabled.json` files.

## Docker Runtime

The API exposes `GET /api/runtime/docker` for daemon status. Real container creation requires Docker to be running and access to the configured Terraria images:

- Vanilla: `ryshe/terraria:latest`
- tModLoader: `jacobsmile/tmodloader1.4:latest`

Each server instance uses an isolated directory under `GAMEPANEL_DATA_DIR/instances/{instanceId}`. World, backup, and mod files use separate per-instance directories.

## Safety

- Uploaded world files must end in `.wld`.
- Uploaded mod files must be `.tmod`, `install.txt`, or `enabled.json`.
- File names and joined paths are checked to prevent path traversal.
- UI destructive actions are represented with explicit destructive controls; production confirmation dialogs should be expanded as the API wiring deepens.
- Secrets, tokens, and machine-specific absolute paths must stay out of committed config.

## Known Limitations

- Backup restore currently acknowledges the request but does not extract archives yet; restore should stop running servers before file replacement.
- Individual server detail pages still use mock detail data for console and side panels.
- Docker image pull and container lifecycle were compiled but not manually verified against a running daemon in this run.
- Playwright is not configured in this project yet, so e2e browser flows were not run.

## Roadmap

- Wire create-server form state fully to `POST /api/servers`.
- Add restore extraction with running-server guardrails.
- Add Playwright smoke tests for dashboard, create server, copy join info, backup, and logs.
- Add richer live log and command console behavior.
