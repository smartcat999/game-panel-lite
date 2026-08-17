#!/usr/bin/env sh
set -eu

: "${GAMEPANEL_DOCKER_BIN:?Missing Docker executable path}"
: "${GAMEPANEL_CERTBOT_IMAGE:?Missing Certbot image}"
: "${GAMEPANEL_CERTBOT_WWW:?Missing Certbot webroot path}"
: "${GAMEPANEL_CERTBOT_CONF:?Missing Certbot configuration path}"
: "${GAMEPANEL_COMPOSE_PROJECT:?Missing Compose project name}"
: "${GAMEPANEL_RENEWAL_STATUS_FILE:?Missing renewal status file path}"

STARTED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
write_status() {
  status=$1
  checked_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  printf '{"enabled":true,"method":"systemd","lastCheckedAt":"%s","lastStatus":"%s"}\n' "$checked_at" "$status" > "$GAMEPANEL_RENEWAL_STATUS_FILE"
  chmod 0644 "$GAMEPANEL_RENEWAL_STATUS_FILE"
}
on_exit() {
  code=$?
  if [ "$code" -eq 0 ]; then
    write_status success
  else
    write_status failed
  fi
}
trap on_exit EXIT

printf '{"enabled":true,"method":"systemd","lastCheckedAt":"%s","lastStatus":"running"}\n' "$STARTED_AT" > "$GAMEPANEL_RENEWAL_STATUS_FILE"
chmod 0644 "$GAMEPANEL_RENEWAL_STATUS_FILE"

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
