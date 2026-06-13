# Goal 1: Monorepo Foundation

This goal initializes the GamePanel Lite codebase as a pnpm workspace monorepo.

## Included

- Next.js frontend shell in `apps/web`.
- Fastify API shell in `apps/api`.
- Shared TypeScript types and Zod schemas in `packages/shared`.
- Terraria presets and typed `serverconfig.txt` rendering in shared code.
- Prisma + SQLite schema for V1 Terraria server management entities.
- Core backend interfaces for providers, runtime control, backups, and worlds.
- `MockRuntimeAdapter` for local development before Docker integration.
- Vitest coverage for shared schemas and provider registry behavior.

## Verification Commands

Goal 1 must pass:

```bash
pnpm install
pnpm lint
pnpm typecheck
pnpm test
pnpm build
```

## Out Of Scope

- Docker runtime integration.
- Authentication.
- Non-Terraria game support.
- Full V1 UI pages and workflows.
