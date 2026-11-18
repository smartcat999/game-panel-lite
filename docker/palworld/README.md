# Palworld Server Image

This image packages the stable `thijsvanloef/palworld-server-docker` runtime under the GamePanel Lite image namespace. GamePanel sets `UPDATE_ON_BOOT=false` after the initial installation so ordinary container restarts reuse the installed Palworld Dedicated Server files instead of verifying roughly 5 GB through SteamCMD every time. Runtime image upgrades are managed explicitly by GamePanel.

The image validates the upstream `pal_logger.py` FIFO lifecycle at build time.
Older upstream releases are patched to publish the FIFO with its final owner
atomically. Current releases manage FIFO creation and ownership in `init.sh`;
the validator accepts that fixed implementation and fails the build for unknown
layouts so a logging permission regression cannot ship silently.

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
PALWORLD_RUNTIME_VERSION=v2.7.1 scripts/build-game-images.sh palworld --load
```

Keep the upstream base version separate from an immutable GamePanel image tag:

```bash
PALWORLD_RUNTIME_VERSION=v2.7.1 \
PALWORLD_IMAGE_VERSION=v2.7.1-gamepanel.1 \
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
  smartcat99999/palworld-server:v2.7.1-gamepanel.1 \
  /tmp/test_fifo_runtime.sh
```
