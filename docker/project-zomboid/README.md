# Project Zomboid 42.21 candidate runtime

This runtime targets the official **Steam dedicated server App 380870, public,
Build 25485538 / Core 42.21**, Linux amd64. It does not support Workshop/modded
installs. No image has been published or selected as a production default.

## Image layers and local build

`Dockerfile.base` contains Debian dependencies and the complete official game,
including its bundled JRE. It verifies the installed App/Build/StateFlags and
required official files. Supply a complete Steam installation via the named
`gamefiles` context; base builds never download or update the game implicitly.

`Dockerfile` adds only the entrypoint and supervisor. Changing startup helpers
does not reinstall/copy the game into the data volume or invoke SteamCMD.

From the repository root on the Linux Docker host:

```sh
scripts/build-project-zomboid-images.sh base \
  --base-build-id 25485538 --game-files-dir /path/to/verified/server
scripts/build-project-zomboid-images.sh runtime \
  --local-base-image gamepanel-local/project-zomboid-base:build-25485538
```

The default namespace is `gamepanel-local`; both commands use `--load` and do
not push. For a published, separately verified base, use
`--base-image namespace/project-zomboid-base@sha256:<digest>` instead. There is
no fabricated default digest. `--platform` accepts only `linux/amd64`.

The official source must be installed once with anonymous SteamCMD and an
explicit `+force_install_dir` before `+login anonymous +app_update 380870
validate +quit`. Steam public can move; if the manifest no longer matches
25485538 the base build fails rather than silently shipping another version.
Keep the audited installation for configuration/preset extraction and local
verification; do not run Steam updates on it while building or testing.

## Provider/runtime contract

Mount each instance's isolated data directory at `/data`. Provider writes:

- `Zomboid/Server/gamepanel.ini.pending` (complete desired INI input)
- `Zomboid/Server/gamepanel_SandboxVars.lua`
- `Zomboid/Server/gamepanel_spawnregions.lua`

`gamepanel.ini` is the native, persistent configuration. At each startup the
runtime validates the pending input and, when canonical INI already exists,
keeps exactly its `ResetID` and `ServerPlayerID`. Every other option comes from
pending, including settings edited in the panel; unknown canonical-only options
are not merged. First boot without canonical INI uses the two pending identities.
The pending file remains in place for subsequent container starts.

This split is required by official 42.21 behavior: `GameServer.main()` checks the
current save's `z_outfits.bin`; if absent it calls `Rand.Next(100000000)` and
`ServerOptions.changeOption("ResetID", ...)`. `changeOption` immediately calls
`saveServerTextFile`. A Provider that writes its old desired ResetID directly
over native INI on container recreation discards this first-world identity.

The runtime requires pending even when native INI exists: there is **no legacy
fallback** that silently accepts a Provider still writing canonical directly.
Provider and runtime must be updated together. Existing canonical identity must
be valid; missing/malformed identity fails instead of being reseeded from desired
input. ResetID accepts official integer bounds 0..2147483647; ServerPlayerID is
nonempty single-line text, without an invented numeric constraint or length cap.
Both files reject duplicate keys, malformed options, CRLF/control characters,
symlinks and non-regular files. INI input is bounded to 1 MiB.

Canonical output uses a new 0600 temporary file in the same directory, flush and
fsync, atomic rename, then directory fsync. No shutdown callback or sidecar is
needed to keep a native-written identity after an ungraceful process exit. This
does not repair an identity already overwritten by an earlier Provider or recover
a damaged native INI: that requires a separately validated stopped restore from
the instance's own backup. No game DB/world files are changed by this merge.

Server identity is fixed to `gamepanel` inside each isolated volume. The native
launcher receives `-servername gamepanel` and `-cachedir=/data/Zomboid`.
The expected game-owned paths are `/data/Zomboid/Saves/Multiplayer/gamepanel`
and `/data/Zomboid/db/gamepanel.db`. Back up the **entire `Zomboid` subtree
after graceful stop**, including Server, Saves, db and any game-owned identity
files. Do not regenerate ResetID or an existing database when restarting.

