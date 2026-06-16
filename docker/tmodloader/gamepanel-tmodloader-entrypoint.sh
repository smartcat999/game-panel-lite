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

if [[ -s "${MODS_DIR}/install.txt" ]]; then
  export STEAMCMDPATH="/opt/steamcmd/steamcmd.sh"
  if command -v steamcmd >/dev/null 2>&1 || command -v steamcmd.sh >/dev/null 2>&1; then
    echo "Syncing Workshop mods from install.txt..."
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
