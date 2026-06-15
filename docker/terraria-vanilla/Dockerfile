FROM ghcr.io/parkervcp/installers:debian AS installer

ARG TERRARIA_VERSION=1.4.5.6
ARG TERRARIA_DOWNLOAD_ID=1456
ARG TERRARIA_DOWNLOAD_URL=https://terraria.org/api/download/pc-dedicated-server/terraria-server-${TERRARIA_DOWNLOAD_ID}.zip

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl unzip \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /opt/terraria \
    && curl -fsSL "${TERRARIA_DOWNLOAD_URL}" -o /tmp/terraria-server.zip \
    && unzip -q /tmp/terraria-server.zip -d /tmp/terraria \
    && cp -R "/tmp/terraria/${TERRARIA_DOWNLOAD_ID}/Linux/." /opt/terraria/ \
    && chmod +x /opt/terraria/TerrariaServer /opt/terraria/TerrariaServer.bin.x86_64

FROM ghcr.io/parkervcp/yolks:debian

ARG TERRARIA_VERSION=1.4.5.6

LABEL org.opencontainers.image.title="GamePanel Lite Terraria Vanilla"
LABEL org.opencontainers.image.description="Prebuilt Terraria vanilla dedicated server runtime for GamePanel Lite"
LABEL org.opencontainers.image.version="${TERRARIA_VERSION}"

USER root
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && if [ "$(dpkg --print-architecture)" != "amd64" ]; then \
        apt-get install -y --no-install-recommends \
            mono-runtime \
            libmono-i18n-west4.0-cil \
            libmono-system-data4.0-cil \
            libmono-system-drawing4.0-cil \
            libmono-system-runtime-serialization4.0-cil \
            libmono-system-windows-forms4.0-cil; \
    fi \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /home/container/server /home/container/Worlds /home/container/logs \
    && chown -R container:container /home/container

USER container
WORKDIR /home/container
COPY --from=installer --chown=container:container /opt/terraria/ /home/container/server/
COPY --chmod=755 --chown=container:container docker/terraria-vanilla/gamepanel-terraria-entrypoint.sh /home/container/server/gamepanel-terraria-entrypoint.sh
USER root
RUN if [ "$(dpkg --print-architecture)" != "amd64" ]; then \
        rm -f /home/container/server/mscorlib.dll \
            /home/container/server/System*.dll \
            /home/container/server/Mono.*.dll \
            /home/container/server/WindowsBase.dll; \
    fi \
    && chown -R container:container /home/container/server
USER container

CMD ["./server/gamepanel-terraria-entrypoint.sh", "-config", "/home/container/serverconfig.txt", "-noupnp"]
