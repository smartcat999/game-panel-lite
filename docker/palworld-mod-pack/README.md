# Palworld Mod Pack Image

This image is the file mirror for recommended Palworld mods and their runtime dependencies.

Build input must contain a Palworld runtime tree under `mods/Pal`. Empty images are rejected at build time.

Supported inputs:

- Put approved files under `docker/palworld-mod-pack/mods/Pal/...`.
- Use `Pal/Content/Paks/LogicMods` for LogicMods `.pak/.ucas/.utoc/.json` assets.
- Use `Pal/Binaries/Win64/ue4ss/Mods` for UE4SS Lua/DLL mods.
- Use `Pal/Binaries/Win64/ue4ss/Mods/PalSchema/mods` for PalSchema blueprint mods.

Build and push:

```sh
docker buildx build \
  -f docker/palworld-mod-pack/Dockerfile \
  -t smartcat99999/game-panel-lite-palworld-mod-pack:v0.1.0 \
  .
```
