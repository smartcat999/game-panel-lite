import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

from check_storage import check_storage, required_bytes


class StorageCheckTest(unittest.TestCase):
    def setUp(self):
        self.root = tempfile.TemporaryDirectory()
        self.addCleanup(self.root.cleanup)
        self.seed = Path(self.root.name) / "seed"
        self.data = Path(self.root.name) / "data"
        self.seed.mkdir()
        self.data.mkdir()
        (self.seed / "game.bin").write_bytes(b"game files")
        (self.data / "world.save").write_bytes(b"existing day 49")

    def test_seed_needs_program_space_plus_reserve(self):
        self.assertEqual(required_bytes(self.seed, self.data, 100), 110)
        with patch("check_storage.os.statvfs", return_value=SimpleNamespace(f_bavail=109, f_frsize=1)):
            with self.assertRaisesRegex(RuntimeError, "Insufficient disk space"):
                check_storage(self.seed, self.data, 100)
        self.assertEqual((self.data / "world.save").read_bytes(), b"existing day 49")
        self.assertFalse((self.data / ".gamepanel-installed").exists())

    def test_installed_server_only_needs_running_reserve(self):
        (self.data / ".gamepanel-installed").touch()
        self.assertEqual(required_bytes(self.seed, self.data, 100), 100)
        with patch("check_storage.os.statvfs", return_value=SimpleNamespace(f_bavail=100, f_frsize=1)):
            check_storage(self.seed, self.data, 100)

    def test_prepared_install_is_not_counted_twice(self):
        (self.data / "7DaysToDieServer.x86_64").touch()
        (self.data / "steamapps").mkdir()
        (self.data / "steamapps/appmanifest_294420.acf").touch()
        self.assertEqual(required_bytes(self.seed, self.data, 100), 100)

    def test_existing_server_cannot_consume_reserved_operating_space(self):
        (self.data / ".gamepanel-installed").touch()
        with patch("check_storage.os.statvfs", return_value=SimpleNamespace(f_bavail=99, f_frsize=1)):
            with self.assertRaisesRegex(RuntimeError, "Insufficient disk space"):
                check_storage(self.seed, self.data, 100)


if __name__ == "__main__":
    unittest.main()
