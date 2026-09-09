# Context Map

## Contexts

- [Identity](docs/contexts/identity.md): identifies a human and stores account-level preferences.
- [Workspace](docs/contexts/workspace.md): owns tenant membership and the customer resource boundary.
- [Commerce](docs/contexts/commerce.md): owns plans, orders, payments, and entitlements.
- [Instance Control](docs/contexts/instance-control.md): owns logical instances, configuration revisions, desired state, and Region placement.
- [Region Execution](docs/contexts/region-execution.md): turns an authorized placement into a scheduled regional deployment.
- [Node Execution](docs/contexts/node-execution.md): reconciles an assigned workload through provider adapters on one machine.

## Relationships

- **Identity → Workspace**: a User acts in a Workspace through a Membership.
- **Commerce → Instance Control**: an Entitlement authorizes one Logical Instance; Instance Control never infers payment state.
- **Instance Control → Region Execution**: a transactional outbox publishes a versioned Deployment Desired event.
- **Region Execution → Node Execution**: the regional scheduler creates a Work Assignment for one Node.
- **Node Execution → Region Execution**: the Node reports sequenced Workload Observations.
- **Region Execution → Instance Control**: the Region publishes deployment summaries; the global plane stores a customer-facing projection.
- Contexts exchange identifiers and versioned contracts. They do not read each other's tables.
