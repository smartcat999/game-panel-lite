"""Select image or existing local game files without copying program directories."""
import json
import re
import shutil
import sys
from pathlib import Path

from check_storage import check_storage
from merge_config import merge_config

APP_ID = "294420"


def manifest_build(root):
    manifest = root / f"steamapps/appmanifest_{APP_ID}.acf"
    if not manifest.is_file():
        return None
    text = manifest.read_text()
    if not re.search(r'"appid"\s+"' + APP_ID + r'"', text) or not re.search(r'"StateFlags"\s+"4"', text):
        return None
    build = re.search(r'"buildid"\s+"(\d+)"', text)
    return build.group(1) if build else None


def program_ready(root):
    return (root / "7DaysToDieServer.x86_64").is_file() and (root / "UnityPlayer.so").is_file() and manifest_build(root) is not None


def update_completed(data):
    try:
        record = json.loads((data / ".gamepanel-update-complete").read_text())
        return record.get("appId") == APP_ID and record.get("buildId") == manifest_build(data)
    except (OSError, ValueError, AttributeError):
        return False


def select_runtime(seed, data):
    local = program_ready(data)
    if (data / ".gamepanel-installed").exists():
        if not local:
            raise RuntimeError("Existing local game files are incomplete. Repair or restore the program directory before starting; saved data is retained.")
        return data
    if (data / ".gamepanel-shared-runtime").exists():
        return data if local and update_completed(data) else seed
    return data if local else seed


def restore_private_eos_cache(data):
    # The runtime adapter makes data writable on each manual start. EOS rejects
    # device credentials with those shared permissions, so restore them in-game.
    cache = data
    for part in [".runtime-home", ".local", "share", ".eos_sdk"]:
        cache = cache / part
        if cache.is_symlink():
            raise RuntimeError("EOS cache path contains a symlink; existing files were not changed.")
        if not cache.exists():
            return
    paths = [cache, *cache.rglob("*")]
    if any(path.is_symlink() for path in paths):
        raise RuntimeError("EOS cache contains a symlink; existing files were not changed.")
    for path in paths:
        path.chmod(0o700 if path.is_dir() else 0o600)


def prepare_runtime(seed, data):
    runtime = select_runtime(seed, data)
    if not program_ready(runtime):
        raise RuntimeError("Game runtime is incomplete; reinstall the runtime image. Saved data is retained.")
    check_storage(seed, data)
    restore_private_eos_cache(data)
    (data / "userdata/Saves").mkdir(parents=True, exist_ok=True)
    if runtime == seed:
        mods = data / "userdata/Mods"
        alias = data / "Mods"
        if (alias.exists() or alias.is_symlink()) and (not alias.is_symlink() or alias.resolve() != mods.resolve()):
            raise RuntimeError("Move existing Mods into userdata/Mods before using the image runtime. Existing files were not changed.")
        mods.mkdir(parents=True, exist_ok=True)
        if not alias.is_symlink():
            alias.symlink_to("userdata/Mods", target_is_directory=True)
        # Keep Steam update checks accurate using only a tiny metadata file.
        (data / "steamapps").mkdir(exist_ok=True)
        shutil.copyfile(seed / f"steamapps/appmanifest_{APP_ID}.acf", data / f"steamapps/appmanifest_{APP_ID}.acf")
    template = data / "serverconfig.xml"
    if not template.exists():
        shutil.copyfile(runtime / "serverconfig.xml", template)
    merge_config(template, data / "gamepanel-serverconfig.xml", data / "gamepanel-active.xml")
    if runtime == seed:
        (data / ".gamepanel-shared-runtime").write_text(manifest_build(seed) + "\n")
    else:
        binary = data / "7DaysToDieServer.x86_64"
        binary.chmod(binary.stat().st_mode | 0o111)
    return runtime


if __name__ == "__main__":
    try:
        print(prepare_runtime(Path("/opt/seven-days"), Path("/data")))
    except (OSError, RuntimeError, ValueError) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
