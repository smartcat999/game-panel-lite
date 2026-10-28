# syntax=docker/dockerfile:1.7
# A verified base reference is supplied explicitly; there is no published default yet.
ARG PZ_BASE_IMAGE
FROM ${PZ_BASE_IMAGE}
ARG PZ_BASE_IMAGE
COPY --chmod=755 docker/project-zomboid/entrypoint.sh /usr/local/bin/gamepanel-project-zomboid
COPY docker/project-zomboid/runtime.py /opt/gamepanel/runtime.py
WORKDIR /data
EXPOSE 16261/udp 16262/udp
STOPSIGNAL SIGTERM
LABEL org.opencontainers.image.base.name="${PZ_BASE_IMAGE}" \
      org.opencontainers.image.title="GamePanel Lite Project Zomboid" \
      org.opencontainers.image.source="https://github.com/smartcat999/game-panel-lite"
CMD ["/usr/local/bin/gamepanel-project-zomboid"]
