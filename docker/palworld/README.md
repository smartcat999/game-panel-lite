# Palworld Server Image

This image packages the stable `thijsvanloef/palworld-server-docker` runtime under the GamePanel Lite image namespace. The runtime installs or updates Palworld Dedicated Server (Steam app `2394010`) when the container starts because the provider sets `UPDATE_ON_BOOT=true`.

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
