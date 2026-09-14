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
GAME_UPDATE_MODE="${DST_GAME_UPDATE_MODE:-${MOD_SYNC_MODE}}"
DST_STEAM_APP_ID="${DST_STEAM_APP_ID:-343050}"
STEAMCMD_BIN="${DST_STEAMCMD_BIN:-/usr/games/steamcmd}"
LEGACY_WORKSHOP_FALLBACK="${DST_LEGACY_WORKSHOP_FALLBACK:-1}"
WORKSHOP_DETAILS_ENDPOINT="${DST_WORKSHOP_DETAILS_ENDPOINT:-https://api.steampowered.com/ISteamRemoteStorage/GetPublishedFileDetails/v1/}"
WORKSHOP_REFRESH_ATTEMPTS="${DST_WORKSHOP_REFRESH_ATTEMPTS:-6}"

record_installed_build() {
  local manifest="${SERVER_DIR}/steamapps/appmanifest_${DST_STEAM_APP_ID}.acf"
  local build_id=""
  [[ -f "${manifest}" ]] || return 0
  build_id="$(sed -n 's/^[[:space:]]*"buildid"[[:space:]]*"\([0-9][0-9]*\)".*/\1/p' "${manifest}" | head -n 1)"
  [[ -n "${build_id}" ]] || return 0
  mkdir -p "${PERSISTENT_ROOT}/.gamepanel"
  printf '%s\n' "${build_id}" >"${PERSISTENT_ROOT}/.gamepanel/dst-build-id"
}

update_server_game() {
  if [[ "${GAME_UPDATE_MODE}" != "refresh" ]]; then
    record_installed_build
    return 0
  fi
  if [[ ! -x "${STEAMCMD_BIN}" ]]; then
    echo "DST restart update requires SteamCMD at ${STEAMCMD_BIN}." >&2
    exit 1
  fi

  local attempt
  echo "Checking for DST game updates before restart..."
  for attempt in 1 2 3; do
    if "${STEAMCMD_BIN}" \
      +force_install_dir "${SERVER_DIR}" \
      +login anonymous \
      +app_update "${DST_STEAM_APP_ID}" \
      +quit; then
      record_installed_build
      echo "DST game files are up to date."
      return 0
    fi
    echo "DST game update attempt ${attempt} failed." >&2
  done
  echo "DST game update failed after 3 attempts; the server will not start with stale files." >&2
  exit 1
}

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
  local setup_file="${CLUSTER_DIR}/dedicated_server_mods_setup.lua"
  if [[ ! -f "${setup_file}" ]]; then
    setup_file="${SERVER_DIR}/mods/dedicated_server_mods_setup.lua"
  fi
  [[ -f "${setup_file}" ]] || return 0
  sed -n 's/^[[:space:]]*ServerModSetup("\([0-9][0-9]*\)").*/\1/p' "${setup_file}"
}

legacy_mod_dir() {
  local ugc_dir="$1"
  local workshop_id="$2"
  printf '%s\n' "${ugc_dir}/legacy/workshop-${workshop_id}"
}

mod_is_cached() {
  local ugc_dir="$1"
  local workshop_id="$2"
  [[ -f "${ugc_dir}/content/322330/${workshop_id}/modinfo.lua" ]] \
    || [[ -f "$(legacy_mod_dir "${ugc_dir}" "${workshop_id}")/modinfo.lua" ]]
}

workshop_file_url() {
  local workshop_id="$1"
  local response
  response="$(curl -fsS --retry 2 --connect-timeout 10 --max-time 30 \
    -X POST "${WORKSHOP_DETAILS_ENDPOINT}" \
    --data-urlencode "itemcount=1" \
    --data-urlencode "publishedfileids[0]=${workshop_id}")" || return 1
  printf '%s' "${response}" | sed -n 's/.*"file_url":"\([^"]*\)".*/\1/p'
}

download_legacy_workshop_mod() {
  local ugc_dir="$1"
  local workshop_id="$2"
  local file_url="$3"
  local target archive staging
  target="$(legacy_mod_dir "${ugc_dir}" "${workshop_id}")"
  archive="${ugc_dir}/.legacy-${workshop_id}.zip"
  staging="${ugc_dir}/.legacy-${workshop_id}.tmp"

  rm -rf "${archive}" "${staging}"
  mkdir -p "${ugc_dir}" "${staging}"
  echo "Downloading legacy DST Workshop mod ${workshop_id}..."
  if ! curl -fL --retry 2 --connect-timeout 10 --max-time 180 --max-filesize 1073741824 \
    "${file_url}" -o "${archive}"; then
    rm -rf "${archive}" "${staging}"
    return 1
  fi
  if unzip -Z1 "${archive}" | grep -Eq '(^/|(^|[/\\])\.\.([/\\]|$))'; then
    echo "Legacy DST Workshop archive ${workshop_id} contains an unsafe path." >&2
    rm -rf "${archive}" "${staging}"
    return 1
  fi
  local unzip_status=0
  unzip -oq "${archive}" -d "${staging}" || unzip_status=$?
  # Info-ZIP returns 1 for warnings such as legacy archives that use Windows
  # backslashes as separators, even when every file was extracted correctly.
  if [[ "${unzip_status}" -gt 1 ]]; then
    rm -rf "${archive}" "${staging}"
    return 1
  fi
  rm -f "${archive}"
  if [[ ! -f "${staging}/modinfo.lua" ]]; then
    echo "Legacy DST Workshop archive ${workshop_id} has no root modinfo.lua." >&2
    rm -rf "${staging}"
    return 1
  fi
  rm -rf "${target}"
  mkdir -p "$(dirname "${target}")"
  mv "${staging}" "${target}"
  echo "Downloaded legacy DST Workshop mod ${workshop_id}."
}

