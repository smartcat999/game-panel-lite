#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE="$ROOT_DIR/.env"

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker is required. Install Docker first, then run this script again."
  exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
  echo "Docker Compose is required. Install the Docker Compose plugin first."
  exit 1
fi

if [ ! -f "$ENV_FILE" ]; then
  cat > "$ENV_FILE" <<EOF
GAMEPANEL_WORKSPACE_PATH="$ROOT_DIR"
GAMEPANEL_DOCKER_SOCKET_PATH="/var/run/docker.sock"
GAMEPANEL_WEB_PORT="3001"
GAMEPANEL_API_PORT="4000"
NEXT_PUBLIC_API_BASE_URL=""
GAMEPANEL_IMAGE_REGION="global"
GAMEPANEL_IMAGE_REGISTRY="smartcat99999"
GAMEPANEL_IMAGE_TAG="v0.1.0"
EOF
fi

mkdir -p "$ROOT_DIR/data"

cd "$ROOT_DIR"
docker compose -f compose.prod.yaml pull
docker compose -f compose.prod.yaml up -d api web nginx gamepanel-exporter prometheus cadvisor node-exporter

echo
echo "GamePanel Lite is starting."
echo "Open: http://localhost:3001"
echo "Data directory: $ROOT_DIR/data"
