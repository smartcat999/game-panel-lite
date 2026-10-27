"""Check operating space on the mounted instance filesystem before starting."""
import os
import sys
from pathlib import Path

RESERVE_BYTES = 2 * 1024 ** 3


def required_bytes(seed, data, reserve=RESERVE_BYTES):
    # Image files are shared and never seeded into the instance on startup.
    # Explicit Steam updates have their own 25 GiB preflight in the panel.
    return reserve


def check_storage(seed, data, reserve=RESERVE_BYTES):
    stat = os.statvfs(data)
    available = stat.f_bavail * stat.f_frsize
    required = required_bytes(seed, data, reserve)
    if available < required:
        raise RuntimeError(f"Insufficient disk space: estimated required {required} bytes, available {available} bytes. Existing server data is retained.")


if __name__ == "__main__":
    try:
        check_storage(Path("/opt/seven-days"), Path("/data"))
    except (OSError, RuntimeError) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
