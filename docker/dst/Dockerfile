# syntax=docker/dockerfile:1.7

# SteamCMD occasionally returns "Missing configuration" for the public DST app
# even though app 343050 supports anonymous installation. Use a digest-pinned,
# preinstalled upstream image as the binary source so builds stay reproducible
# and never require a personal Steam account.
ARG DST_BASE_IMAGE=webhippie/dst@sha256:f4479666ebb55903f2f1e5a94f4c4dcd5fae5d6a7eb896cb6285d1860d789bad
FROM ${DST_BASE_IMAGE} AS installer

FROM debian:bookworm-slim

LABEL org.opencontainers.image.title="GamePanel Lite Don't Starve Together"
LABEL org.opencontainers.image.description="Prebuilt Don't Starve Together dedicated server runtime for GamePanel Lite"
LABEL org.opencontainers.image.base.name="docker.io/webhippie/dst@sha256:f4479666ebb55903f2f1e5a94f4c4dcd5fae5d6a7eb896cb6285d1860d789bad"

RUN dpkg --add-architecture i386 \
    && apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates lib32gcc-s1 libcurl4 libcurl4:i386 libcurl3-gnutls libcurl3-gnutls:i386 libstdc++6 libstdc++6:i386 \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -m -d /home/container -s /bin/bash container \
    && mkdir -p /home/container/server /data \
    && chown -R container:container /home/container /data

USER container
WORKDIR /home/container
COPY --from=installer --chown=container:container /usr/share/game/ /home/container/server/
COPY --chmod=755 --chown=container:container docker/dst/gamepanel-dst-entrypoint.sh /home/container/gamepanel-dst-entrypoint.sh

ENV DST_PERSISTENT_ROOT=/data
ENV DST_CONF_DIR=dst
ENV DST_CLUSTER_NAME=GamePanelLite

CMD ["./gamepanel-dst-entrypoint.sh"]