Provider supplies the random initial admin password using `PZ_ADMIN_PASSWORD`.
It is passed to the official `-adminusername admin -adminpassword` CLI, with no
shell interpolation or log echo by the wrapper. It is removed from the child
environment, but the native process arguments contain it: do not expose process
inspection or Docker access to untrusted users. The password CLI flag is supplied **only when `db/gamepanel.db` does not
exist**: native 42.21 changes an existing admin password if that flag is repeated.
Existing databases do not require the initial password environment variable.
The supervisor masks the password in stdout/stderr because the official launcher
echoes CLI arguments. Missing initial credentials or missing/symlinked Provider
config fail before launching;
the wrapper never supplies fallback settings over the Provider's files.

Steam mode is enabled by the bundled official JVM JSON (`-Dzomboid.steam=1`).
Use game UDP **16261 and 16262** (INI `DefaultPort` and `UDPPort`). The Dockerfile
exposes only these two UDP ports; publish/map them per instance in RuntimeAdapter.
Do not expose RCON. Provider must leave `RCONPassword` empty and disallow RCON
configuration. Set `UPnP=false` to prevent automatic gateway mappings. The
native smoke test publishes no ports and uses an isolated Docker bridge.

## Native launcher, memory, console and stop

The wrapper uses `ProjectZomboid64 -pzexeconfig <temporary-json>` from the
immutable game root. It preserves the official classpath, Steam/native library
settings, headless mode and ZGC. It also sets the bundled JRE on PATH and
preloads the official `jre64/lib/libjsig.so`, matching `start-server.sh`. Only `-Xms`/`-Xmx` are replaced in a temporary
JSON; the official JSON remains untouched. JVM options inherited through
`JAVA_TOOL_OPTIONS`, `_JAVA_OPTIONS` and `JDK_JAVA_OPTIONS` are removed.

Heap maximum is 60% of the smaller of cgroup v2 `memory.max`, cgroup v1
`memory/memory.limit_in_bytes`, and host MemTotal, capped at 8192 MiB. A budget
below 1024 MiB fails early; this lower bound is a validation guard, not a claim
that 1 GiB is sufficient for a playable server. Leave room for native buffers,
Steam and ZGC. The value is a runtime JVM setting, never a fictitious INI option.

Use Docker `OpenStdin`/`-i` for console attachment. The Python PID 1 supervisor
forwards stdin to the native console, leaves the pipe open after attach EOF,
and translates SIGTERM/SIGINT into one `quit\n`. It waits for the native process
to exit and preserves its status. There is no fallback SIGKILL in the wrapper.
Configure Docker's stop timeout for world saving (at least 180 seconds initially;
large worlds may require more) and never back up until the container has stopped
successfully. Docker may kill the process when its external stop deadline expires.
Writable HOME caches live at `/data/.runtime-home`, outside the save subtree.

## Checks

```sh
python3 -m unittest discover -s docker/project-zomboid -p 'test_*.py'
python3 -m unittest discover -s scripts -p test_build_project_zomboid_images.py
bash -n scripts/build-project-zomboid-images.sh
sh -n docker/project-zomboid/entrypoint.sh
```

The opt-in native test creates a unique temporary data directory/container,
starts twice, verifies both UDP sockets and disabled RCON, stops once through
SIGTERM→quit and once through an attached console `quit`, checks the
saved world and admin DB/ResetID, and changes the initial password on the second
boot to detect accidental account replacement. The fixture writes pending input
and re-renders it with different desired identities between boots, exercising
the actual Provider-recreation failure pattern. Artifacts remain for inspection;
only its own uniquely named container is removed.

```sh
PZ_SMOKE_IMAGE=gamepanel-local/project-zomboid:candidate-42.21 \
PZ_SMOKE_SOURCE=/path/to/verified/server \
python3 -m unittest discover -s docker/project-zomboid -p test_native_smoke.py
```

