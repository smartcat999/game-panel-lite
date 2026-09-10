# Clean Platform Development Plan

## Working rules

All new product work stays under `platform/`. The only required reading for a new session is:

1. `platform/README.md`
2. `platform/AGENTS.md`
3. `platform/CONTEXT-MAP.md` and its linked contexts
4. `platform/docs/architecture.md`
5. `platform/docs/information-architecture.md`
6. `platform/DESIGN.md`
7. this plan

Do not read legacy UI or SaaS implementation unless the current task is explicitly the provider migration in Phase 6. Keep commits confined to one phase and record completed acceptance evidence in this file.

## Phase 1 — contracts and executable architecture

Deliver:

- Scaffold `platform/backend` as an independent Go module and `platform/frontend` as an independent Next.js application.
- Define typed IDs and versioned envelopes for Workspace, Logical Instance, Region, Regional Deployment, Node, command, event, and idempotency identity.
- Write initial OpenAPI contracts for session, User Preferences, Workspace selection, instance create/read, Region catalog, and platform Region operations.
- Define message schemas for deployment desired, entitlement changed, deployment observed, and backup observed.
- Add architecture tests that forbid imports from legacy `apps/`, cross-context repositories, and SQL containing `JOIN`.
- Add local Compose dependencies for global PostgreSQL, one Region PostgreSQL, and a message broker. Do not implement production behavior yet.

Acceptance:

- All three Go binaries compile with health endpoints.
- OpenAPI and message schemas parse and have compatibility tests.
- Dependency-boundary and no-JOIN tests fail on deliberate violation and pass normally.
- Frontend displays a locale-aware, theme-aware empty shell for the three operating areas.
- No production source imports a legacy path.

## Phase 2 — User, Workspace, and authority

Deliver:

- Implement User, Identity, User Preference, Workspace, Membership, Workspace Role, and Platform Operator modules.
- Create account preference APIs for locale, theme, and time zone.
- Implement session restoration and authorization policies at module interfaces.
- Build Account settings, Workspace switcher, Member management, and explicit Platform Console entry.
- Seed a local User, Workspace, Membership, and Platform Operator for preview.

Acceptance:

- A User can belong to two Workspaces and switch without leaking cached data.
- Locale and theme persist to the User and apply before protected pages render.
- Workspace roles cannot access Platform Console; Platform authority does not manufacture Workspace membership.
- Authorization integration tests cover cross-Workspace ID attempts.

## Phase 3 — global product and instance control

Deliver:

- Implement Region Directory, immutable Plan versions, Logical Instance, Instance Revision, Placement, Order, Payment, and Entitlement modules.
- Implement atomic create checkout and verified payment activation.
- Implement transactional outbox and global inbox with idempotent handlers.
- Build Workspace instance list, create flow, instance detail, billing state, orders, and Platform views for Workspaces, plans, orders, Regions, and global instances.
- Keep Node absent from customer commands and screens.

Acceptance:

- Failed checkout leaves no partial instance or order.
- Repeating a command with the same idempotency identity returns the same result.
- An instance without an active Entitlement cannot emit executable deployment authority.
- Request-path SQL uses indexed single-table and bounded batch queries only.
- UI covers pending payment, waiting for Region, running, stopped, failed, stale, and forbidden states in both locales and themes.

## Phase 4 — Region control and scheduling

Deliver:

- Implement the Region durable inbox/outbox, Regional Deployment, Node registry, lease, Reservation, Regional Task, and sequenced observation modules.
- Implement Region-local scheduler filtering and scoring with capacity reservation and explicit unschedulable reasons.
- Add Control Plane projection consumers for customer-facing deployment summaries.
- Build Region Operations overview, Nodes, Deployments, Tasks, Capacity, Storage, and Monitoring pages.
- Add an operator-only Node placement override with explicit authority, reason, audit record, and no fallback.

Acceptance:

- Duplicate and out-of-order events cannot regress regional or global state.
- Two concurrent scheduler attempts cannot reserve the same capacity.
- A Region continues reconciling existing deployments during a simulated global outage.
- Region Operations clearly separates global Logical Instance context from regional execution ownership.

## Phase 5 — Node Agent and backup path

Deliver:

- Implement bounded polling, backoff, leases, fencing tokens, assignment claims, workload reconciliation, and sequenced observations.
- Define scoped file roots and direct object-storage transfer contracts.
- Implement global Backup Request, Region task execution, Node archive/upload, and asynchronous completion reporting.
- Add observability for task latency, reconciliation failures, capacity, stale Nodes, inbox lag, and outbox lag.

Acceptance:

- A stale Node or stale fencing token cannot mutate the active workload record.
- Restarting Region Controller or Node Agent resumes durable work without losing or duplicating terminal effects.
- Backup bytes bypass the Control Plane and restore works within the owning Region.
- Cross-Region migration remains absent from the critical path.

