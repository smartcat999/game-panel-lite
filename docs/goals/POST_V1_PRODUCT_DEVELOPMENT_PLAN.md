# GamePanel Lite Post-V1 Product Development Plan

## Purpose

This plan turns `docs/product/product-roadmap.md` into executable development goals.

The focus is new product functionality that ordinary users can understand and use:

- Local admin login.
- More popular games.
- Game-specific creation flows.
- Save management.
- Player management.
- Friend invite features.

V1 Terraria UI polish and existing-flow optimization are deferred unless they block these goals.

## Product Direction

GamePanel Lite should feel like a friendly game server launcher for small friend groups.

Users should think in terms of:

- "Create a Palworld server"
- "Invite my friends"
- "Manage my save"
- "See who is online"

Users should not need to understand:

- Docker images
- Container names
- Startup commands
- Bind mounts
- Runtime internals

## Technical Direction

Keep the existing architecture, but make it multi-game ready:

- Add each game through `GameProvider`.
- Keep Docker-specific behavior inside `RuntimeAdapter`.
- Keep game-specific config rendering inside provider packages.
- Keep shared server lifecycle APIs generic.
- Add game-specific UI only where it improves user clarity.

## Delivery Order

1. Local admin account and login.
2. Multi-game provider foundation.
3. Palworld provider.
4. Game-specific create server flow.
5. Don't Starve Together provider.
6. Minecraft Java provider.
7. Cross-game save management.
8. Player management by provider capability.
9. Friend invite pages and copy text.
10. Game library presentation.

## Milestones

### Milestone A: Safe Single-User Product

Goal:
Make the panel safe enough for a normal user to run at home or on a small remote machine.

Included goals:
- Goal 1: Local Admin Account and Login.

Exit criteria:
- Fresh installs require admin setup.
- Existing app pages and APIs are protected after setup.
- A user can login, logout, and change password.

### Milestone B: Multi-Game Foundation

Goal:
Prepare the product to support more games without hardcoding every screen around Terraria.

Included goals:
- Goal 2: Multi-Game Provider Foundation.
- Goal 4: Game-Specific Create Server Flow.

Exit criteria:
- Backend exposes game catalog and provider capabilities.
- Create-server flow is game-first.
- Terraria remains compatible.
- At least one non-Terraria provider can be rendered without custom branching.

### Milestone C: First New Game

Goal:
Prove the multi-game direction with a popular game that users recognize immediately.

Included goals:
- Goal 3: Palworld Provider.

Exit criteria:
- User can create, start, stop, delete, and invite friends to a Palworld server.
- Palworld uses game-specific labels and config.
- Palworld save data is isolated under the server instance directory.

### Milestone D: Broader Game Coverage

Goal:
Expand beyond one additional game and prove the provider model across different server setup patterns.

Included goals:
- Goal 5: Don't Starve Together Provider.
- Goal 6: Minecraft Java Provider.

Exit criteria:
- DST token/caves setup is handled in a friendly flow.
- Minecraft EULA and version selection are handled in a friendly flow.
- Both games expose clear join information.

### Milestone E: Cross-Game User Features

Goal:
Turn supported games into a coherent product experience instead of separate provider demos.

Included goals:
- Goal 7: Cross-Game Save Management.
- Goal 8: Player Management by Provider Capability.
- Goal 9: Friend Invite Experience.
- Goal 10: Game Version Selection and Library Presentation.

Exit criteria:
- Saves, players, invite text, versions, and game library presentation work consistently across supported games.
- Unsupported provider capabilities are hidden instead of shown as broken controls.

## Dependency Notes

- Goal 1 can start immediately.
- Goal 2 should land before adding multiple new games.
- Goal 3 can start after or alongside Goal 2 if provider seams are kept narrow.
- Goal 4 should be completed before adding DST and Minecraft UI flows at full quality.
- Goal 7 depends on at least one non-Terraria provider to prove cross-game naming.
- Goal 8 depends on provider capability metadata from Goal 2.
- Goal 9 depends on provider join-info contracts from Goal 2.
- Goal 10 should land after at least two games exist, so the game library has real content.

## Development Rules

- Do not expose Docker image names or startup commands in user-facing creation flows.
- Do not add RBAC, OAuth, SaaS tenancy, or billing while implementing the local admin goal.
- Do not force all games through Terraria's world/mod/console vocabulary.
- Do not show unsupported actions as disabled clutter; hide them unless the disabled state teaches the user something important.
- Do not make V1 UI polish a prerequisite for post-V1 feature work.
- Preserve Terraria behavior while adding new providers.

