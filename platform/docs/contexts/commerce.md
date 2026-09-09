# Commerce

Commerce records what a Workspace may buy and what paid authority currently exists.

## Language

**Plan**:
An immutable versioned offer containing price, billing period, Region availability, and resource specification.
_Avoid_: Instance type

**Order**:
A time-limited request by a Workspace to purchase a specific Plan version for a specific Logical Instance.
_Avoid_: Transaction

**Payment**:
A verified provider notification that settles an Order.
_Avoid_: Frontend callback

**Entitlement**:
Time-bounded commercial authority for one Logical Instance to consume the resources defined by its purchased Plan.
_Avoid_: Credits, permission
