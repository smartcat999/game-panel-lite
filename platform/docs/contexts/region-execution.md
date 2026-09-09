# Region Execution

Region Execution owns regional scheduling, durable task delivery, capacity reservation, and operational telemetry.

## Language

**Region**:
An independently deployable failure and operations domain with its own database, message ingress, scheduler, monitoring, and object storage configuration.
_Avoid_: Cell, availability-zone selector

**Regional Deployment**:
The Region-owned execution record for one versioned global Placement.
_Avoid_: Logical Instance

**Reservation**:
The scheduler-owned claim on Node capacity for a Regional Deployment.
_Avoid_: Live utilization

**Regional Task**:
A durable, retryable command whose result is recorded before acknowledgement.
_Avoid_: In-memory job

**Region Operator**:
A Platform Operator whose authority is scoped to one or more Regions.
_Avoid_: Workspace administrator