## Goal 1: Local Admin Account and Login

### User Value

A user can safely deploy GamePanel Lite on a home LAN or remote machine without everyone on the network being able to control servers.

### Scope

- First-run admin setup.
- Login.
- Logout.
- Change password.
- Session persistence.
- Protect all non-health API routes.
- Frontend route guard for app pages.

### Backend Tasks

1. Add account model:
   - `id`
   - `username`
   - `passwordHash`
   - `createdAt`
   - `updatedAt`
2. Add session model:
   - `id`
   - `accountId`
   - `tokenHash`
   - `expiresAt`
   - `createdAt`
3. Add password hashing.
4. Add auth middleware.
5. Add APIs:
   - `GET /api/auth/bootstrap`
   - `POST /api/auth/setup`
   - `POST /api/auth/login`
   - `POST /api/auth/logout`
   - `GET /api/auth/me`
   - `POST /api/auth/password`
6. Keep `GET /healthz` public.
7. Return `401` for protected API routes when unauthenticated.

### Frontend Tasks

1. Add setup page.
2. Add login page.
3. Add logout action.
4. Add password change form in Settings.
5. Add route guard:
   - unauthenticated users go to login.
   - uninitialized instance goes to setup.
6. Hide app shell until auth state is known.

### Acceptance Criteria

- Fresh install opens setup first.
- User can create the first admin account.
- User can login and access dashboard.
- User can logout.
- API routes reject unauthenticated requests.
- User can change password and login with the new password.

### Verification

```bash
go test ./...
pnpm lint
pnpm typecheck
pnpm build
```

### Suggested Commit

`feat: add local admin login`

## Goal 2: Multi-Game Provider Foundation

### User Value

The product can add new games without turning the codebase into Terraria-specific branching.

### Scope

- Provider capabilities.
- Generic game metadata.
- Generic server config envelope.
- Game-specific config payloads.
- Game-specific join info.

### Backend Tasks

1. Extend `GameProvider` with capability metadata:
   - supports console commands
   - supports player list
   - supports kick
   - supports ban
   - supports save snapshots
   - supports mods
   - supports versions
2. Add game catalog API:
   - `GET /api/games`
   - `GET /api/games/{gameKey}`
3. Add provider version API:
   - `GET /api/games/{gameKey}/versions`
4. Add provider config schema metadata:
   - field name
   - label
   - type
   - default
   - required
   - options
5. Keep provider runtime options responsible for:
   - image selection
   - env variables
   - command
   - data mounts
   - generated config files

### Frontend Tasks

1. Add game catalog data loader.
2. Replace hardcoded Terraria-only creation assumptions with provider metadata.
3. Keep existing Terraria flow working.
4. Display provider capabilities in server detail only when supported.

### Acceptance Criteria

- Terraria Vanilla and tModLoader still work.
- Game list is fetched from the backend.
- Create-server flow can render at least one non-Terraria provider stub without special-case UI crashes.
- Unsupported tabs/actions are hidden by provider capability.

### Verification

```bash
go test ./...
pnpm lint
pnpm typecheck
pnpm build
```

### Suggested Commit

`feat: add multi-game provider foundation`

## Goal 3: Palworld Provider

### User Value

A user can create and run a Palworld server without knowing Docker, images, or startup arguments.

### Scope

- Create Palworld server.
- Start, stop, restart, delete.
- Show join information.
- Manage save data location.
- Palworld-specific configuration.

### Backend Tasks

1. Add `internal/provider/palworld`.
2. Add Palworld config model:
   - server name
   - server password
   - admin password
   - max players
   - public/private setting when supported
   - selected safe gameplay rates if needed
3. Add Palworld config renderer.
4. Add Palworld runtime options.
5. Add Palworld provider registration.
6. Add save file paths for Palworld.
7. Add provider tests:
   - default config
   - validation
   - config rendering
   - runtime options

### Frontend Tasks

1. Add Palworld game card.
2. Add Palworld create form.
3. Add Palworld server detail labels.
4. Add Palworld join info copy text.
5. Hide Terraria-only tabs and commands.

### Acceptance Criteria

- User can select Palworld in create server.
- User can create a Palworld server with recommended defaults.
- User can start and stop the server.
- User can copy friend join info.
- Palworld save data is stored under the server instance directory.

### Verification

```bash
go test ./...
pnpm lint
pnpm typecheck
pnpm build
```

Manual verification:

