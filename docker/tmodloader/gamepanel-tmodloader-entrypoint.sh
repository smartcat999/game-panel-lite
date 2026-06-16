#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="/home/container"
MODS_DIR="${ROOT_DIR}/Mods"
WORKSHOP_DIR="${ROOT_DIR}/steamapps/workshop/content/1281930"
MANAGE_SCRIPT="${ROOT_DIR}/server/DedicatedServerUtils/manage-tModLoaderServer.sh"

mkdir -p "${MODS_DIR}"

if [[ -x "${MANAGE_SCRIPT}" || -r "${MANAGE_SCRIPT}" ]]; then
  chmod +x "${MANAGE_SCRIPT}" 2>/dev/null || true
fi

workshop_mod_cached() {
  local workshop_id="$1"
  [[ -n "${workshop_id}" ]] || return 1
  [[ -d "${WORKSHOP_DIR}/${workshop_id}" ]] || return 1
  find "${WORKSHOP_DIR}/${workshop_id}" -type f -name '*.tmod' -print -quit 2>/dev/null | grep -q .
}

workshop_sync_needed() {
  while IFS= read -r workshop_id || [[ -n "${workshop_id}" ]]; do
    workshop_id="${workshop_id//[$'\t\r\n ']}"
    [[ -n "${workshop_id}" ]] || continue
    if ! workshop_mod_cached "${workshop_id}"; then
      return 0
    fi
  done <"${MODS_DIR}/install.txt"
  return 1
}

if [[ -s "${MODS_DIR}/install.txt" ]]; then
  export STEAMCMDPATH="/opt/steamcmd/steamcmd.sh"
  arch="$(uname -m)"
  if [[ "${arch}" != "x86_64" && "${arch}" != "amd64" ]]; then
    echo "Workshop sync skipped: downloading Workshop mods is only supported on x86_64. Upload .tmod files on ARM hosts."
  elif ! workshop_sync_needed; then
    echo "Workshop sync skipped: cached Workshop mods are already present."
  elif command -v steamcmd >/dev/null 2>&1 || command -v steamcmd.sh >/dev/null 2>&1; then
    echo "Syncing missing Workshop mods from install.txt..."
    bash "${MANAGE_SCRIPT}" install-mods -f "${ROOT_DIR}"
  else
    echo "Workshop sync skipped: steamcmd is not available in the container"
  fi
fi

tmp_enabled="$(mktemp)"
printf '[\n' >"${tmp_enabled}"
first=1
while IFS= read -r mod_name; do
  [[ -z "${mod_name}" ]] && continue
  if [[ "${first}" -eq 0 ]]; then
    printf ',\n' >>"${tmp_enabled}"
  fi
  printf '  "%s"' "${mod_name}" >>"${tmp_enabled}"
  first=0
done < <(
  {
    find "${MODS_DIR}" -maxdepth 1 -type f -name '*.tmod' -exec basename {} .tmod \; 2>/dev/null
    find "${WORKSHOP_DIR}" -type f -name '*.tmod' -exec basename {} .tmod \; 2>/dev/null
  } | sort -u
)
printf '\n]\n' >>"${tmp_enabled}"
mv "${tmp_enabled}" "${MODS_DIR}/enabled.json"

exec ./server/start-tModLoaderServer.sh "$@"
