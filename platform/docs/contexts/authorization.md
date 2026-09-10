# Authorization

Authorization describes what an authenticated Principal may do to resources under one explicit Scope.

## Language

**Principal**:
An authenticated User or machine identity presented for an authorization decision.
_Avoid_: Current user, actor account

**Scope**:
The single Platform, Workspace, or Region authority domain under which a resource is governed.
_Avoid_: Tenant ID supplied by a request

**Role**:
A named built-in set of Permissions valid for one Scope type.
_Avoid_: Admin flag, permission JSON

**Permission**:
One exact Action a Role may perform on a resource type.
_Avoid_: Access level

**Role Binding**:
The durable association of one Principal, one Role, and one Scope.
_Avoid_: User role field, synthetic membership