- Create a Palworld server.
- Start it with Docker.
- Confirm container remains running.
- Confirm save directory exists.
- Confirm join info displays expected host and port.

### Suggested Commit

`feat: add palworld provider`

## Goal 4: Game-Specific Create Server Flow

### User Value

Users can create servers by selecting a game and answering friendly game-specific questions instead of filling a generic technical form.

### Scope

- Game-first creation.
- Provider-specific forms.
- Recommended defaults.
- Friendly labels.
- Review step with join information.

### Frontend Tasks

1. Redesign create server flow around:
   - choose game
   - choose game mode/provider if needed
   - fill game-specific setup
   - review and create
2. Use provider metadata where possible.
3. Use custom UI for fields that need better explanation.
4. Keep Terraria create flow compatible.
5. Add Palworld-specific copy.
6. Add empty state for unsupported provider features.

### Backend Tasks

1. Ensure create API accepts provider-specific config.
2. Validate config through the selected provider.
3. Return generated join info after create.

### Acceptance Criteria

- Terraria server creation still works.
- Palworld server creation works.
- The form does not show irrelevant fields for a selected game.
- The review step explains how friends will join.

### Verification

```bash
go test ./...
pnpm lint
pnpm typecheck
pnpm build
```

### Suggested Commit

`feat: add game-specific server creation`

## Goal 5: Don't Starve Together Provider

### User Value

A user can create a private Don't Starve Together server with the minimum fields needed to play with friends.

### Scope

- Create DST server.
- Cluster token setup.
- Master world.
- Optional caves.
- Server name.
- Password.
- Max players.
- World preset.
- Workshop mod IDs in a DST-specific flow.

### Backend Tasks

1. Add `internal/provider/dst`.
2. Add DST config model:
   - cluster token
   - server name
   - password
   - max players
   - world preset
   - caves enabled
   - workshop mod IDs
3. Render DST cluster and world config files.
4. Add runtime options.
5. Add save paths.
6. Add provider tests.

### Frontend Tasks

1. Add DST game card.
2. Add DST create form.
3. Add guidance for cluster token.
4. Add caves toggle.
5. Add DST join info copy text.

### Acceptance Criteria

- User can create a DST server.
- User can provide a cluster token.
- User can enable or disable caves.
- User can start and stop the server.
- Join information is clear.

### Verification

```bash
go test ./...
pnpm lint
pnpm typecheck
pnpm build
```

Manual verification:

- Create DST server.
- Start it.
- Confirm generated config files exist.
- Confirm container remains running.

### Suggested Commit

`feat: add dont starve together provider`

## Goal 6: Minecraft Java Provider

### User Value

A user can create the most expected self-hosted game server type with a simple setup flow.

### Scope

- Minecraft Java Vanilla server.
- Version selection.
- EULA acceptance.
- Server name.
- Max players.
- Game mode.
- Difficulty.
- Whitelist toggle.
- Online-mode toggle.
- Join info as `host:port`.

### Backend Tasks

1. Add `internal/provider/minecraft`.
2. Add Minecraft config model.
3. Render `server.properties`.
4. Store EULA acceptance.
5. Add runtime options.
6. Add save path support for `world`.
7. Add provider tests.

### Frontend Tasks

1. Add Minecraft game card.
2. Add Minecraft create form.
3. Add EULA acceptance step or checkbox.
4. Add Minecraft join info copy text.
5. Add server detail labels.

### Acceptance Criteria

- User can create a Minecraft Java server.
- User must explicitly accept the EULA before creating.
- User can start and stop the server.
- Join info displays as `host:port`.
- World files are stored under the server instance directory.

### Verification

```bash
go test ./...
pnpm lint
pnpm typecheck
pnpm build
```

Manual verification:

- Create Minecraft Java server.
- Start it.
- Confirm container remains running.
- Confirm world directory exists.

### Suggested Commit

`feat: add minecraft java provider`

## Goal 7: Cross-Game Save Management

### User Value

Users can protect and move their game progress using names they understand: world, save, or cluster save.

### Scope

- Show saves per server.
- Create save snapshot.
- Restore save snapshot.
- Download save snapshot.
- Use game-specific naming.

### Backend Tasks

1. Extend provider save metadata:
   - display name
   - runtime paths
   - snapshot paths
2. Generalize world/backup concepts into save snapshots where needed.
3. Keep Terraria world behavior working.
4. Add APIs:
   - `GET /api/servers/{id}/saves`
   - `POST /api/servers/{id}/saves/snapshot`
   - `POST /api/servers/{id}/saves/{saveId}/restore`
   - `GET /api/servers/{id}/saves/{saveId}/download`
