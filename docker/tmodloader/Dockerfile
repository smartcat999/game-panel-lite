FROM ghcr.io/parkervcp/installers:debian AS installer

ARG TML_VERSION=v2026.04.3.0
ARG MANAGE_URL=https://raw.githubusercontent.com/tModLoader/tModLoader/stable/patches/tModLoader/Terraria/release_extras/DedicatedServerUtils/manage-tModLoaderServer.sh

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl unzip wget \
    && if [ "$(dpkg --print-architecture)" = "amd64" ] || [ "$(dpkg --print-architecture)" = "i386" ]; then \
        apt-get install -y --no-install-recommends lib32gcc-s1; \
    fi \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /opt/tmodloader/Mods /opt/tmodloader/Worlds /opt/tmodloader/logs /opt/tmodloader/steamapps \
    && cd /opt/tmodloader \
    && curl -fsSL "${MANAGE_URL}" -o manage-tModLoaderServer.sh \
    && chmod +x manage-tModLoaderServer.sh \
    && bash ./manage-tModLoaderServer.sh install-tml --github --folder /opt/tmodloader --tml-version "${TML_VERSION}" \
    && chmod +x /opt/tmodloader/server/*.sh

FROM ghcr.io/parkervcp/yolks:debian

LABEL org.opencontainers.image.title="GamePanel Lite tModLoader"
LABEL org.opencontainers.image.description="Prebuilt tModLoader server runtime for GamePanel Lite"

ARG STEAMCMD_URL=https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz

USER root
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl tar \
    && if [ "$(dpkg --print-architecture)" = "amd64" ] || [ "$(dpkg --print-architecture)" = "i386" ]; then \
        apt-get install -y --no-install-recommends lib32gcc-s1; \
    fi \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /opt/steamcmd \
    && curl -fsSL "${STEAMCMD_URL}" -o /tmp/steamcmd_linux.tar.gz \
    && tar -xzf /tmp/steamcmd_linux.tar.gz -C /opt/steamcmd \
    && chmod -R a+rx /opt/steamcmd \
    && printf '#!/bin/sh\nexec /opt/steamcmd/steamcmd.sh "$@"\n' >/usr/local/bin/steamcmd \
    && chmod +x /usr/local/bin/steamcmd \
    && /opt/steamcmd/steamcmd.sh +quit || true \
    && rm -f /tmp/steamcmd_linux.tar.gz \
    && mkdir -p /home/container \
    && chown -R container:container /home/container

USER container
WORKDIR /home/container
COPY --from=installer --chown=container:container /opt/tmodloader/ /home/container/
COPY --chown=container:container docker/tmodloader/gamepanel-tmodloader-entrypoint.sh /home/container/gamepanel-tmodloader-entrypoint.sh
RUN chmod +x /home/container/gamepanel-tmodloader-entrypoint.sh

CMD ["sh", "-c", "exec ./gamepanel-tmodloader-entrypoint.sh -config /home/container/serverconfig.txt -tmlsavedirectory /home/container -noupnp"]
