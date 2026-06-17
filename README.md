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
pnpm dev:api
```

Run the web app:

```bash
pnpm dev:web
```

The top-bar service badge checks the Go API `GET /healthz` endpoint. If only the web app is running, the badge will show unavailable until `pnpm dev:api` is started.

Run both services with Docker Compose:

```bash
docker compose up
```

For OrbStack on macOS, keep the machine-specific socket path in your untracked `.env`:

```bash
GAMEPANEL_DOCKER_SOCKET_PATH="/Users/<you>/.orbstack/run/docker.sock"
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
- `apps/web` - Next.js, React, TypeScript, Tailwind CSS frontend with local shadcn/ui-style source components.
- `packages/contracts` - OpenAPI contract.
- `packages/shared` - shared TypeScript schemas used by the frontend.

Default local ports:

- Web app: `http://localhost:3000`
- API: `http://localhost:4000`

## V1 Scope

V1 is intentionally limited to Terraria. It includes provider boundaries for Vanilla and tModLoader, SQLite persistence, server configuration rendering, isolated instance data directories, and a Docker runtime adapter boundary.

Do not add auth, billing, cloud provisioning, OAuth, RBAC, Kubernetes, or plugin marketplace features in V1.

## V1 Usage

1. Start the API with `pnpm dev:api`.
2. Start the web UI with `pnpm dev:web`.
3. Open `http://localhost:3000/dashboard`.
4. Use **Create Server** to choose Terraria, Vanilla or tModLoader, a preset, and config values.
5. Use **Preview serverconfig.txt** to render the Go backend config output.
6. Start a server from the Servers page. The API will reuse an existing runtime container when it is present, or create a new container from the persisted server config and data directory when it is missing.
7. Use Worlds to import `.wld` files, Backups to manage zip backups, and Mods for tModLoader `.tmod` files, mod packs, and Workshop ID imports.

## Docker Runtime

The API exposes `GET /api/runtime/docker` for daemon status. Game server records are persisted in SQLite; Docker containers are runtime instances. Starting a server creates or reuses a container mounted to that server's isolated data directory, so a missing old container can be recreated without losing world/config data. Real container creation requires Docker to be running and access to the configured Terraria images:

- Vanilla: `smartcat99999/terraria-vanilla:<version>` selected from the provider version list.
- tModLoader: `smartcat99999/tmodloader:<version>` selected from the provider version list.

Build the supported game images with:

```bash
scripts/build-game-images.sh
```

For local arm64 testing:

```bash
scripts/build-game-images.sh all --platform linux/arm64 --load
```

For multi-platform registry publishing:

```bash
scripts/build-game-images.sh all --platform linux/amd64,linux/arm64 --push
```

Configure the Docker socket or host with `GAMEPANEL_DOCKER_HOST`. If it is not set, the API falls back to `DOCKER_HOST`, then `unix:///var/run/docker.sock`.

Common examples:

```bash
GAMEPANEL_DOCKER_HOST="unix:///var/run/docker.sock"
GAMEPANEL_DOCKER_HOST="unix:///Users/<you>/.orbstack/run/docker.sock"
GAMEPANEL_DOCKER_HOST="tcp://127.0.0.1:2375"
```

The Settings page displays the configured Docker Host and runtime status only. Docker Host changes should be made in the backend config or environment, then applied by restarting the backend process.

When using `compose.yaml`, the API service always reads Docker through `unix:///var/run/docker.sock` inside the API container. Override `GAMEPANEL_DOCKER_SOCKET_PATH` in local `.env` to decide which host socket is mounted there.

Each server instance uses an isolated directory under `GAMEPANEL_DATA_DIR/instances/{instanceId}`. World, backup, and mod files use separate per-instance directories.

## Safety

- Uploaded world files must end in `.wld`.
- Uploaded mod files use the `.tmod` workflow; Workshop ID imports generate `install.txt` for tModLoader.
- File names, joined paths, and restored backup archive entries are checked to prevent path traversal.
- Stop, restart, and delete server actions require an in-app confirmation before the API call.
- Secrets, tokens, and machine-specific absolute paths must stay out of committed config.
- Keep machine-specific Docker socket paths in local `.env` or shell environment only.

## Known Limitations

- Backup restore extracts archives into the server data directory and refuses to run while a server is running or restarting.
- Server detail pages stream logs from the backend SSE endpoint when the server/container log stream is available.
- Server detail console input sends commands to the running container stdin.
- Vanilla Terraria was verified against a real OrbStack Docker daemon: image pull, create, start, auto-create world, clean SSE logs, TCP port probe, and delete cleanup.
- tModLoader was verified against a real OrbStack Docker daemon with versioned `radioactivehydra/tmodloader` tags: image pull, create, start, auto-create world from `/data/serverconfig.txt`, clean SSE logs, TCP port probe, and delete cleanup.
- Playwright E2E smoke tests cover the Chinese app shell, Docker scan feedback, game cover/avatar rendering, create-server selection states, server detail logs, copy join info, world migration, and backup restore confirmation.
- Actual Terraria client join still needs manual verification with the desktop game client; see `docs/goals/V1_MANUAL_VERIFICATION.md`.
- World and backup migration APIs are implemented for copying assets between server instances.

## Roadmap

- Add an optional Playwright suite that runs against a live local Go API and Docker daemon instead of mocked API responses.
- Add richer live log search, filtering, and history.
