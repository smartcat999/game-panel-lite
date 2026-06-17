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

latest_workshop_mod_file() {
  local workshop_id="$1"
  [[ -n "${workshop_id}" ]] || return 1
  [[ -d "${WORKSHOP_DIR}/${workshop_id}" ]] || return 1
  find "${WORKSHOP_DIR}/${workshop_id}" -type f -name '*.tmod' 2>/dev/null | sort -V | tail -n 1
}

write_enabled_json() {
	local names_file="$1"
	local first mod_name tmp_enabled
	tmp_enabled="$(mktemp)"

  printf '[\n' >"${tmp_enabled}"
  first=1
  while IFS= read -r mod_name || [[ -n "${mod_name}" ]]; do
    [[ -n "${mod_name}" ]] || continue
    if [[ "${first}" -eq 0 ]]; then
      printf ',\n' >>"${tmp_enabled}"
    fi
    printf '  "%s"' "${mod_name}" >>"${tmp_enabled}"
    first=0
  done < <(
    {
      if [[ -s "${MODS_DIR}/enabled.json" ]]; then
        grep -Eo '"[^"]+"' "${MODS_DIR}/enabled.json" | tr -d '"' || true
      fi
      cat "${names_file}"
    } | sort -u
  )
  printf '\n]\n' >>"${tmp_enabled}"
  mv "${tmp_enabled}" "${MODS_DIR}/enabled.json"
}

install_cached_workshop_mods() {
	local file_name mod_name source_file target_file tmp_names tmp_target workshop_id
	tmp_names="$(mktemp)"

  while IFS= read -r workshop_id || [[ -n "${workshop_id}" ]]; do
    workshop_id="${workshop_id//[$'\t\r\n ']}"
    [[ -n "${workshop_id}" ]] || continue

    source_file="$(latest_workshop_mod_file "${workshop_id}")"
    if [[ -z "${source_file}" || ! -f "${source_file}" ]]; then
      echo "Workshop mod ${workshop_id} is not cached yet."
      continue
    fi

    file_name="$(basename "${source_file}")"
    mod_name="${file_name%.tmod}"
    tmp_target="${MODS_DIR}/.${file_name}.tmp"
    target_file="${MODS_DIR}/${file_name}"

    cp "${source_file}" "${tmp_target}"
    mv "${tmp_target}" "${target_file}"
    printf '%s\n' "${mod_name}" >>"${tmp_names}"
    echo "Installed Workshop mod ${workshop_id}: ${file_name}"
  done <"${MODS_DIR}/install.txt"

  write_enabled_json "${tmp_names}"
  rm -f "${tmp_names}"
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
  install_cached_workshop_mods
fi

exec ./server/start-tModLoaderServer.sh "$@"
