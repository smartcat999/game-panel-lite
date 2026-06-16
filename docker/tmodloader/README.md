# GamePanel Lite tModLoader image

Build a fixed tModLoader server image before starting tModLoader servers:

```bash
scripts/build-game-images.sh tmodloader
```

For a specific local platform:

```bash
scripts/build-game-images.sh tmodloader --platform linux/arm64 --load
```

The image is built from the official tModLoader `manage-tModLoaderServer.sh` release flow. GamePanel Lite starts this prebuilt image directly; the server can read `Mods/install.txt` for Workshop ID based mod sync when a server has imported Workshop IDs.
