"""Opt-in Docker tests. Use an already installed game image; never pull in tests."""
import os
import json
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

IMAGE = os.environ.get("GAMEPANEL_SHARED_RUNTIME_IMAGE")
SCRIPTS = Path(__file__).resolve().parent
MANIFEST = '"AppState" { "appid" "294420" "StateFlags" "4" "buildid" "100" }'


@unittest.skipUnless(IMAGE, "set GAMEPANEL_SHARED_RUNTIME_IMAGE for isolated Docker tests")
class RuntimeIntegrationTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.seed = self.root / "seed"
        self.data = self.root / "data"
        self.seed.mkdir()
        self.data.mkdir()
        (self.seed / "steamapps").mkdir()
        (self.seed / "steamapps/appmanifest_294420.acf").write_text(MANIFEST)
        (self.seed / "UnityPlayer.so").touch()
        binary = self.seed / "7DaysToDieServer.x86_64"
        binary.write_text('#!/bin/sh\nprintf "%s\\n" "$0" > /data/launched-from\n')
        binary.chmod(0o755)
        (self.seed / "serverconfig.xml").write_text('<ServerSettings><property name="UserDataFolder" value="/data/userdata"/></ServerSettings>')
        (self.data / "gamepanel-serverconfig.xml").write_text('<ServerSettings><property name="ServerName" value="Existing"/></ServerSettings>')
        (self.data / "userdata/Saves").mkdir(parents=True)
        self.world = self.data / "userdata/Saves/world.save"
        self.world.write_bytes(b"existing day 49")

    def run_runtime(self):
        args = ["docker", "run", "--rm", "--pull", "never", "--network", "none", "--read-only",
                "--mount", f"type=bind,src={self.seed},dst=/opt/seven-days,readonly",
                "--mount", f"type=bind,src={self.data},dst=/data",
                "--mount", f"type=bind,src={SCRIPTS},dst=/opt/gamepanel,readonly",
                "--entrypoint", "/bin/sh", IMAGE, "/opt/gamepanel/entrypoint.sh"]
        return subprocess.run(args, capture_output=True, text=True, timeout=30)

    def test_new_instance_does_not_copy_game_programs(self):
        result = self.run_runtime()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse((self.data / "7DaysToDieServer.x86_64").exists(), "first start copied game programs")
        self.assertEqual((self.data / "launched-from").read_text().strip(), "/opt/seven-days/7DaysToDieServer.x86_64")
        self.assertEqual(self.world.read_bytes(), b"existing day 49")

    def test_manual_restart_repairs_eos_permissions_without_resetting_identity(self):
        self.assertEqual(self.run_runtime().returncode, 0)
        cache = self.data / ".runtime-home/.local/share/.eos_sdk"
        credential = cache / "keychain/product/device_id/credential"
        credential.parent.mkdir(parents=True)
        credential.write_bytes(b"existing device identity")
        # Reproduce the writable-tree preparation used by panel manual starts.
        for path in [cache, *cache.rglob("*")]:
            path.chmod(0o777 if path.is_dir() else 0o666)
        result = self.run_runtime()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(credential.stat().st_mode & 0o777, 0o600)
        self.assertEqual(credential.parent.stat().st_mode & 0o777, 0o700)
        self.assertEqual(credential.read_bytes(), b"existing device identity")
        self.assertEqual(self.world.read_bytes(), b"existing day 49")

    def install_local(self, build="100"):
        (self.data / "steamapps").mkdir(exist_ok=True)
        for name in ["7DaysToDieServer.x86_64", "UnityPlayer.so", "serverconfig.xml"]:
            shutil.copyfile(self.seed / name, self.data / name)
        (self.data / "7DaysToDieServer.x86_64").chmod(0o755)
        (self.data / "steamapps/appmanifest_294420.acf").write_text(MANIFEST.replace('"100"', f'"{build}"'))

    def test_legacy_instance_keeps_local_programs_and_world(self):
        self.install_local()
        (self.data / ".gamepanel-installed").touch()
        result = self.run_runtime()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.data / "launched-from").read_text().strip(), "/data/7DaysToDieServer.x86_64")
        self.assertEqual(self.world.read_bytes(), b"existing day 49")

    def test_failed_update_does_not_switch_shared_instance(self):
        self.assertEqual(self.run_runtime().returncode, 0)
        self.install_local("200")
        result = self.run_runtime()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.data / "launched-from").read_text().strip(), "/opt/seven-days/7DaysToDieServer.x86_64")
        self.assertEqual(self.world.read_bytes(), b"existing day 49")

    def test_verified_update_can_switch_to_local_programs(self):
        self.assertEqual(self.run_runtime().returncode, 0)
        self.install_local("200")
        (self.data / ".gamepanel-update-complete").write_text(json.dumps({"appId": "294420", "buildId": "200"}))
        result = self.run_runtime()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.data / "launched-from").read_text().strip(), "/data/7DaysToDieServer.x86_64")
        self.assertEqual(self.world.read_bytes(), b"existing day 49")

    def test_broken_legacy_install_is_not_replaced_silently(self):
        (self.data / ".gamepanel-installed").touch()
        result = self.run_runtime()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("incomplete", result.stderr)
        self.assertFalse((self.data / "launched-from").exists())
        self.assertEqual(self.world.read_bytes(), b"existing day 49")
