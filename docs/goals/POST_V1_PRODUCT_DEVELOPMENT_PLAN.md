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
11. Mobile-friendly controls.
12. Configuration presets.
13. Shareable server page.
14. More games.

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

### Milestone F: Everyday Use Expansion

Goal:
Add practical features that make the product easier to use repeatedly after the first server is running.

Included goals:
- Goal 11: Mobile-Friendly Controls.
- Goal 12: Configuration Presets.
- Goal 13: Shareable Server Page.

Exit criteria:
- A user can start, stop, restart, and share a server comfortably from a phone.
- A user can reuse common non-world setup choices without confusing them with world snapshots.
- Friends can open a simple read-only page with join information.

### Milestone G: More Game Coverage

Goal:
Grow GamePanel Lite into a useful small game server launcher beyond the initial Terraria, Palworld, DST, and Minecraft set.

Included goals:
- Goal 14: More Games.

Exit criteria:
- New games can be added through provider packages without leaking Docker concepts into the UI.
- Each added game has game-specific labels, join information, save paths, and a minimal friendly create flow.

## Dependency Notes

- Goal 1 can start immediately.
- Goal 2 should land before adding multiple new games.
- Goal 3 can start after or alongside Goal 2 if provider seams are kept narrow.
- Goal 4 should be completed before adding DST and Minecraft UI flows at full quality.
- Goal 7 depends on at least one non-Terraria provider to prove cross-game naming.
- Goal 8 depends on provider capability metadata from Goal 2.
- Goal 9 depends on provider join-info contracts from Goal 2.
- Goal 10 should land after at least two games exist, so the game library has real content.
- Goal 11 depends on the stable server list/detail action model from Goals 1-10.
- Goal 12 depends on provider-specific config payloads from Goal 2, the create flow from Goal 4, and clear separation from world snapshots.
- Goal 13 depends on join-info contracts and public host settings from Goal 9.
- Goal 14 depends on the provider foundation, game-specific create flow, and cross-game save conventions.

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

## Goal 11: Mobile-Friendly Controls

### User Value

A user can start, stop, restart, and share a server from a phone without opening a desktop-sized admin interface.

### Scope

- Responsive server list.
- Mobile-friendly server detail actions.
- Copy invite from mobile.
- View basic server status.
- Keep advanced management on desktop-friendly screens where needed.

### Frontend Tasks

1. Audit the server list, server detail header, join info panel, and primary action buttons on common mobile widths.
2. Add mobile action grouping for:
   - start
   - stop
   - restart
   - copy invite
3. Keep destructive actions behind confirmation.
4. Ensure server cards, tabs, and right-side panels collapse into a readable single-column layout.
5. Verify touch targets are comfortable and text does not overflow.

### Backend Tasks

No new backend domain work is expected. Use existing lifecycle and join-info APIs.

### Acceptance Criteria

- A user can start, stop, restart, and copy invite text on a phone-width viewport.
- The server list remains scannable on mobile.
- Destructive actions still require confirmation.
- No desktop-only hover interaction is required for critical actions.

### Verification

```bash
pnpm lint
pnpm typecheck
pnpm build
```

Manual verification:

- Test server list and server detail at 390px, 430px, and tablet width.
- Confirm copy invite works on mobile.

### Suggested Commit

`feat: add mobile server controls`

## Goal 12: Configuration Presets

### User Value

A user can reuse common setup choices without re-entering the same game settings, while still understanding that world snapshots are the way to reuse an actual played world/save.

### Scope

- Save reusable configuration only.
- Pre-fill the create-server flow from a preset.
- Include:
  - game
  - provider
  - version
  - friendly config values
  - resource limits
  - selected mod pack when applicable
- Do not include world/save data.
- Do not include server-specific runtime state, container IDs, logs, backups, or secrets.
- Keep "create from world/snapshot" as the product path for recreating a playable server state.

### Backend Tasks

1. Add configuration preset model:
   - id
   - name
   - gameKey
   - providerKey
   - version
   - config payload
   - resource limits
   - mod pack reference when applicable
   - createdAt
   - updatedAt
