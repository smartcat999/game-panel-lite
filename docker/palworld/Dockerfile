ARG PALWORLD_RUNTIME_VERSION=v2.5.0
FROM thijsvanloef/palworld-server-docker:${PALWORLD_RUNTIME_VERSION}

COPY docker/palworld/patch_pal_logger.py /tmp/patch_pal_logger.py

# The upstream logger is started asynchronously as root. It can recreate its
# private FIFO after init.sh has already chowned /home/steam, leaving the game
# helper unable to write log events. Patch the logger to publish a correctly
# owned FIFO atomically and fail the build if the upstream block changes.
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
