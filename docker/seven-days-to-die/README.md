# 7 Days to Die vanilla runtime

Linux amd64 only. SteamCMD installs dedicated server app `294420` into the
image. New instances run the programs directly from `/opt/seven-days`; startup
does not copy the program directory, download files or validate them with Steam.
Each container shares the image layers and keeps its own writable data.

Persistent layout:

- `userdata/Saves`: world and player progress
- `userdata/GeneratedWorlds`: randomly generated maps
- `userdata/Mods`: instance mods; new instances also provide `Mods` as a compatibility symlink
- `.runtime-home`: small Unity/EOS/Steam caches, isolated from the image
- `gamepanel-serverconfig.xml`: panel-managed properties
- `gamepanel-active.xml`: managed properties merged into the installed game's defaults
- `steamapps/appmanifest_294420.acf`: active Steam Build ID metadata, copied without program files
- `.gamepanel-shared-runtime`: records image-backed initialization

Keep the whole `userdata` directory when migrating or backing up a server.
Backups intentionally exclude installed game files. Stop the server before
restoring a backup. Do not change the save name, map type or seed to continue an
existing world; those values identify the save.

The first version requires a stopped server for manual backups; scheduled live
backups are not offered until a confirmed save-command path is implemented.
Restores replace `userdata` after extracting successfully, so world regions
created after the backup do not remain mixed into the restored world. Existing
world identity is read-only in the panel to avoid accidentally selecting a new
save.

Existing instances with local game programs continue using them; startup never
removes those programs or migrates their mods automatically. Incomplete legacy
installations fail with a repair message rather than silently switching versions.
For an image-backed instance, move an existing real `Mods` directory into
`userdata/Mods` before starting; the runtime does not move or discard it for you.
The game's `UserDataFolder` remains `/data/userdata` so existing saves retain the
same location.

Normal starts require a minimum 2 GiB free operating reserve on the data
filesystem. This is not an estimate of total world size: generated maps, saves,
mods and logs need additional space as they grow.

Explicit in-panel Steam updates remain available and require 25 GiB free space
before and after backup. An image-backed instance receives a writable program
copy only when this update is requested. The runtime switches to that copy only
after the updater verifies the installed build and atomically writes
`.gamepanel-update-complete`. A partial first update cannot select unfinished
programs on the next start. Existing local installations keep their current
update path.

Networking: publish the selected base port over TCP and UDP, plus base+1 and
base+2 over UDP. Forward the same range in the host firewall/router. Remote
nodes must expose these ports directly; a single-port tunnel is insufficient.

3.x gameplay rules are supplied using `SandboxCode`. Generate it in the same
game version's New Game → Sandbox Options menu. Legacy difficulty, blood moon,
loot and day-length XML properties no longer work. The panel checks code syntax;
only the game can validate its version-specific contents.

The container receives SIGINT on stop so the dedicated server can save and exit.
The first start, especially random world generation, can take several minutes.

Build:

```sh
scripts/build-game-images.sh seven-days-to-die --platform linux/amd64 --load
```

The runtime tag identifies the tested image. Explicitly updated local programs
and their Steam Build ID are managed separately. Rebuilding an image against Steam's public
branch must be tested before publishing a new immutable tag.

Runtime checks:

```sh
python3 -m unittest discover -s docker/seven-days-to-die -p 'test_*.py'
# Optional isolated Docker checks; use a locally cached image, never pull:
GAMEPANEL_SHARED_RUNTIME_IMAGE=smartcat99999/seven-days-to-die:v3.3.0 \
  python3 -m unittest discover -s docker/seven-days-to-die -p test_runtime_integration.py
```

References:
- [Official A20.5 UserDataFolder and Mods announcement](https://store.steampowered.com/news/posts/?appids=251570&enddate=1659373443&feed=steam_community_announcements)
- https://community.thefunpimps.com/threads/v3-0-dead-hot-summer-dev-diary.46879/page-52
- https://developer.valvesoftware.com/wiki/7_Days_to_Die_Dedicated_Server