2. Add APIs:
   - `GET /api/config-presets`
   - `POST /api/config-presets`
   - `GET /api/config-presets/{id}`
   - `PUT /api/config-presets/{id}`
   - `DELETE /api/config-presets/{id}`
3. Validate preset payloads through the selected provider.
4. Ensure secrets are omitted and re-entered during create.
5. Add tests for preset create, update, validation, and secret stripping.

### Frontend Tasks

1. Add preset selection as an optional start point in the create-server flow.
2. Add "Save as Preset" only for configuration fields, not world/save state.
3. Let users review and override preset fields before final creation.
4. Show clear labels for which game/provider a preset belongs to.
5. Keep world snapshot creation and create-from-world separate in the UI.

### Acceptance Criteria

- User can save reusable non-world server configuration as a preset.
- User can start server creation from a preset and change fields before creating.
- Provider validation still runs after applying a preset.
- World/save data is never copied by configuration presets.
- UI copy clearly distinguishes configuration presets from world snapshots.

### Verification

```bash
go test ./...
go vet ./...
pnpm lint
pnpm typecheck
pnpm build
```

### Suggested Commit

`feat: add configuration presets`

## Goal 13: Shareable Server Page

### User Value

A server owner can send friends a simple link that explains how to join without giving them access to the admin panel.

### Scope

- Read-only server join page.
- Server name, game, status, player count when available.
- Join address and instructions.
- Optional password visibility controlled by the admin.
- No lifecycle controls or admin actions.

### Backend Tasks

1. Add share token or public slug model for a server.
2. Add APIs:
   - `POST /api/servers/{id}/share`
   - `DELETE /api/servers/{id}/share`
   - `GET /api/public/servers/{shareToken}`
3. Public endpoint returns only safe join information.
4. Add option to include or hide password in the shared response.
5. Add tests to ensure public pages do not expose admin-only data.

### Frontend Tasks

1. Add share controls in the invite panel.
2. Add read-only public server page.
3. Add copy share link action.
4. Add password visibility toggle for the share page.
5. Show a clear disabled/unpublished state when sharing is off.

### Acceptance Criteria

- Admin can enable and disable a share page.
- Friend can open the share link without logging in.
- Share page shows only join-safe information.
- Password is hidden unless the admin explicitly enables it.

### Verification

```bash
go test ./...
go vet ./...
pnpm lint
pnpm typecheck
pnpm build
```

### Suggested Commit

`feat: add shareable server page`

## Goal 14: More Games

### User Value

Users can run more popular friend-group games through the same simple GamePanel Lite flow.

### Candidate Order

1. Valheim.
2. Project Zomboid.
3. Enshrouded.
4. Satisfactory.
5. Core Keeper.

### Selection Criteria

- Popular with small groups.
- Dedicated server is stable enough for self-hosting.
- Docker runtime is practical.
- Join information is clear.
- Save data can be isolated under the server instance directory.
- Configuration can be reduced to a friendly first-run form.

### Standard Provider Tasks Per Game

1. Add provider package under `apps/api/internal/provider/<game>`.
2. Add game metadata and catalog entry.
3. Add version metadata with a recommended default.
4. Add config schema with friendly fields.
5. Add provider validation.
6. Add config renderer.
7. Add runtime options.
8. Add join-info provider behavior.
9. Add save metadata.
10. Add player management only when reliable.
11. Add provider tests and HTTP create/runtime tests.
12. Add frontend create flow coverage through provider metadata.

### Acceptance Criteria Per Game

- User can select the game from the game library.
- User can create a server with recommended defaults.
- User can start, stop, restart, and delete the server.
- Join information is clear for friends.
- Save data location is known and snapshot-ready.
- Unsupported actions are hidden.

### Verification

```bash
go test ./...
go vet ./...
pnpm lint
pnpm typecheck
pnpm build
```

Manual verification per game:

- Build or pull the runtime image.
- Create a server.
- Start the server.
- Confirm container remains running.
- Confirm save directory exists.
- Confirm join information matches the game.

### Suggested Commit Pattern

`feat: add <game> provider`

`feat: add <game> runtime support`

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
