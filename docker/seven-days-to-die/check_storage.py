"""Check the mounted instance filesystem before seeding game files or starting."""
import os
import sys
from pathlib import Path

RESERVE_BYTES = 2 * 1024 ** 3


def required_bytes(seed, data, reserve=RESERVE_BYTES):
    installed = (data / ".gamepanel-installed").is_file()
    prepared = (data / "7DaysToDieServer.x86_64").is_file() and (data / "steamapps/appmanifest_294420.acf").is_file()
    if installed or prepared:
        return reserve
    return reserve + sum(p.stat().st_size for p in seed.rglob("*") if p.is_file())


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
