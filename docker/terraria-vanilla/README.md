# GamePanel Lite Terraria vanilla image

Build a fixed Terraria vanilla dedicated server image before starting vanilla servers:

```bash
scripts/build-game-images.sh vanilla
```

For a specific local platform:

```bash
scripts/build-game-images.sh vanilla --platform linux/arm64 --load
```

Known version mappings:

| Terraria version | Download id |
| --- | --- |
| 1.4.5.6 | 1456 |
| 1.4.4.9 | 1449 |

The image is built from the official Terraria dedicated server archive. GamePanel Lite starts this prebuilt image directly; server startup does not download or update Terraria files.

On `linux/amd64`, the image starts the official `TerrariaServer.bin.x86_64` binary. On other Linux architectures, it starts `TerrariaServer.exe` through the system Mono runtime and removes the archive's bundled x86-oriented Mono class libraries to avoid runtime/library mismatches. The arm64 image includes the Mono assemblies Terraria loads while resolving world paths, including `WindowsBase`.
