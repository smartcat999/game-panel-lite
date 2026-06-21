#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE="$ROOT_DIR/.env"
DOMAIN="${1:-}"
EMAIL="${2:-}"

if [ -z "$DOMAIN" ]; then
  echo "Usage: sh scripts/setup-https.sh <domain> [email]"
  echo "Example: sh scripts/setup-https.sh dev.gamepanel.site admin@example.com"
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker is required. Install Docker first, then run this script again."
  exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
  echo "Docker Compose is required. Install the Docker Compose plugin first."
  exit 1
fi

mkdir -p "$ROOT_DIR/data/certbot/www" "$ROOT_DIR/data/certbot/conf" "$ROOT_DIR/data/nginx"

if [ ! -f "$ENV_FILE" ]; then
  cat > "$ENV_FILE" <<EOF
GAMEPANEL_WORKSPACE_PATH="$ROOT_DIR"
GAMEPANEL_DOCKER_SOCKET_PATH="/var/run/docker.sock"
GAMEPANEL_WEB_PORT="80"
GAMEPANEL_HTTPS_PORT="443"
NEXT_PUBLIC_API_BASE_URL=""
GAMEPANEL_IMAGE_REGION="global"
GAMEPANEL_IMAGE_REGISTRY="smartcat99999"
GAMEPANEL_IMAGE_TAG="v0.1.0"
GAMEPANEL_DOMAIN="$DOMAIN"
EOF
else
  TMP_ENV="$ENV_FILE.tmp"
  awk -v domain="$DOMAIN" '
    BEGIN { saw_web=0; saw_https=0; saw_domain=0 }
    /^GAMEPANEL_WEB_PORT=/ { print "GAMEPANEL_WEB_PORT=\"80\""; saw_web=1; next }
    /^GAMEPANEL_HTTPS_PORT=/ { print "GAMEPANEL_HTTPS_PORT=\"443\""; saw_https=1; next }
    /^GAMEPANEL_DOMAIN=/ { print "GAMEPANEL_DOMAIN=\"" domain "\""; saw_domain=1; next }
    { print }
    END {
      if (!saw_web) print "GAMEPANEL_WEB_PORT=\"80\""
      if (!saw_https) print "GAMEPANEL_HTTPS_PORT=\"443\""
      if (!saw_domain) print "GAMEPANEL_DOMAIN=\"" domain "\""
    }
  ' "$ENV_FILE" > "$TMP_ENV"
  mv "$TMP_ENV" "$ENV_FILE"
fi

sed "s/__GAMEPANEL_DOMAIN__/$DOMAIN/g" \
  "$ROOT_DIR/deploy/nginx/gamepanel-https.conf.template" \
  > "$ROOT_DIR/data/nginx/gamepanel-https.conf"

cd "$ROOT_DIR"

docker compose -f compose.prod.yaml -f compose.https.yaml stop nginx-https >/dev/null 2>&1 || true
docker compose -f compose.prod.yaml -f compose.https.yaml rm -f nginx-https >/dev/null 2>&1 || true
docker compose -f compose.prod.yaml pull api web nginx
docker compose -f compose.prod.yaml up -d api web nginx

CERTBOT_ARGS="certonly --webroot --webroot-path /var/www/certbot --agree-tos -d $DOMAIN"
if [ -n "$EMAIL" ]; then
  CERTBOT_ARGS="$CERTBOT_ARGS --email $EMAIL --no-eff-email"
else
  CERTBOT_ARGS="$CERTBOT_ARGS --register-unsafely-without-email"
fi

docker compose -f compose.prod.yaml -f compose.https.yaml run --rm certbot $CERTBOT_ARGS

docker compose -f compose.prod.yaml stop nginx >/dev/null 2>&1 || true
docker compose -f compose.prod.yaml rm -f nginx >/dev/null 2>&1 || true
docker compose -f compose.prod.yaml -f compose.https.yaml up -d api web nginx-https gamepanel-exporter prometheus cadvisor node-exporter

echo
echo "HTTPS is ready."
echo "Open: https://$DOMAIN"
echo "Renew with: sh scripts/renew-https.sh"
