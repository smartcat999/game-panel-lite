# Workspace

A Workspace is the user-facing form of the tenant boundary. All customer resources and commercial ownership belong to exactly one Workspace.

## Language

**Workspace**:
The isolation, ownership, quota, and billing boundary for one customer.
_Avoid_: Organization, project, tenant account

**Tenant**:
The isolation property of a Workspace, used only when discussing security or data partitioning.
_Avoid_: A second resource beside Workspace

**Membership**:
The relationship that grants a User a role in one Workspace.
_Avoid_: Workspace user

**Workspace Role**:
Owner, Administrator, Operator, Billing, or Viewer authority within one Workspace.
_Avoid_: Platform role
