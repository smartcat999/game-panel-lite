#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE="$ROOT_DIR/.env"

read_env_value() {
  key="$1"
  [ -f "$ENV_FILE" ] || return 0
  if [ -r "$ENV_FILE" ]; then
    line=$(sed -n "s/^${key}=//p" "$ENV_FILE" | tail -n 1)
  elif command -v sudo >/dev/null 2>&1; then
    line=$(sudo sed -n "s/^${key}=//p" "$ENV_FILE" | tail -n 1)
  else
    return 0
  fi
  printf '%s' "$line" | sed 's/^"//; s/"$//'
}

update_existing_region() {
  source_file=$(mktemp)
  output_file=$(mktemp)
  if [ -r "$ENV_FILE" ]; then
    cp "$ENV_FILE" "$source_file"
  elif command -v sudo >/dev/null 2>&1; then
    sudo cp "$ENV_FILE" "$source_file"
    sudo chown "$(id -u):$(id -g)" "$source_file"
  else
    echo "Cannot read existing $ENV_FILE" >&2
    return 1
  fi
  awk -v region="$IMAGE_REGION" -v registry="$IMAGE_REGISTRY" '
    BEGIN { saw_region=0; saw_registry=0 }
    /^GAMEPANEL_IMAGE_REGION=/ { print "GAMEPANEL_IMAGE_REGION=\"" region "\""; saw_region=1; next }
    /^GAMEPANEL_IMAGE_REGISTRY=/ { print "GAMEPANEL_IMAGE_REGISTRY=\"" registry "\""; saw_registry=1; next }
    { print }
    END {
      if (!saw_region) print "GAMEPANEL_IMAGE_REGION=\"" region "\""
      if (!saw_registry) print "GAMEPANEL_IMAGE_REGISTRY=\"" registry "\""
    }
  ' "$source_file" > "$output_file"
  if [ -w "$ENV_FILE" ]; then
    cp "$output_file" "$ENV_FILE"
  elif command -v sudo >/dev/null 2>&1; then
    sudo cp "$output_file" "$ENV_FILE"
  else
    echo "Cannot update existing $ENV_FILE" >&2
    return 1
  fi
  rm -f "$source_file" "$output_file"
}

CURRENT_IMAGE_REGION=$(read_env_value GAMEPANEL_IMAGE_REGION)
CURRENT_IMAGE_REGISTRY=$(read_env_value GAMEPANEL_IMAGE_REGISTRY)
case "$CURRENT_IMAGE_REGION" in
  global|cn) ;;
  *)
    case "$CURRENT_IMAGE_REGISTRY" in
      registry.cn-hangzhou.aliyuncs.com/gamepanel-lite) CURRENT_IMAGE_REGION="cn" ;;
      *) CURRENT_IMAGE_REGION="global" ;;
    esac
    ;;
esac

IMAGE_REGION="${GAMEPANEL_IMAGE_REGION:-}"
if [ -z "$IMAGE_REGION" ]; then
  if [ -r /dev/tty ] && [ -w /dev/tty ]; then
    default_selection="1"
    [ "$CURRENT_IMAGE_REGION" = "cn" ] && default_selection="2"
    while :; do
      cat >/dev/tty <<EOF

Select the control-plane image region / 选择控制平面镜像区域：
  1) Global, Docker Hub / 全球，Docker Hub
  2) China Mainland, Alibaba Cloud / 中国大陆，阿里云
EOF
      if [ -f "$ENV_FILE" ]; then
        printf "Current / 当前: %s\n" "$CURRENT_IMAGE_REGION" >/dev/tty
      fi
      printf "Selection / 请选择 [%s]: " "$default_selection" >/dev/tty
      IFS= read -r choice </dev/tty || choice=""
      case "$choice" in
        "") IMAGE_REGION="$CURRENT_IMAGE_REGION"; break ;;
        1) IMAGE_REGION="global"; break ;;
        2) IMAGE_REGION="cn"; break ;;
        *) echo "Enter 1 or 2 / 请输入 1 或 2。" >/dev/tty ;;
      esac
    done
  else
    IMAGE_REGION="$CURRENT_IMAGE_REGION"
    echo "No interactive terminal detected; keeping image region: $IMAGE_REGION"
  fi
fi
case "$IMAGE_REGION" in
  global) DEFAULT_IMAGE_REGISTRY="smartcat99999" ;;
  cn) DEFAULT_IMAGE_REGISTRY="registry.cn-hangzhou.aliyuncs.com/gamepanel-lite" ;;
  *) echo "GAMEPANEL_IMAGE_REGION must be global or cn" >&2; exit 2 ;;
esac
IMAGE_REGISTRY="${GAMEPANEL_IMAGE_REGISTRY:-$DEFAULT_IMAGE_REGISTRY}"

echo "Control-plane image region: $IMAGE_REGION"
echo "Control-plane image registry: $IMAGE_REGISTRY"

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker is required. Install Docker first, then run this script again."
  exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
  echo "Docker Compose is required. Install the Docker Compose plugin first."
  exit 1
fi

if [ ! -f "$ENV_FILE" ]; then
  UPDATER_TOKEN=$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')
  cat > "$ENV_FILE" <<EOF
GAMEPANEL_WORKSPACE_PATH="$ROOT_DIR"
GAMEPANEL_DOCKER_SOCKET_PATH="/var/run/docker.sock"
GAMEPANEL_WEB_PORT="3001"
GAMEPANEL_API_PORT="4000"
NEXT_PUBLIC_API_BASE_URL=""
GAMEPANEL_IMAGE_REGION="$IMAGE_REGION"
GAMEPANEL_IMAGE_REGISTRY="$IMAGE_REGISTRY"
GAMEPANEL_IMAGE_TAG="v0.2.11"
GAMEPANEL_PALWORLD_MOD_PACK_TAG="v0.1.0"
GAMEPANEL_UPDATER_TOKEN="$UPDATER_TOKEN"
EOF
elif [ "$CURRENT_IMAGE_REGION" != "$IMAGE_REGION" ] || [ "$CURRENT_IMAGE_REGISTRY" != "$IMAGE_REGISTRY" ]; then
  backup_file="$ENV_FILE.bak-region-$(date +%Y%m%d-%H%M%S)"
  if [ -r "$ENV_FILE" ] && [ -w "$(dirname "$ENV_FILE")" ]; then
    cp "$ENV_FILE" "$backup_file"
  elif command -v sudo >/dev/null 2>&1; then
    sudo cp "$ENV_FILE" "$backup_file"
  else
    echo "Cannot back up existing $ENV_FILE" >&2
    exit 1
  fi
  update_existing_region
  echo "Updated existing image region. Backup: $backup_file"
else
  echo "Existing image region is unchanged."
fi

mkdir -p "$ROOT_DIR/data"
mkdir -p "$ROOT_DIR/data/prometheus"

if chown 65534:65534 "$ROOT_DIR/data/prometheus" >/dev/null 2>&1; then
  :
elif command -v sudo >/dev/null 2>&1 && sudo chown 65534:65534 "$ROOT_DIR/data/prometheus" >/dev/null 2>&1; then
  :
else
  chmod 777 "$ROOT_DIR/data/prometheus"
fi

cd "$ROOT_DIR"
docker compose -f compose.prod.yaml pull
docker compose -f compose.prod.yaml up -d updater api web nginx gamepanel-exporter prometheus cadvisor node-exporter

echo
echo "GamePanel Lite is starting."
echo "Open: http://localhost:3001"
echo "Data directory: $ROOT_DIR/data"
echo "Manage with: sh scripts/manage.sh <start|update|stop|status>"