## Phase 6 — provider migration and production hardening

Deliver:

- Specify minimal Game Provider and Runtime Provider interfaces from actual Phase 5 call sites.
- Write contract tests and in-memory fakes before migrating adapters.
- Migrate one Game Provider and Docker Runtime Provider from legacy code, preserving validated security behavior while removing legacy model dependencies.
- Add load, failure-injection, migration, security, accessibility, and end-to-end checks.
- Document deployment topology, schema rollout, message compatibility, SLOs, alerts, backup recovery, and Region isolation runbooks.

Acceptance:

- Provider adapters can be replaced through the same interface without changes in Control Plane or Region scheduling modules.
- Load tests demonstrate separately scalable stateless APIs, message consumers, schedulers, and Node Agents.
- Failure tests cover broker redelivery, database restart, Region isolation, Node loss, duplicate payment notification, and stale observation.
- The new frontend has no copied legacy route, stylesheet, or component and passes build, type, locale, theme, accessibility, and main-flow browser checks.

## Suggested commit sequence

1. `docs: define clean platform architecture`
2. `chore: scaffold isolated platform workspace`
3. `feat: add platform contracts and boundary checks`
4. `feat: add user workspace and authority modules`
5. `feat: add global instance control and commerce`
6. `feat: add regional scheduling and operations`
7. `feat: add node reconciliation and backups`
8. `feat: migrate provider adapters`
9. `test: validate platform failure and scale behavior`

## Prompt for a new Codex session

```text
在 /Users/pengwu/Desktop/Projects/go-project/game-panel-lite 中开发全新的 GamePanel 托管平台。

先只阅读 platform/README.md、platform/AGENTS.md、platform/CONTEXT-MAP.md 及其链接文件、platform/docs/architecture.md、platform/docs/information-architecture.md、platform/DESIGN.md、platform/docs/development-plan.md 和根 AGENTS.md。platform/AGENTS.md 对托管平台范围的规则优先。不要读取或修改旧 apps/web、旧 apps/api、旧 docs/architecture；它们不是新版设计依据。

从 development-plan.md 的 Phase 1 开始，严格按 acceptance 顺序完成并验证。所有新代码只能位于 platform/。前端与后端相互独立，通过 platform/contracts 的版本化契约协作。生产 SQL 禁止 JOIN，跨模块数据按 ID 批量查询后在 Go 中组合。不要迁移旧业务模型；Game Provider 和 Runtime Provider 只能在 Phase 6 通过新接口迁移。每完成一个阶段更新 development-plan.md 的证据并提交对应 commit，不要提前实现后续阶段。
```

## Progress evidence

