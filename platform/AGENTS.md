# GamePanel Hosted Platform Instructions

These instructions apply to every file under `platform/` and define a new hosted product independent from the legacy V1 panel.

## Scope override

The root `AGENTS.md` describes the legacy self-hosted V1. Its prohibitions on authentication, billing, SaaS multi-tenancy, OAuth, RBAC, PostgreSQL, and new UI direction do not apply under `platform/`. The hosted platform explicitly requires User identity, Workspace tenancy, authority, commerce, multi-Region execution, and a new design system.

Retain the root repository's general safety, simplicity, Go quality, testing, and surgical-change rules where they do not conflict with this file.

## Isolation

- Put all new hosted-platform source, tests, contracts, migrations, configuration, and documentation under `platform/`.
- Do not import from or modify legacy `apps/web`, `apps/api`, legacy database models, legacy SaaS documents, or legacy UI assets.
- Do not copy legacy frontend code or styling.
- Read the files listed in `docs/development-plan.md` before implementation.
- Game Provider and Runtime Provider are the only legacy capabilities eligible for later migration, and only during Phase 6 through new contract-tested interfaces.

## Architecture

- Deploy three Go processes: Control Plane, Region Controller, and Node Agent.
- Use Next.js, React, TypeScript, Tailwind CSS, shadcn/ui, TanStack Query, and restrained Framer Motion for the console.
- Use PostgreSQL independently for the global plane and for each Region.
- Use transactional outbox, durable inbox, idempotent handlers, versioned contracts, and at-least-once delivery.
- Use NATS JetStream only as transport; PostgreSQL desired/observed state and Outbox/Inbox remain authoritative.
- A Region owns multiple Nodes. Do not add a Cell layer.
- A tenant User chooses a Region. Only the regional scheduler chooses a Node.
- Keep bounded contexts behind small module interfaces. A module cannot access another module's repository or tables.
- Production SQL must not contain JOIN. Query by indexed IDs in bounded batches and compose in Go, or maintain an asynchronous read model.
- Do not commit secrets, machine-specific paths, or external provider credentials.
- Keep HTTP handlers free of authentication and authorization logic. Route filters authenticate, resolve stored resource scope, and batch-check typed Actions through the Authorization module.
- Use built-in Role -> Permission -> Binding authorization. Do not add administrator flags, a custom policy DSL, per-resource authorization queries, or an authorization microservice in V1.
- Keep request work short. External I/O runs as durable idempotent Operations with leases, bounded retries, and reconciliation.
- Allow workload access to the public internet while isolating platform management networks and host control surfaces.

## Product and UI

- Workspace Console, Platform Console, and Region Operations are distinct operating areas.
- Workspace switching is tenant navigation. Entering Platform Console is explicit navigation, not role impersonation.
- Locale and light/dark/system theme are User Preferences and work from the first frontend phase.
- Do not mix languages in a rendered session or branch copy inline by locale.
- Follow `DESIGN.md`; legacy screenshots and CSS are not references.
- Treat `docs/v1-rebaseline.md` as the current product baseline. Plans, Orders, Payments, Entitlements, public password registration, World management, user-selected ports, and user-selected Nodes are outside V1.

## Delivery

- Follow phases in `docs/development-plan.md` in order.
- Do not implement a later phase to make an earlier demo look complete.
- Record acceptance evidence in the development plan after each phase.
- Keep commits limited to the current phase.
- Run formatting, unit, integration, architecture, contract, type, lint, build, accessibility, and browser checks appropriate to the phase.
