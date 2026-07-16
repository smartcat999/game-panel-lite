# Palworld Server Image

This image packages the stable `thijsvanloef/palworld-server-docker` runtime under the GamePanel Lite image namespace. GamePanel sets `UPDATE_ON_BOOT=false` after the initial installation so ordinary container restarts reuse the installed Palworld Dedicated Server files instead of verifying roughly 5 GB through SteamCMD every time. Runtime image upgrades are managed explicitly by GamePanel.

The image also fixes an upstream startup race in `pal_logger.py`. The upstream
logger can recreate its private log FIFO as `root:root` with mode `0600` after
the initialization script has already changed `/home/steam` to the configured
`PUID` and `PGID`. GamePanel's build-time patch creates a private temporary FIFO,
sets its final owner and mode, and atomically publishes it. The build fails if
the expected upstream code changes, preventing a silently stale patch.

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

Keep the upstream base version separate from an immutable GamePanel image tag:

```bash
PALWORLD_RUNTIME_VERSION=v2.5.0 \
PALWORLD_IMAGE_VERSION=v2.5.0-gamepanel.1 \
scripts/build-game-images.sh palworld --platform linux/amd64 --push
```

Run the build-time patch tests without Docker:

```bash
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest docker/palworld/test_patch_pal_logger.py
```

After loading an amd64 image, exercise the patched logger as root while a
non-root game helper writes to the FIFO:

```bash
docker run --rm --platform linux/amd64 \
  --entrypoint /bin/bash \
  -v "$PWD/docker/palworld/test_fifo_runtime.sh:/tmp/test_fifo_runtime.sh:ro" \
  smartcat99999/palworld-server:v2.5.0-gamepanel.1 \
  /tmp/test_fifo_runtime.sh
```
