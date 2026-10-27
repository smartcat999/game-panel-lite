# syntax=docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e
ARG SEVEN_DAYS_BASE_IMAGE=smartcat99999/seven-days-to-die-base@sha256:6b9df95c742f151207f5481dd522de4ae7ccf96d4a823a4d049e28118b630d11
FROM ${SEVEN_DAYS_BASE_IMAGE}
ARG SEVEN_DAYS_BASE_IMAGE
COPY --chmod=755 docker/seven-days-to-die/entrypoint.sh /usr/local/bin/gamepanel-seven-days
COPY docker/seven-days-to-die/merge_config.py /opt/gamepanel/merge_config.py
COPY docker/seven-days-to-die/check_storage.py /opt/gamepanel/check_storage.py
COPY docker/seven-days-to-die/prepare_runtime.py /opt/gamepanel/prepare_runtime.py
WORKDIR /data
STOPSIGNAL SIGINT
LABEL org.opencontainers.image.base.name="${SEVEN_DAYS_BASE_IMAGE}" \
      org.opencontainers.image.title="GamePanel Lite 7 Days to Die" \
      org.opencontainers.image.source="https://github.com/smartcat999/game-panel-lite"
CMD ["/usr/local/bin/gamepanel-seven-days"]
