#!/bin/sh
set -eu
umask 077
runtime_dir="$(python3 /opt/gamepanel/prepare_runtime.py)"
# Unity/EOS and Steam need small per-instance writable caches, not a program copy.
export HOME=/data/.runtime-home
export XDG_CONFIG_HOME="$HOME/.config"
export XDG_CACHE_HOME="$HOME/.cache"
export XDG_DATA_HOME="$HOME/.local/share"
mkdir -p "$XDG_CONFIG_HOME" "$XDG_CACHE_HOME" "$XDG_DATA_HOME" "$HOME/.steam/sdk64"
if [ ! -e "$HOME/.steam/sdk64/steamclient.so" ]; then
  ln -s /root/.steam/sdk64/steamclient.so "$HOME/.steam/sdk64/steamclient.so"
fi
export LD_LIBRARY_PATH="${runtime_dir}:${runtime_dir}/7DaysToDieServer_Data/Plugins/x86_64${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
cd "$runtime_dir"
echo "Starting 7 Days to Die from ${runtime_dir}; world generation can take several minutes."
exec "${runtime_dir}/7DaysToDieServer.x86_64" -quit -batchmode -nographics -dedicated -configfile=/data/gamepanel-active.xml -logfile /dev/stdout
