# GamePanel Lite Don't Starve Together image

This image packages the Don't Starve Together dedicated server for GamePanel Lite.

## Build

```bash
scripts/build-game-images.sh dst --platform linux/amd64 --load
```

To push the image:

```bash
scripts/build-game-images.sh dst --platform linux/amd64 --push
```

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
