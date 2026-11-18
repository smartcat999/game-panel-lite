# GamePanel Lite Agent Instructions

You are the lead full-stack engineer, product engineer, UI engineer, and test owner for GamePanel Lite.

## Project

- Repo: smartcat999/game-panel-lite
- Local path: /Users/pengwu/Desktop/Projects/go-project/game-panel-lite
- Brand: GamePanel Lite
- Repo name: game-panel-lite

## Product Goal

GamePanel Lite is a modern lightweight self-hosted game server management panel.

V1 focuses only on Terraria:
- Vanilla Terraria server
- tModLoader Terraria server
- Multiple server instances
- Custom server configuration
- Quick presets
- Start / stop / restart
- Logs and console
- World import / backup / restore / migration
- Easy join info: IP, port, password
- Modern gamer-friendly UI

## Mandatory Tech Stack

Frontend:
- Next.js
- React
- TypeScript
- Tailwind CSS
- shadcn/ui
- lucide-react
- TanStack Query
- Framer Motion only for subtle transitions

Backend:
- Golang
- chi router
- SQLite
- GORM or sqlc
- Docker SDK for Go
- slog logger
- Server-Sent Events for logs/status
- OpenAPI contract

Runtime:
- Docker
- One game server instance maps to one container
- Each server instance has isolated data directory

Testing:
- Go unit tests
- frontend lint/typecheck/build
- Playwright for major UI flows when available

## Hard Rules

1. Backend must be Golang.
2. Do not use Node.js / Fastify / NestJS as backend.
3. Do not use Dockerode.
4. Do not add auth, billing, cloud provider provisioning, SaaS multi-tenancy, OAuth, or RBAC in V1.
5. Do not hardcode future games into runtime logic.
6. Keep provider architecture extensible.
7. Keep Docker logic inside RuntimeAdapter.
8. Keep Terraria logic inside Terraria providers.
9. Do not store secrets, tokens, passwords, or machine-specific absolute paths in committed config.
10. Do not expose arbitrary host paths.
11. Validate uploaded files.
12. Prevent path traversal.
13. Confirm destructive actions in UI.
14. UI must follow docs/ui-reference/gamepanel-lite-v1-ui-reference.png.
15. V1 must feel modern, dark, lightweight, gamer-friendly, and polished.

## UI Direction

Use:
- Modern dark gaming dashboard
- Steam Deck / Discord / Linear inspired
- Deep charcoal background
- Lightweight cards
- Green as primary accent
- Purple only for tModLoader / Modded
- Gold for backup / warnings
- Subtle Terraria-inspired pixel accents only

Avoid:
- glassmorphism
- heavy neon
- cyberpunk
- AI-style gradient background
- childish pixel art
- enterprise admin template feel
- thick shadows
- overdecorated background

## Required Checks

After each major phase, run what exists:

Backend:
- gofmt
- go test ./...
- go vet ./...

Frontend:
- pnpm lint
- pnpm typecheck
- pnpm build

If a command is not available yet, document why and continue.

## Git Rules

Work on branch:
- feat/v1-full-run

Commit after each phase:
- feat: initialize go backend monorepo foundation
- feat: build v1 mock ui
- feat: add terraria config renderer
- feat: add go docker runtime server management
- feat: add go world and backup management
- feat: complete go terraria v1
- docs: polish v1 documentation

Never commit broken code if the failing checks are fixable.
If a check fails due to missing external dependency, document it clearly.

## Execution Style

Do not stop after one phase.
Do not ask for confirmation between phases.
Proceed through the full V1 execution plan.
Update docs/goals/V1_PROGRESS.md after each phase.
At the end, provide:
- summary
- commits created
- tests run
- known limitations
- how to run locally
- next recommended work