download_missing_legacy_mods() {
  local ugc_dir="$1"
  local workshop_id file_url
  [[ "${LEGACY_WORKSHOP_FALLBACK}" == "1" ]] || return 0
  while IFS= read -r workshop_id; do
    [[ -n "${workshop_id}" ]] || continue
    if mod_is_cached "${ugc_dir}" "${workshop_id}"; then
      continue
    fi
    file_url="$(workshop_file_url "${workshop_id}")" || continue
    [[ -n "${file_url}" ]] || continue
    if ! download_legacy_workshop_mod "${ugc_dir}" "${workshop_id}" "${file_url}"; then
      echo "Legacy DST Workshop download failed for ${workshop_id}; trying the native downloader." >&2
    fi
  done < <(configured_workshop_ids)
}

install_native_workshop_manifest() {
  local ugc_dir="$1"
  local setup_file="${SERVER_DIR}/mods/dedicated_server_mods_setup.lua"
  local tmp_file="${setup_file}.tmp"
  local workshop_id count=0
  mkdir -p "${SERVER_DIR}/mods"
  : >"${tmp_file}"
  while IFS= read -r workshop_id; do
    [[ -n "${workshop_id}" ]] || continue
    if [[ -f "$(legacy_mod_dir "${ugc_dir}" "${workshop_id}")/modinfo.lua" ]]; then
      continue
    fi
    printf 'ServerModSetup("%s")\n' "${workshop_id}" >>"${tmp_file}"
    count=$((count + 1))
  done < <(configured_workshop_ids)
  if [[ "${count}" == "0" ]]; then
    printf 'return nil\n' >"${tmp_file}"
  fi
  chmod 0644 "${tmp_file}"
  mv "${tmp_file}" "${setup_file}"
  printf '%s\n' "${count}"
}

install_legacy_mod_links() {
  local workshop_id target link existing_target
  mkdir -p "${SERVER_DIR}/mods"
  while IFS= read -r link; do
    existing_target="$(readlink "${link}")"
    if [[ "${existing_target}" == "${UGC_DIR}/legacy/"* ]]; then
      rm -f "${link}"
    fi
  done < <(find "${SERVER_DIR}/mods" -maxdepth 1 -type l -name 'workshop-*' -print)
  while IFS= read -r workshop_id; do
    [[ -n "${workshop_id}" ]] || continue
    target="$(legacy_mod_dir "${UGC_DIR}" "${workshop_id}")"
    link="${SERVER_DIR}/mods/workshop-${workshop_id}"
    if [[ -f "${target}/modinfo.lua" ]]; then
      rm -rf "${link}"
      ln -s "${target}" "${link}"
      echo "Linked legacy DST Workshop mod ${workshop_id}."
    fi
  done < <(configured_workshop_ids)
}

filter_native_workshop_noise() {
  # Steam emits these internal contentupdatecontext assertions even when a V2
  # Workshop download completes successfully. Keep all other stderr output;
  # missing mods are still rejected by missing_workshop_ids after the process.
  sed -E \
    -e '/^src\/clientdll\/contentupdatecontext\.cpp \(2036\) : Staging library folder not found$/d' \
    -e '/^src\/clientdll\/contentupdatecontext\.cpp \(2037\) : Install library folder not found$/d'
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
  local bin native_count
  bin="$(server_bin)"
  mkdir -p \
    "${ugc_dir}/content/322330" \
    "${ugc_dir}/downloads/322330" \
    "${ugc_dir}/temp/322330"
  download_missing_legacy_mods "${ugc_dir}"
  native_count="$(install_native_workshop_manifest "${ugc_dir}")"
  if [[ "${native_count}" == "0" ]]; then
    echo "No native UGC Workshop mods require downloading."
    return 0
  fi
  cd "${SERVER_DIR}/bin64" 2>/dev/null || cd "${SERVER_DIR}/bin"
  "${bin}" \
    -only_update_server_mods \
    -persistent_storage_root "${PERSISTENT_ROOT}" \
    -conf_dir "${CONF_DIR}" \
    -cluster "${CLUSTER_NAME}" \
    -shard "Master" \
    -ugc_directory "${ugc_dir}" \
    2>&1 | filter_native_workshop_noise
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
    echo "Refreshing all ${configured_count} GamePanel DST server mods..."
    local attempt missing=""
    for ((attempt = 1; attempt <= WORKSHOP_REFRESH_ATTEMPTS; attempt++)); do
      echo "DST Workshop refresh attempt ${attempt}/${WORKSHOP_REFRESH_ATTEMPTS}..."
      if ! download_server_mods "${refresh_dir}"; then
        echo "DST Workshop refresh attempt ${attempt} failed; preserving downloaded files for retry." >&2
      fi
      missing="$(missing_workshop_ids "${refresh_dir}")"
      if [[ -z "${missing}" ]]; then
        break
      fi
      echo "DST Workshop refresh attempt ${attempt} incomplete; missing IDs: ${missing//$'\n'/, }" >&2
    done
    if [[ -n "${missing}" ]]; then
      echo "DST Workshop refresh failed; missing IDs: ${missing//$'\n'/, }" >&2
      if [[ -z "$(missing_workshop_ids "${UGC_DIR}")" ]]; then
        echo "Continuing with the previous verified GamePanel DST Workshop cache."
        return 0
      fi
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
  exec "${bin}" \
    -persistent_storage_root "${PERSISTENT_ROOT}" \
    -conf_dir "${CONF_DIR}" \
    -cluster "${CLUSTER_NAME}" \
    -shard "${shard}" \
    -skip_update_server_mods \
    -ugc_directory "${UGC_DIR}"
}

