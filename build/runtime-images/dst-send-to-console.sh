#!/bin/sh
set -eu

cmd="$*"
if [ -z "$cmd" ]; then
  exit 0
fi

write_fifo() {
  if [ -p /tmp/dst-caves.in ]; then
    python3 - "$1" << 'PY' 2>/dev/null || true
import os, sys
msg = (sys.argv[1] + "\n").encode("utf-8")
try:
    fd = os.open("/tmp/dst-caves.in", os.O_WRONLY | os.O_NONBLOCK)
    try:
        os.write(fd, msg)
    finally:
        os.close(fd)
except OSError:
    pass
PY
  fi
}

case "$cmd" in
  __GAMEPANEL_DST_CONSOLE__:caves:*)
    b64="${cmd#__GAMEPANEL_DST_CONSOLE__:caves:}"
    decoded="$(printf '%s' "$b64" | base64 -d)"
    write_fifo "$decoded"
    ;;
  *)
    printf '%s\n' "$cmd" > /proc/1/fd/0
    case "$cmd" in
      "c_save()"|"c_shutdown(true)"|"c_shutdown()")
        write_fifo "$cmd"
        ;;
    esac
    ;;
esac
