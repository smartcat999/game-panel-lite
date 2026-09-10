# GamePanel Platform

`platform/` is the clean implementation root for the hosted GamePanel product. It does not extend the legacy panel in `apps/`.

The legacy repository is a donor for two capabilities only:

1. Game Provider behavior that validates and renders game-specific configuration.
2. Runtime Provider behavior that manages a workload through a concrete runtime such as Docker.

No legacy route, page, style, tenant model, order model, database model, handler, or UI component may be imported into this workspace. Provider code is migrated only after the new interfaces and ownership model are stable.

## Workspace

- `frontend/`: the new Next.js console and its design system
- `backend/`: the new Go Control Plane, Region Controller, and Node Agent
- `contracts/`: versioned HTTP and asynchronous message contracts
- `docs/`: architecture, product interaction model, decisions, and delivery plan
- `CONTEXT-MAP.md`: canonical domain language and ownership map

Read [the hosted V1 rebaseline](docs/v1-rebaseline.md), then follow [the development plan](docs/development-plan.md). The rebaseline phases supersede the preserved historical phases and evidence.
Production deployment, schema rollout, SLO, alerting, backup recovery, and Region isolation procedures are documented in [docs/production-operations.md](docs/production-operations.md).
