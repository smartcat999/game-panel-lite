#!/usr/bin/env sh
set -eu

: "${GAMEPANEL_DOCKER_BIN:?Missing Docker executable path}"
: "${GAMEPANEL_CERTBOT_IMAGE:?Missing Certbot image}"
: "${GAMEPANEL_CERTBOT_WWW:?Missing Certbot webroot path}"
: "${GAMEPANEL_CERTBOT_CONF:?Missing Certbot configuration path}"
: "${GAMEPANEL_COMPOSE_PROJECT:?Missing Compose project name}"

if [ ! -x "$GAMEPANEL_DOCKER_BIN" ]; then
  echo "Docker executable is unavailable: $GAMEPANEL_DOCKER_BIN"
  exit 1
fi

for path in "$GAMEPANEL_CERTBOT_WWW" "$GAMEPANEL_CERTBOT_CONF"; do
  if [ ! -d "$path" ] || [ "$(readlink -f "$path")" != "$path" ]; then
    echo "Certificate data path is missing or has become a symlink: $path"
    exit 1
  fi
done

"$GAMEPANEL_DOCKER_BIN" run --rm --pull=missing \
  --mount "type=bind,src=$GAMEPANEL_CERTBOT_WWW,dst=/var/www/certbot" \
  --mount "type=bind,src=$GAMEPANEL_CERTBOT_CONF,dst=/etc/letsencrypt" \
  "$GAMEPANEL_CERTBOT_IMAGE" renew

set -- $("$GAMEPANEL_DOCKER_BIN" ps \
  --filter "label=com.docker.compose.project=$GAMEPANEL_COMPOSE_PROJECT" \
  --filter "label=com.docker.compose.service=nginx" \
  --format '{{.ID}}')
if [ "$#" -ne 1 ]; then
  echo "Expected exactly one running GamePanel Lite Nginx container, found $#."
  exit 1
fi

"$GAMEPANEL_DOCKER_BIN" exec "$1" nginx -t
"$GAMEPANEL_DOCKER_BIN" exec "$1" nginx -s reload

echo "HTTPS certificate renewal check completed."
