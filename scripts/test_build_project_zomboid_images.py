"""Scoped CLI checks; never downloads games or contacts Docker."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent
PIN = 'example.test/project-zomboid-base@sha256:' + 'a' * 64


class BuildProjectZomboidImagesTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.calls = self.root / 'calls'
        docker = self.root / 'docker'
        docker.write_text("#!/usr/bin/env python3\nimport os, json, sys\nwith open(os.environ['BUILD_TEST_CALLS'], 'a') as f: f.write(json.dumps(sys.argv[1:]) + '\\n')\n")
        docker.chmod(0o755)
        self.env = dict(os.environ, PATH=str(self.root) + os.pathsep + os.environ['PATH'], BUILD_TEST_CALLS=str(self.calls))
        self.game = self.root / 'official install'
        (self.game / 'steamapps').mkdir(parents=True)
        (self.game / 'ProjectZomboid64').touch()
        (self.game / 'steamapps/appmanifest_380870.acf').touch()

    def run_build(self, *args):
        return subprocess.run(['bash', str(ROOT / 'scripts/build-project-zomboid-images.sh'), *args], env=self.env, capture_output=True, text=True)

    def commands(self):
        return [json.loads(s) for s in self.calls.read_text().splitlines()] if self.calls.exists() else []

    def test_runtime_has_only_helper_build_on_explicit_pinned_base(self):
        result = self.run_build('--base-image', PIN)
        self.assertEqual(result.returncode, 0, result.stderr)
        command, = self.commands()
        self.assertIn('PZ_BASE_IMAGE=' + PIN, command)
        self.assertIn(str(ROOT / 'docker/project-zomboid/Dockerfile'), command)
        self.assertNotIn('--build-context', command)
        self.assertIn('--load', command)

    def test_base_reuses_local_install_without_steam(self):
        result = self.run_build('base', '--base-build-id', '25485538', '--game-files-dir', str(self.game))
        self.assertEqual(result.returncode, 0, result.stderr)
        command, = self.commands()
        self.assertIn('gamefiles=' + str(self.game), command)
        self.assertIn('PZ_BUILD_ID=25485538', command)
        self.assertIn('gamepanel-local/project-zomboid-base:build-25485538', command)
        self.assertNotIn('steamcmd', ' '.join(command))

    def test_local_candidate_is_explicit_and_cannot_push(self):
        self.assertEqual(self.run_build('--local-base-image', 'gamepanel-local/base:candidate').returncode, 0)
        self.assertNotEqual(self.run_build('--local-base-image', 'gamepanel-local/base:candidate', '--push').returncode, 0)
        self.assertEqual(len(self.commands()), 1)

    def test_invalid_inputs_never_call_docker(self):
        for args in [(), ('--base-image', 'example/base:latest'), ('--platform', 'linux/arm64'),
                     ('base',), ('base', '--base-build-id', '123'),
                     ('--base-image',), ('--push',),
                     ('--base-image', PIN, '--game-files-dir', str(self.game)),
                     ('--base-image', PIN, '--local-base-image', 'local:tag')]:
            self.assertNotEqual(self.run_build(*args).returncode, 0, args)
        self.assertEqual(self.commands(), [])

    def test_help_does_not_build(self):
        self.assertEqual(self.run_build('--help').returncode, 0)
        self.assertEqual(self.commands(), [])


if __name__ == '__main__':
    unittest.main()
