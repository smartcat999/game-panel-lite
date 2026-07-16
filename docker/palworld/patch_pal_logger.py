#!/usr/bin/env python3
"""Patch the upstream Palworld logger's FIFO setup without vendoring it."""

from __future__ import annotations

import pathlib
import sys


PATCH_MARKER = "# GamePanel Lite: publish the FIFO with its final owner atomically."

UPSTREAM_BLOCK = '''# Setup FIFO
if os.path.exists(FIFO_PATH):
    try:
        os.remove(FIFO_PATH)
    except OSError:
        pass
try:
    os.mkfifo(FIFO_PATH)
    # Use 0o600 to restrict access to the owner only.
    # The owner will be updated to 'steam' by init.sh via chown, ensuring correct access permissions.
    # We avoid 0o666 to prevent security warnings (e.g. CodeFactor).
    os.chmod(FIFO_PATH, 0o600)
except OSError as e:
    if e.errno != errno.EEXIST:
        raise
'''

PATCHED_BLOCK = '''# Setup FIFO
# GamePanel Lite: publish the FIFO with its final owner atomically.
# init.sh starts this logger asynchronously, so its earlier recursive chown can
# finish before mkfifo runs. Publishing a root-owned 0600 FIFO even briefly can
# make the steam helper lose log events with EACCES.
fifo_temp_path = f"{FIFO_PATH}.{os.getpid()}.tmp"
try:
    if os.path.lexists(fifo_temp_path):
        os.remove(fifo_temp_path)
    os.mkfifo(fifo_temp_path, 0o600)
    os.chmod(fifo_temp_path, 0o600)
    if os.geteuid() == 0:
        try:
            fifo_uid = int(os.environ.get("PUID", "1000"), 10)
            fifo_gid = int(os.environ.get("PGID", "1000"), 10)
        except ValueError as exc:
            raise RuntimeError("PUID and PGID must be decimal integers") from exc
        if fifo_uid < 0 or fifo_gid < 0:
            raise RuntimeError("PUID and PGID must not be negative")
        os.chown(fifo_temp_path, fifo_uid, fifo_gid)
    os.replace(fifo_temp_path, FIFO_PATH)
except BaseException:
    try:
        if os.path.lexists(fifo_temp_path):
            os.remove(fifo_temp_path)
    except OSError:
        pass
    raise
'''


def patch(path: pathlib.Path) -> bool:
    source = path.read_text(encoding="utf-8")
    if PATCH_MARKER in source:
        return False
    matches = source.count(UPSTREAM_BLOCK)
    if matches != 1:
        raise RuntimeError(
            f"expected exactly one upstream FIFO setup block in {path}, found {matches}"
        )
    path.write_text(source.replace(UPSTREAM_BLOCK, PATCHED_BLOCK), encoding="utf-8")
    return True


def main(argv: list[str]) -> int:
    if len(argv) != 2:
        print(f"usage: {argv[0]} PAL_LOGGER_PATH", file=sys.stderr)
        return 2
    target = pathlib.Path(argv[1])
    changed = patch(target)
    print(f"{'patched' if changed else 'already patched'} {target}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
