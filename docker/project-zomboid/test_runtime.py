import importlib.util
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location('pz_runtime', Path(__file__).with_name('runtime.py'))
runtime = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(runtime)


class RuntimeTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.game = self.root / 'game'
        self.game.mkdir()
        self.config = {'mainClass': 'zombie/network/GameServer', 'classpath': ['java/.', 'java/projectzomboid.jar'],
                       'vmArgs': ['-Xmx8g', '-Xms2g', '-Dzomboid.steam=1', '-XX:+UseZGC']}
        (self.game / 'ProjectZomboid64.json').write_text(json.dumps(self.config))
        self.data = self.root / 'data'
        self.server = self.data / 'Zomboid/Server'
        self.server.mkdir(parents=True)
        self.files = {name: f'provider owns {name}\n' for name in
                      ('gamepanel.ini.pending', 'gamepanel_SandboxVars.lua', 'gamepanel_spawnregions.lua')}
        self.files['gamepanel.ini.pending'] = 'ResetID=1234567890\nServerPlayerID=fixture-identity\nMaxPlayers=2\n'
        self.files['../db/gamepanel.db'] = 'existing identity'
        for name, value in self.files.items():
            path = self.server / name
            path.parent.mkdir(exist_ok=True)
            path.write_text(value)
        self.env = patch.dict(os.environ, {'PZ_ADMIN_PASSWORD': 'test-random-password'})
        self.env.start()
        self.addCleanup(self.env.stop)

    def test_cgroup_memory_budgets_and_unlimited(self):
        cgroup = self.root / 'cgroup'
        cgroup.mkdir()
        meminfo = self.root / 'meminfo'
        meminfo.write_text('MemTotal: 16777216 kB\n')
        for limit, expected in [('2147483648', 1228), ('4294967296', 2457), ('max', 8192)]:
            (cgroup / 'memory.max').write_text(limit)
            self.assertEqual(runtime.heap_mib(cgroup, meminfo), expected)
        (cgroup / 'memory').mkdir()
        (cgroup / 'memory/memory.limit_in_bytes').write_text('3221225472')
        self.assertEqual(runtime.heap_mib(cgroup, meminfo), 1843)
        (cgroup / 'memory.max').write_text('536870912')
        with self.assertRaisesRegex(ValueError, '1024 MiB'):
            runtime.heap_mib(cgroup, meminfo)

    def test_prepare_preserves_provider_files_db_and_official_json(self):
        with patch.object(runtime, 'heap_mib', return_value=1228):
            command, env, heap = runtime.prepare(self.game, self.data, self.root / 'scratch')
            command2, _, _ = runtime.prepare(self.game, self.data, self.root / 'scratch')
        self.assertEqual(command, command2)
        self.assertEqual(heap, 1228)
        self.assertIn(f'-cachedir={self.data}/Zomboid', command)
        self.assertEqual(command[command.index('-servername') + 1], 'gamepanel')
        self.assertNotIn('-adminpassword', command)
        self.assertNotIn('PZ_ADMIN_PASSWORD', env)
        config = json.loads((self.root / 'scratch/ProjectZomboid64.json').read_text())
        self.assertEqual(config['vmArgs'], ['-Dzomboid.steam=1', '-XX:+UseZGC', '-Xmx1228m'])
        self.assertEqual(json.loads((self.game / 'ProjectZomboid64.json').read_text()), self.config)
        for name, value in self.files.items():
            self.assertEqual((self.server / name).read_text(), value)

    def test_missing_and_symlink_configuration_fail_without_regenerating(self):
        path = self.server / 'gamepanel.ini.pending'
        path.unlink()
        with self.assertRaisesRegex(ValueError, 'configuration missing'):
            runtime.prepare(self.game, self.data)
        path.symlink_to(self.game / 'ProjectZomboid64.json')
        with self.assertRaisesRegex(ValueError, 'unsafe'):
            runtime.prepare(self.game, self.data)

    def test_missing_password_never_prompts(self):
        (self.server / '../db/gamepanel.db').unlink()
        for value in ('', 'bad\nvalue'):
            with patch.dict(os.environ, {'PZ_ADMIN_PASSWORD': value}):
                with self.assertRaisesRegex(ValueError, 'PZ_ADMIN_PASSWORD'):
                    runtime.prepare(self.game, self.data)

    def test_environment_jvm_override_is_removed(self):
        with patch.dict(os.environ, {'JAVA_TOOL_OPTIONS': '-Xmx32g', '_JAVA_OPTIONS': '-Xmx32g', 'JDK_JAVA_OPTIONS': '-Xmx32g'}), patch.object(runtime, 'heap_mib', return_value=1228):
            _, env, _ = runtime.prepare(self.game, self.data, self.root / 'scratch')
        for key in ('JAVA_TOOL_OPTIONS', '_JAVA_OPTIONS', 'JDK_JAVA_OPTIONS'):
            self.assertNotIn(key, env)

    def test_signal_becomes_console_quit_even_after_stdin_eof(self):
        child = self.root / 'child.py'
        ready = self.root / 'ready'
        saved = self.root / 'saved'
        child.write_text(f"from pathlib import Path\nimport sys\nPath({str(ready)!r}).touch()\nfor line in sys.stdin:\n if line.strip() == 'quit':\n  Path({str(saved)!r}).write_text('saved'); break\n")
        wrapper = self.root / 'wrapper.py'
        wrapper.write_text(f"import sys, os\nsys.path.insert(0, {str(Path(__file__).parent)!r})\nimport runtime\nsys.exit(runtime.supervise([sys.executable, {str(child)!r}], os.environ, {str(self.root)!r}))\n")
        proc = subprocess.Popen([sys.executable, str(wrapper)], stdin=subprocess.DEVNULL)
        try:
            deadline = time.monotonic() + 5
            while not ready.exists() and time.monotonic() < deadline:
                time.sleep(.02)
            self.assertTrue(ready.exists())
            proc.send_signal(signal.SIGTERM)
            self.assertEqual(proc.wait(timeout=5), 0)
            self.assertEqual(saved.read_text(), 'saved')
        finally:
            if proc.poll() is None:
                proc.kill()
                proc.wait()


    def test_console_input_and_nonzero_native_exit_are_forwarded(self):
        wrapper = self.root / 'console-wrapper.py'
        child = "import sys; line = sys.stdin.readline(); sys.exit(7 if line == 'players\\n' else 9)"
        wrapper.write_text(f"import sys, os\nsys.path.insert(0, {str(Path(__file__).parent)!r})\nimport runtime\nsys.exit(runtime.supervise([sys.executable, '-c', {child!r}], os.environ, {str(self.root)!r}))\n")
        result = subprocess.run([sys.executable, str(wrapper)], input='players\n', text=True, capture_output=True, timeout=5)
        self.assertEqual(result.returncode, 7, result.stderr)



    def test_initial_admin_cli_and_existing_db_identity(self):
        db = self.server / '../db/gamepanel.db'
        db.unlink()
        with patch.object(runtime, 'heap_mib', return_value=1228):
            command, _, _ = runtime.prepare(self.game, self.data, self.root / 'scratch')
            self.assertEqual(command[-2:], ['-adminpassword', 'test-random-password'])
            db.write_text('existing identity')
            with patch.dict(os.environ, {'PZ_ADMIN_PASSWORD': ''}):
                command, _, _ = runtime.prepare(self.game, self.data, self.root / 'scratch')
        self.assertNotIn('-adminpassword', command)
        self.assertEqual(db.read_text(), 'existing identity')

    def test_native_output_redacts_password(self):
        wrapper = self.root / 'redact-wrapper.py'
        child = "import sys; print('pzexe: arg: private-password'); sys.exit(0)"
        wrapper.write_text(f"import sys, os\nsys.path.insert(0, {str(Path(__file__).parent)!r})\nimport runtime\nsys.exit(runtime.supervise([sys.executable, '-c', {child!r}], os.environ, {str(self.root)!r}, redactions=('private-password',)))\n")
        result = subprocess.run([sys.executable, str(wrapper)], input='', text=True, capture_output=True, timeout=5)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('pzexe: arg: [redacted]', result.stdout)
        self.assertNotIn('private-password', result.stdout + result.stderr)


class StagedIniTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.server = Path(self.tmp.name) / 'Zomboid/Server'
        self.server.mkdir(parents=True)
        self.pending = self.server / 'gamepanel.ini.pending'
        self.canonical = self.server / 'gamepanel.ini'
        self.desired = 'ResetID=1234567890\nServerPlayerID=desired-identity\nMaxPlayers=2\nPublicName=Desired\n'
        self.native = 'ResetID=7654321\nServerPlayerID=native-identity\nMaxPlayers=8\nNativeOnly=discard\n'
        self.pending.write_text(self.desired)

    def merge(self):
        runtime.merge_server_ini(self.server)

    def test_first_boot_uses_pending_and_keeps_input(self):
        self.merge()
        self.assertEqual(self.canonical.read_text(), self.desired)
        self.assertEqual(self.pending.read_text(), self.desired)
        self.assertEqual(self.canonical.stat().st_mode & 0o777, 0o600)

    def test_native_first_reset_survives_provider_rerender_and_only_two_keys_merge(self):
        self.canonical.write_text(self.native)
        self.merge()
        expected = 'ResetID=7654321\nServerPlayerID=native-identity\nMaxPlayers=2\nPublicName=Desired\n'
        self.assertEqual(self.canonical.read_text(), expected)
        self.pending.write_text(self.desired.replace('MaxPlayers=2', 'MaxPlayers=3'))
        self.merge()
        self.assertEqual(self.canonical.read_text(), expected.replace('MaxPlayers=2', 'MaxPlayers=3'))

    def test_missing_pending_never_accepts_legacy_canonical(self):
        self.canonical.write_text(self.native)
        self.pending.unlink()
        with self.assertRaisesRegex(ValueError, 'pending'):
            self.merge()
        self.assertEqual(self.canonical.read_text(), self.native)

    def test_identity_range_and_string_validation_do_not_replace_existing_file(self):
        for identity in ('-1', '2147483648', '1.5', '+1', '', '1e4', ' 1'):
            with self.subTest(identity=identity):
                self.canonical.write_text(self.native.replace('7654321', identity))
                before = self.canonical.read_bytes()
                with self.assertRaises(ValueError):
                    self.merge()
                self.assertEqual(self.canonical.read_bytes(), before)
        for identity in ('', ' ', 'trailing ', '\x1f'):
            with self.subTest(player_id=repr(identity)):
                self.canonical.write_text(self.native.replace('native-identity', identity))
                with self.assertRaises(ValueError):
                    self.merge()

    def test_official_reset_boundaries_and_nonnumeric_player_ids_are_accepted(self):
        for value in ('0', '2147483647'):
            self.canonical.write_text(self.native.replace('7654321', value))
            self.merge()
            self.assertIn('ResetID=' + value + '\n', self.canonical.read_text())
            self.assertIn('ServerPlayerID=native-identity\n', self.canonical.read_text())

    def test_crlf_nul_duplicate_and_malformed_lines_rejected_in_both_files(self):
        for target in (self.pending, self.canonical):
            for invalid in (self.desired.replace('\n', '\r\n'), self.desired + 'ResetID=42\n',
                            self.desired + 'BadLine\n', self.desired + 'PublicName=bad\x00value\n'):
                with self.subTest(target=target.name, invalid=repr(invalid)):
                    self.pending.write_text(self.desired)
                    self.canonical.write_text(self.native)
                    target.write_bytes(invalid.encode())
                    before = self.canonical.read_bytes()
                    with self.assertRaises(ValueError):
                        self.merge()
                    self.assertEqual(self.canonical.read_bytes(), before)

    def test_invalid_pending_identity_is_not_masked_by_existing_canonical(self):
        self.canonical.write_text(self.native)
        self.pending.write_text(self.desired.replace('1234567890', '2147483648'))
        with self.assertRaises(ValueError):
            self.merge()
        self.assertEqual(self.canonical.read_text(), self.native)

    def test_missing_canonical_identity_fails_instead_of_reseeding(self):
        for key in ('ResetID', 'ServerPlayerID'):
            self.canonical.write_text('\n'.join(line for line in self.native.splitlines() if not line.startswith(key + '=')) + '\n')
            with self.assertRaises(ValueError):
                self.merge()

    def test_pending_and_canonical_symlinks_are_rejected(self):
        other = self.server / 'other.ini'
        other.write_text(self.native)
        for target in (self.pending, self.canonical):
            with self.subTest(target=target.name):
                self.pending.unlink(missing_ok=True)
                self.canonical.unlink(missing_ok=True)
                self.pending.write_text(self.desired)
                target.unlink(missing_ok=True)
                target.symlink_to(other)
                with self.assertRaisesRegex(ValueError, 'unsafe'):
                    self.merge()
                self.assertEqual(other.read_text(), self.native)

    def test_server_and_zomboid_parent_symlinks_are_rejected(self):
        alias = self.server.parent / 'linked-server'
        alias.symlink_to(self.server, target_is_directory=True)
        with self.assertRaisesRegex(ValueError, 'unsafe'):
            runtime.merge_server_ini(alias)
        parent_alias = self.server.parent.parent / 'linked-zomboid'
        parent_alias.symlink_to(self.server.parent, target_is_directory=True)
        with self.assertRaisesRegex(ValueError, 'unsafe'):
            runtime.merge_server_ini(parent_alias / 'Server')
        self.assertFalse(self.canonical.exists())

    def test_non_regular_pending_is_rejected_without_blocking(self):
        self.pending.unlink()
        os.mkfifo(self.pending)
        with self.assertRaisesRegex(ValueError, 'regular'):
            self.merge()

    def test_failed_atomic_replace_retains_canonical_and_removes_temp(self):
        self.canonical.write_text(self.native)
        with patch.object(runtime.os, 'replace', side_effect=OSError('injected replace failure')):
            with self.assertRaises(OSError):
                self.merge()
        self.assertEqual(self.canonical.read_text(), self.native)
        self.assertEqual(list(self.server.glob('.gamepanel-*.tmp')), [])


if __name__ == '__main__':
    unittest.main()
