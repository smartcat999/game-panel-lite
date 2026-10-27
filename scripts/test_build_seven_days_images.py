"""Exercise build entry points without downloading games or contacting Docker."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent
PIN = "example.test/game-base@sha256:" + "a" * 64


class BuildSevenDaysImagesTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        self.calls = self.directory / "calls.jsonl"
        docker = self.directory / "docker"
        docker.write_text("#!/usr/bin/env python3\nimport json, os, sys\nwith open(os.environ['BUILD_TEST_CALLS'], 'a') as f: f.write(json.dumps(sys.argv[1:]) + '\\n')\n")
        docker.chmod(0o755)
        self.env = {**os.environ, "PATH": str(self.directory) + os.pathsep + os.environ["PATH"], "BUILD_TEST_CALLS": str(self.calls)}

    def run_build(self, *args, dispatcher=False):
        script = "build-game-images.sh" if dispatcher else "build-seven-days-images.sh"
        return subprocess.run(["bash", str(ROOT / "scripts" / script), *args], env=self.env, capture_output=True, text=True)

    def commands(self):
        return [json.loads(line) for line in self.calls.read_text().splitlines()] if self.calls.exists() else []

    def test_help_never_starts_a_build(self):
        for flag in ["-h", "--help"]:
            result = self.run_build(flag)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("Usage:", result.stdout)
        self.assertEqual(self.commands(), [])

    def test_default_build_only_uses_runtime_dockerfile(self):
        result = self.run_build()
        self.assertEqual(result.returncode, 0, result.stderr)
        command, = self.commands()
        self.assertIn(str(ROOT / "docker/seven-days-to-die/Dockerfile"), command)
        self.assertNotIn("--build-context", command)
        self.assertTrue(any(arg.startswith("SEVEN_DAYS_BASE_IMAGE=") and "@sha256:" in arg for arg in command))
        self.assertNotIn("SEVEN_DAYS_BUILD_ID", " ".join(command))
        self.assertIn("smartcat99999/seven-days-to-die:v3.3.0", command)

    def test_dispatcher_routes_runtime_and_base_independently(self):
        self.assertEqual(self.run_build("seven-days-to-die", dispatcher=True).returncode, 0)
        self.assertEqual(self.run_build("seven-days-to-die-base", "--base-build-id", "123", dispatcher=True).returncode, 0)
        runtime, base = self.commands()
        self.assertIn(str(ROOT / "docker/seven-days-to-die/Dockerfile"), runtime)
        self.assertIn(str(ROOT / "docker/seven-days-to-die/Dockerfile.base"), base)

    def test_base_requires_explicit_expected_build(self):
        for args in [("base",), ("base", "--base-build-id", "latest")]:
            result = self.run_build(*args)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("--base-build-id", result.stderr)
        self.assertEqual(self.commands(), [])

    def test_import_base_reuses_only_game_files_from_pinned_context(self):
        result = self.run_build("base", "--base-build-id", "123", "--game-source-image", PIN)
        self.assertEqual(result.returncode, 0, result.stderr)
        command, = self.commands()
        self.assertIn("SEVEN_DAYS_BUILD_ID=123", command)
        self.assertIn("smartcat99999/seven-days-to-die-base:build-123", command)
        self.assertIn("SEVEN_DAYS_BASE_STAGE=imported-base", command)
        self.assertIn("verified-runtime=docker-image://" + PIN, command)

    def test_rejects_mutable_base_and_import_references(self):
        for args in [("runtime", "--base-image", "example.test/base:latest"), ("base", "--base-build-id", "123", "--game-source-image", "example.test/runtime:latest")]:
            result = self.run_build(*args)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("pinned", result.stderr)
        self.assertEqual(self.commands(), [])

    def test_pinned_runtime_override_and_uncached_build(self):
        result = self.run_build("runtime", "--base-image", PIN, "--no-cache", "--version", "verify-split", "--builder", "test-builder")
        self.assertEqual(result.returncode, 0, result.stderr)
        command, = self.commands()
        self.assertIn("SEVEN_DAYS_BASE_IMAGE=" + PIN, command)
        self.assertIn("--no-cache", command)
        self.assertIn("test-builder", command)
        self.assertNotIn("--build-context", command)

    def test_mirror_push_uses_one_build_with_two_tags(self):
        result = self.run_build("runtime", "--push", "--mirror-registry", "mirror.test/gamepanel")
        self.assertEqual(result.returncode, 0, result.stderr)
        command, = self.commands()
        self.assertIn("--push", command)
        self.assertIn("smartcat99999/seven-days-to-die:v3.3.0", command)
        self.assertIn("mirror.test/gamepanel/seven-days-to-die:v3.3.0", command)
        self.assertNotEqual(self.run_build("--mirror-registry", "mirror.test/gamepanel").returncode, 0)
        self.assertEqual(len(self.commands()), 1)

    def test_invalid_platform_and_missing_option_values_fail_before_docker(self):
        for args in [("--platform", "linux/arm64"), ("--base-image",), ("--builder", "--push")]:
            self.assertNotEqual(self.run_build(*args).returncode, 0)
        self.assertEqual(self.commands(), [])

    def test_base_only_flags_cannot_change_runtime_build_mode(self):
        self.assertNotEqual(self.run_build("runtime", "--base-build-id", "123").returncode, 0)
        self.assertNotEqual(self.run_build("runtime", "--game-source-image", PIN).returncode, 0)
        self.assertEqual(self.commands(), [])

    def test_debian_mirror_is_an_explicit_base_only_origin(self):
        result = self.run_build("base", "--base-build-id", "123", "--debian-mirror", "https://mirror.test/")
        self.assertEqual(result.returncode, 0, result.stderr)
        command, = self.commands()
        self.assertIn("DEBIAN_MIRROR=https://mirror.test", command)
        self.assertNotEqual(self.run_build("runtime", "--debian-mirror", "http://mirror.test").returncode, 0)
        self.assertNotEqual(self.run_build("base", "--base-build-id", "123", "--debian-mirror", "http://mirror.test|bad").returncode, 0)
        self.assertEqual(len(self.commands()), 1)

    def test_migration_cannot_silently_ignore_a_dependency_refresh(self):
        result = self.run_build("base", "--base-build-id", "123", "--game-source-image", PIN, "--debian-mirror", "https://mirror.test")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("verified dependencies", result.stderr)
        self.assertEqual(self.commands(), [])

    def test_runtime_tag_cannot_silently_retag_a_base(self):
        result = self.run_build("base", "--base-build-id", "123", "--version", "candidate")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("runtime", result.stderr)
        self.assertEqual(self.commands(), [])

    def test_other_game_build_entry_points_stay_unchanged(self):
        result = self.run_build("vanilla", "--registry", "test.local", "--load", dispatcher=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        command, = self.commands()
        self.assertIn("test.local/terraria-vanilla:1.4.5.6", command)


if __name__ == "__main__":
    unittest.main()
