#!/usr/bin/env bash
set -euo pipefail

REPO_RAW_BASE="${GP_RAW_BASE:-https://raw.githubusercontent.com/smartcat999/game-panel-lite/main}"
INSTALL_DIR="${GP_INSTALL_DIR:-game-panel-lite}"

info() {
  printf '[GamePanel Lite] %s\n' "$*"
}

fail() {
  printf '[GamePanel Lite] ERROR: %s\n' "$*" >&2
  exit 1
}

rand_hex() {
  bytes="${1:-24}"
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex "$bytes"
  else
    head -c "$bytes" /dev/urandom | od -An -tx1 | tr -d ' \n'
  fi
}

rand_pass() {
  len="${1:-12}"
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 24 | LC_ALL=C tr -dc 'A-Za-z0-9' | cut -c1-"$len"
  else
    dd if=/dev/urandom bs=64 count=1 2>/dev/null | LC_ALL=C tr -dc 'A-Za-z0-9' | cut -c1-"$len"
  fi
}

download_file() {
  rel_path="$1"
  mkdir -p "$(dirname "$rel_path")"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "${REPO_RAW_BASE}/${rel_path}" -o "$rel_path"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$rel_path" "${REPO_RAW_BASE}/${rel_path}"
  else
    fail "curl or wget is required to download deployment files."
  fi
}

upsert_env_val() {
  key="$1"
  val="$2"
  tmp_file=".env.tmp.$$"
  awk -v k="$key" -v v="$val" '
    BEGIN { found = 0 }
    index($0, k "=") == 1 { print k "=" v; found = 1; next }
    { print }
    END { if (!found) print k "=" v }
  ' .env >"$tmp_file" && mv "$tmp_file" .env
}

get_env_val() {
  key="$1"
  fallback="$2"
  val=$(awk -v k="$key" 'index($0, k "=") == 1 { sub("^" k "=", ""); print; exit }' .env 2>/dev/null || true)
  if [ -n "$val" ]; then
    printf '%s' "$val"
  else
    printf '%s' "$fallback"
  fi
}

port_in_use() {
  p="$1"
  if command -v ss >/dev/null 2>&1; then
    ss -tuln 2>/dev/null | awk -v p="$p" '
      NR>1 {
        col=$5; n=length(p); l=length(col)
        if (l > n && substr(col, l-n) == ":" p) found=1
      }
      END { exit !found }
    ' && return 0
  elif command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"$p" -sTCP:LISTEN >/dev/null 2>&1 && return 0
  elif command -v netstat >/dev/null 2>&1; then
    netstat -tuln 2>/dev/null | awk -v p="$p" '
      {
        col=$4; n=length(p); l=length(col)
        if (l > n && (substr(col, l-n) == ":" p || substr(col, l-n) == "." p)) found=1
      }
      END { exit !found }
    ' && return 0
  fi
  return 1
}

USED_PORTS=""
pick_free_port() {
  default_port="$1"
  fallback_port="$2"
  candidate="$default_port"
  if port_in_use "$candidate" || printf '%s\n' "$USED_PORTS" | grep -qx "$candidate"; then
    candidate="$fallback_port"
  fi
  while port_in_use "$candidate" || printf '%s\n' "$USED_PORTS" | grep -qx "$candidate"; do
    candidate=$((candidate + 1))
  done
  USED_PORTS="${USED_PORTS}
${candidate}"
  printf '%s' "$candidate"
}

work_dir_in_use() {
  target_dir="$1"
  running_ids="$(docker ps -q 2>/dev/null || true)"
  [ -z "$running_ids" ] && return 1
  # shellcheck disable=SC2086
  docker inspect --format '{{.Name}} {{range .Mounts}}{{.Source}} {{end}}' $running_ids 2>/dev/null \
    | awk -v dir=" ${target_dir} " '!/^\/gamepanel-lite-/ && index($0, dir) > 0 { found=1 } END { exit !found }'
}

