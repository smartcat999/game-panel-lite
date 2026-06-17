#!/bin/sh
set -eu

cd /home/container/server

case "$(uname -m)" in
  x86_64|amd64)
    exec ./TerrariaServer.bin.x86_64 "$@"
    ;;
  *)
    exec mono ./TerrariaServer.exe "$@"
    ;;
esac
