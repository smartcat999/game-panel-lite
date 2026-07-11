# Palworld Server Image

This image packages the stable `thijsvanloef/palworld-server-docker` runtime under the GamePanel Lite image namespace. GamePanel sets `UPDATE_ON_BOOT=false` after the initial installation so ordinary container restarts reuse the installed Palworld Dedicated Server files instead of verifying roughly 5 GB through SteamCMD every time. Runtime image upgrades are managed explicitly by GamePanel.

Build and load the current version for the local Docker architecture:

```bash
scripts/build-game-images.sh palworld --load
```

Build and push both supported architectures:

```bash
scripts/build-game-images.sh palworld --platform linux/amd64,linux/arm64 --push
```

Override the upstream runtime tag when testing a newer release:

```bash
PALWORLD_RUNTIME_VERSION=v2.5.0 scripts/build-game-images.sh palworld --load
```
