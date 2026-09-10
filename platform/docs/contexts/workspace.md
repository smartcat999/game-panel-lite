# Workspace

A Workspace is the user-facing customer boundary. All customer resources, quotas, and prepaid funds belong to exactly one Workspace.

## Language

**Workspace**:
The isolation, ownership, quota, and billing boundary for one customer.
_Avoid_: Organization, project, tenant account

**Tenant**:
The isolation property of a Workspace, used only when discussing security or data partitioning.
_Avoid_: A second resource beside Workspace

**Workspace Invitation**:
A time-limited request for a User to receive a Workspace Role Binding.
_Avoid_: Platform invitation, membership email
