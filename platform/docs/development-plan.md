# Clean Platform Development Plan

## Working rules

All new product work stays under `platform/`. The only required reading for a new session is:

1. `platform/README.md`
2. `platform/docs/v1-rebaseline.md`
3. `platform/AGENTS.md`
4. `platform/CONTEXT-MAP.md` and its linked contexts
5. `platform/docs/architecture.md`
6. `platform/docs/information-architecture.md`
7. `platform/DESIGN.md`
8. this plan

Do not read legacy UI or SaaS implementation unless the current task is explicitly the provider migration in Phase 6. Keep commits confined to one phase and record completed acceptance evidence in this file.

The original six phases below were completed against assumptions that the product review has superseded. Their evidence remains historical and cannot satisfy the rebaseline phases. The current product baseline is `docs/v1-rebaseline.md`.

## Rebaseline Phase 1 — contracts and clickable product flow — accepted 2026-09-10

Deliver:

- Replace Plan/Order/Payment/Entitlement contracts with Region Catalog, Price Book, Quote, Capacity Hold, Wallet, Ledger Entry, Usage Record, and Funding Authorization contracts.
- Add GitHub sign-in, local credential, invitation, built-in Role Binding, typed Action, batch authorization, Operation, Endpoint Binding, Provider Manifest, configuration schema, and observability envelopes.
- Build the complete Workspace create-to-operate flow as a clickable frontend prototype using contract fixtures, including failure, stale, forbidden, unsupported-capability, and insufficient-credit states.
- Update architecture tests to reject administrator flags, handler-local authorization, per-resource authorization SQL loops, legacy commerce names in production code, and World-management routes.

Acceptance:

- Contract compatibility tests parse every HTTP and message schema and prove unknown configuration field types fail closed.
- Authorization contract tests cover one Principal with Platform, Region, and Workspace bindings plus a 100-resource batch without per-resource SQL.
- The prototype completes sign-in, invitation, credit receipt, create, configuration, conditional mods, quote, deploy progress, endpoint display, lifecycle action, logs, backup, restore, and low-balance stop.
- Design review confirms the selected prototype visual language, compact density, no duplicate facts, no decorative counts, and no unsupported controls.
- No backend production behavior is changed in this phase.

## Rebaseline Phase 2 — identity and authorization — accepted 2026-09-10

Deliver GitHub OAuth, local credentials, sessions, Operator TOTP, invitations, the single Role Binding store, route filters, typed resource resolvers, and bounded batch decisions.

Acceptance requires cross-Scope and cross-Workspace denial tests, no handler permission branches, constant authorization query count for a 100-resource batch, and complete session security checks.

## Rebaseline Phase 3 — pricing, wallet, and resource catalog — accepted 2026-09-10

Deliver Region resource catalogs, immutable Price Books, Quotes, Capacity Holds, promotional credit grants, Wallet/Ledger, usage metering, and balance-exhaustion policy.

Acceptance requires concurrent debit safety, immutable correction entries, price-version transitions, no negative balance, accurate stopped-resource charging, and no simulated payment UI.

## Rebaseline Phase 4 — asynchronous instance delivery — accepted 2026-09-10

Deliver durable Operations, NATS JetStream transport, revised Outbox/Inbox contracts, idempotent step workers, Endpoint allocation, resource scheduling, readiness, and reconciliation.

Acceptance requires broker outage recovery, duplicate and out-of-order delivery safety, crash-at-every-step recovery, automatic residual cleanup, and honest stable-versus-changeable endpoint display.

## Rebaseline Phase 5 — provider-driven operation

Deliver signed Provider contracts and fakes, generic configuration rendering, immutable revisions, explicit apply behavior, controlled mod catalog/lockfile contracts, instance-keyed logs and metrics, game console, and instance backup/same-Region restore without migrating legacy adapters.

Acceptance requires zero game-specific frontend branches, schema and capability compatibility tests, preserved logs across fake Runtime replacement, conditional game metrics, and complete provider-neutral flows against contract fixtures.

## Rebaseline Phase 6 — hardening and replacement deployment

Migrate the Terraria, tModLoader, and Docker adapters only through the accepted Phase 5 interfaces. Deliver security and failure tests, production migrations, SLOs, runbooks, um773 deployment, smoke tests, and reversible cutover from the old deployment without importing old business models.

Acceptance requires all checks green, fresh-schema deployment, one complete Terraria and tModLoader flow, rollback instructions, verified login/create/operate/backup flow on um773, and removal of the old deployment only after the new health and browser checks pass.

