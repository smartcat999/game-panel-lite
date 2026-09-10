# Commerce

Commerce owns regional resource pricing, prepaid Workspace funds, measured usage, and immutable financial history.

## Language

**Price Book**:
An immutable published version of regional resource unit prices and their effective time.
_Avoid_: Plan, package

**Quote**:
A short-lived price calculation for one requested resource specification.
_Avoid_: Order, invoice

**Wallet**:
The Workspace-owned prepaid balance derived from immutable Ledger Entries.
_Avoid_: Account balance, entitlement

**Ledger Entry**:
An immutable credit or debit that changes one Wallet balance.
_Avoid_: Mutable balance row, payment

**Funding Authorization**:
A short-lived decision that sufficient available funds permit a command to proceed.
_Avoid_: Charge, entitlement

**Usage Record**:
An immutable measured interval for one billable resource owned by a Workspace.
_Avoid_: Estimate, runtime status
