#!/bin/sh
set -eu
mkdir -p /data/userdata/Saves
python3 /opt/gamepanel/check_storage.py
# Seed game files once. Never copy template data over an existing save.
if [ ! -f /data/.gamepanel-installed ]; then
  # An explicit update may have installed the game before its first start.
  if [ ! -f /data/7DaysToDieServer.x86_64 ] || [ ! -f /data/steamapps/appmanifest_294420.acf ]; then
    echo "Preparing 7 Days to Die game files..."
    cp -a /opt/seven-days/. /data/
  fi
  touch /data/.gamepanel-installed
fi
test -f /data/gamepanel-serverconfig.xml
python3 /opt/gamepanel/merge_config.py /data/serverconfig.xml /data/gamepanel-serverconfig.xml /data/gamepanel-active.xml
chmod +x /data/7DaysToDieServer.x86_64 /data/UnityPlayer.so
export LD_LIBRARY_PATH="/data:/data/7DaysToDieServer_Data/Plugins/x86_64${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
echo "Starting 7 Days to Die; world generation can take several minutes."
exec /data/7DaysToDieServer.x86_64 -quit -batchmode -nographics -dedicated -configfile=/data/gamepanel-active.xml -logfile /dev/stdout
