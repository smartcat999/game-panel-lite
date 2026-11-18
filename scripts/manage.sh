#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ACTION="${1:-status}"

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker is required. Install Docker first, then run this script again."
  exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
  echo "Docker Compose is required. Install the Docker Compose plugin first."
  exit 1
fi

cd "$ROOT_DIR"
set -- -f compose.prod.yaml
MODE="HTTP"
if [ -f "$ROOT_DIR/data/nginx/gamepanel-https.conf" ]; then
  set -- "$@" -f compose.https.yaml
  MODE="HTTPS"
fi

SERVICES="updater api web nginx gamepanel-exporter prometheus cadvisor node-exporter"

case "$ACTION" in
  start)
    docker compose "$@" up -d --remove-orphans --pull never $SERVICES
    ;;
  update)
    docker compose "$@" pull $SERVICES
    docker compose "$@" up -d --remove-orphans --pull never $SERVICES
    # Refresh Docker DNS resolutions after API and Web containers are replaced.
    docker compose "$@" up -d --remove-orphans --pull never --no-deps --force-recreate nginx
    ;;
  stop)
    docker compose "$@" stop $SERVICES
    ;;
  status)
    docker compose "$@" ps
    ;;
  *)
    echo "Usage: sh scripts/manage.sh <start|update|stop|status>"
    exit 1
    ;;
esac

echo "GamePanel Lite control plane mode: $MODE"
