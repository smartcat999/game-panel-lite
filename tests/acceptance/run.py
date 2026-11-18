#!/usr/bin/env python3
"""Run the current binaries against an isolated real Docker host.

The local entrypoint builds (or accepts) Linux binaries, transfers a private
bundle over SSH, then retrieves the report. The remote supervisor owns only its
random run directory, two fixture containers and workloads for its new Node UID.
It never invokes compose down, docker prune, or a shared database reset.
"""
from __future__ import annotations

import argparse
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import secrets
import shlex
import signal
import socket
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.error
import urllib.request

RUN_PATTERN = re.compile(r"rc-[0-9]{8}T[0-9]{6}-[0-9a-f]{12}")
REMOTE_PARENT = Path("/tmp/gamepanel-docker-acceptance")
LABEL = "io.gamepanel.acceptance"
BINARIES = ("apiserver", "controller-manager", "node-agent")


def execute(argv, *, check=True, **kwargs):
    result = subprocess.run(argv, capture_output=True, text=True, **kwargs)
    if check and result.returncode:
        # argv may contain generated credentials: never render the command.
        raise RuntimeError(f"{Path(argv[0]).name} exited {result.returncode}: {result.stderr[-1500:].strip()}")
    return result


def wait_for(predicate, *, seconds=45, description="condition"):
    deadline = time.monotonic() + seconds
    error = None
    while time.monotonic() < deadline:
        try:
            value = predicate()
            if value:
                return value
        except Exception as exc:
            error = str(exc)
        time.sleep(0.3)
    raise RuntimeError(f"timeout waiting for {description}: {error or 'not ready'}")


def local_build(source, output):
    output.mkdir(parents=True, exist_ok=True)
    env = os.environ.copy()
    env.update(GOOS="linux", GOARCH="amd64", CGO_ENABLED="0")
    def build(name):
        execute(["go", "build", "-trimpath", "-o", str(output / name), "./cmd/" + name], cwd=source, env=env)
    with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
        list(pool.map(build, BINARIES))