- 2026-09-07: architecture, domain language, console information architecture, clean design foundation, and six-phase plan established. Product code has intentionally not been scaffolded before Phase 1 review.
- 2026-09-10: Phase 1 accepted. The isolated `platform/backend` Go module compiles the Control Plane, Region Controller, and Node Agent, and live probes returned healthy responses from all three processes. Versioned OpenAPI documents cover session, User Preferences, Workspace selection, Logical Instance create/read, the Region catalog, and Platform Region operations; four v1 event schemas cover deployment desired, entitlement changed, deployment observed, and backup observed. `go test ./...` parses all contract documents, preserves the required v1 API and message surface, round-trips typed identities, and proves the architecture rules reject deliberate legacy imports, cross-context repository imports, and SQL `JOIN` while the production tree passes. The independent Next.js console renders locale-aware and theme-aware Workspace, Platform, and Region Operations shells; browser checks covered English, Simplified Chinese, dark theme, 390 px navigation, all three routes, and a zero-error console. Final checks passed: `gofmt`, `go test ./...`, `go vet ./...`, three-binary `go build`, `pnpm lint`, `pnpm typecheck`, `pnpm build`, and `docker compose config --quiet`.
- 2026-09-10: Phase 2 accepted. Identity owns User, sign-in Identity, User Preferences, session restoration, and explicit Platform Operator authority; Workspace separately owns Workspace, Membership, Workspace Role, and per-User selection. Integration tests prove one User can switch between two Workspaces without another User's selected scope changing, Workspace members cannot enter Platform APIs, Platform Operators gain no synthetic Workspace membership, and cross-Workspace member IDs return `403`. The member endpoint reads Memberships by Workspace ID, batches User IDs through the Identity module interface, and composes the response in Go without SQL `JOIN`; the global PostgreSQL migration adds indexed single-context tables. The console consumes the expanded v1 contract through TanStack Query with Workspace-scoped keys, provides a functional Workspace switcher, scoped member management, Account preferences, and an explicit Platform Console entry. Browser integration against the Go Control Plane verified Workspace selection persistence, member role isolation, preference writes, server-side locale restoration before protected rendering, and a zero-error console. Final checks passed: `gofmt`, `go test ./...`, `go vet ./...`, three-binary `go build`, `pnpm lint`, `pnpm typecheck`, and `pnpm build`.
- 2026-09-10: Phase 3 accepted. The Global Control Plane now has separate Region Directory, Commerce, Instance Control, and Messaging modules with immutable Plan versions, versioned Instance Revisions and Placements, pending Orders, verified Payments, time-bounded Entitlements, transactional outbox writes, and idempotent inbox handling. PostgreSQL adapters keep module tables private, use indexed single-table point/list queries and bounded ID batches, and compose related records in Go without SQL `JOIN`. A real PostgreSQL integration test forces a late Order constraint failure after Instance insertion and proves full rollback; it also proves checkout command replay returns the original Instance and Order, unentitled Instances cannot write deployment authority, verified payment redelivery creates one Payment and Entitlement, outbox writes remain exactly-once by message type and identity, and inbox redelivery invokes its handler once. Workspace Console now provides instance list/create/detail and billing Orders, while Platform Console provides Workspaces, immutable Plans, Orders, Regions, and global Logical Instances. The customer create contract and screen expose Plan, Region, game version, and configuration but no Node. Playwright checks against the live Go API covered pending payment, waiting for Region, running, stopped, failed, stale, and simulated `403` states in English and Simplified Chinese under both light and dark themes, plus all Workspace and Platform resource views. Final checks passed: PostgreSQL-backed `go test ./... -count=1`, `go vet ./...`, `go test -race ./...`, `pnpm lint`, `pnpm typecheck`, and `pnpm build`.
- 2026-09-10: Phase 4 accepted. Region Execution now owns a durable Region inbox/outbox, Regional Deployments, Node registry and leases, capacity Reservations with monotonically increasing fencing tokens, durable Regional Tasks, sequenced observations, and operator audit records. The local scheduler filters expired or non-ready Nodes, game incompatibility, and insufficient capacity with explicit reasons, then scores capable Nodes and reserves capacity transactionally. A real Region PostgreSQL concurrency test proves two simultaneous scheduling attempts cannot claim the same capacity; durable redelivery survives a module restart, duplicate and lower-sequence observations do not regress state, and only the newest observation reaches the Region outbox. A Region-local reconciliation loop operates solely from durable regional state, and its outage test continues scheduling and producing one durable task with no global-plane call. The Control Plane consumes `deployment.observed.v1` through its durable inbox, sequence-guards a Deployment Summary, and combines bounded summary batches with Logical Instances in Go; a Global PostgreSQL integration test proves an older projection cannot replace a newer customer-visible state. Region Operator authority requires both explicit Platform authority and Region scope. Placement override requires a concrete Node and reason, writes an audit record, and rejects an unavailable Node without changing or falling back from the current placement. Region Operations now provides Overview, Nodes, Deployments, Tasks, Capacity, Storage, and Monitoring pages; deployment rows visibly separate Region-owned execution from linked global Logical Instance and Workspace context. Playwright verified every page, successful and rejected override flows, English and Simplified Chinese, dark theme, 390 px navigation, localized operational state, and a clean zero-error console. Final checks passed: Global/Region PostgreSQL-backed `go test ./... -count=1`, `go vet ./...`, `go test -race ./...`, `pnpm lint`, `pnpm typecheck`, and `pnpm build`.
- 2026-09-10: Phase 5 accepted. Region Execution persists bounded Work Assignments with claim leases, attempt counts, terminal states, and Reservation fencing tokens; the Node Agent polls in batches capped at 100, uses bounded exponential backoff, reclaims only expired claims after restart, and records task latency and reconciliation failures. Unit and real Region PostgreSQL tests prove stale Nodes and stale fencing tokens cannot complete an active workload mutation, an unexpired claim is not stolen, an expired claim is resumed by a restarted Agent, and repeated terminal completion has no effect. Global Backup Control persists idempotent Backup Requests and emits `backup.requested.v1`; Region ingress converts each request into one durable regional task and Node assignment, then emits a sequence-guarded `backup.observed.v1` result that the Global database applies without regression. Scoped roots reject absolute paths, traversal, symlinks, and malicious archive entries. Archive creation streams through an object-transfer interface directly from Node to object storage with a checksum and byte count, so Control Plane code never receives archive bytes; restore downloads directly into the owning Region root. Restore commands can only reference a completed backup for the same Workspace, Logical Instance, and Region, leaving cross-Region migration absent. Region Monitoring now exposes pending inbox/outbox lag, stale Nodes, task latency, reconciliation failures, and the existing capacity view. Browser checks against live Go services covered the seeded completed backup, create and same-Region restore requests, English and Simplified Chinese, dark theme, 390 px navigation, the expanded Monitoring page, and a zero-error console. Final checks passed: Global/Region PostgreSQL-backed `go test ./... -count=1`, `go vet ./...`, `go test -race ./...`, three-binary `go build`, `pnpm lint`, `pnpm typecheck`, and `pnpm build`.
