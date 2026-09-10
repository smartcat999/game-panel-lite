#!/usr/bin/env bash
set -euo pipefail

install_file=/home/tml/.local/share/Terraria/tModLoader/Mods/install.txt
artifact_lock=/home/tml/.local/share/Terraria/tModLoader/Mods/artifacts.lock
if [[ -s "$install_file" ]]; then
  mkdir -p /data/steamcmd /data/steamapps
  if [[ ! -x /data/steamcmd/steamcmd.sh ]]; then
    cp -a /opt/steamcmd-seed/. /data/steamcmd/
  fi
  command=(/data/steamcmd/steamcmd.sh +force_install_dir /data +login anonymous)
  count=0
  while IFS= read -r workshop_id; do
    [[ -z "$workshop_id" ]] && continue
    [[ "$workshop_id" =~ ^[0-9]{1,20}$ ]] || { echo "invalid Workshop ID" >&2; exit 64; }
    count=$((count + 1))
    (( count <= 64 )) || { echo "too many Workshop items" >&2; exit 64; }
    command+=(+workshop_download_item 1281930 "$workshop_id")
  done < "$install_file"
  if (( count > 0 )); then
    HOME=/data/steamcmd "${command[@]}" +quit
  fi
fi

if [[ -s "$artifact_lock" ]]; then
  while read -r workshop_id relative_path expected_sha256; do
    [[ "$workshop_id" =~ ^[0-9]{1,20}$ ]] || { echo "invalid Workshop artifact ID" >&2; exit 64; }
    artifact=/data/steamapps/workshop/content/1281930/$workshop_id/$relative_path
    [[ -f "$artifact" ]] || { echo "missing pinned Workshop artifact: $workshop_id/$relative_path" >&2; exit 65; }
    echo "$expected_sha256  $artifact" | sha256sum --check --status || { echo "Workshop artifact checksum mismatch: $workshop_id/$relative_path" >&2; exit 65; }
  done < "$artifact_lock"
fi

exec /opt/tmodloader/start-tModLoaderServer.sh "$@"
