# 7 Days to Die vanilla runtime

Linux amd64 only. SteamCMD installs dedicated server app `294420` into the
image. The first container start seeds the instance directory; subsequent starts
do not download or validate game files. Explicit in-panel updates use SteamCMD.

Persistent layout:

- `userdata/Saves`: world and player progress
- `userdata/GeneratedWorlds`: randomly generated maps
- `gamepanel-serverconfig.xml`: panel-managed properties
- `gamepanel-active.xml`: managed properties merged into the installed game's defaults
- `steamapps/appmanifest_294420.acf`: installed Steam Build ID

Keep the whole `userdata` directory when migrating or backing up a server.
Backups intentionally exclude installed game files. Stop the server before
restoring a backup. Do not change the save name, map type or seed to continue an
existing world; those values identify the save.

The first version requires a stopped server for manual backups; scheduled live
backups are not offered until a confirmed save-command path is implemented.
Restores replace `userdata` after extracting successfully, so world regions
created after the backup do not remain mixed into the restored world. Existing
world identity is read-only in the panel to avoid accidentally selecting a new
save. Explicit updates require 25 GiB free disk space before and after backup.

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

The runtime tag identifies the tested image. Per-instance game files and their
Steam Build ID are managed separately. Rebuilding an image against Steam's public
branch must be tested before publishing a new immutable tag.

References:
- https://community.thefunpimps.com/threads/v3-0-dead-hot-summer-dev-diary.46879/page-52
- https://developer.valvesoftware.com/wiki/7_Days_to_Die_Dedicated_Server
