#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${DST_ROOT_DIR:-/home/container}"
SERVER_DIR="${ROOT_DIR}/server"
PERSISTENT_ROOT="${DST_PERSISTENT_ROOT:-/data}"
CONF_DIR="${DST_CONF_DIR:-dst}"
CLUSTER_NAME="${DST_CLUSTER_NAME:-GamePanelLite}"
CLUSTER_DIR="${PERSISTENT_ROOT}/${CONF_DIR}/${CLUSTER_NAME}"
UGC_DIR="${DST_UGC_DIRECTORY:-${PERSISTENT_ROOT}/ugc_mods}"
MOD_SYNC_MODE="${DST_MOD_SYNC_MODE:-reuse}"

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
  mkdir -p "${CLUSTER_DIR}/Master" "${UGC_DIR}"
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

  local generated_mod_setup="${CLUSTER_DIR}/dedicated_server_mods_setup.lua"
  local runtime_mod_setup="${SERVER_DIR}/mods/dedicated_server_mods_setup.lua"
  if [[ -f "${generated_mod_setup}" ]]; then
    mkdir -p "${SERVER_DIR}/mods"
    cp "${generated_mod_setup}" "${runtime_mod_setup}.tmp"
    chmod 0644 "${runtime_mod_setup}.tmp"
    mv "${runtime_mod_setup}.tmp" "${runtime_mod_setup}"
    echo "Installed GamePanel DST Workshop manifest."
  fi
}

configured_workshop_ids() {
  local setup_file="${SERVER_DIR}/mods/dedicated_server_mods_setup.lua"
  [[ -f "${setup_file}" ]] || return 0
  sed -n 's/^[[:space:]]*ServerModSetup("\([0-9][0-9]*\)").*/\1/p' "${setup_file}"
}

mod_is_cached() {
  local ugc_dir="$1"
  local workshop_id="$2"
  [[ -f "${ugc_dir}/content/322330/${workshop_id}/modinfo.lua" ]] \
    || [[ -f "${SERVER_DIR}/mods/workshop-${workshop_id}/modinfo.lua" ]]
}

missing_workshop_ids() {
  local ugc_dir="$1"
  local workshop_id
  while IFS= read -r workshop_id; do
    [[ -n "${workshop_id}" ]] || continue
    if ! mod_is_cached "${ugc_dir}" "${workshop_id}"; then
      printf '%s\n' "${workshop_id}"
    fi
  done < <(configured_workshop_ids)
}

download_server_mods() {
  local ugc_dir="$1"
  local bin
  bin="$(server_bin)"
  mkdir -p "${ugc_dir}" "${ugc_dir}/content/322330" "${ugc_dir}/downloads" "${ugc_dir}/temp"
  cd "${SERVER_DIR}/bin64" 2>/dev/null || cd "${SERVER_DIR}/bin"
  "${bin}" \
    -only_update_server_mods \
    -persistent_storage_root "${PERSISTENT_ROOT}" \
    -conf_dir "${CONF_DIR}" \
    -cluster "${CLUSTER_NAME}" \
    -shard "Master" \
    -ugc_directory "${ugc_dir}"
}

sync_server_mods() {
  local configured_count
  configured_count="$(configured_workshop_ids | sed '/^$/d' | wc -l | tr -d ' ')"
  if [[ "${configured_count}" == "0" ]]; then
    echo "No GamePanel DST server mods configured."
    return 0
  fi

  if [[ "${MOD_SYNC_MODE}" == "refresh" ]]; then
    local refresh_dir="${UGC_DIR}.refresh"
    local previous_dir="${UGC_DIR}.previous"
    rm -rf "${refresh_dir}"
    echo "Refreshing all ${configured_count} GamePanel DST server mods..."
    if ! download_server_mods "${refresh_dir}"; then
      echo "DST Workshop refresh failed; keeping the previous cache." >&2
      rm -rf "${refresh_dir}"
      exit 1
    fi
    local missing
    missing="$(missing_workshop_ids "${refresh_dir}")"
    if [[ -n "${missing}" ]]; then
      echo "DST Workshop refresh failed; missing IDs: ${missing//$'\n'/, }" >&2
      rm -rf "${refresh_dir}"
      exit 1
    fi
    rm -rf "${previous_dir}"
    if [[ -d "${UGC_DIR}" ]]; then
      mv "${UGC_DIR}" "${previous_dir}"
    fi
    mv "${refresh_dir}" "${UGC_DIR}"
    rm -rf "${previous_dir}"
    echo "Refreshed and verified all GamePanel DST server mods."
    return 0
  fi

  local missing
  missing="$(missing_workshop_ids "${UGC_DIR}")"
  if [[ -z "${missing}" ]]; then
    echo "Reusing verified GamePanel DST Workshop cache."
    return 0
  fi
  echo "Completing missing GamePanel DST Workshop cache: ${missing//$'\n'/, }"
  if ! download_server_mods "${UGC_DIR}"; then
    echo "DST Workshop cache completion failed." >&2
    exit 1
  fi
  missing="$(missing_workshop_ids "${UGC_DIR}")"
  if [[ -n "${missing}" ]]; then
    echo "DST Workshop cache is incomplete; missing IDs: ${missing//$'\n'/, }" >&2
    exit 1
  fi
  echo "GamePanel DST Workshop cache is complete."
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
    -ugc_directory "${UGC_DIR}" \
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
sync_server_mods
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
