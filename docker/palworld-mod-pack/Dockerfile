FROM debian:bookworm-slim AS modpack

COPY docker/palworld-mod-pack/mods /palworld-mod-pack
COPY docker/palworld-mod-pack/mod-dependencies.json /palworld-mod-pack/mod-dependencies.json

RUN set -eu; \
  if ! find /palworld-mod-pack -type f | grep -q .; then \
    echo "palworld mod-pack is empty: add Palworld mod files under docker/palworld-mod-pack/mods" >&2; \
    exit 1; \
  fi; \
  if [ ! -d /palworld-mod-pack/Pal ]; then \
    echo "palworld mod-pack must contain a Pal/ runtime tree" >&2; \
    exit 1; \
  fi; \
  find /palworld-mod-pack -type d -exec chmod 0755 {} +; \
  find /palworld-mod-pack -type f -exec chmod 0644 {} +

FROM debian:bookworm-slim AS runner

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates \
  && rm -rf /var/lib/apt/lists/*

COPY docker/palworld-mod-pack/sync-palworld-mod-pack.sh /usr/local/bin/sync-palworld-mod-pack.sh
COPY --from=modpack /palworld-mod-pack /palworld-mod-pack

RUN chmod 0755 /usr/local/bin/sync-palworld-mod-pack.sh

ENV GAMEPANEL_DATA_DIR=/app/data \
  GAMEPANEL_PALWORLD_MOD_SOURCE_DIR=/palworld-mod-pack

CMD ["sync-palworld-mod-pack.sh"]
