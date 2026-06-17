FROM debian:bookworm-slim AS installer

ARG STEAMCMD_URL=https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz
ARG DST_APP_ID=343050

RUN dpkg --add-architecture i386 \
    && apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl tar lib32gcc-s1 \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /opt/steamcmd /opt/dst \
    && curl -fsSL "${STEAMCMD_URL}" -o /tmp/steamcmd_linux.tar.gz \
    && tar -xzf /tmp/steamcmd_linux.tar.gz -C /opt/steamcmd \
    && /opt/steamcmd/steamcmd.sh +force_install_dir /opt/dst +login anonymous +app_update "${DST_APP_ID}" validate +quit \
    && rm -f /tmp/steamcmd_linux.tar.gz

FROM debian:bookworm-slim

LABEL org.opencontainers.image.title="GamePanel Lite Don't Starve Together"
LABEL org.opencontainers.image.description="Prebuilt Don't Starve Together dedicated server runtime for GamePanel Lite"

RUN dpkg --add-architecture i386 \
    && apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates lib32gcc-s1 libcurl4 libcurl4:i386 libstdc++6 libstdc++6:i386 \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -m -d /home/container -s /bin/bash container \
    && mkdir -p /home/container/server /data \
    && chown -R container:container /home/container /data

USER container
WORKDIR /home/container
COPY --from=installer --chown=container:container /opt/dst/ /home/container/server/
COPY --chmod=755 --chown=container:container docker/dst/gamepanel-dst-entrypoint.sh /home/container/gamepanel-dst-entrypoint.sh

ENV DST_PERSISTENT_ROOT=/data
ENV DST_CONF_DIR=dst
ENV DST_CLUSTER_NAME=GamePanelLite

CMD ["./gamepanel-dst-entrypoint.sh"]