5. Preserve existing Terraria world routes during transition.

### Frontend Tasks

1. Add game-aware Save tab labels.
2. Use:
   - Terraria: worlds
   - Palworld: saves
   - DST: cluster saves
   - Minecraft: worlds
3. Add save snapshot list.
4. Add restore confirmation.

### Acceptance Criteria

- Terraria world management still works.
- Palworld save snapshots work.
- DST cluster save snapshots work.
- Minecraft world snapshots work.
- UI language matches the selected game.

### Verification

```bash
go test ./...
pnpm lint
pnpm typecheck
pnpm build
```

### Suggested Commit

`feat: add cross-game save management`

## Goal 8: Player Management by Provider Capability

### User Value

Server owners can see and manage players when a game supports it.

### Scope

- Online player list.
- Kick player.
- Ban player.
- Whitelist or admin list when supported.

### Backend Tasks

1. Extend provider player capability:
   - list players command/API
   - kick command/API
   - ban command/API
   - whitelist support
2. Add generic APIs:
   - `GET /api/servers/{id}/players`
   - `POST /api/servers/{id}/players/{player}/kick`
   - `POST /api/servers/{id}/players/{player}/ban`
3. Only enable APIs when provider supports the action.
4. Add provider tests for command generation and parsing.

### Frontend Tasks

1. Add Players panel on server detail.
2. Show supported actions only.
3. Add confirmation for kick and ban.
4. Add unsupported state when a game cannot list players.

### Acceptance Criteria

- Player panel is visible only where useful.
- Unsupported games do not show broken controls.
- Kick and ban require confirmation.

### Verification

```bash
go test ./...
pnpm lint
pnpm typecheck
pnpm build
```

### Suggested Commit

`feat: add provider-based player management`

## Goal 9: Friend Invite Experience

### User Value

After a server starts, the owner can quickly send friends clear joining instructions.

### Scope

- Copy join address.
- Copy password when needed.
- Copy full invite text.
- LAN address.
- Configured public address.
- Game-specific instructions.

### Backend Tasks

1. Add configurable public host setting.
2. Add join-info provider contract:
   - fields
   - display labels
   - copy text
   - join instructions
3. Add `GET /api/servers/{id}/join-info`.

### Frontend Tasks

1. Add friend invite panel.
2. Add copy full invite text.
3. Add copy address.
4. Add copy password when present.
5. Add game-specific instructions.
6. Add shareable read-only server page if enabled.

### Acceptance Criteria

- Terraria invite text is clear.
- Palworld invite text is clear.
- DST invite text is clear.
- Minecraft invite text is clear.
- Password is not shown where none exists.

### Verification

```bash
go test ./...
pnpm lint
pnpm typecheck
pnpm build
```

### Suggested Commit

`feat: add friend invite flows`

## Goal 10: Game Version Selection and Library Presentation

### User Value

Users can choose recognizable game versions and browse supported games like a small game library.

### Scope

- Game cards.
- Supported game list.
- Per-game server counts.
- Recommended version labels.
- Current server version display.

### Backend Tasks

1. Add provider version metadata:
   - version id
   - display name
   - recommended flag
   - deprecated flag when needed
2. Add game metadata:
   - title
   - summary
   - cover image key
   - supported features
3. Add per-game server counts in game catalog API.

### Frontend Tasks

1. Add game library page or dashboard section.
2. Add game cards with cover art.
3. Add create-from-game action.
4. Add version selector with recommended default.
5. Show version on server cards and details.

### Acceptance Criteria

- User can browse supported games.
- User can create a server from a game card.
- User can select a version without seeing image names.
- Existing server cards show game and version clearly.

### Verification

```bash
go test ./...
pnpm lint
pnpm typecheck
pnpm build
```

### Suggested Commit

`feat: add game library and version selection`

## Deferred Items

Do not prioritize these until the roadmap goals above are underway:

- V1 visual polish.
- Detailed monitoring charts.
- Backup retention policies.
- Generic file manager.
- Full audit logs.
- Webhook or Discord notifications.
- Multi-user permissions.
- Plugin marketplace.
- Advanced Docker runtime settings.

## Progress Tracking

When work starts, create or update:

- `docs/goals/POST_V1_PROGRESS.md`

Track:

- current goal
- completed tasks
- verification commands
- manual testing notes
- known limitations
- next recommended work
