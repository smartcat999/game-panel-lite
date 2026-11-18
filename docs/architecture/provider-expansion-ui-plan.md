# Provider Expansion UI Plan

This document captures the next UI and product changes for provider-specific configuration, console/logs, and mod management. The goal is to keep the new controller-style backend architecture while avoiding a giant cross-game common form.

## Sources Checked

- Klei dedicated server setup guide: https://forums.kleientertainment.com/forums/topic/64441-dedicated-server-quick-setup-guide-linux/
- Nexus Mods Palworld directory: https://www.nexusmods.com/games/palworld

## 1. Console and Logs

Current problem:
- The server detail page exposes both `Console` and `Logs`.
- For providers with command support, the console already contains the same live log stream, so the two tabs partially duplicate each other.

UI decision:
- Providers with `consoleCommands=true` show one `Console` tab.
- The console tab contains:
  - live output stream
  - pause / clear controls
  - command input
  - provider-specific quick actions when available
- Providers without command support show one `Logs` tab.
- Logs stay read-only and focused on output inspection.

Why:
- Terraria and tModLoader users need commands and logs in one operational surface.
- DST and Palworld can start with read-only logs until reliable command semantics are implemented.

Implementation status:
- Server detail tabs now hide the read-only `Logs` tab when a provider supports console commands.

## 2. Don't Starve Together Configuration

DST configuration is not a simple flat server config. A real server has:
- cluster-level settings
- master shard settings
- optional cave shard settings
- world generation overrides
- mod setup files

Klei's dedicated server guide relies on downloaded server settings from the Klei account page, and those settings are placed under the user's `DoNotStarveTogether` directory. In GamePanel Lite, the provider should render the same structure into the isolated server data directory.

### UI Layout

Keep the creation flow stable:
- Step 1: Game
- Step 2: Mode
- Step 3: Preset
- Step 4: Game Config
- Step 5: Runtime Resources
- Step 6: Review

Within DST Game Config, use sections instead of a long form:

1. Identity and Access
   - server name
   - cluster name
   - description
   - password
   - Klei cluster token
   - public / LAN / offline mode

2. Gameplay
   - max players
   - game mode: survival, endless, wilderness
   - PvP
   - pause when empty
   - console enabled

3. World Generation
   - world preset first
   - world size
   - branching / loops
   - day cycle
   - seasons
   - weather / lightning / wildfires

4. World Resources
   - grass, saplings, reeds, flowers
   - berries, carrots, mushrooms
   - rocks, flint, gold
   - trees and biome density

5. Creatures and Threats
   - spiders, hounds, beefalo, pigs, merms
   - tentacles, tallbirds, walrus camps
   - hound attacks and season threats

6. Boss and Special Encounters
   - deerclops, bearger, moose/goose, dragonfly-style encounter settings where supported by DST overrides
   - keep these behind an "Advanced world overrides" disclosure because naming and availability can vary by DST version.

7. Caves
   - enable caves
   - cave preset
   - cave resources: lichen, light flowers, mushtrees, ponds
   - cave creatures: bats, spiders, worms, monkeys, slurpers
   - ruins and sinkhole density

8. Advanced Overrides
   - search/filter override rows
   - reset section to preset
   - show changed-only toggle
   - raw preview of generated shard config for debugging, hidden by default

### Data Model

Do not force DST into common fields. Use a provider-owned structure:

```go
type DSTConfig struct {
    Identity DSTIdentityConfig
    Gameplay DSTGameplayConfig
    World    DSTWorldConfig
    Caves    *DSTCaveConfig
    Mods     DSTModConfig
}
```

For overrides:
- curated fields should be typed and localized
- advanced provider-only maps can exist under `World.Overrides` and `Caves.Overrides`
- the API should preserve unknown provider fields but the UI should not surface unknown values as first-class controls

### Interaction Rules

- The default view shows only Identity, Gameplay, and World Preset.
- Selecting "Customize world" expands World Generation, Resources, Creatures, and Boss sections.
- Enabling caves reveals Cave sections.
- Each section shows a compact summary chip, for example `Resources: rich`, `Bosses: default`, `Caves: enabled`.
- Review step shows the summary, not raw Lua.

## 3. DST Workshop Mods and Mod Packs

DST mods are Workshop-ID driven. The provider should manage:
- `dedicated_server_mods_setup.lua`
- shard-level `modoverrides.lua`
- mod enabled state
- mod configuration options when known

UI direction:
- Add a DST tab to the mod catalog.
- Seed a `recommended_dst_mods.json` catalog similar to tModLoader.
- Support mod packs as named lists of Workshop IDs.
- In server detail, the install dialog must show:
  - installed
  - queued to install
  - dependency warning if known
  - restart required

Controller behavior:
- Creating or editing a server only changes desired mods.
- The controller reconcile loop writes mod files, prepares/downloads Workshop content, restarts if required, and records lifecycle progress.
- API handlers should not perform mod installation directly.

## 4. Palworld Mods

Palworld should not be modeled as a Workshop-first provider until an official Workshop flow is verified. The visible public ecosystem is file/collection oriented, with Nexus Mods listing Palworld mods and collections.

UI direction:
- Treat Palworld mods as uploaded or imported assets.
- First supported type: `.pak` asset bundle.
- Later supported types can include script-loader based mods only after the runtime image supports the loader explicitly.

Server detail install flow:
- Select from uploaded Palworld mod assets.
- Show installed / pending / incompatible status.
- Installation target is provider-owned, not user-editable.
- Any mod change marks the server as restart required.

Safety:
- Validate extensions and archive contents.
- Prevent path traversal.
- Keep mod files inside the server data directory.
- Do not expose arbitrary host paths.

## 5. Official Site Copy

The official site should no longer describe GamePanel Lite as only a Terraria panel.

Updated positioning:
- "Self-hosted game server panel"
- "Docker-backed game servers"
- "provider-owned config"
- "controller-style lifecycle"
- "Terraria and tModLoader first-class today"
- "DST and Palworld expanding through provider architecture"

Avoid overclaiming:
- Do not promise one-click Palworld Workshop installation.
- Do not claim complete DST world override coverage until provider schema and renderer are implemented.
- Do not present world/backup flows as primary while those UI surfaces are intentionally hidden.

## Next Implementation Order

1. DST provider schema split
   - identity, gameplay, world, caves, mods
   - typed overrides for the most common world/cave controls

2. DST config renderer
   - cluster config
   - master shard config
   - optional cave shard config
   - `leveldataoverride.lua`

3. DST mod catalog
   - recommended Workshop list
   - mod pack support
   - installed and pending states in dialog

4. Palworld file mod assets
   - upload validation
   - provider-owned install target
   - restart-required lifecycle event

5. UI polish
   - section summaries
   - changed-only view
   - generated config preview behind debug disclosure
