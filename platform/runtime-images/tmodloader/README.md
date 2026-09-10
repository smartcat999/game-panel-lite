# tModLoader runtime image

This image pins the official `tModLoader.zip` release and verifies its GitHub-published SHA-256 digest during build. It also verifies a pinned Valve SteamCMD archive and uses it to install only the Workshop IDs written by the accepted Game Provider into `Mods/install.txt`; `Mods/enabled.json` controls which downloaded mods are enabled. The game process runs as UID 10001 with no root privileges. GamePanel supplies configuration, Worlds, Mods, Workshop cache, SteamCMD state, and logs only through instance-scoped mounts.

Build from `platform/`:

```sh
docker build -t gamepanel/tmodloader:v2026.07.3.0 runtime-images/tmodloader
```
