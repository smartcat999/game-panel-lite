# GamePanel Lite Don't Starve Together image

This image packages the Don't Starve Together dedicated server for GamePanel Lite.

## Build

Build this image on a native amd64 Docker host:

```bash
scripts/build-game-images.sh dst --platform linux/amd64 --load
```

Docker buildx is supported. The build script passes `--platform` and `--builder` through to buildx, the same as the other runtime images. On Apple Silicon or other arm hosts, make sure the active buildx builder has an amd64 node. If the builder falls back to local QEMU emulation, SteamCMD can exit with a segmentation fault before the DST server files are downloaded.

Check the selected builder:

```bash
docker buildx ls
```

If the active builder already includes a `linux/amd64` node, the build script can use it automatically:

```bash
scripts/build-game-images.sh dst --platform linux/amd64 --push
```

Otherwise, create or select a remote amd64 buildx builder and push the image:

```bash
docker buildx create --name gamepanel-amd64 --driver docker-container --platform linux/amd64 ssh://user@amd64-host --use
docker buildx inspect --bootstrap
scripts/build-game-images.sh dst --builder gamepanel-amd64 --platform linux/amd64 --push
```

Use `--push` with a remote builder so the image is available to the host running GamePanel Lite.

## Runtime layout

GamePanel Lite mounts the server instance directory at `/data` and writes DST config files under:

```text
/data/dst/<cluster-name>/
```

The entrypoint starts the Master shard and starts the Caves shard when `Caves/server.ini` exists.

## Notes

- The image currently targets `linux/amd64`.
- A Klei cluster token is required at creation time.
- Docker image names and startup commands stay internal to GamePanel Lite; users should create DST servers through the product UI.