Client joining, panel UI, world import/restore and production operation are
outside this image's scoped checks. Native validation results are recorded below
when available; offline unit success alone does not verify game startup.

## um773 validation — 2026-10-28

- Downloaded exactly once into
  `/var/tmp/gpl-pz-42.21-build-25485538/server`, using the existing SteamCMD
  from a local base image in a separate 1 CPU / 2 GiB host-network container.
  Anonymous install finished with exit 0; manifest App 380870, StateFlags 4,
  Build 25485538; 7,206,115,414 bytes installed. The source remains available
  for the preset audit and full panel validation and must not be removed yet.
- Built local base `gamepanel-local/project-zomboid-base:build-25485538`
  (image ID `sha256:c6932b2a261f2d554d73496616734d67379f5934dbe33c916fa359517feea7f3`)
  and the initial runtime `gamepanel-local/project-zomboid:candidate-42.21`
  (image ID `sha256:64ed66823f3ae7d2b190ac52df0de2834d0207c812500433eeedc72f31c09e28`).
  These are local image IDs, not registry manifest digests or published defaults.
- Native log reports `version=42.21.0`; bundled JRE is Azul 25.0.1+8-LTS.
  The actual native executable loads the temporary JSON, bundled libjvm, Steam
  and ZGC; the 3 GiB test budget produced `-Xmx1843m`.
- Passed the two-boot opt-in smoke test in 195.971 seconds, with isolated data,
  2 CPUs / 3 GiB, no host port publication. Both boots reached `SERVER STARTED`
  and `Steam is enabled`; both UDP sockets existed and RCON TCP 27015 did not.
  SIGTERM→console quit and an attached console quit each ended with exit 0,
  no OOM, `Saving finish` and `Shutdown handling finished`.
- Confirmed actual paths: `Zomboid/Server/gamepanel.ini`,
  `Zomboid/Saves/Multiplayer/gamepanel/map_t.bin`, and
  `Zomboid/db/gamepanel.db` below `/data`. Changed ENV initial password on boot
  two; admin password hash and ResetID remained identical. Initial launcher
  stdout was masked as `[redacted]`.
- Evidence on um773: `/var/tmp/gpl-pz-42.21-build-25485538/native-smoke-fixed.log`
  and `/tmp/gpl-pz-native-gb148jbu/` (boot/stopped logs, UDP socket records,
  console quit transcript and isolated saved data). The earlier smoke test
  reproduced native password replacement before the wrapper fix.
- Official startup emits asset warnings and
  `SteamNetworkingUtils004 before SteamAPI_Init succeeded` before reaching
  `SERVER STARTED`; Steam client joining and external reachability remain
  unverified. No existing room, proxy, ongoing upload, production deployment,
  registry push, or v0.2.19 release was changed.

## Staged identity contract revision — 2026-10-28

The initial native smoke result above predates the pending contract. It preserved
native INI between boots and did not simulate Provider rewriting that file. The
panel-level check later caught ResetID replacement; read-only comparisons showed
current INI equals the DB desired spec while the stopped backup equals the
baseline, and all other world/admin identities stayed equal. Official bytecode
confirmed the first-world `z_outfits.bin` condition described above.

The revised runtime has a dedicated offline regression for native identity versus
re-rendered Provider input, plus malformed input, official identity bounds,
symlink/regular-file checks and failed atomic replacement. The opt-in native
fixture now uses pending throughout and changes desired IDs between starts.
The revised native fixture has not been rerun yet; the parent owns the panel
stop/backup/start validation. Only the local candidate runtime is rebuilt for
this change; the audited game source/base is retained without downloading again.

Revised local candidate build succeeded on um773 (pending contract):
`sha256:be6c9fe51a57bcf643a74e21916d3b83cd4c410585224bf88377aa8fb04da740`
for `gamepanel-local/project-zomboid:candidate-42.21`. The game base is unchanged.
21 scoped runtime tests and 5 build-entry tests passed; the revised long-running
native smoke was deliberately not rerun. Existing containers were not stopped,
recreated or retagged into registry namespaces by this runtime task.
