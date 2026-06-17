#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="/home/container"
SERVER_DIR="${ROOT_DIR}/server"
PERSISTENT_ROOT="${DST_PERSISTENT_ROOT:-/data}"
CONF_DIR="${DST_CONF_DIR:-dst}"
CLUSTER_NAME="${DST_CLUSTER_NAME:-GamePanelLite}"
CLUSTER_DIR="${PERSISTENT_ROOT}/${CONF_DIR}/${CLUSTER_NAME}"

server_bin() {
  if [[ -x "${SERVER_DIR}/bin64/dontstarve_dedicated_server_nullrenderer_x64" ]]; then
    printf '%s\n' "${SERVER_DIR}/bin64/dontstarve_dedicated_server_nullrenderer_x64"
    return
  fi
  if [[ -x "${SERVER_DIR}/bin/dontstarve_dedicated_server_nullrenderer" ]]; then
    printf '%s\n' "${SERVER_DIR}/bin/dontstarve_dedicated_server_nullrenderer"
    return
  fi
  echo "Don't Starve Together server binary was not found." >&2
  exit 1
}

ensure_cluster_layout() {
  mkdir -p "${CLUSTER_DIR}/Master"
  if [[ -f "${CLUSTER_DIR}/server_token.txt" && ! -f "${CLUSTER_DIR}/cluster_token.txt" ]]; then
    cp "${CLUSTER_DIR}/server_token.txt" "${CLUSTER_DIR}/cluster_token.txt"
  fi
  if [[ ! -f "${CLUSTER_DIR}/cluster_token.txt" ]]; then
    echo "Missing Klei server token at ${CLUSTER_DIR}/cluster_token.txt" >&2
    exit 1
  fi
  if [[ ! -f "${CLUSTER_DIR}/cluster.ini" ]]; then
    echo "Missing DST cluster config at ${CLUSTER_DIR}/cluster.ini" >&2
    exit 1
  fi
}

start_shard() {
  local shard="$1"
  local bin
  bin="$(server_bin)"
  cd "${SERVER_DIR}/bin64" 2>/dev/null || cd "${SERVER_DIR}/bin"
  "${bin}" \
    -persistent_storage_root "${PERSISTENT_ROOT}" \
    -conf_dir "${CONF_DIR}" \
    -cluster "${CLUSTER_NAME}" \
    -shard "${shard}" \
    -console
}

terminate_children() {
  if [[ -n "${caves_pid:-}" ]]; then
    kill "${caves_pid}" 2>/dev/null || true
  fi
  if [[ -n "${master_pid:-}" ]]; then
    kill "${master_pid}" 2>/dev/null || true
  fi
}

ensure_cluster_layout
trap terminate_children TERM INT

if [[ -f "${CLUSTER_DIR}/Caves/server.ini" ]]; then
  echo "Starting DST Caves shard..."
  start_shard "Caves" &
  caves_pid="$!"
fi

echo "Starting DST Master shard..."
if [[ -n "${caves_pid:-}" ]]; then
  start_shard "Master" &
  master_pid="$!"
  wait -n "${master_pid}" "${caves_pid}"
  exit_code="$?"
  terminate_children
  wait "${master_pid}" 2>/dev/null || true
  wait "${caves_pid}" 2>/dev/null || true
  exit "${exit_code}"
fi

start_shard "Master"
