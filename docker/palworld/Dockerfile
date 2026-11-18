ARG PALWORLD_RUNTIME_VERSION=v2.7.1
FROM thijsvanloef/palworld-server-docker:${PALWORLD_RUNTIME_VERSION}

COPY docker/palworld/patch_pal_logger.py /tmp/patch_pal_logger.py

# Validate that the upstream logger either uses its fixed init.sh-managed FIFO
# lifecycle or matches the older layout that GamePanel patches atomically.
# Unknown layouts fail the build instead of silently reintroducing log loss.
RUN python3 /tmp/patch_pal_logger.py /home/steam/server/pal_logger.py \
    && python3 -m py_compile /home/steam/server/pal_logger.py \
    && rm -rf /tmp/patch_pal_logger.py /home/steam/server/__pycache__

# SteamCMD can replace PalServer.sh without preserving its executable bit.
# The upstream start script validates the bit immediately after updating, so
# repair it after InstallServer and before STARTCOMMAND is validated.
RUN sed -i '/^STARTCOMMAND=("\.\/PalServer\.sh")$/i chmod +x /palworld/PalServer.sh || exit 1' /home/steam/server/start.sh

LABEL org.opencontainers.image.title="GamePanel Lite Palworld Server" \
      org.opencontainers.image.description="Palworld dedicated server runtime for GamePanel Lite" \
      org.opencontainers.image.source="https://github.com/smartcat999/game-panel-lite"
