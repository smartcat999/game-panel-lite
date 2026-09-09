# Node Execution

Node Execution owns the machine-local reconciliation of assigned workloads and the observation of their actual state.

## Language

**Node**:
A compute machine registered in exactly one Region.
_Avoid_: Region, host selected by tenant

**Work Assignment**:
A fenced, versioned instruction from a Region for a Node to reconcile one workload.
_Avoid_: Global task

**Workload**:
The Node-local runtime realization of one Regional Deployment.
_Avoid_: Logical Instance

**Game Provider**:
An adapter that validates and materializes game-specific configuration behind the platform's game interface.
_Avoid_: Runtime driver

**Runtime Provider**:
An adapter that realizes a workload through a concrete execution runtime behind the platform's runtime interface.
_Avoid_: Game plugin
