import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from prepare_runtime import prepare_runtime, select_runtime


def install_fixture(root, build="100"):
    (root / "steamapps").mkdir(parents=True, exist_ok=True)
    (root / "steamapps/appmanifest_294420.acf").write_text(f'"AppState" {{ "appid" "294420" "StateFlags" "4" "buildid" "{build}" }}')
    (root / "7DaysToDieServer.x86_64").write_bytes(b"program")
    (root / "UnityPlayer.so").write_bytes(b"library")
    (root / "serverconfig.xml").write_text('<ServerSettings><property name="UserDataFolder" value="/data/userdata"/></ServerSettings>')


class PrepareRuntimeTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.seed = self.root / "seed"
        self.data = self.root / "data"
        self.seed.mkdir()
        self.data.mkdir()
        install_fixture(self.seed)
        (self.data / "gamepanel-serverconfig.xml").write_text('<ServerSettings><property name="GameName" value="Existing"/></ServerSettings>')
        (self.data / "userdata/Saves").mkdir(parents=True)
        self.world = self.data / "userdata/Saves/world.save"
        self.world.write_bytes(b"day 49")
        self.space = patch("prepare_runtime.check_storage")
        self.space.start()
        self.addCleanup(self.space.stop)

    def test_restart_restores_private_eos_credentials_without_changing_contents(self):
        cache = self.data / ".runtime-home/.local/share/.eos_sdk"
        credential = cache / "keychain/product/device_id/credential"
        credential.parent.mkdir(parents=True)
        credential.write_bytes(b"existing device identity")
        for path in [cache, *cache.rglob("*")]:
            path.chmod(0o777 if path.is_dir() else 0o666)
        prepare_runtime(self.seed, self.data)
        for path in [cache, *cache.rglob("*")]:
            self.assertEqual(path.stat().st_mode & 0o777, 0o700 if path.is_dir() else 0o600)
        self.assertEqual(credential.read_bytes(), b"existing device identity")
        self.assertEqual(self.world.read_bytes(), b"day 49")

    def test_private_eos_cache_does_not_follow_symlinks(self):
        home = self.data / ".runtime-home"
        home.mkdir()
        outside = self.root / "outside"
        (outside / "share/.eos_sdk").mkdir(parents=True)
        credential = outside / "share/.eos_sdk/credential"
        credential.write_bytes(b"untouched")
        credential.chmod(0o666)
        (home / ".local").symlink_to(outside, target_is_directory=True)
        with self.assertRaisesRegex(RuntimeError, "symlink"):
            prepare_runtime(self.seed, self.data)
        self.assertEqual(credential.stat().st_mode & 0o777, 0o666)
        self.assertEqual(credential.read_bytes(), b"untouched")

    def test_new_instance_reuses_image_and_only_copies_metadata(self):
        before = {p.relative_to(self.seed): p.read_bytes() for p in self.seed.rglob("*") if p.is_file()}
        self.assertEqual(prepare_runtime(self.seed, self.data), self.seed)
        self.assertFalse((self.data / "7DaysToDieServer.x86_64").exists())
        self.assertFalse((self.data / "UnityPlayer.so").exists())
        self.assertTrue((self.data / ".gamepanel-shared-runtime").exists())
        self.assertTrue((self.data / "Mods").is_symlink())
        (self.data / "Mods/kept.xml").write_bytes(b"mod")
        self.assertEqual((self.data / "userdata/Mods/kept.xml").read_bytes(), b"mod")
        self.assertEqual(self.world.read_bytes(), b"day 49")
        self.assertEqual(before, {p.relative_to(self.seed): p.read_bytes() for p in self.seed.rglob("*") if p.is_file()})
        self.assertIn('value="Existing"', (self.data / "gamepanel-active.xml").read_text())

    def test_legacy_writable_instance_stays_on_its_own_programs(self):
        install_fixture(self.data, "200")
        (self.data / ".gamepanel-installed").touch()
        (self.data / "Mods").mkdir()
        (self.data / "Mods/legacy.xml").write_bytes(b"legacy mod")
        self.assertEqual(prepare_runtime(self.seed, self.data), self.data)
        self.assertEqual((self.data / "Mods/legacy.xml").read_bytes(), b"legacy mod")
        self.assertEqual(self.world.read_bytes(), b"day 49")

    def test_incomplete_legacy_runtime_fails_without_falling_back(self):
        (self.data / ".gamepanel-installed").touch()
        with self.assertRaisesRegex(RuntimeError, "incomplete"):
            prepare_runtime(self.seed, self.data)
        self.assertEqual(self.world.read_bytes(), b"day 49")

    def test_partial_update_cannot_replace_shared_runtime(self):
        prepare_runtime(self.seed, self.data)
        install_fixture(self.data, "200")
        self.assertEqual(prepare_runtime(self.seed, self.data), self.seed)
        self.assertEqual(self.world.read_bytes(), b"day 49")

    def test_only_verified_update_switches_to_writable_programs(self):
        prepare_runtime(self.seed, self.data)
        install_fixture(self.data, "200")
        marker = self.data / ".gamepanel-update-complete"
        marker.write_text(json.dumps({"appId": "294420", "buildId": "199"}))
        self.assertEqual(select_runtime(self.seed, self.data), self.seed)
        marker.write_text(json.dumps({"appId": "294420", "buildId": "200"}))
        self.assertEqual(prepare_runtime(self.seed, self.data), self.data)
        self.assertEqual(self.world.read_bytes(), b"day 49")

    def test_foreign_update_record_is_not_accepted(self):
        prepare_runtime(self.seed, self.data)
        install_fixture(self.data)
        (self.data / ".gamepanel-update-complete").write_text('{"appId":"2394010","buildId":"100"}')
        self.assertEqual(select_runtime(self.seed, self.data), self.seed)

    def test_existing_root_mods_are_not_deleted_or_silently_ignored(self):
        (self.data / "Mods").mkdir()
        mod = self.data / "Mods/legacy.xml"
        mod.write_bytes(b"existing mod")
        with self.assertRaisesRegex(RuntimeError, "userdata/Mods"):
            prepare_runtime(self.seed, self.data)
        self.assertEqual(mod.read_bytes(), b"existing mod")

    def test_restarts_preserve_custom_config_and_mods(self):
        prepare_runtime(self.seed, self.data)
        (self.data / "serverconfig.xml").write_text('<ServerSettings><property name="CustomSetting" value="keep"/></ServerSettings>')
        mod = self.data / "userdata/Mods/kept.xml"
        mod.write_bytes(b"mod")
        self.assertEqual(prepare_runtime(self.seed, self.data), self.seed)
        self.assertIn('name="CustomSetting" value="keep"', (self.data / "gamepanel-active.xml").read_text())
        self.assertEqual(mod.read_bytes(), b"mod")
        self.assertEqual(self.world.read_bytes(), b"day 49")

    def test_image_upgrade_refreshes_only_metadata(self):
        prepare_runtime(self.seed, self.data)
        install_fixture(self.seed, "200")
        self.assertEqual(prepare_runtime(self.seed, self.data), self.seed)
        self.assertIn('"200"', (self.data / "steamapps/appmanifest_294420.acf").read_text())
        self.assertFalse((self.data / "7DaysToDieServer.x86_64").exists())
        self.assertEqual(self.world.read_bytes(), b"day 49")

    def test_insufficient_operating_space_does_not_prepare_files(self):
        with patch("prepare_runtime.check_storage", side_effect=RuntimeError("Insufficient disk space")):
            with self.assertRaisesRegex(RuntimeError, "Insufficient disk space"):
                prepare_runtime(self.seed, self.data)
        self.assertFalse((self.data / ".gamepanel-shared-runtime").exists())
        self.assertFalse((self.data / "7DaysToDieServer.x86_64").exists())
        self.assertEqual(self.world.read_bytes(), b"day 49")


if __name__ == "__main__":
    unittest.main()
