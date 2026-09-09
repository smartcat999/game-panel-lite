# Identity

Identity describes the human using GamePanel and account-level settings that follow that human across Workspaces.

## Language

**User**:
A human identity that can authenticate and belong to multiple Workspaces.
_Avoid_: Account, tenant user

**Identity**:
An external or local sign-in identity linked to one User.
_Avoid_: OAuth user

**User Preference**:
The User-owned locale, color theme, time zone, and accessibility preferences.
_Avoid_: Workspace setting

**Platform Operator**:
A User with explicit platform authority outside Workspace membership.
_Avoid_: Super tenant, workspace owner
