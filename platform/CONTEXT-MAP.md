# Context Map

## Contexts

- [Identity](docs/contexts/identity.md): identifies a human and stores account-level preferences.
- [Authorization](docs/contexts/authorization.md): binds a Principal to a built-in Role at one Scope.
- [Workspace](docs/contexts/workspace.md): owns the customer resource, quota, and billing boundary.
- [Commerce](docs/contexts/commerce.md): owns pricing, prepaid funds, metering, and ledger posting.
- [Instance Control](docs/contexts/instance-control.md): owns logical instances, configuration revisions, desired state, and Region placement.
- [Region Execution](docs/contexts/region-execution.md): turns an authorized placement into a scheduled regional deployment.
- [Node Execution](docs/contexts/node-execution.md): reconciles an assigned workload through provider adapters on one machine.

## Relationships

- **Identity → Authorization**: a User acts as a Principal through explicit Role Bindings.
- **Authorization → Workspace**: a Workspace Role Binding grants Actions under one Workspace Scope.
- **Commerce → Instance Control**: a Funding Authorization permits a create command; usage and balance never become instance state.
- **Instance Control → Region Execution**: a transactional outbox publishes a versioned Deployment Desired event.
- **Region Execution → Node Execution**: the regional scheduler creates a Work Assignment for one Node.
- **Node Execution → Region Execution**: the Node reports sequenced Workload Observations.
- **Region Execution → Instance Control**: the Region publishes deployment summaries; the global plane stores a customer-facing projection.
- Contexts exchange identifiers and versioned contracts. They do not read each other's tables.