def ssh_prefix(args):
    prefix = ["ssh", "-T", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10"]
    if args.ipv6:
        # Preserve the existing hostname's pinned SSH key when using the same
        # registered host's alternative Tailscale address.
        settings = execute(["ssh", "-G", args.host]).stdout.splitlines()
        hostname = next(line.split(" ", 1)[1] for line in settings if line.startswith("hostname "))
        prefix += ["-6", "-o", "HostName=" + args.ipv6, "-o", "HostKeyAlias=" + hostname]
    return prefix + [args.host]


def local_run(args):
    source = Path(__file__).resolve().parents[2]
    git_revision = execute(["git", "rev-parse", "HEAD"], cwd=source, check=False).stdout.strip()
    run_id = time.strftime("rc-%Y%m%dT%H%M%S-") + secrets.token_hex(6)
    remote = REMOTE_PARENT / run_id
    output = Path(args.output).resolve() if args.output else source / "tests/acceptance/artifacts" / run_id
    output.mkdir(parents=True, exist_ok=False)
    binaries = Path(args.binary_dir).resolve() if args.binary_dir else Path(tempfile.mkdtemp(prefix="gpc-acceptance-binaries-"))
    if not args.binary_dir:
        local_build(source, binaries)
    for name in BINARIES:
        if not (binaries / name).is_file():
            raise RuntimeError(f"missing binary: {name}")
    prefix = ssh_prefix(args)
    bootstrap = """import pathlib,sys,tarfile,re,shutil,os
root=pathlib.Path(sys.argv[1]).resolve()
assert root.parent==pathlib.Path('/tmp/gamepanel-docker-acceptance')
assert re.fullmatch(r'rc-[0-9]{8}T[0-9]{6}-[0-9a-f]{12}',root.name)
os.umask(0o077)
root.mkdir(parents=True,exist_ok=False)
allowed={'run.py','scenarios.py','bin/apiserver','bin/controller-manager','bin/node-agent'}
with tarfile.open(fileobj=sys.stdin.buffer,mode='r|gz') as archive:
 for member in archive:
  assert member.isfile() and member.name in allowed
  target=root/member.name
  target.parent.mkdir(parents=True,exist_ok=True)
  with archive.extractfile(member) as incoming,target.open('wb') as outgoing:shutil.copyfileobj(incoming,outgoing)
  target.chmod(0o700 if member.name.startswith('bin/') else 0o600)
print(root)
"""
    with tempfile.TemporaryFile() as package:
        with tarfile.open(fileobj=package, mode="w:gz") as archive:
            for name in ("run.py", "scenarios.py"):
                archive.add(Path(__file__).parent / name, arcname=name, recursive=False)
            for name in BINARIES:
                archive.add(binaries / name, arcname="bin/" + name, recursive=False)
        package.seek(0)
        uploaded = subprocess.run(prefix + [shlex.join(["python3", "-c", bootstrap, str(remote)])], stdin=package)
        if uploaded.returncode:
            return uploaded.returncode
    print(f"Acceptance host={args.host}, run={run_id}, output={output}", flush=True)
    command = ["python3", "-u", str(remote / "run.py"), "--remote", str(remote)]
    if git_revision:
        command += ["--git-revision", git_revision]
    result = subprocess.run(prefix + [shlex.join(command)])
    # The supervisor exports only selected, redacted evidence. Raw process logs
    # and secrets remain private and are not copied to the workspace.
    fetch = """import pathlib,sys,tarfile
root=pathlib.Path(sys.argv[1]).resolve()
assert root.parent==pathlib.Path('/tmp/gamepanel-docker-acceptance')
with tarfile.open(fileobj=sys.stdout.buffer,mode='w|gz') as archive:
 for name in ['report.json','report.md','supervisor.json']:
  p=root/name
  if p.is_file():archive.add(p,arcname=name,recursive=False)
"""
    with tempfile.TemporaryFile() as package:
        transfer = subprocess.run(prefix + [shlex.join(["python3", "-c", fetch, str(remote)])], stdout=package)
        if transfer.returncode:
            raise RuntimeError("could not retrieve remote acceptance evidence")
        package.seek(0)
        with tarfile.open(fileobj=package, mode="r:gz") as archive:
            for member in archive:
                if not member.isfile() or member.name not in {"report.json", "report.md", "supervisor.json"}:
                    raise RuntimeError("unexpected report archive member")
                with archive.extractfile(member) as incoming, (output / member.name).open("wb") as outgoing:
                    outgoing.write(incoming.read())
    print(f"Evidence saved to {output}", flush=True)
    return result.returncode


class Environment:
    def __init__(self, root):
        self.root = root.resolve()
        if self.root.parent != REMOTE_PARENT or not RUN_PATTERN.fullmatch(self.root.name):
            raise RuntimeError("invalid acceptance root")
        self.run_id = self.root.name
        os.environ["DOCKER_HOST"] = "unix:///var/run/docker.sock"
        os.environ.pop("DOCKER_CONTEXT", None)
        self.processes = {}
        self.log_files = []
        self.containers = []
        self.cleanup_errors = []
        self.fallback_workloads = []
        self.credentials = [secrets.token_urlsafe(32) for _ in range(5)]
        self.ports = self.reserve_ports(6)
        self.context = {
            "run_id": self.run_id,
            "api_url": f"http://127.0.0.1:{self.ports[0]}",
            "node_name": "accept-" + self.run_id.lower(),
            "work_dir": str(self.root / "work"),
            "pg_container": "gpc-" + self.run_id + "-postgres",
            "pg_user": "acceptance", "pg_db": "acceptance",
            "admin_user": "accept-admin", "admin_password": self.credentials[1],
            "workload_image": "python:3.12-alpine",
            "s3_bucket": "acceptance-backups",
        }
        self.redis_container = "gpc-" + self.run_id + "-redis"
        self.s3_container = "gpc-" + self.run_id + "-s3"
        self.env = None
        self.before = self.snapshot_containers()
        self.host = self.host_evidence()
        self.check_headroom()
        self.manifest()

    @staticmethod
    def reserve_ports(count):
        sockets = []
        try:
            for _ in range(count):
                listener = socket.socket()
                listener.bind(("127.0.0.1", 0))
                sockets.append(listener)
            return [listener.getsockname()[1] for listener in sockets]
        finally:
            for listener in sockets:
                listener.close()

    def scrub(self, text):
        for secret in self.credentials:
            text = text.replace(secret, "[redacted]")
        return text

    def manifest(self):
        data = {"run_id": self.run_id, "root": str(self.root), "fixture_containers": self.containers, "node_uid": self.context.get("node_uid"), "processes": {name: proc.pid for name, proc in self.processes.items() if proc.poll() is None}}
        (self.root / "supervisor.json").write_text(json.dumps(data, indent=2))

    @staticmethod
    def snapshot_containers():
        result = execute(["docker", "ps", "--no-trunc", "--format", "{{.ID}}\t{{.Names}}"])
        return dict(line.split("\t", 1) for line in result.stdout.splitlines())

    @staticmethod
    def host_evidence():
        info = json.loads(execute(["docker", "info", "--format", "{{json .}}"], timeout=20).stdout)
        fs = execute(["findmnt", "-no", "FSTYPE,OPTIONS", "/"], check=False).stdout.strip()
        return {"hostname": platform.node(), "system": platform.platform(), "machine": platform.machine(), "docker": {key: info.get(key) for key in ["ServerVersion", "Driver", "CgroupVersion", "CgroupDriver", "NCPU", "MemTotal"]}, "root_filesystem": fs, "existing_running_containers": len(Environment.snapshot_containers())}

    @staticmethod
    def check_headroom():
        mem = dict(line.split(":", 1) for line in Path("/proc/meminfo").read_text().splitlines())
        if int(mem["MemAvailable"].split()[0]) < 1536 * 1024:
            raise RuntimeError("insufficient free memory for isolated acceptance (requires 1.5 GiB)")
        free = os.statvfs(str(REMOTE_PARENT))
        if free.f_bavail * free.f_frsize < 2 * 1024**3:
            raise RuntimeError("insufficient free disk for isolated acceptance (requires 2 GiB)")

    def owned_container(self, name):
        inspected = execute(["docker", "inspect", "--format", "{{json .Config.Labels}}", name], check=False)
        if inspected.returncode:
            return False
        labels = json.loads(inspected.stdout)
        if labels.get(LABEL) != self.run_id:
            raise RuntimeError("refusing to operate on a container without this run's owner label")
        return True

    @staticmethod
    def tcp_open(host, port):
        try:
            with socket.create_connection((host, port), timeout=1):
                return True
        except OSError:
            return False

    def fixture(self, name, image, port, data_dir, extra, *, memory="256m", cmd_args=None):
        execute(["docker", "image", "inspect", image], timeout=15)
        if execute(["docker", "inspect", name], check=False).returncode == 0:
            raise RuntimeError("fixture name already exists")
        path = (self.root / "data" / data_dir).resolve()
        if self.root not in path.parents:
            raise RuntimeError("invalid fixture data path")
        path.mkdir(parents=True, exist_ok=False)
        self.containers.append(name)
        self.manifest()
        port_index = {"postgres": 3, "redis": 4, "seaweedfs": 5}[data_dir]
        host_port = self.ports[port_index]
        proxy_env = [
            "-e", "HTTP_PROXY=", "-e", "HTTPS_PROXY=", "-e", "ALL_PROXY=",
            "-e", "http_proxy=", "-e", "https_proxy=", "-e", "all_proxy=",
            "-e", "NO_PROXY=127.0.0.1,localhost", "-e", "no_proxy=127.0.0.1,localhost",
        ]
        argv = ["docker", "run", "-d", "--name", name, "--label", LABEL + "=" + self.run_id, "--cpus", "1", "--memory", memory, "--pids-limit", "128"] + proxy_env + ["-p", f"127.0.0.1:{host_port}:{port}", "-v", str(path) + ":" + ("/var/lib/postgresql/data" if data_dir == "postgres" else "/data")]
        default_args = ["redis-server", "--appendonly", "yes"] if data_dir == "redis" else []
        execute(argv + extra + [image] + (cmd_args if cmd_args is not None else default_args), timeout=45)
        binding = execute(["docker", "port", name, f"{port}/tcp"]).stdout.strip()
        if int(binding.rsplit(":", 1)[1]) != host_port:
            raise RuntimeError("fixture did not preserve its reserved loopback port")
        return host_port

    def prepare(self):
        pg_port = self.fixture(self.context["pg_container"], "postgres:16-alpine", 5432, "postgres", ["-e", "POSTGRES_USER=acceptance", "-e", "POSTGRES_DB=acceptance", "-e", "POSTGRES_PASSWORD=" + self.credentials[0]])
        wait_for(lambda: execute(["docker", "exec", self.context["pg_container"], "pg_isready", "-U", "acceptance", "-d", "acceptance"], check=False).returncode == 0, description="private PostgreSQL")
        redis_port = self.fixture(self.redis_container, "redis:7-alpine", 6379, "redis", [])
        wait_for(lambda: execute(["docker", "exec", self.redis_container, "redis-cli", "ping"], check=False).stdout.strip() == "PONG", description="private Redis")
        data_root = (self.root / "data").resolve()
        data_root.mkdir(mode=0o700, parents=True, exist_ok=True)
        s3_cfg = data_root / "s3.json"
        s3_cfg.write_text(json.dumps({"identities": [{"name": "acceptance", "credentials": [{"accessKey": "acceptance", "secretKey": self.credentials[4]}], "actions": ["Admin", "Read", "List", "Tagging", "Write"]}]}))
        if os.geteuid() == 0:
            os.chown(s3_cfg, 1000, 1000)
            s3_cfg.chmod(0o400)
        else:
            s3_cfg.chmod(0o644)
        s3_args = ["server", "-dir=/data", "-ip=127.0.0.1", "-ip.bind=0.0.0.0", "-master.volumeSizeLimitMB=64", "-volume.max=0", "-filer", "-s3", "-s3.port=8333", "-s3.autoCreateBucket", "-s3.config=/etc/seaweedfs/s3.json"]
        s3_port = self.fixture(self.s3_container, "chrislusf/seaweedfs:4.53", 8333, "seaweedfs", ["-v", str(s3_cfg) + ":/etc/seaweedfs/s3.json:ro"], memory="384m", cmd_args=s3_args)
        wait_for(lambda: execute(["docker", "exec", self.s3_container, "wget", "-Y", "off", "-qO-", "http://127.0.0.1:9333/cluster/status"], check=False).returncode == 0 and self.tcp_open("127.0.0.1", s3_port), description="private SeaweedFS S3")
        wait_for(
            lambda: subprocess.run(
                ["docker", "exec", "-i", self.s3_container, "weed", "shell", "-master=127.0.0.1:9333", "-filer=127.0.0.1:8888"],
                input="s3.bucket.create -name acceptance-backups\n",
                text=True,
                capture_output=True,
                timeout=15,
                check=False,
            ).returncode == 0,
            seconds=30,
            description="SeaweedFS bucket",
        )
        execute(["docker", "image", "inspect", self.context["workload_image"]], timeout=15)
        # Deliberately do not inherit unrelated production GP_*, Docker endpoint, or proxy settings.
        scrub_prefixes = ("GP_", "DOCKER_")
        scrub_keys = {"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"}
        self.env = {key: value for key, value in os.environ.items() if not key.startswith(scrub_prefixes) and key not in scrub_keys}
        self.env.update({
            "NO_PROXY": "127.0.0.1,localhost",
            "no_proxy": "127.0.0.1,localhost",
            "GP_DB_DSN": f"postgres://acceptance:{self.credentials[0]}@127.0.0.1:{pg_port}/acceptance?sslmode=disable",
            "GP_DB_MAX_OPEN_CONNS": "8", "GP_DB_MAX_IDLE_CONNS": "2",
            "GP_CACHE_DRIVER": "redis", "GP_CACHE_ADDR": f"127.0.0.1:{redis_port}", "GP_CACHE_DB": "0",
            "GP_MQ_DRIVER": "redis", "GP_MQ_ADDR": f"127.0.0.1:{redis_port}", "GP_MQ_DB": "1", "GP_MQ_GROUP": self.run_id,
            "GP_S3_ENDPOINT": f"http://127.0.0.1:{s3_port}",
            "GP_S3_PUBLIC_ENDPOINT": f"http://127.0.0.1:{s3_port}",
            "GP_S3_REGION": "us-east-1",
            "GP_S3_BUCKET": "acceptance-backups",
            "GP_S3_ACCESS_KEY": "acceptance",
            "GP_S3_SECRET_KEY": self.credentials[4],
            "GP_S3_USE_PATH_STYLE": "true",
            "GP_S3_PRESIGN_TTL": "15m",
            "GP_SERVER_ADDR": f"127.0.0.1:{self.ports[0]}",
            "GP_CONTROLLER_ADDR": f"127.0.0.1:{self.ports[1]}", "GP_CONTROLLER_RESYNC_PERIOD": "1s", "GP_CONTROLLER_NODE_GRACE_DURATION": "30s", "GP_CONTROLLER_WORKERS": "4",
            "GP_AUTH_JWT_SECRET": self.credentials[2],
            "GP_BOOTSTRAP_ADMIN": self.context["admin_user"], "GP_BOOTSTRAP_PASSWORD": self.context["admin_password"], "GP_BOOTSTRAP_EMAIL": "acceptance@example.invalid",
            "GP_AGENT_TOKEN": self.credentials[3], "GP_AGENT_NODE_NAME": self.context["node_name"],
            "GP_AGENT_WORK_DIR": self.context["work_dir"], "GP_AGENT_ADDR": f"127.0.0.1:{self.ports[2]}",
            "GP_AGENT_INTERNAL_IP": "127.0.0.1", "GP_AGENT_EXTERNAL_IP": "127.0.0.1", "GP_AGENT_APISERVER_URL": self.context["api_url"], "GP_AGENT_HEARTBEAT_INTERVAL": "1s",
            "GP_LOG_FORMAT": "json", "GP_LOG_LEVEL": "warn", "DOCKER_HOST": "unix:///var/run/docker.sock",
        })
        self.control("start", "apiserver")
        self.control("start", "controller")
        self.control("start", "agent")

    def health(self, name):
        index = {"apiserver": 0, "controller": 1, "agent": 2}[name]
        url = f"http://127.0.0.1:{self.ports[index]}/" + ("readyz" if name != "agent" else "livez")
        try:
            with urllib.request.urlopen(url, timeout=2) as response:
                return response.status == 200
        except (urllib.error.URLError, TimeoutError):
            return False

    def control(self, action, service):
        if action not in {"start", "stop", "kill"}:
            raise ValueError("unsupported service action")
        if service in {"postgres", "redis"}:
            name = self.context["pg_container"] if service == "postgres" else self.redis_container
            if not self.owned_container(name):
                raise RuntimeError("missing private fixture")
            execute(["docker", "start" if action == "start" else "stop", name], timeout=30)
            if action == "start":
                probe = ["pg_isready", "-U", "acceptance", "-d", "acceptance"] if service == "postgres" else ["redis-cli", "ping"]
                wait_for(lambda: execute(["docker", "exec", name] + probe, check=False).returncode == 0, description=service + " restart")
            return
        binary = {"apiserver": "apiserver", "controller": "controller-manager", "agent": "node-agent"}.get(service)
        if not binary:
            raise ValueError("unsupported process service")
        proc = self.processes.get(service)
        if action in {"stop", "kill"}:
            if proc is not None and proc.poll() is None:
                if os.getpgid(proc.pid) != proc.pid:
                    raise RuntimeError("unexpected owned process group")
                os.killpg(proc.pid, signal.SIGKILL if action == "kill" else signal.SIGTERM)
                try:
                    proc.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    os.killpg(proc.pid, signal.SIGKILL)
                    proc.wait(timeout=5)
            self.manifest()
            return
        if proc is not None and proc.poll() is None:
            return
        log_dir = self.root / "private-logs"
        log_dir.mkdir(mode=0o700, exist_ok=True)
        log = (log_dir / (service + ".log")).open("ab")
        self.log_files.append(log)
        self.processes[service] = subprocess.Popen([str(self.root / "bin" / binary)], env=self.env, cwd=self.root, stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
        self.manifest()
        def started():
            status = self.processes[service].poll()
            if status is not None:
                raise RuntimeError(f"{service} exited with code {status}")
            return self.health(service)
        wait_for(started, seconds=45, description=service + " health")
        print(f"service {service}: ready", flush=True)

    def cleanup(self):
        # Stop all owned callers before the final fallback removes their objects.
        for name in ["agent", "controller", "apiserver"]:
            try:
                self.control("stop", name)
            except Exception as exc:
                self.cleanup_errors.append(self.scrub(str(exc)))
        node_uid = self.context.get("node_uid")
        if node_uid:
            if not re.fullmatch(r"[A-Za-z0-9_-]+", node_uid):
                raise RuntimeError("invalid Node UID for cleanup")
            found = execute(["docker", "ps", "-aq", "--no-trunc", "--filter", "label=gamepanel.io/node-uid=" + node_uid]).stdout.splitlines()
            for container in found:
                if container in self.before:
                    self.cleanup_errors.append("refused to remove a pre-existing container")
                    continue
                labels = json.loads(execute(["docker", "inspect", "--format", "{{json .Config.Labels}}", container]).stdout)
                if labels.get("gamepanel.io/node-uid") == node_uid:
                    self.fallback_workloads.append(container)
                    execute(["docker", "rm", "-f", container], timeout=20)
        for name in reversed(self.containers):
            try:
                if self.owned_container(name):
                    execute(["docker", "rm", "-f", "-v", name], timeout=30)
            except Exception as exc:
                self.cleanup_errors.append(self.scrub(str(exc)))
        for log in self.log_files:
            log.close()
        after = self.snapshot_containers()
        missing = {key: value for key, value in self.before.items() if key not in after}
        if missing:
            self.cleanup_errors.append("pre-existing running containers changed: " + json.dumps(missing))
        # Only data created beneath this exact root is removed; the supervisor,
        # binaries, reports and private diagnostic logs remain for inspection.
        import shutil
        for relative in ["data", "work"]:
            target = (self.root / relative).resolve()
            if target.parent != self.root or target.name != relative:
                raise RuntimeError("unsafe cleanup target")
            if target.exists():
                shutil.rmtree(target)
        self.manifest()


def remote_run(root, git_revision=""):
    os.umask(0o077)
    for key in ["HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"]:
        os.environ.pop(key, None)
    os.environ["NO_PROXY"] = "127.0.0.1,localhost"
    os.environ["no_proxy"] = "127.0.0.1,localhost"
    def interrupted(signum, frame):
        raise RuntimeError(f"acceptance supervisor interrupted by signal {signum}")
    for sig in [signal.SIGINT, signal.SIGTERM, signal.SIGHUP]:
        signal.signal(sig, interrupted)
    started = time.time()
    environment = None
    report = {"started_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), "git_revision": git_revision, "checks": []}
    try:
        environment = Environment(root)
        report.update(run_id=environment.run_id, host=environment.host)
        report["builds"] = {name: hashlib.sha256((environment.root / "bin" / name).read_bytes()).hexdigest() for name in BINARIES}
        report["scripts"] = {name: hashlib.sha256((environment.root / name).read_bytes()).hexdigest() for name in ["run.py", "scenarios.py"]}
        environment.prepare()
        import scenarios
        result = scenarios.run(environment.context, environment.control)
        if not isinstance(result, dict) or not isinstance(result.get("checks"), list):
            raise RuntimeError("scenario module did not return a check list")
        report.update(result)
    except Exception as exc:
        message = environment.scrub(str(exc)) if environment else str(exc)
        report["checks"].append({"name": "acceptance execution", "status": "blocked", "evidence": message})
        print("acceptance execution blocked: " + message, flush=True)
    finally:
        if environment:
            try:
                environment.cleanup()
            except Exception as exc:
                environment.cleanup_errors.append(environment.scrub(str(exc)))
            report["cleanup"] = {"passed": not environment.cleanup_errors, "errors": environment.cleanup_errors, "fallback_removed_workloads": environment.fallback_workloads}
        report["duration_seconds"] = round(time.time() - started, 3)
        report["summary"] = {status: sum(item.get("status") == status for item in report["checks"]) for status in ["passed", "failed", "blocked"]}
        root = root.resolve()
        if root.parent != REMOTE_PARENT or not RUN_PATTERN.fullmatch(root.name):
            raise RuntimeError("invalid report root")
        encoded = json.dumps(report, ensure_ascii=False, indent=2)
        if environment:
            encoded = environment.scrub(encoded)
        (root / "report.json").write_text(encoded)
        lines = ["# Real Docker acceptance", "", "Run: " + root.name, "", "Host: " + str(report.get("host", {}).get("hostname", "unavailable")), "", "| Check | Result |", "| --- | --- |"]
        for item in report["checks"]:
            lines.append("| " + str(item.get("name", "")).replace("|", "\\|") + " | " + str(item.get("status", "")) + " |")
        lines += ["", "Selected evidence and exact observations are in report.json.", "", "Cleanup: " + json.dumps(report.get("cleanup", {}), ensure_ascii=False), ""]
        (root / "report.md").write_text("\n".join(lines))
        print("Acceptance summary: " + json.dumps(report["summary"]), flush=True)
    return 0 if report["checks"] and not report["summary"]["failed"] and not report["summary"]["blocked"] and report.get("cleanup", {}).get("passed") else 2


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default="um773")
    parser.add_argument("--ipv6", help="registered alternative address; reuses the configured host's pinned key")
    parser.add_argument("--binary-dir", help="prebuilt Linux amd64 binaries")
    parser.add_argument("--output", help="new local directory for redacted evidence")
    parser.add_argument("--remote", type=Path, help=argparse.SUPPRESS)
    parser.add_argument("--git-revision", default="", help=argparse.SUPPRESS)
    args = parser.parse_args()
    return remote_run(args.remote, args.git_revision) if args.remote else local_run(args)


if __name__ == "__main__":
    raise SystemExit(main())
