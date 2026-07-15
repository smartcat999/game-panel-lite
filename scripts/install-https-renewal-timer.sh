#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
RUNNER_SOURCE="$ROOT_DIR/scripts/https-renewal-runner.sh"
UNIT_NAME="gamepanel-lite-https-renewal"
CONFIG_DIR="/etc/gamepanel-lite"
CONFIG_FILE="$CONFIG_DIR/https-renewal.env"
RUNNER_DIR="/usr/local/libexec/gamepanel-lite"
RUNNER_FILE="$RUNNER_DIR/https-renewal"
SERVICE_FILE="/etc/systemd/system/$UNIT_NAME.service"
TIMER_FILE="/etc/systemd/system/$UNIT_NAME.timer"

if [ "$(id -u)" -ne 0 ]; then
  echo "Root privileges are required. Run: sudo sh scripts/install-https-renewal-timer.sh"
  exit 1
fi

if ! command -v systemctl >/dev/null 2>&1 || [ ! -d /run/systemd/system ]; then
  echo "A running systemd service manager is required for the automatic HTTPS renewal timer."
  exit 1
fi

if ! systemctl show-environment >/dev/null 2>&1; then
  echo "The systemd service manager is not reachable."
  exit 1
fi

if ! command -v docker >/dev/null 2>&1 || ! docker compose version >/dev/null 2>&1; then
  echo "Docker with the Compose plugin is required for automatic HTTPS renewal."
  exit 1
fi

if [ ! -f "$ROOT_DIR/data/nginx/gamepanel-https.conf" ]; then
  echo "HTTPS is not configured. Run scripts/setup-https.sh before installing the renewal timer."
  exit 1
fi

if [ ! -f "$RUNNER_SOURCE" ]; then
  echo "Renewal runner not found: $RUNNER_SOURCE"
  exit 1
fi

# Values written to the root-owned environment file are deliberately restricted
# so systemd and Docker receive unambiguous arguments.
case "$ROOT_DIR" in
  *[!A-Za-z0-9_./-]*)
    echo "The install path contains unsupported characters: $ROOT_DIR"
    exit 1
    ;;
esac

CERTBOT_ROOT="$ROOT_DIR/data/certbot"
CERTBOT_WWW="$CERTBOT_ROOT/www"
CERTBOT_CONF="$CERTBOT_ROOT/conf"
for path in "$CERTBOT_ROOT" "$CERTBOT_WWW" "$CERTBOT_CONF"; do
  if [ ! -d "$path" ] || [ "$(readlink -f "$path")" != "$path" ]; then
    echo "Certificate data must be an existing directory without symlinks: $path"
    exit 1
  fi
done

cd "$ROOT_DIR"
set -- $(docker compose -f compose.prod.yaml -f compose.https.yaml ps -q nginx)
if [ "$#" -ne 1 ]; then
  echo "Exactly one running HTTPS Nginx container is required before installing the timer."
  exit 1
fi
NGINX_CONTAINER="$1"
COMPOSE_PROJECT=$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.project" }}' "$NGINX_CONTAINER")
case "$COMPOSE_PROJECT" in
  ""|*[!A-Za-z0-9_.-]*)
    echo "Unable to determine a safe Compose project name from the Nginx container."
    exit 1
    ;;
esac

DOCKER_BIN=$(command -v docker)
case "$DOCKER_BIN" in
  *[!A-Za-z0-9_./-]*)
    echo "Docker executable path contains unsupported characters: $DOCKER_BIN"
    exit 1
    ;;
esac

TMP_DIR=$(mktemp -d)
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup 0
trap 'exit 130' INT
trap 'exit 143' TERM

cat > "$TMP_DIR/https-renewal.env" <<EOF
GAMEPANEL_DOCKER_BIN=$DOCKER_BIN
GAMEPANEL_CERTBOT_IMAGE=certbot/certbot:v2.11.0
GAMEPANEL_CERTBOT_WWW=$CERTBOT_WWW
GAMEPANEL_CERTBOT_CONF=$CERTBOT_CONF
GAMEPANEL_COMPOSE_PROJECT=$COMPOSE_PROJECT
EOF

cat > "$TMP_DIR/$UNIT_NAME.service" <<EOF
[Unit]
Description=Check and renew the GamePanel Lite HTTPS certificate
Wants=network-online.target docker.service
After=network-online.target docker.service

[Service]
Type=oneshot
EnvironmentFile=$CONFIG_FILE
ExecStart=$RUNNER_FILE
TimeoutStartSec=45min
UMask=0022
EOF

cat > "$TMP_DIR/$UNIT_NAME.timer" <<EOF
[Unit]
Description=Daily GamePanel Lite HTTPS certificate renewal check

[Timer]
OnCalendar=daily
RandomizedDelaySec=6h
Persistent=true
Unit=$UNIT_NAME.service

[Install]
WantedBy=timers.target
EOF

install -d -m 0755 "$CONFIG_DIR" "$RUNNER_DIR"
install -m 0600 "$TMP_DIR/https-renewal.env" "$CONFIG_FILE"
install -m 0755 "$RUNNER_SOURCE" "$RUNNER_FILE"
install -m 0644 "$TMP_DIR/$UNIT_NAME.service" "$SERVICE_FILE"
install -m 0644 "$TMP_DIR/$UNIT_NAME.timer" "$TIMER_FILE"

systemctl daemon-reload
if command -v systemd-analyze >/dev/null 2>&1; then
  systemd-analyze verify "$SERVICE_FILE" "$TIMER_FILE"
fi
systemctl enable --now "$UNIT_NAME.timer"

echo "Automatic HTTPS renewal is enabled."
systemctl show "$UNIT_NAME.timer" \
  --property=ActiveState \
  --property=NextElapseUSecRealtime \
  --no-pager
echo "Run a check now with: sudo systemctl start $UNIT_NAME.service"
echo "View logs with: sudo journalctl -u $UNIT_NAME.service"