setup_console_router() {
  master_console_fifo="/tmp/gamepanel-dst-master-console"
  caves_console_fifo="/tmp/gamepanel-dst-caves-console"
  rm -f "${master_console_fifo}" "${caves_console_fifo}"
  mkfifo "${master_console_fifo}" "${caves_console_fifo}"
  exec 3<&0
  exec 4<>"${master_console_fifo}"
  exec 5<>"${caves_console_fifo}"
}

route_console_commands() {
  local line target payload command
  while IFS= read -r line; do
    if [[ "${line}" == __GAMEPANEL_DST_CONSOLE__:*:* ]]; then
      target="${line#__GAMEPANEL_DST_CONSOLE__:}"
      payload="${target#*:}"
      target="${target%%:*}"
      if ! command="$(printf '%s' "${payload}" | base64 -d 2>/dev/null)"; then
        echo "Rejected malformed GamePanel DST console payload." >&2
        continue
      fi
    else
      target="master"
      command="${line}"
    fi
    case "${target}" in
      master) printf '%s\n' "${command}" >&4 ;;
      caves)
        if [[ -n "${caves_pid:-}" ]]; then
          printf '%s\n' "${command}" >&5
        else
          echo "Rejected DST console command: caves shard is not running." >&2
        fi
        ;;
      *) echo "Rejected DST console command for unknown shard ${target}." >&2 ;;
    esac
  done
}

terminate_children() {
  if [[ -n "${console_router_pid:-}" ]]; then
    kill "${console_router_pid}" 2>/dev/null || true
  fi
  if [[ -n "${caves_pid:-}" ]]; then
    kill "${caves_pid}" 2>/dev/null || true
  fi
  if [[ -n "${master_pid:-}" ]]; then
    kill "${master_pid}" 2>/dev/null || true
  fi
}

shutdown_children() {
  trap - TERM INT
  echo "Gracefully stopping DST shards..."
  if [[ -n "${console_router_pid:-}" ]]; then
    kill "${console_router_pid}" 2>/dev/null || true
  fi
  if [[ -n "${master_pid:-}" ]]; then
    kill "${master_pid}" 2>/dev/null || true
    wait "${master_pid}" 2>/dev/null || true
  fi
  if [[ -n "${caves_pid:-}" ]]; then
    kill "${caves_pid}" 2>/dev/null || true
    wait "${caves_pid}" 2>/dev/null || true
  fi
  echo "DST shards stopped."
  exit 0
}

wait_for_any_shard() {
  while kill -0 "${master_pid}" 2>/dev/null && kill -0 "${caves_pid}" 2>/dev/null; do
    sleep 1
  done
  if ! kill -0 "${master_pid}" 2>/dev/null; then
    wait "${master_pid}"
    return $?
  fi
  wait "${caves_pid}"
}

update_server_game
ensure_cluster_layout
sync_server_mods
install_native_workshop_manifest "${UGC_DIR}" >/dev/null
install_legacy_mod_links
setup_console_router
trap shutdown_children TERM INT

if [[ -f "${CLUSTER_DIR}/Caves/server.ini" ]]; then
  echo "Starting DST Caves shard..."
  start_shard "Caves" <"${caves_console_fifo}" &
  caves_pid="$!"
fi

echo "Starting DST Master shard..."
start_shard "Master" <"${master_console_fifo}" &
master_pid="$!"
route_console_commands <&3 &
console_router_pid="$!"
if [[ -n "${caves_pid:-}" ]]; then
  set +e
  wait_for_any_shard
  exit_code="$?"
  set -e
  terminate_children
  wait "${master_pid}" 2>/dev/null || true
  wait "${caves_pid}" 2>/dev/null || true
  exit "${exit_code}"
fi

set +e
wait "${master_pid}"
exit_code="$?"
set -e
exit "${exit_code}"
