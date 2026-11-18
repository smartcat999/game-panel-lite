"""Opt-in native image smoke test. Run on a Docker host with a local candidate.

PZ_SMOKE_IMAGE=gamepanel-local/project-zomboid:candidate-42.21 \
PZ_SMOKE_SOURCE=/path/to/verified/official/server python3 -m unittest \
  discover -s docker/project-zomboid -p test_native_smoke.py

Creates only a unique container/data directory, publishes no ports, keeps the
artifacts for inspection, and never mounts an existing game room.
"""
import os
from pathlib import Path
import secrets
import sqlite3
import subprocess
import tempfile
import time
import unittest


@unittest.skipUnless(os.environ.get('PZ_SMOKE_IMAGE') and os.environ.get('PZ_SMOKE_SOURCE'), 'requires explicit local candidate and official source')
class NativeSmokeTest(unittest.TestCase):
    def docker(self, *args, timeout=60):
        return subprocess.run(['docker', *args], text=True, capture_output=True, check=True, timeout=timeout).stdout

    def test_native_boot_quit_restart(self):
        source = Path(os.environ['PZ_SMOKE_SOURCE'])
        directory = Path(tempfile.mkdtemp(prefix='gpl-pz-native-'))
        data = directory / 'data'
        server = data / 'Zomboid/Server'
        server.mkdir(parents=True)
        (server / 'gamepanel.ini.pending').write_text('ResetID=1234567890\nServerPlayerID=smoke-provider-identity\nPublic=false\nUPnP=false\nDefaultPort=16261\nUDPPort=16262\nRCONPassword=\nMods=\nWorkshopItems=\nMap=Muldraugh, KY\nMaxPlayers=2\n')
        preset = (source / 'media/lua/shared/Sandbox/Apocalypse.lua').read_text()
        (server / 'gamepanel_SandboxVars.lua').write_text(preset.replace('return {', 'SandboxVars = {', 1))
        (server / 'gamepanel_spawnregions.lua').write_text('function SpawnRegions()\n return {{ name = "Muldraugh, KY", file = "media/maps/Muldraugh, KY/spawnpoints.lua" }}\nend\n')
        password = secrets.token_urlsafe(24)
        name = 'gpl-pz-native-' + secrets.token_hex(4)
        print(f'Native test artifacts: {directory}', flush=True)
        self.docker('create', '--name', name, '--memory', '3g', '--cpus', '2', '-i',
                    '--stop-timeout', '180', '-v', f'{data}:/data', '-e', f'PZ_ADMIN_PASSWORD={password}',
                    os.environ['PZ_SMOKE_IMAGE'])
        try:
            identity = None
            for attempt in range(2):
                self.docker('start', name)
                deadline = time.monotonic() + 240
                while time.monotonic() < deadline:
                    log = self.docker('logs', name)
                    (directory / f'boot-{attempt}.log').write_text(log)
                    if '*** SERVER STARTED ****' in log or '*** SERVER STARTED ***' in log:
                        break
                    state = self.docker('inspect', '--format', '{{.State.Running}}', name).strip()
                    self.assertEqual(state, 'true', log[-5000:])
                    time.sleep(2)
                else:
                    self.fail('Native startup timed out; inspect ' + str(directory))
                self.assertIn('42.21', log)
                self.assertIn('*** Steam is enabled', log)
                self.assertNotIn(password, log)
                self.assertNotIn('Enter new administrator password', log)
                # Check actual namespace sockets without publishing anything.
                sockets = self.docker('exec', name, 'cat', '/proc/net/udp')
                (directory / f'udp-{attempt}.txt').write_text(sockets)
                self.assertIn(':3F85', sockets)  # UDP 16261
                self.assertIn(':3F86', sockets)  # UDP 16262
                tcp = self.docker('exec', name, 'cat', '/proc/net/tcp')
                self.assertNotIn(':6987', tcp)  # no default RCON TCP 27015
                if attempt == 0:
                    self.docker('stop', '-t', '180', name, timeout=195)
                else:
                    attached = subprocess.run(['docker', 'attach', '--sig-proxy=false', name], input='quit\n', text=True, capture_output=True, timeout=185, check=True)
                    (directory / 'console-quit.log').write_text(attached.stdout + attached.stderr)
                    self.docker('wait', name, timeout=185)
                state = self.docker('inspect', '--format', '{{.State.ExitCode}} {{.State.OOMKilled}}', name).strip()
                self.assertEqual(state, '0 false')
                final_log = self.docker('logs', name)
                (directory / f'stopped-{attempt}.log').write_text(final_log)
                self.assertIn('quit', final_log.lower())
                self.assertTrue((data / 'Zomboid/Saves/Multiplayer/gamepanel').is_dir())
                db = data / 'Zomboid/db/gamepanel.db'
                self.assertTrue(db.is_file())
                with sqlite3.connect(db) as connection:
                    admins = connection.execute("SELECT username, password FROM whitelist WHERE username = 'admin'").fetchall()
                self.assertEqual(len(admins), 1)
                ini = (server / 'gamepanel.ini').read_text()
                reset = next(line for line in ini.splitlines() if line.startswith('ResetID='))
                current = (admins, reset)
                if identity is not None:
                    self.assertEqual(current, identity)
                identity = current
                # The initial password may change in ENV without rewriting old DB.
                if attempt == 0:
                    self.docker('rm', name)
                    # Re-render the Provider input after native changed ResetID.
                    pending = server / 'gamepanel.ini.pending'
                    pending.write_text(pending.read_text().replace('ResetID=1234567890', 'ResetID=2147483647').replace('ServerPlayerID=smoke-provider-identity', 'ServerPlayerID=rerendered-provider-identity'))
                    password = secrets.token_urlsafe(24)
                    self.docker('create', '--name', name, '--memory', '3g', '--cpus', '2', '-i',
                                '--stop-timeout', '180', '-v', f'{data}:/data', '-e', 'PZ_ADMIN_PASSWORD=' + password,
                                os.environ['PZ_SMOKE_IMAGE'])
        finally:
            self.docker('rm', '-f', name)


if __name__ == '__main__':
    unittest.main()