apply_port_defaults() {
  port_specs="
GP_WEB_PORT:3000:3001
GP_APISERVER_PORT:8080:38080
GP_AGENT_PORT:8082:18086
GP_POSTGRES_PORT:5432:15432
GP_REDIS_PORT:6379:16380
GP_NATS_PORT:4222:14222
GP_NATS_MONITOR_PORT:8222:18222
GP_SEAWEEDFS_S3_PORT:8333:18333
GP_SEAWEEDFS_MASTER_PORT:9333:19333
GP_SEAWEEDFS_ADMIN_PORT:23646:33646
GP_PROMETHEUS_PORT:9090:19090
"
  for spec in $port_specs; do
    key="${spec%%:*}"
    rest="${spec#*:}"
    def_port="${rest%%:*}"
    alt_port="${rest#*:}"
    caller_val="$(printenv "$key" 2>/dev/null || true)"
    if [ -n "$caller_val" ]; then
      chosen="$caller_val"
      USED_PORTS="${USED_PORTS}
${chosen}"
    else
      chosen="$(pick_free_port "$def_port" "$alt_port")"
      if [ "$chosen" != "$def_port" ]; then
        info "Port ${def_port} is busy, using ${key}=${chosen}"
      fi
    fi
    upsert_env_val "$key" "$chosen"
  done
}

apply_caller_overrides() {
  override_keys="
GP_WEB_BIND_IP GP_BIND_IP GP_BOOTSTRAP_ADMIN GP_BOOTSTRAP_EMAIL GP_BOOTSTRAP_PASSWORD
GP_AUTH_JWT_SECRET GP_AGENT_TOKEN GP_REGISTRY_WORKER_TOKEN
POSTGRES_USER POSTGRES_PASSWORD POSTGRES_DB
GP_S3_ACCESS_KEY GP_S3_SECRET_KEY GP_S3_BUCKET GP_S3_PUBLIC_ENDPOINT SEAWEEDFS_ADMIN_PASSWORD
GP_NODE_NAME GP_AGENT_REGION GP_AGENT_EXTERNAL_IP GP_AGENT_INTERNAL_IP
GP_NETWORK_PORT_RANGE_MIN GP_NETWORK_PORT_RANGE_MAX GP_AGENT_WORK_DIR
GP_AGENT_RESERVED_CPU GP_AGENT_RESERVED_MEMORY
STEAM_WEB_API_KEY GP_AGENT_UPSTREAM_PROXY GP_AGENT_DOWNLOAD_PROXY
GP_ADDON_MIRROR_IMAGE_HOST_MIRRORS GP_IMAGE_REGISTRY GP_IMAGE_TAG
"
  for key in $override_keys; do
    val="$(printenv "$key" 2>/dev/null || true)"
    if [ -n "$val" ]; then
      upsert_env_val "$key" "$val"
    fi
  done
}

wait_for_web() {
  probe_host="$1"
  probe_port="$2"
  url="http://${probe_host}:${probe_port}/login"
  info "Waiting for web console at ${url} ..."
  i=0
  while [ "$i" -lt 45 ]; do
    if command -v curl >/dev/null 2>&1; then
      code="$(curl -s -o /dev/null -w '%{http_code}' "$url" || true)"
      if [ "$code" = "200" ] || [ "$code" = "307" ]; then
        return 0
      fi
    elif command -v wget >/dev/null 2>&1; then
      if wget -q --spider "$url" >/dev/null 2>&1; then
        return 0
      fi
    else
      return 0
    fi
    i=$((i + 1))
    sleep 2
  done
  info "Web console is still initializing; inspect logs with: docker compose logs -f web apiserver"
}

command -v docker >/dev/null 2>&1 || fail "Docker is not installed. Please install Docker first: https://docs.docker.com/engine/install/"
docker compose version >/dev/null 2>&1 || fail "Docker Compose v2 plugin is required ('docker compose version' failed)."

