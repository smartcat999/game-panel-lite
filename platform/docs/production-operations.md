# Production operations

## Deployment topology

Deploy the Control Plane API and message consumers as independent stateless workloads backed by the Global PostgreSQL database. Each Region runs its own Region Controller replicas, Region PostgreSQL database, object-storage endpoint, and Node Agent fleet. A Region database and its Nodes are never shared with another Region. The broker connects planes through versioned events; it is not a synchronous request dependency for existing regional workloads.

Scale Control Plane APIs, event consumers, Region schedulers, and Node Agents independently. Scheduler concurrency is safe because capacity Reservations and monotonically increasing fencing tokens are committed in serializable Region transactions. Node Agents use bounded polling, claim leases, and capped exponential backoff.

## Schema rollout

1. Back up the target database and record the deployed application revision.
2. Apply Global migrations in filename order to the Global database and Region migrations independently to one Region database at a time.
3. Verify migration checksums and indexes before deploying readers or writers that require the new columns or tables.
4. Deploy backward-compatible consumers before producers for additive event changes.
5. Pause a Region rollout if inbox lag, outbox lag, or reconciliation failures exceed its alert threshold. Other Regions continue operating.

Migrations are forward-only. A failed migration is restored from the pre-rollout database backup; application rollback must remain compatible with all migrations already committed.

## Message compatibility

Every event name carries its major schema version, such as `deployment.desired.v1`. The initial v1 required field set is frozen at the first production release. After that freeze, producers must retain required v1 fields and may only add optional fields accepted by the v1 schema. Consumers persist `messageId` before committing an effect, ignore redelivery, and sequence-guard mutable projections. Introduce a v2 producer only after v2 consumers are deployed, then dual-publish during the documented transition window.

## SLOs and alerts

| Signal | Objective | Alert |
| --- | --- | --- |
| Control Plane availability | 99.9% monthly | 5-minute success rate below 99% |
| Region reconciliation | 99% of tasks terminal within 60 seconds | p95 task latency above 60 seconds for 10 minutes |
| Inbox lag | less than 30 seconds | oldest pending inbox message above 60 seconds |
| Outbox lag | less than 30 seconds | oldest unpublished outbox message above 60 seconds |
| Node freshness | heartbeat within one lease interval | any ready Node misses two lease intervals |
| Instance readiness | 99% of accepted creates ready within 10 minutes | any create exceeds 15 minutes or reports ready before listener and Provider marker checks pass |
| Backup completion | 99% within 15 minutes | failure ratio above 5% or age above 30 minutes |

Page on sustained Region database unavailability, fencing rejection spikes, or backup integrity mismatch. Ticket isolated single-task failures with their Region, assignment, Logical Instance, and fencing token identifiers; do not include passwords or signed transfer URLs.

## Backup recovery runbook

1. Confirm the Backup Request is `completed`, belongs to the target Logical Instance, and names the same Region as the active Placement.
2. Verify object metadata, recorded byte size, and checksum. Generate a short-lived signed download URL.
3. Submit an idempotent restore request referencing that completed backup. Never proxy archive bytes through the Control Plane.
4. Watch the Region task and Node assignment. A restarted Agent reclaims the assignment only after its claim lease expires.
5. Verify the terminal `backup.observed.v1` sequence and game workload health before reopening the instance.

Cross-Region restore is intentionally unsupported. Migration requires a separately designed workflow with source export, destination import, ownership transfer, and a new Placement version.

## Region isolation runbook

When a Region loses broker or Control Plane connectivity, keep Region Controller, Region PostgreSQL, and Node Agents running. Existing deployments continue reconciling from regional durable state. Stop new global placement into the Region, inspect pending inbox/outbox age, and avoid manual database edits. After connectivity returns, allow idempotent redelivery to drain naturally and confirm sequence guards reject stale observations.

When a Node is lost, let its lease expire, mark it stale, and schedule or explicitly override onto a ready compatible Node. Never extend a dead Node lease or reuse its fencing token. Escalate if capacity cannot host the displaced workload.

## Workload network runbook

Game workloads need unrestricted internet egress for Steam updates and controlled mod downloads, but must not reach host, private management, metadata, or link-local networks. The `workload-firewall` service owns the `DOCKER-USER` policy for containers attached to `gamepanel-workloads`; application containers do not modify host firewall state.

Before and after each rollout, run the firewall self-check, prove an anchor workload can reach a public HTTPS endpoint, and prove it cannot reach the management address. A failed self-check blocks rollout. Do not solve update failures by attaching game workloads to the management network or by removing RFC1918/link-local rejection rules.

## Replacement deployment runbook

Bring up the new Compose project on port 3006 with fresh databases and a separate data root. Complete health, login, Terraria, tModLoader, lifecycle, console, logs, backup/restore, egress isolation, and real-browser checks before touching the old project. Stop the old project, bind the candidate nginx to port 3005, and repeat health and browser checks. Only then remove old containers and their network without `--volumes`; retain the old release directory, database volumes, and data root for the rollback window.

Rollback changes traffic ownership only: stop the new nginx, restart the recorded previous Compose release on port 3005, and verify health and login. Never import or rewrite old business records as part of either cutover direction.

## Validation commands

The scenario-to-test mapping is maintained in [failure-testing.md](failure-testing.md).

Run from `platform/backend`:

```sh
go test ./...
go test -race ./...
go test ./internal/scaletest -bench . -benchtime=100x
go vet ./...
```

Run from `platform/frontend`:

```sh
pnpm lint
pnpm typecheck
pnpm build
pnpm test:e2e
```
