from __future__ import annotations

import importlib.util
import pathlib
import subprocess
import sys
import tempfile
import unittest


SCRIPT_PATH = pathlib.Path(__file__).with_name("patch_pal_logger.py")
SPEC = importlib.util.spec_from_file_location("patch_pal_logger", SCRIPT_PATH)
assert SPEC is not None and SPEC.loader is not None
PATCHER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(PATCHER)


class PatchPalLoggerTest(unittest.TestCase):
    def test_patch_is_exact_and_idempotent(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory, "pal_logger.py")
            path.write_text("import os\nimport errno\n\n" + PATCHER.UPSTREAM_BLOCK, encoding="utf-8")

            self.assertTrue(PATCHER.patch(path))
            first = path.read_text(encoding="utf-8")
            self.assertIn(PATCHER.PATCH_MARKER, first)
            self.assertIn("os.chown(fifo_temp_path, fifo_uid, fifo_gid)", first)
            self.assertIn("os.replace(fifo_temp_path, FIFO_PATH)", first)
            self.assertFalse(PATCHER.patch(path))
            self.assertEqual(first, path.read_text(encoding="utf-8"))

    def test_patch_rejects_changed_upstream_block(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory, "pal_logger.py")
            path.write_text("# upstream changed\n", encoding="utf-8")
            with self.assertRaisesRegex(RuntimeError, "found 0"):
                PATCHER.patch(path)

    def test_non_root_fifo_is_atomically_published_with_private_mode(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            fifo = pathlib.Path(directory, "events.fifo")
            script = pathlib.Path(directory, "fixture.py")
            source = (
                "import errno\nimport os\nimport stat\n"
                f"FIFO_PATH = {str(fifo)!r}\n"
                + PATCHER.UPSTREAM_BLOCK
                + "result = os.stat(FIFO_PATH)\n"
                + "assert stat.S_ISFIFO(result.st_mode)\n"
                + "assert result.st_uid == os.geteuid()\n"
                + "assert result.st_gid == os.getegid()\n"
                + "assert stat.S_IMODE(result.st_mode) == 0o600\n"
            )
            script.write_text(source, encoding="utf-8")
            PATCHER.patch(script)
            subprocess.run([sys.executable, str(script)], check=True)


if __name__ == "__main__":
    unittest.main()
