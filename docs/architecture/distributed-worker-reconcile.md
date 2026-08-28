# Distributed worker reconciliation

GamePanel Lite treats a remote game server as durable desired state, not as a sequence of imperative Docker commands.

## Ownership

- HTTP handlers mutate `GameServer.Spec` only.
- The control-plane controller materializes one `WorkloadAssignment` for the selected remote node.
- The worker agent continuously compares that assignment with real Docker state and applies the smallest required change.
- The worker reports a `WorkloadObservation` after inspecting Docker again.
- The control plane derives `GameServer.Status` only from an observation matching the assignment UID, node, and generation.

Lifecycle ACKs must never set a server to running or stopped. Console input and other one-shot operations may use a separate operation queue, but they do not own lifecycle state.

## Generation and fencing

Every assignment carries the server spec generation and a placement UID. Containers are labeled with both values. A new placement receives a new UID, so a delayed observation from an older placement cannot update the current server.

An observation with an older generation leaves the server reconciling. The server is running only when the real container is running and the observed generation has caught up with the assignment generation.

## Deletion

Deletion remains asynchronous. The assignment carries desired state `deleted` until the assigned worker reports the runtime missing. Only then may the controller remove the assignment and the server resource.

## Current compatibility slice

The initial implementation keeps the existing node operation queue for console input. Legacy lifecycle tasks are rejected by the agent because lifecycle now converges through assignments. Log snapshots retry on content changes and advance their local digest only after a successful upload; an ordered, durable log cursor remains future work.

Automatic failover is intentionally excluded. An unreachable node produces unknown state; the control plane must not start the same server on another node without a fencing lease or an explicit migration procedure.
