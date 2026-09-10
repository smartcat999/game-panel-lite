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

**Runtime Attempt**:
One replaceable execution generation of a Workload whose identity is retained as observation provenance.
_Avoid_: Logical Instance, customer server

**Game Provider**:
An adapter for one signed Provider release that validates and materializes game-specific configuration and capabilities behind the platform's game interface.
_Avoid_: Runtime driver

**Runtime Provider**:
An adapter that realizes a workload through a concrete execution runtime behind the platform's runtime interface.
_Avoid_: Game plugin
