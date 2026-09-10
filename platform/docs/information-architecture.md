# Console Information Architecture

The product has three operating areas. Authority controls whether an area is visible; changing areas does not change the User's identity or silently impersonate another role.

## Workspace Console

URL scope: `/w/:workspaceSlug`

The top bar contains the product entry, Workspace switcher, actionable notifications, and User menu. It does not contain global search or a Region switcher. Region is a property of a resource and appears in create flows, filters, and resource details.

Primary navigation:

- Instances
- Backups
- Operations
- Billing
- Members
- Workspace settings

The Workspace opens on Instances. Navigation does not display decorative resource counts.

## Platform Console

URL scope: `/platform`

Platform Operators enter this area through an operating-area item in the User menu. The header clearly says “Platform”. The Workspace switcher is absent because this area operates across Workspaces.

Primary navigation:

- Workspaces
- Users
- Invitations
- Credit grants
- Price books
- Logical instances
- Regions
- Audit

The Platform Console opens on Regions and surfaces actionable failures without a decorative dashboard.

## Region Operations

URL scope: `/platform/regions/:regionId`

Region Operations is nested under the Platform Console. A Region selector is local to the Region page header. Its navigation focuses on execution:

- Health
- Nodes
- Deployments
- Tasks
- Capacity
- Storage
- Monitoring
- Region settings

A Regional Deployment links to its global Logical Instance and Workspace as related context. Operators return to the Platform Console to edit global ownership, purchase, configuration, or desired state.

## Account settings

URL scope: `/account`

Locale, theme (`light`, `dark`, `system`), time zone, profile, identities, and sessions belong to the User. Locale strings come from message catalogs; pages do not branch on language or mix languages in one rendered interface.
