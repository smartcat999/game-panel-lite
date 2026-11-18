#!/usr/bin/env bash
set -euo pipefail

readonly fifo=/home/steam/server/.palserver_log_fifo
readonly stdout_file=/tmp/pal-logger.stdout
readonly stderr_file=/tmp/pal-logger.stderr
readonly test_uid=12001
readonly test_gid=12002

rm -f "${fifo}" "${stdout_file}" "${stderr_file}"
# Mirror init.sh changing the steam home to the configured runtime identity.
chown "${test_uid}:${test_gid}" /home/steam
# Upstream v2.7.1 and newer manage the FIFO in init.sh before starting the
# logger. Older GamePanel-patched loggers safely replace this FIFO themselves.
mkfifo -m 600 "${fifo}"
chown "${test_uid}:${test_gid}" "${fifo}"

tail -f /dev/null | env PUID="${test_uid}" PGID="${test_gid}" \
  python3 /home/steam/server/pal_logger.py >"${stdout_file}" 2>"${stderr_file}" &
logger_pid=$!

cleanup() {
  kill "${logger_pid}" 2>/dev/null || true
  wait "${logger_pid}" 2>/dev/null || true
}
trap cleanup EXIT

for _ in $(seq 1 100); do
  if [[ -p "${fifo}" ]]; then
    break
  fi
  sleep 0.05
done

[[ -p "${fifo}" ]]
[[ "$(stat -c '%u:%g %a' "${fifo}")" == "${test_uid}:${test_gid} 600" ]]

python3 -c 'import os; os.setgid(12002); os.setuid(12001); fd = os.open("/home/steam/server/.palserver_log_fifo", os.O_WRONLY); os.write(fd, b"LOG:gamepanel-fifo-probe\nLOG_FLUSH\n"); os.close(fd)'

for _ in $(seq 1 100); do
  if grep -q 'gamepanel-fifo-probe' "${stdout_file}"; then
    break
  fi
  sleep 0.05
done

grep -q 'gamepanel-fifo-probe' "${stdout_file}"
if grep -Eqi 'permission denied|failed to flush log to fifo' "${stderr_file}"; then
  cat "${stderr_file}" >&2
  exit 1
fi

echo "FIFO runtime test passed: $(stat -c '%u:%g %a %F' "${fifo}")"
