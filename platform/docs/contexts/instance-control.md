# Instance Control

Instance Control owns the customer's durable intent. It does not own a container, process, Node, or live runtime state.

## Language

**Logical Instance**:
The global customer resource representing one game server throughout configuration and deployment changes.
_Avoid_: Container, regional instance

**Instance Revision**:
An immutable version of resource specification, Provider release, and game configuration for a Logical Instance.
_Avoid_: Mutable config row

**Configuration Draft**:
A mutable proposed configuration that has not yet become an Instance Revision.
_Avoid_: Active configuration

**Desired State**:
The global requested lifecycle state of a Logical Instance.
_Avoid_: Runtime status

**Placement**:
The versioned assignment of a Logical Instance to a Region. It never selects a Node.
_Avoid_: Node placement

**Deployment Summary**:
The global, customer-facing projection of the latest sequenced Region observation.
_Avoid_: Regional source of truth
