# Hosted V1 Rebaseline

This document is the compact product baseline agreed after the first implementation review. It replaces the earlier plan-based checkout and dashboard assumptions. Architecture detail remains in `architecture.md`; domain words remain in `CONTEXT-MAP.md` and its linked contexts.

## Product boundary

- China-first hosted game-server platform, priced in CNY minor units.
- Launch games: Terraria and tModLoader through first-party signed Provider releases.
- Users choose Region and custom vCPU, memory, and disk within Region-provided limits. They never choose a Node, public port, protocol, or bandwidth.
- World upload, import, migration, and world-management screens are outside V1. Instance-level backup and same-Region restore remain in scope.
- V1 must complete one real flow: GitHub sign-in, invite redemption, promotional credit grant, create, configure, optional mods, quote, deploy, connect, operate, observe, back up, restore, and balance-exhaustion stop.

## Identity and authority

- Public sign-in uses GitHub OAuth only. There is no public password registration.
- A GitHub-authenticated User may set a local password after recent reauthentication. A Platform Operator may create a local User with a one-time password that expires after 24 hours and must be changed on first sign-in.
- Identity owns Users, sign-in identities, credentials, sessions, MFA, and preferences. Authorization separately owns Role Bindings.
- Built-in roles are `platform.admin`, `region.operator`, `workspace.owner`, `workspace.operator`, and `workspace.viewer`. Every permission comes through Role -> Permission -> Binding; there is no administrator bypass flag.
- V1 persists one authorization table for Role Bindings. Built-in role definitions and exact Action sets are type-safe Go definitions. No custom Policy DSL or remote authorization process is introduced.
- HTTP routes declare an Action and typed resource resolver. Authentication and authorization run in filters before handlers. Scope is resolved from stored resource ownership; path or query values are never accepted as proof of ownership.
- Authorization supports bounded batch decisions. Bindings are read once per request and all checks are evaluated in Go. Collection routes authorize the collection before listing.
- Workspace and Region are independent scope trees. Every resource has one authorization scope; placement and execution are explicit references. Cross-scope commands declare every target and the required decision rule.
- Platform administrators do not automatically receive customer secret reveal, backup download, or console-command permissions.

## Commerce

- There are no Plans, Orders, Payments, or per-instance Entitlements in the V1 model.
- A versioned Price Book prices vCPU, memory, instance disk, backup storage, and dedicated IP by Region.
- A Quote is valid for 10 minutes and creates a Capacity Hold only at the final create step.
- A Workspace owns one prepaid Wallet and immutable Ledger Entries. Promotional credit is consumed before cash. V1 exposes operator-granted promotional credit and no simulated payment UI.
- Create requires available credit covering the current 24-hour estimate but does not precharge it. Metering records actual allocation; Ledger posting is idempotent and cannot produce a negative balance.
- Compute is released and stops charging when an instance is stopped. Disk, backups, and retained dedicated IP continue charging. At zero balance all running instances stop; data is retained for 7 days, backup-only retention continues through day 30, then deletion follows notice policy.

## Create and configuration

- Create has three compact steps: Instance and Resources, Game Configuration, and conditional Mod Configuration. There is no review page or repeated summary.
- Region supplies resource ranges, steps, availability, endpoint delivery capabilities, and prices. The frontend hardcodes none of them.
- A Provider release supplies a versioned Manifest, a controlled JSON Schema subset, UI metadata, capabilities, game versions, endpoint requirements, and apply behavior. No Provider may inject frontend code.
- Configuration drafts autosave. Applying creates an immutable Instance Revision. Fields declare hot reload, restart required, recreate required, or create-only behavior. Schema migration is explicit.
- Mod-capable releases use a controlled catalog, exact version lockfile, dependency checks, and explicit upgrades. Unknown field types or unresolved dependencies block submission.

## Execution and connectivity

- APIs perform short validation and database transactions only, persist an Operation and transactional Outbox record, then return `202` with the Operation ID.
- NATS JetStream transports versioned events. PostgreSQL Outbox/Inbox and durable desired/observed state remain authoritative. Delivery is at least once; every handler and step is idempotent.
- Region workers execute external I/O as durable, leased steps. Reconcilers repair expired leases, missing delivery, stuck operations, and residual resources.
- A Provider declares listener purpose, transport, internal port, external-port policy, address mode, and primary binding. Region allocates compatible Gateway, dedicated-IP, or Node-Direct delivery. The UI renders returned connection instructions and never asks users to configure public ports or protocols.
- Runtime workloads may access the public internet for game startup, Steam, updates, and mods. Platform management networks, databases, NATS, Node management endpoints, cloud metadata, Docker sockets, host networking, and arbitrary host paths remain inaccessible.

## Observability and recovery

- Customer logs, metrics, retention, and queries are keyed by stable Logical Instance ID. Regional Deployment, Work Assignment, and Runtime IDs are provenance dimensions only; deleting a container must not delete history.
- Platform-guaranteed metrics are CPU, memory, network, instance disk, process state, restart count, and sample time. Player and game metrics appear only when a Provider declares a reliable source, freshness, and confidence.
- Console means a game command console, never a host shell. Logs and status use SSE; commands use authenticated HTTPS requests.
- Backups cover complete instance data and carry Provider version, game version, mod lockfile, configuration, checksums, and Region ownership. Restore is same-Region in V1.

## Console design

- The selected light prototype is the visual reference for color, type, controls, tables, borders, radius, and density. It is not a reason to copy empty dashboards or repeated summaries.
- Use self-hosted Inter and Noto Sans SC. Use monospace only for identifiers, endpoints, logs, versions, money, and measurements.
- Primary actions use near-black navy. Green is reserved for brand and success; purple for mods; amber for warning; red for failure or destructive actions.
- Workspace opens Instances. Navigation is Instances, Backups, Operations, Billing, Members, and Settings. Sidebar count badges, global search, promotional headers, duplicate cancel actions, and decorative overview cards are absent.
- Instance rows show only useful, supported information. Player count and latency columns disappear when unavailable. Each row has one state-dependent primary action and one overflow menu.
- Instance detail owns each fact once. Fixed tabs are Overview, Console, Backups, and Configuration; Files and Mods are capability-driven.
