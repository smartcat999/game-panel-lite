#!/bin/sh
set -eu

DATA_DIR="${GAMEPANEL_DATA_DIR:-/app/data}"
SOURCE_DIR="${GAMEPANEL_PALWORLD_MOD_SOURCE_DIR:-/palworld-mod-pack}"
TARGET_DIR="${GAMEPANEL_PALWORLD_MOD_TARGET_DIR:-$DATA_DIR/mods/palworld-pack}"

copied=0
updated=0
skipped=0

if [ ! -d "$SOURCE_DIR" ]; then
  echo "palworld mod pack source dir not found: $SOURCE_DIR" >&2
  exit 1
fi

mkdir -p "$TARGET_DIR"

file_hash() {
  sha256sum "$1" | awk '{print $1}'
}

file_list="$(mktemp)"
trap 'rm -f "$file_list"' EXIT
find "$SOURCE_DIR" -type f >"$file_list"

while IFS= read -r source_path; do
  [ -e "$source_path" ] || continue
  [ -f "$source_path" ] || continue
  relative_path="${source_path#"$SOURCE_DIR"/}"
  if [ "$relative_path" = ".gitkeep" ]; then
    skipped=$((skipped + 1))
    continue
  fi

  source_hash="$(file_hash "$source_path")"
  target_path="$TARGET_DIR/$relative_path"
  mkdir -p "$(dirname "$target_path")"
  if [ -f "$target_path" ]; then
    if [ "$(file_hash "$target_path")" = "$source_hash" ]; then
      skipped=$((skipped + 1))
      continue
    fi
    cp "$source_path" "$target_path"
    chmod 644 "$target_path"
    updated=$((updated + 1))
    continue
  fi

  cp "$source_path" "$target_path"
  chmod 644 "$target_path"
  copied=$((copied + 1))
done <"$file_list"

echo "palworld mod pack synced copied=$copied updated=$updated skipped=$skipped target=$TARGET_DIR"
