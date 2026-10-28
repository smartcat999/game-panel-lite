"""Launch the official native executable with per-container JVM settings.

No installer, updater, or world migration runs here. Provider INI input is
merged with only the two game-owned persistent identity fields before launch.
"""
import json
import os
from pathlib import Path
import selectors
import re
import stat
import signal
import subprocess
import sys
import tempfile
import threading

MIB = 1024 * 1024


def heap_mib(cgroup=Path('/sys/fs/cgroup'), meminfo=Path('/proc/meminfo')):
    limits = []
    for relative in ('memory.max', 'memory/memory.limit_in_bytes'):
        try:
            value = int((cgroup / relative).read_text().strip())
            if 0 < value < 1 << 60:
                limits.append(value)
        except (OSError, ValueError):
            pass
    for line in meminfo.read_text().splitlines():
        if line.startswith('MemTotal:'):
            limits.append(int(line.split()[1]) * 1024)
            break
    if not limits:
        raise ValueError('Cannot determine available memory')
    # Leave room for native buffers, the OS and ZGC. No forced initial heap.
    budget = min(limits) // MIB
    if budget < 1024:
        raise ValueError('Project Zomboid requires a container memory budget of at least 1024 MiB')
    return min(8192, budget * 60 // 100)



IDENTITY_KEYS = ('ResetID', 'ServerPlayerID')


def read_ini(path, optional=False):
    if path.is_symlink():
        raise ValueError('Provider/native configuration is unsafe: ' + path.name)
    try:
        descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    except FileNotFoundError:
        if optional:
            return None
        raise ValueError('Provider configuration missing: ' + path.name) from None
    with os.fdopen(descriptor, 'rb') as stream:
        if not stat.S_ISREG(os.fstat(stream.fileno()).st_mode):
            raise ValueError('INI configuration must be a regular file')
        raw = stream.read(1024 * 1024 + 1)
    if len(raw) > 1024 * 1024:
        raise ValueError('INI configuration exceeds 1 MiB')
    text = raw.decode('utf-8')
    if any(ord(char) < 32 and char != '\n' for char in text) or '\x7f' in text:
        raise ValueError('INI configuration contains CRLF or control characters')
    values = {}
    lines = text.splitlines()
    for line in lines:
        if not line.strip() or line.lstrip().startswith('#'):
            continue
        key, separator, value = line.partition('=')
        if not separator or not re.fullmatch(r'[A-Za-z][A-Za-z0-9_]*', key):
            raise ValueError('Malformed INI option')
        if key in values:
            raise ValueError('Duplicate INI option: ' + key)
        values[key] = value
    reset = values.get('ResetID', '')
    if not re.fullmatch(r'[0-9]+', reset) or not 0 <= int(reset) <= 2147483647:
        raise ValueError('ResetID must be an integer in the official range 0..2147483647')
    player = values.get('ServerPlayerID', '')
    if not player or player != player.strip():
        raise ValueError('ServerPlayerID must be nonempty single-line identity text')
    return lines, values


def merge_server_ini(server):
    if server.is_symlink() or server.parent.is_symlink():
        raise ValueError('Zomboid/Server directory is unsafe')
    pending = server / 'gamepanel.ini.pending'
    canonical = server / 'gamepanel.ini'
    lines, desired = read_ini(pending)
    existing = read_ini(canonical, optional=True)
    identities = existing[1] if existing is not None else desired
    output = []
    for line in lines:
        key = line.partition('=')[0]
        output.append(key + '=' + identities[key] if key in IDENTITY_KEYS else line)
    content = '\n'.join(output) + '\n'
    descriptor, temporary = tempfile.mkstemp(prefix='.gamepanel-', suffix='.tmp', dir=server)
    try:
        with os.fdopen(descriptor, 'w', encoding='utf-8', newline='\n') as stream:
            stream.write(content)
            stream.flush()
            os.fsync(stream.fileno())
        # Same-directory rename is atomic. A crash before it leaves the previous
        # native INI intact; no shutdown callback or identity sidecar is needed.
        os.replace(temporary, canonical)
        directory = os.open(server, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        Path(temporary).unlink(missing_ok=True)


def prepare(game=Path('/opt/project-zomboid'), data=Path('/data'), scratch=None):
    cache = data / 'Zomboid'
    if cache.is_symlink() or (cache / 'Server').is_symlink():
        raise ValueError('Zomboid and Server must be real directories')
    server = cache / 'Server'
    required = ('gamepanel.ini.pending', 'gamepanel_SandboxVars.lua', 'gamepanel_spawnregions.lua')
    for name in required:
        path = server / name
        if path.is_symlink() or not path.is_file():
            raise ValueError('Provider configuration missing or unsafe: ' + name)
    password = os.environ.get('PZ_ADMIN_PASSWORD', '')
    database = cache / 'db/gamepanel.db'
    if database.is_symlink():
        raise ValueError('Administrator database must not be a symlink')
    first_boot = not database.is_file()
    if first_boot and (not password or any(c in password for c in '\r\n\0')):
        raise ValueError('PZ_ADMIN_PASSWORD must contain a nonempty single-line initial admin password')
    config = json.loads((game / 'ProjectZomboid64.json').read_text())
    heap = heap_mib()
    config['vmArgs'] = [arg for arg in config['vmArgs'] if not arg.startswith(('-Xmx', '-Xms'))]
    config['vmArgs'].append(f'-Xmx{heap}m')
    if '-Dzomboid.steam=1' not in config['vmArgs']:
        raise ValueError('Official Steam-enabled launcher configuration expected')
    merge_server_ini(server)
    scratch = Path(scratch or tempfile.mkdtemp(prefix='gamepanel-pz-'))
    scratch.mkdir(parents=True, exist_ok=True)
    config_path = scratch / 'ProjectZomboid64.json'
    config_path.write_text(json.dumps(config))
    config_path.chmod(0o600)
    # cachedir IS the Zomboid directory; it does not append another Zomboid.
    command = [str(game / 'ProjectZomboid64'), '-pzexeconfig', str(config_path),
               f'-cachedir={cache}', '-servername', 'gamepanel', '-adminusername', 'admin']
    if first_boot:
        command.extend(['-adminpassword', password])
    home = data / '.runtime-home'
    home.mkdir(exist_ok=True)
    env = dict(os.environ, HOME=str(home))
    env.pop('PZ_ADMIN_PASSWORD', None)
    # Prevent inherited JVM flags from silently replacing the container budget.
    for key in ('JAVA_TOOL_OPTIONS', '_JAVA_OPTIONS', 'JDK_JAVA_OPTIONS'):
        env.pop(key, None)
    env['PATH'] = str(game / 'jre64/bin') + os.pathsep + env.get('PATH', '/usr/bin:/bin')
    env['LD_LIBRARY_PATH'] = ':'.join(str(path) for path in (game / 'linux64', game, game / 'jre64/lib', game / 'jre64/lib/amd64'))
    env['LD_PRELOAD'] = str(game / 'jre64/lib/libjsig.so')
    return command, env, heap


def supervise(command, env, cwd, input_stream=sys.stdin, redactions=()):
    stopping = False

    def request_stop(_signum, _frame):
        nonlocal stopping
        stopping = True

    old_handlers = {sig: signal.signal(sig, request_stop) for sig in (signal.SIGTERM, signal.SIGINT)}
    child = subprocess.Popen(command, cwd=cwd, env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    secrets = [value.encode() for value in redactions if value]

    def forward_output():
        # The official native launcher prints every CLI argument on its own line.
        for line in iter(child.stdout.readline, b''):
            for secret in secrets:
                line = line.replace(secret, b'[redacted]')
            sys.stdout.buffer.write(line)
            sys.stdout.buffer.flush()

    output = threading.Thread(target=forward_output, daemon=True)
    output.start()
    selector = selectors.DefaultSelector()
    try:
        try:
            selector.register(input_stream, selectors.EVENT_READ)
        except (OSError, ValueError):
            pass
        quit_sent = False
        while child.poll() is None:
            if stopping and not quit_sent:
                try:
                    child.stdin.write(b'quit\n')
                    child.stdin.flush()
                except BrokenPipeError:
                    pass
                quit_sent = True
            for key, _ in selector.select(timeout=0.2):
                line = os.read(key.fileobj.fileno(), 4096)
                if not line:
                    selector.unregister(key.fileobj)
                    continue
                if not quit_sent:
                    try:
                        child.stdin.write(line)
                        child.stdin.flush()
                    except BrokenPipeError:
                        pass
        code = child.wait()
        output.join()
        return 128 - code if code < 0 else code
    finally:
        selector.close()
        child.stdin.close()
        child.stdout.close()
        for sig, handler in old_handlers.items():
            signal.signal(sig, handler)


def main():
    os.umask(0o077)
    try:
        command, env, heap = prepare()
        print(f'Starting Project Zomboid: Steam, servername=gamepanel, cachedir=/data/Zomboid, heap={heap} MiB', flush=True)
        return supervise(command, env, '/opt/project-zomboid', redactions=(os.environ.get('PZ_ADMIN_PASSWORD', ''),))
    except (OSError, ValueError, KeyError) as error:
        print('Project Zomboid startup failed: ' + str(error), file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