## Superseded implementation history

### Historical Phase 1 — contracts and executable architecture

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

### Historical Phase 2 — User, Workspace, and authority

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

### Historical Phase 3 — global product and instance control

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

### Historical Phase 4 — Region control and scheduling

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

### Historical Phase 5 — Node Agent and backup path

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

### Historical Phase 6 — provider migration and production hardening

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

- 2026-09-10: Rebaseline Phase 2 follow-up verified that every administrator-issued one-time password carries an explicit 24-hour expiry and is rejected at the boundary instant unless changed earlier; PostgreSQL persists the expiry with the credential.

- 2026-09-07: architecture, domain language, console information architecture, clean design foundation, and six-phase plan established. Product code has intentionally not been scaffolded before Phase 1 review.
- 2026-09-10: Product and architecture review replaced plan-based checkout with custom regional resources and a prepaid Wallet/Ledger, established GitHub-first identity and one-table Role Binding authorization, made operations asynchronous through PostgreSQL Outbox/Inbox and NATS JetStream, pinned observability to Logical Instance identity, removed World management from V1, and selected the compact professional prototype as the visual reference. The rebaseline phases above now govern further work; all earlier phase evidence below is historical only.
- 2026-09-10: Rebaseline Phase 1 accepted. The v1 OpenAPI surface now defines GitHub-first sign-in, invitation redemption, typed Role Bindings and Actions, a bounded 100-resource authorization batch, Region Catalog, immutable Price Book, Quote, Capacity Hold, CNY Wallet/Ledger, Usage Record, Funding Authorization, durable Operation, system-allocated Endpoint Binding, and immutable Provider Manifest/configuration contracts. Eight HTTP/message JSON documents parse; compatibility tests prove the controlled configuration type allowlist fails closed and the old Plan/Order/Payment/Entitlement schemas are absent. Rebaseline architecture fixtures reject admin flags, handler-local authorization, per-resource SQL loops, legacy commerce models, World routes, legacy frontend imports, and forbidden frontend routes while leaving the superseded backend behavior unchanged for replacement in later phases. The contract-fixture prototype completes GitHub or local sign-in, invitation, platform credit grant, custom resource create, generic game configuration, conditional tModLoader mods, hourly quote, asynchronous deployment steps, allocated Endpoint display, start/stop/restart, console, instance-keyed logs, backup/restore, and wallet-exhaustion stop; fixture query states cover forbidden, failed, stale, unsupported, and insufficient credit. Side-by-side Product Design review against the selected list and detail references passed at desktop-source and narrow responsive states with no decorative counts, duplicate facts, unsupported network controls, global World controls, or plan-based sales UI; `platform/frontend/design-qa.md` records the visual acceptance. Final checks passed: JSON parse, `gofmt`, `go test ./...`, `pnpm lint`, `pnpm typecheck`, and `pnpm build`. No backend production behavior or Provider/Runtime implementation changed.
- 2026-09-10: Rebaseline Phase 2 accepted. The new access path implements GitHub OAuth with hashed, expiring, single-use state and verified-primary-email identity resolution; administrator-created local accounts use atomic User/Credential writes, bcrypt hashes, one-time-password change state, and no public registration route. Sessions persist only SHA-256 token hashes, enforce idle and absolute expiry, revoke and rotate after reauthentication or Operator TOTP, and emit HttpOnly, Secure-by-default, SameSite cookies. Operator TOTP secrets are AES-GCM encrypted, returned only at enrollment, accepted within a bounded RFC 6238 window, and protected against counter replay. Workspace invitations persist only token hashes, reject non-Workspace roles, cannot be claimed by another User, and resume idempotently with a deterministic Role Binding after a transient binding write failure. Authorization persists only `authorization_role_bindings`; the five built-in roles and typed Action sets remain in Go, Platform/Region/Workspace scopes are independent, and Platform authority does not inherit customer secret, console, or backup-download access. Authentication and authorization route filters run before endpoint handlers, resolve each resource from one authoritative table, verify nested path Scope claims, load Role Bindings once, cap batches at 100, and decide in Go. Tests prove cross-Scope, cross-Workspace, cross-parent-path, missing-resource, and oversized-batch denial; a 100-resource request performs one resolver load and one binding load and only invokes the handler after authorization. The Control Plane can overlay the production access API on the transitional product API when PostgreSQL and GitHub/TOTP configuration are present. Fresh-schema PostgreSQL integration applied global migrations 0001 and 0005 and verified atomic local account creation, hashed credentials/sessions, and the one-table Role Binding round trip. Final checks passed: JSON parse, `gofmt`, PostgreSQL-backed `go test ./... -count=1`, focused `go test -race`, `go vet ./...`, three-binary `go build ./...`, `pnpm lint`, `pnpm typecheck`, `pnpm build`, and `git diff --check`.
- 2026-09-10: Rebaseline Phase 3 accepted. The production billing path replaces plan checkout with immutable per-Region resource catalogs and Price Books, ten-minute Quotes pinned to one Price Book, Capacity Holds, signed Funding Authorizations requiring the available 24-hour estimate, promotional credits, and a CNY prepaid Wallet/Ledger. The database addition is deliberately limited to six single-purpose tables: catalogs, Price Books, combined Quote/Hold/Funding state, Wallet projections, immutable Ledger entries, and immutable Usage records. Ledger entries remain the authority while the locked Wallet row keeps promotional, cash, sequence, and state projections for constant-time reads and serialized debits. Usage is inserted in one bounded JSON batch without `JOIN` or SQL-in-loop, consumes promotional funds before cash, rejects arithmetic overflow, never makes either bucket negative, and remains idempotent under replay and concurrent debit. Metering pins each allocation to its Region's Price Book; stopped instances omit CPU and memory while retained instance disk, backup storage, and dedicated IP remain chargeable. The default exhaustion policy stops running instances and defines seven-day full-data plus thirty-day backup-only retention for Phase 4 workers to enact. Route filters protect Workspace wallet/ledger/quote operations and TOTP-gate Platform credit/catalog/price writes; quote requests require the contracted Provider Release identity without implementing a Provider early. Unit and real fresh-schema PostgreSQL tests prove concurrent debit safety, exact-zero exhaustion, no negative balance, immutable corrections and database rows, idempotent HTTP credit replay, price-version transitions, cross-Region price rejection, stopped-resource charging, and Wallet-projection/Ledger consistency. A frontend source audit confirms there is no recharge, checkout, or simulated payment control. Final checks passed: all eight JSON documents parsed, `gofmt`, PostgreSQL-backed `go test ./... -count=1`, focused `go test -race`, `go vet ./...`, `go build ./...`, `pnpm lint`, `pnpm typecheck`, `pnpm build`, and `git diff --check`.
- 2026-09-10: Rebaseline Phase 4 accepted. Instance creation is now one serializable Global transaction that locks and validates the authoritative Quote/Hold/Funding Authorization, writes the Logical Instance, durable Operation, and Outbox message, and consumes the Capacity Hold atomically; command replay returns the committed result without recharging even after Quote expiry, while changed replay input is rejected. Global and Region dispatchers claim bounded PostgreSQL Outbox batches with leases and retry metadata, publish versioned envelopes through NATS JetStream with message-ID deduplication, and acknowledge only after durable Inbox handling. Region delivery advances one committed, reclaimable step at a time through scheduling, Endpoint allocation, Node assignment, readiness, observation, and failed-resource cleanup. Scheduling reserves bounded Node capacity without SQL `JOIN`; fencing tokens come from a monotonic Region sequence; expired reconcile and assignment leases are reclaimable; stale tokens cannot mutate an assignment. Endpoint requirements are authority-signed Provider input, never customer network controls: gateway and dedicated-IP pools are preferred stable bindings, node-direct is explicitly `may-change`, TCP/UDP sets remain provider-declared, and IP-only bindings omit a port. The database addition is limited to two Global authority tables plus four Region execution tables and retry metadata on the existing outboxes. Real fresh-schema dual-PostgreSQL tests prove forged funding and desired payload rejection, broker outage recovery, duplicate and out-of-order safety, crash recovery at every committed boundary, assignment redelivery after worker loss, stable and changeable Endpoint truth, and automatic capacity/Endpoint residual cleanup. Actual JetStream tests prove publish deduplication, transient NAK/redelivery, permanent-message termination, and ACK-after-handler success. Browser acceptance proves stable, IP-only, and changeable Endpoint presentation without duplicate detail facts, preserves the conditional mod step, and reports no serious or critical axe violations. No Game Provider or Runtime Provider implementation was added or migrated. Final checks passed: all eight JSON documents parsed, `gofmt`, PostgreSQL- and NATS-backed `go test ./... -count=1`, focused `go test -race`, `go vet ./...`, `go build ./...`, `pnpm lint`, `pnpm typecheck`, `pnpm build`, `pnpm test:e2e` (3/3), and `git diff --check`.
- 2026-09-10: Phase 1 accepted. The isolated `platform/backend` Go module compiles the Control Plane, Region Controller, and Node Agent, and live probes returned healthy responses from all three processes. Versioned OpenAPI documents cover session, User Preferences, Workspace selection, Logical Instance create/read, the Region catalog, and Platform Region operations; four v1 event schemas cover deployment desired, entitlement changed, deployment observed, and backup observed. `go test ./...` parses all contract documents, preserves the required v1 API and message surface, round-trips typed identities, and proves the architecture rules reject deliberate legacy imports, cross-context repository imports, and SQL `JOIN` while the production tree passes. The independent Next.js console renders locale-aware and theme-aware Workspace, Platform, and Region Operations shells; browser checks covered English, Simplified Chinese, dark theme, 390 px navigation, all three routes, and a zero-error console. Final checks passed: `gofmt`, `go test ./...`, `go vet ./...`, three-binary `go build`, `pnpm lint`, `pnpm typecheck`, `pnpm build`, and `docker compose config --quiet`.
- 2026-09-10: Phase 2 accepted. Identity owns User, sign-in Identity, User Preferences, session restoration, and explicit Platform Operator authority; Workspace separately owns Workspace, Membership, Workspace Role, and per-User selection. Integration tests prove one User can switch between two Workspaces without another User's selected scope changing, Workspace members cannot enter Platform APIs, Platform Operators gain no synthetic Workspace membership, and cross-Workspace member IDs return `403`. The member endpoint reads Memberships by Workspace ID, batches User IDs through the Identity module interface, and composes the response in Go without SQL `JOIN`; the global PostgreSQL migration adds indexed single-context tables. The console consumes the expanded v1 contract through TanStack Query with Workspace-scoped keys, provides a functional Workspace switcher, scoped member management, Account preferences, and an explicit Platform Console entry. Browser integration against the Go Control Plane verified Workspace selection persistence, member role isolation, preference writes, server-side locale restoration before protected rendering, and a zero-error console. Final checks passed: `gofmt`, `go test ./...`, `go vet ./...`, three-binary `go build`, `pnpm lint`, `pnpm typecheck`, and `pnpm build`.
- 2026-09-10: Phase 3 accepted. The Global Control Plane now has separate Region Directory, Commerce, Instance Control, and Messaging modules with immutable Plan versions, versioned Instance Revisions and Placements, pending Orders, verified Payments, time-bounded Entitlements, transactional outbox writes, and idempotent inbox handling. PostgreSQL adapters keep module tables private, use indexed single-table point/list queries and bounded ID batches, and compose related records in Go without SQL `JOIN`. A real PostgreSQL integration test forces a late Order constraint failure after Instance insertion and proves full rollback; it also proves checkout command replay returns the original Instance and Order, unentitled Instances cannot write deployment authority, verified payment redelivery creates one Payment and Entitlement, outbox writes remain exactly-once by message type and identity, and inbox redelivery invokes its handler once. Workspace Console now provides instance list/create/detail and billing Orders, while Platform Console provides Workspaces, immutable Plans, Orders, Regions, and global Logical Instances. The customer create contract and screen expose Plan, Region, game version, and configuration but no Node. Playwright checks against the live Go API covered pending payment, waiting for Region, running, stopped, failed, stale, and simulated `403` states in English and Simplified Chinese under both light and dark themes, plus all Workspace and Platform resource views. Final checks passed: PostgreSQL-backed `go test ./... -count=1`, `go vet ./...`, `go test -race ./...`, `pnpm lint`, `pnpm typecheck`, and `pnpm build`.
- 2026-09-10: Phase 4 accepted. Region Execution now owns a durable Region inbox/outbox, Regional Deployments, Node registry and leases, capacity Reservations with monotonically increasing fencing tokens, durable Regional Tasks, sequenced observations, and operator audit records. The local scheduler filters expired or non-ready Nodes, game incompatibility, and insufficient capacity with explicit reasons, then scores capable Nodes and reserves capacity transactionally. A real Region PostgreSQL concurrency test proves two simultaneous scheduling attempts cannot claim the same capacity; durable redelivery survives a module restart, duplicate and lower-sequence observations do not regress state, and only the newest observation reaches the Region outbox. A Region-local reconciliation loop operates solely from durable regional state, and its outage test continues scheduling and producing one durable task with no global-plane call. The Control Plane consumes `deployment.observed.v1` through its durable inbox, sequence-guards a Deployment Summary, and combines bounded summary batches with Logical Instances in Go; a Global PostgreSQL integration test proves an older projection cannot replace a newer customer-visible state. Region Operator authority requires both explicit Platform authority and Region scope. Placement override requires a concrete Node and reason, writes an audit record, and rejects an unavailable Node without changing or falling back from the current placement. Region Operations now provides Overview, Nodes, Deployments, Tasks, Capacity, Storage, and Monitoring pages; deployment rows visibly separate Region-owned execution from linked global Logical Instance and Workspace context. Playwright verified every page, successful and rejected override flows, English and Simplified Chinese, dark theme, 390 px navigation, localized operational state, and a clean zero-error console. Final checks passed: Global/Region PostgreSQL-backed `go test ./... -count=1`, `go vet ./...`, `go test -race ./...`, `pnpm lint`, `pnpm typecheck`, and `pnpm build`.
- 2026-09-10: Phase 5 accepted. Region Execution persists bounded Work Assignments with claim leases, attempt counts, terminal states, and Reservation fencing tokens; the Node Agent polls in batches capped at 100, uses bounded exponential backoff, reclaims only expired claims after restart, and records task latency and reconciliation failures. Unit and real Region PostgreSQL tests prove stale Nodes and stale fencing tokens cannot complete an active workload mutation, an unexpired claim is not stolen, an expired claim is resumed by a restarted Agent, and repeated terminal completion has no effect. Global Backup Control persists idempotent Backup Requests and emits `backup.requested.v1`; Region ingress converts each request into one durable regional task and Node assignment, then emits a sequence-guarded `backup.observed.v1` result that the Global database applies without regression. Scoped roots reject absolute paths, traversal, symlinks, and malicious archive entries. Archive creation streams through an object-transfer interface directly from Node to object storage with a checksum and byte count, so Control Plane code never receives archive bytes; restore downloads directly into the owning Region root. Restore commands can only reference a completed backup for the same Workspace, Logical Instance, and Region, leaving cross-Region migration absent. Region Monitoring now exposes pending inbox/outbox lag, stale Nodes, task latency, reconciliation failures, and the existing capacity view. Browser checks against live Go services covered the seeded completed backup, create and same-Region restore requests, English and Simplified Chinese, dark theme, 390 px navigation, the expanded Monitoring page, and a zero-error console. Final checks passed: Global/Region PostgreSQL-backed `go test ./... -count=1`, `go vet ./...`, `go test -race ./...`, three-binary `go build`, `pnpm lint`, `pnpm typecheck`, and `pnpm build`.
- 2026-09-10: Phase 6 accepted. Minimal Game Provider and Runtime Provider interfaces were derived at the Phase 5 Node execution call site, with replaceable in-memory fakes proving neither Control Plane nor Region scheduling changes when adapters change. The versioned desired-deployment contract now carries game version and configuration through Global outbox, Region PostgreSQL, Work Assignment, Terraria Provider, and Docker Runtime; the migrated Terraria adapter validates version, configuration, path-sensitive world names, player limits, and multiline injection, while the Docker adapter owns all runtime mutation, validates scoped mounts and symlinks, applies CPU/memory/PID limits, drops capabilities, and enables `no-new-privileges`. Real PostgreSQL tests apply all Global and Region migrations to fresh schemas and prove provider inputs and sequenced workload results survive restart. Parallel benchmarks separately exercised stateless APIs (4,055 ns/op), idempotent consumers (628.8 ns/op), schedulers (1,520 ns/op), and Node Agents (1,742 ns/op) for 100 iterations on Apple M1. The failure matrix covers broker redelivery, both database/controller restarts, Region isolation, Node loss and fencing, duplicate payment notifications, stale observations, backup replay, and malicious archive/mount paths. Production documentation now defines topology, forward-only schema rollout, frozen message compatibility, SLOs and alerts, backup recovery, and Region/Node isolation runbooks. A source-isolation test rejects legacy frontend references; Chromium checks against live independent Go services covered versioned Terraria checkout, backup and same-Region restore, English and Simplified Chinese, light and dark themes, 390 px navigation, zero browser errors, and zero serious or critical axe violations. Final checks passed: Global/Region PostgreSQL-backed `go test ./... -count=1`, `go vet ./...`, PostgreSQL-backed `go test -race ./... -count=1`, three-binary `go build`, the read-only Docker daemon integration check, `pnpm lint`, `pnpm typecheck`, `pnpm build`, and `pnpm test:e2e`.
