# GamePanel Lite tModLoader image

Build a fixed tModLoader server image before starting tModLoader servers:

```bash
docker build \
  -f docker/tmodloader/Dockerfile \
  --build-arg TML_VERSION=v2026.04.3.0 \
  -t smartcat99999/tmodloader:v2026.04.3.0 \
  .
```

The image is built from the official tModLoader `manage-tModLoaderServer.sh` release flow. GamePanel Lite starts this prebuilt image directly; server startup does not download or update tModLoader files.