if [ ! -f "docker-compose.yaml" ] || [ ! -f "configs/prometheus.yaml" ] || [ ! -f "deploy/seaweedfs/s3.json" ]; then
  info "Preparing deployment files in ./${INSTALL_DIR} ..."
  mkdir -p "$INSTALL_DIR"
  cd "$INSTALL_DIR"
  for f in docker-compose.yaml .env.example configs/prometheus.yaml deploy/seaweedfs/s3.json; do
    if [ ! -f "$f" ]; then
      download_file "$f"
    fi
  done
fi

created_env=0
if [ ! -f ".env" ]; then
  created_env=1
  info "Generating .env with randomized secrets and conflict-free host ports ..."
  cp .env.example .env

  upsert_env_val "GP_BOOTSTRAP_PASSWORD" "$(rand_pass 12)"
  upsert_env_val "GP_AUTH_JWT_SECRET" "$(rand_hex 32)"
  upsert_env_val "GP_AGENT_TOKEN" "$(rand_hex 24)"
  upsert_env_val "GP_REGISTRY_WORKER_TOKEN" "$(rand_hex 24)"
  upsert_env_val "POSTGRES_PASSWORD" "$(rand_hex 16)"
  upsert_env_val "SEAWEEDFS_ADMIN_PASSWORD" "$(rand_hex 16)"

  apply_port_defaults

  if [ -z "${GP_AGENT_WORK_DIR:-}" ] && work_dir_in_use "/var/lib/gamepanel/instances"; then
    info "/var/lib/gamepanel/instances is already mounted by another container, using /var/lib/gamepanel-lite/instances"
    upsert_env_val "GP_AGENT_WORK_DIR" "/var/lib/gamepanel-lite/instances"
    if [ -z "${GP_NETWORK_PORT_RANGE_MIN:-}" ] && [ -z "${GP_NETWORK_PORT_RANGE_MAX:-}" ]; then
      upsert_env_val "GP_NETWORK_PORT_RANGE_MIN" "40000"
      upsert_env_val "GP_NETWORK_PORT_RANGE_MAX" "49999"
    fi
  fi

  apply_caller_overrides
else
  info "Using existing .env configuration."
fi

info "Pulling images and starting GamePanel Lite containers ..."
docker compose -f docker-compose.yaml up -d

web_port="$(get_env_val GP_WEB_PORT 3000)"
web_bind="$(get_env_val GP_WEB_BIND_IP 0.0.0.0)"
probe_ip="$web_bind"
[ "$probe_ip" = "0.0.0.0" ] && probe_ip="127.0.0.1"
wait_for_web "$probe_ip" "$web_port"

port_min="$(get_env_val GP_NETWORK_PORT_RANGE_MIN 20000)"
port_max="$(get_env_val GP_NETWORK_PORT_RANGE_MAX 29999)"
admin_user="$(get_env_val GP_BOOTSTRAP_ADMIN admin)"
admin_pass="$(get_env_val GP_BOOTSTRAP_PASSWORD ChangeMe!2026)"

host_ip=""
if command -v hostname >/dev/null 2>&1; then
  host_ip="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
fi
[ -z "$host_ip" ] && host_ip="<SERVER_IP>"

printf '\n==============================================================\n'
printf ' GamePanel Lite is ready!\n'
printf ' Panel URL      : http://%s:%s\n' "$host_ip" "$web_port"
if [ "$created_env" -eq 1 ]; then
  printf ' Admin User     : %s\n' "$admin_user"
  printf ' Admin Password : %s\n' "$admin_pass"
  printf ' Config File    : %s/.env (saved)\n' "$(pwd)"
fi
printf ' Firewall Ports : %s/tcp (Web), %s-%s/tcp+udp (Games)\n' "$web_port" "$port_min" "$port_max"
printf '==============================================================\n'
