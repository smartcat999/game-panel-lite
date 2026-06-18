# GamePanel Lite Product Roadmap

## Purpose

This roadmap records the next product direction after the Terraria-focused V1 work.

The priority is based on what ordinary self-hosted users can immediately understand and use. The product should feel like a simple game server panel, not a Docker or infrastructure console.

V1 UI polish and existing-flow optimization are intentionally deferred unless they block a new feature.

Development execution plan: `docs/goals/POST_V1_PRODUCT_DEVELOPMENT_PLAN.md`.

## Product Principles

- Prefer features users can name in plain language: login, create a Palworld server, invite friends, manage saves.
- Hide Docker, image, container, and runtime concepts behind game-specific flows.
- Each supported game should have its own friendly configuration page instead of a generic technical form.
- Defaults should be good enough for a small friend group.
- Advanced operations can exist later, but they should not be the first thing a new user sees.

## Priority 0: Core Product Features

### 1. Local Admin Account and Login

Why:
GamePanel Lite can start, stop, delete, and modify game servers. A deployed panel needs basic access protection before it is comfortable to use on a home LAN or public host.

Scope:
- First-run admin account setup.
- Login and logout.
- Change admin password.
- Protect API routes with a logged-in session.
- Keep V1 simple: one local admin account first, no RBAC.

Out of scope:
- OAuth.
- Cloud accounts.
- Team permissions.
- Billing or SaaS tenancy.

### 2. Palworld Support

Why:
Palworld is one of the most useful next games for small friend-group servers. It gives the product a clear reason to exist beyond Terraria.

Scope:
- Create Palworld server.
- Start, stop, restart, delete.
- Join information.
- Save data location.
- Game-specific configuration:
  - Server name.
  - Password.
  - Admin password.
  - Max players.
  - Public or private server setting when supported.
  - Core gameplay rates only if they are safe and easy to explain.
- Save snapshot and restore through the existing world/backup concepts where applicable.

Product direction:
Users should select "Palworld", fill a short form, and start the server. They should not need to know the image name or startup command.

### 3. Don't Starve Together Support

Why:
Don't Starve Together is popular with small groups and benefits from a friendly setup flow because server tokens, caves, and world presets are confusing for new users.

Scope:
- Create DST server.
- Cluster token setup in a guided field.
- Master world and optional caves.
- Server name, password, max players.
- World preset selection.
- Mod support through Workshop IDs in a game-specific flow.
- Join information and save management.

Product direction:
The setup should explain only the minimum needed to run a private friend server.

### 4. Minecraft Java Support

Why:
Minecraft is the most expected game for a self-hosted game panel. Supporting it makes GamePanel Lite useful to a much larger group of users.

Scope:
- Create Minecraft Java server.
- Version selection.
- Vanilla first.
- Server name, max players, game mode, difficulty, whitelist toggle, online-mode toggle.
- EULA acceptance handled explicitly during creation.
- Join information as `host:port`.
- World save management.

Later:
- Paper / Fabric / Forge profiles.
- Plugin and mod management.

### 5. Game-Specific Create Server Flows

Why:
As more games are added, a generic server form becomes confusing. New users think in terms of games, players, password, and saves.

Scope:
- Game selection first.
- Game-specific setup form.
- Recommended defaults.
- Friendly field labels and help text.
- Review step that shows how friends will join.

Product direction:
The creation flow should feel like creating a game lobby, not configuring a container.

## Priority 1: High-Value User Features

### 6. Game Save Management

Why:
Users care about game progress. "Save" or "world" is easier to understand than data directories, volumes, or archives.

Scope:
- Show saves per server.
- Create save snapshot.
- Restore save snapshot.
- Download save snapshot.
- Use game-specific naming:
  - Terraria: world.
  - Palworld: save.
  - DST: cluster save.
  - Minecraft: world.

### 7. Player Management

Why:
Small server owners frequently need to see who is online and remove disruptive players.

Scope:
- Online player list when the game supports it.
- Kick player when supported.
- Ban player when supported.
- Whitelist or admin list when supported.

Product direction:
Only show actions that work for the selected game. Do not expose unavailable controls.

### 8. Friend Invite Experience

Why:
The most common task after starting a server is inviting friends.

Scope:
- Copy join address.
- Copy password when needed.
- Copy full invite text.
- Support LAN address and configured public address.
- Game-specific join instructions.

Examples:
- Minecraft: `host:port`.
- Terraria: IP, port, password.
- Palworld and DST: game-specific join guidance.

### 9. Game Version Selection

Why:
Users recognize game versions. They should not need to think about Docker tags.

Scope:
- Create-time version selection for each supported game.
- Recommended version marked clearly.
- Show current server game version.
- Keep image mapping internal.

### 10. Game Library Presentation

Why:
When multiple games are supported, the product should feel like a game server launcher rather than an admin table.

Scope:
- Game cards with cover art.
- Clear supported game list.
- Per-game server counts.
- Create server from a selected game card.

## Priority 2: Expansion Features

### 11. Mobile-Friendly Controls

Why:
Users often want to start, stop, or share a server from a phone.

Scope:
- Responsive server list.
- Start, stop, restart from mobile.
- Copy invite from mobile.
- View basic server status.

### 12. Configuration Presets

Why:
Users may want a fast way to reuse common setup choices such as max players, difficulty, resource limits, and mod pack selections.

This must not duplicate world snapshots. A world snapshot already represents a reusable playable state: save data plus the configuration needed to create another server from that world.

Scope:
- Save reusable configuration only.
- Create server with a preset that pre-fills the create flow.
- Include game, provider, version, friendly config values, resource limits, and selected mod pack where applicable.
- Exclude world/save data.
- Exclude runtime state, backups, logs, container IDs, and secrets.

Product direction:
Prioritize world snapshots for "make another server like this playable server." Only add configuration presets if users still need reusable non-world setup after the multi-game create flow matures.

### 13. Shareable Server Page

Why:
Server owners want an easy page to send to friends.

Scope:
- Read-only server join page.
- Server name, game, status, player count, join instructions.
- Optional password visibility controlled by the admin.

### 14. More Games

Candidate games:
- Valheim.
- Project Zomboid.
- Enshrouded.
- Satisfactory.
- Core Keeper.

Selection criteria:
- Popular with small groups.
- Has a stable dedicated server.
- Can run well in Docker.
- Has clear join information and save data.

## Deferred for Now

These may be useful later, but they are not the next product focus:

- Detailed monitoring charts.
- Backup retention policies.
- Generic file manager.
- Full audit logs.
- Webhook or Discord notifications.
- Multi-user permissions.
- Plugin marketplace.
- Advanced Docker runtime settings.

They should not block the user-facing feature roadmap above.

## Recommended Delivery Order

1. Local admin account and login.
2. Palworld provider.
3. Multi-game create server flow.
4. Don't Starve Together provider.
5. Minecraft Java provider.
6. Game save management across supported games.
7. Player management by game capability.
8. Friend invite experience.
9. Game version selection.
10. Game library presentation.
