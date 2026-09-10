# Backend

This directory will contain the new Go platform implementation with three deployable binaries:

- `control-plane`
- `region-controller`
- `node-agent`

Each bounded context owns its schema and exposes a small module interface. Cross-context access uses module interfaces or versioned events, never another module's repository. Production SQL must not use JOIN; compose bounded batch results in Go or maintain an asynchronous read model.

The legacy backend is not a dependency. Game Provider and Runtime Provider code can be migrated later through adapters after their new interfaces have contract tests.

## Identity and authorization

The rebaseline access API is enabled when the Control Plane has a global PostgreSQL URL and `GAMEPANEL_GITHUB_CLIENT_ID` is set. It requires:

- `GAMEPANEL_GITHUB_CLIENT_ID`
- `GAMEPANEL_GITHUB_CLIENT_SECRET`
- `GAMEPANEL_GITHUB_REDIRECT_URL`
- `GAMEPANEL_TOTP_KEY_BASE64`, a base64-encoded 32-byte AES key

Session cookies are Secure by default. `GAMEPANEL_INSECURE_COOKIES=true` is only for local HTTP development. Apply global migrations in numeric order before enabling the access API.

Authentication data and authorization data remain separate. The only persisted authorization policy table is `authorization_role_bindings`; built-in roles, typed actions, and permission sets live in Go. Route filters authenticate, load a resource's authoritative Scope in one bounded query, and evaluate all decisions in memory.
