# syntax=docker/dockerfile:1.7

ARG STEAMCMD_IMAGE=steamcmd/steamcmd@sha256:aed94b9fa7322488a36f97d0842cd0df1d73d79d592ea0b6f39b3a8fba7076de
FROM ${STEAMCMD_IMAGE} AS installer
ARG DST_IMAGE_VERSION=v2026.08.14

RUN --mount=type=cache,id=gamepanel-dst-steam-library,target=/var/cache/dst,sharing=locked \
    echo "Building DST runtime ${DST_IMAGE_VERSION}" \
    && installed=0 \
    && for attempt in 1 2 3; do \
         if /usr/games/steamcmd +force_install_dir /var/cache/dst +login anonymous +app_update 343050 validate +quit; then \
           installed=1; \
           break; \
         fi; \
         echo "DST image install attempt ${attempt} failed" >&2; \
       done \
    && test "${installed}" = "1" \
    && test -x /var/cache/dst/bin64/dontstarve_dedicated_server_nullrenderer_x64 \
    && grep -Eq '"StateFlags"[[:space:]]+"4"' /var/cache/dst/steamapps/appmanifest_343050.acf \
    && rm -rf /opt/dst \
    && mkdir -p /opt/dst \
    && cp -a /var/cache/dst/. /opt/dst/

FROM debian:bookworm-slim

LABEL org.opencontainers.image.title="GamePanel Lite Don't Starve Together"
LABEL org.opencontainers.image.description="Prebuilt Don't Starve Together dedicated server runtime for GamePanel Lite"
LABEL org.opencontainers.image.base.name="docker.io/steamcmd/steamcmd@sha256:aed94b9fa7322488a36f97d0842cd0df1d73d79d592ea0b6f39b3a8fba7076de"

RUN dpkg --add-architecture i386 \
    && apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl lib32gcc-s1 libcurl4 libcurl4:i386 libcurl3-gnutls libcurl3-gnutls:i386 libstdc++6 libstdc++6:i386 unzip \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -m -d /home/container -s /bin/bash container \
    && mkdir -p /home/container/server /data \
    && chown -R container:container /home/container /data

USER container
WORKDIR /home/container
COPY --from=installer --chown=container:container /opt/dst/ /home/container/server/
COPY --from=installer /usr/games/steamcmd /usr/games/steamcmd
COPY --from=installer /usr/lib/games/steam/ /usr/lib/games/steam/
COPY --chmod=755 --chown=container:container docker/dst/gamepanel-dst-entrypoint.sh /home/container/gamepanel-dst-entrypoint.sh

ENV DST_PERSISTENT_ROOT=/data
ENV DST_CONF_DIR=dst
ENV DST_CLUSTER_NAME=GamePanelLite
ENV HOME=/home/container

CMD ["./gamepanel-dst-entrypoint.sh"]
