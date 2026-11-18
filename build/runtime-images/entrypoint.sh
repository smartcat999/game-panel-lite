#!/bin/sh
set -eu

export LANG="${LANG:-C.UTF-8}"
export LC_ALL="${LC_ALL:-C.UTF-8}"

mkdir -p /data /data/.steam/sdk32 /data/.steam/sdk64 /root/.steam/sdk32 /root/.steam/sdk64
if [ -f /opt/steamcmd/linux32/steamclient.so ]; then
  ln -sf /opt/steamcmd/linux32/steamclient.so /data/.steam/sdk32/steamclient.so 2>/dev/null || true
  ln -sf /opt/steamcmd/linux32/steamclient.so /root/.steam/sdk32/steamclient.so 2>/dev/null || true
fi
if [ -f /opt/steamcmd/linux64/steamclient.so ]; then
  ln -sf /opt/steamcmd/linux64/steamclient.so /data/.steam/sdk64/steamclient.so 2>/dev/null || true
  ln -sf /opt/steamcmd/linux64/steamclient.so /root/.steam/sdk64/steamclient.so 2>/dev/null || true
elif [ -f /data/linux64/steamclient.so ]; then
  ln -sf /data/linux64/steamclient.so /data/.steam/sdk64/steamclient.so 2>/dev/null || true
fi
rm -f /data/*/session.lock /tmp/.X*-lock 2>/dev/null || true
cd /data

# 1. Explicit container command arguments
if [ "$#" -gt 0 ]; then
  exec "$@"
fi

# 2. Declarative PRE_START_HOOK rendered from App.PreStartCommand
if [ -n "${PRE_START_HOOK:-}" ]; then
  sh -e -c "${PRE_START_HOOK}"
fi

# 3. Declarative STARTUP_CMD rendered from App.StartupCommand
if [ -n "${STARTUP_CMD:-}" ]; then
  exec sh -c "${STARTUP_CMD}"
fi

echo "[GamePanel Runtime] No STARTUP_CMD configured. Waiting..."
exec sleep infinity
