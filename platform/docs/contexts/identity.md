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

**Credential**:
A local password or second factor used by an Identity to authenticate.
_Avoid_: User secret
