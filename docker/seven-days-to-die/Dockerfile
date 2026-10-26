# syntax=docker/dockerfile:1.7
ARG STEAMCMD_IMAGE=steamcmd/steamcmd@sha256:aed94b9fa7322488a36f97d0842cd0df1d73d79d592ea0b6f39b3a8fba7076de
FROM ${STEAMCMD_IMAGE} AS steamcmd
FROM steamcmd AS steam-runtime
RUN /usr/games/steamcmd +quit \
    && test -f /root/.local/share/Steam/steamcmd/linux64/steamclient.so
FROM steam-runtime AS installer
ARG SEVEN_DAYS_RUNTIME_VERSION=v3.3.0
RUN --mount=type=cache,id=gamepanel-seven-days-steam-library,target=/var/cache/seven-days,sharing=locked \
    echo "Building seven-days-to-die ${SEVEN_DAYS_RUNTIME_VERSION}" \
    && /usr/games/steamcmd -tcp +force_install_dir /var/cache/seven-days +login anonymous +app_update 294420 validate +quit \
    && test -f /var/cache/seven-days/7DaysToDieServer.x86_64 \
    && grep -Eq '"StateFlags"[[:space:]]+"4"' /var/cache/seven-days/steamapps/appmanifest_294420.acf \
    && mkdir -p /opt/seven-days \
    && cp -a /var/cache/seven-days/. /opt/seven-days/

FROM debian:bookworm-slim AS runtime-base
RUN dpkg --add-architecture i386 \
    && apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates lib32gcc-s1 libstdc++6 libstdc++6:i386 libcurl4 libglu1-mesa python3 \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /data /home/steam/steamcmd /root/.steam/sdk64
COPY --from=steamcmd /usr/games/steamcmd /usr/games/steamcmd
COPY --from=steamcmd /usr/lib/games/steam/ /usr/lib/games/steam/
COPY --from=steam-runtime /root/.local/share/Steam/steamcmd/ /root/.local/share/Steam/steamcmd/
RUN printf '#!/bin/sh\nexec /usr/games/steamcmd "$@"\n' > /home/steam/steamcmd/steamcmd.sh \
    && chmod +x /home/steam/steamcmd/steamcmd.sh \
    && cp /root/.local/share/Steam/steamcmd/linux64/steamclient.so /root/.steam/sdk64/steamclient.so

FROM runtime-base
COPY --from=installer /opt/seven-days/ /opt/seven-days/
COPY --chmod=755 docker/seven-days-to-die/entrypoint.sh /usr/local/bin/gamepanel-seven-days
COPY docker/seven-days-to-die/merge_config.py /opt/gamepanel/merge_config.py
WORKDIR /data
STOPSIGNAL SIGINT
LABEL org.opencontainers.image.title="GamePanel Lite 7 Days to Die" \
      org.opencontainers.image.source="https://github.com/smartcat999/game-panel-lite"
CMD ["/usr/local/bin/gamepanel-seven-days"]
