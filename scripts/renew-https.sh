#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker is required. Install Docker first, then run this script again."
  exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
  echo "Docker Compose is required. Install the Docker Compose plugin first."
  exit 1
fi

cd "$ROOT_DIR"
docker compose -f compose.prod.yaml -f compose.https.yaml run --rm certbot renew
docker compose -f compose.prod.yaml -f compose.https.yaml exec -T nginx-https nginx -s reload

echo "HTTPS certificate renewal check completed."
