#!/bin/sh
set -e

uid="$(id -u)"
gid="$(id -g)"

if [ -n "${STEAMCMD_RUNTIME_DIR:-}" ]; then
  RUNTIME_STEAMCMD="$STEAMCMD_RUNTIME_DIR"
  STEAM_HOME="${STEAMCMD_HOME_DIR:-/tmp/steam-home}"
elif [ -d /data ] && [ -w /data ]; then
  RUNTIME_STEAMCMD="/data/.steam/steamcmd"
  STEAM_HOME="/data/.steam/home"
else
  RUNTIME_STEAMCMD="/tmp/steamcmd"
  STEAM_HOME="/tmp/steam-home"
fi

if [ ! -x "$RUNTIME_STEAMCMD/steamcmd.sh" ]; then
  mkdir -p "$RUNTIME_STEAMCMD"
  cp -RPp --no-preserve=ownership /opt/steamcmd/. "$RUNTIME_STEAMCMD/"
fi

if [ ! -d "$STEAM_HOME/Steam" ] && [ -d /opt/steam-home/Steam ]; then
  mkdir -p "$STEAM_HOME"
  cp -RPp --no-preserve=ownership /opt/steam-home/. "$STEAM_HOME/"
fi

mkdir -p "$STEAM_HOME"
if [ "$uid" = "0" ]; then
  chown -R 0:0 "$RUNTIME_STEAMCMD" "$STEAM_HOME" 2>/dev/null || true
  if [ -d /data/steamapps ]; then
    chown -R 0:0 /data/steamapps 2>/dev/null || true
  fi
fi

HOME="$STEAM_HOME" exec "$RUNTIME_STEAMCMD/steamcmd.sh" "$@"
