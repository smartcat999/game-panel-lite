ARG PALWORLD_RUNTIME_VERSION=v2.5.0
FROM thijsvanloef/palworld-server-docker:${PALWORLD_RUNTIME_VERSION}

# SteamCMD can replace PalServer.sh without preserving its executable bit.
# The upstream start script validates the bit immediately after updating, so
# repair it after InstallServer and before STARTCOMMAND is validated.
RUN sed -i '/^STARTCOMMAND=("\.\/PalServer\.sh")$/i chmod +x /palworld/PalServer.sh || exit 1' /home/steam/server/start.sh

LABEL org.opencontainers.image.title="GamePanel Lite Palworld Server" \
      org.opencontainers.image.description="Palworld dedicated server runtime for GamePanel Lite" \
      org.opencontainers.image.source="https://github.com/smartcat999/game-panel-lite"
