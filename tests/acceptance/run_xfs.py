#!/usr/bin/env python3
"""Run isolated Linux XFS (reflink=1 + prjquota) acceptance and CoW benchmarks."""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import shlex
import shutil
import signal
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.error
import urllib.request

import run as base_run

RUN_PATTERN = re.compile(r"rc-[0-9]{8}T[0-9]{6}-[0-9a-f]{12}")
REMOTE_PARENT = Path("/tmp/gamepanel-xfs-acceptance")
LABEL = "io.gamepanel.acceptance"
BINARIES = ("apiserver", "controller-manager", "node-agent")
SCRIPTS = ("run.py", "scenarios.py", "run_xfs.py", "xfs_scenarios.py")


def execute(argv, *, check=True, **kwargs):
    return base_run.execute(argv, check=check, **kwargs)


def wait_for(predicate, *, seconds=45, description="condition"):
    return base_run.wait_for(predicate, seconds=seconds, description=description)


def local_run(args):
    source = Path(__file__).resolve().parents[2]
    git_revision = execute(["git", "rev-parse", "HEAD"], cwd=source, check=False).stdout.strip()
    run_id = time.strftime("rc-%Y%m%dT%H%M%S-") + secrets.token_hex(6)
    remote = REMOTE_PARENT / run_id
    output = Path(args.output).resolve() if args.output else source / "tests/acceptance/artifacts" / ("xfs-" + run_id)
    output.mkdir(parents=True, exist_ok=False)
    binaries = Path(args.binary_dir).resolve() if args.binary_dir else Path(tempfile.mkdtemp(prefix="gpc-xfs-binaries-"))
    if not args.binary_dir:
        base_run.local_build(source, binaries)
    for name in BINARIES:
        if not (binaries / name).is_file():
            raise RuntimeError(f"missing binary: {name}")
    prefix = base_run.ssh_prefix(args)
    bootstrap = """import pathlib,sys,tarfile,re,shutil,os
root=pathlib.Path(sys.argv[1]).resolve()
assert root.parent==pathlib.Path('/tmp/gamepanel-xfs-acceptance')
assert re.fullmatch(r'rc-[0-9]{8}T[0-9]{6}-[0-9a-f]{12}',root.name)
os.umask(0o077)
root.mkdir(parents=True,exist_ok=False)
allowed={'run.py','scenarios.py','run_xfs.py','xfs_scenarios.py','bin/apiserver','bin/controller-manager','bin/node-agent'}
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
            for name in SCRIPTS:
                archive.add(Path(__file__).parent / name, arcname=name, recursive=False)
            for name in BINARIES:
                archive.add(binaries / name, arcname="bin/" + name, recursive=False)
        package.seek(0)
        uploaded = subprocess.run(prefix + [shlex.join(["python3", "-c", bootstrap, str(remote)])], stdin=package)
        if uploaded.returncode:
            return uploaded.returncode
    print(f"XFS acceptance host={args.host}, run={run_id}, output={output}", flush=True)
    command = ["python3", "-u", str(remote / "run_xfs.py"), "--remote", str(remote)]
    if git_revision:
        command += ["--git-revision", git_revision]
    result = subprocess.run(prefix + [shlex.join(command)])
    fetch = """import pathlib,sys,tarfile
root=pathlib.Path(sys.argv[1]).resolve()
assert root.parent==pathlib.Path('/tmp/gamepanel-xfs-acceptance')
with tarfile.open(fileobj=sys.stdout.buffer,mode='w|gz') as archive:
 for name in ['report.json','report.md','supervisor.json']:
  p=root/name
  if p.is_file():archive.add(p,arcname=name,recursive=False)
"""
    with tempfile.TemporaryFile() as package:
        transfer = subprocess.run(prefix + [shlex.join(["python3", "-c", fetch, str(remote)])], stdout=package)
        if transfer.returncode:
            raise RuntimeError("could not retrieve remote XFS acceptance evidence")
        package.seek(0)
        with tarfile.open(fileobj=package, mode="r:gz") as archive:
            for member in archive:
                if not member.isfile() or member.name not in {"report.json", "report.md", "supervisor.json"}:
                    raise RuntimeError("unexpected report archive member")
                with archive.extractfile(member) as incoming, (output / member.name).open("wb") as outgoing:
                    outgoing.write(incoming.read())
    print(f"XFS evidence saved to {output}", flush=True)
    return result.returncode


class XFSEnvironment(base_run.Environment):
    def __init__(self, root):
        self.xfs_mounted = False
        self.xfs_img = root.resolve() / "xfs.img"
        self.xfs_mount = root.resolve() / "xfs-mnt"
        self.ext4_work = root.resolve() / "ext4-work"
        super(base_run.Environment, self).__init__()  # bypass base path check below; assign explicitly
        self.root = root.resolve()
        if self.root.parent != REMOTE_PARENT or not RUN_PATTERN.fullmatch(self.root.name):
            raise RuntimeError("invalid XFS acceptance root")
        self.run_id = self.root.name
        os.environ["DOCKER_HOST"] = "unix:///var/run/docker.sock"
        os.environ.pop("DOCKER_CONTEXT", None)
        self.processes = {}
        self.log_files = []
        self.containers = []
        self.cleanup_errors = []
        self.fallback_workloads = []
        self.credentials = [secrets.token_urlsafe(32) for _ in range(5)]
        self.ports = self.reserve_ports(7)
        self.context = {
            "run_id": self.run_id,
            "api_url": f"http://127.0.0.1:{self.ports[0]}",
            "node_name": "accept-" + self.run_id.lower(),
            "work_dir": str(self.xfs_mount / "work"),
            "xfs_mount": str(self.xfs_mount),
            "ext4_node_name": "accept-ext4-" + self.run_id.lower(),
            "ext4_work_dir": str(self.ext4_work),
            "pg_container": "gpc-" + self.run_id + "-postgres",
            "pg_user": "acceptance",
            "pg_db": "acceptance",
            "admin_user": "accept-admin",
            "admin_password": self.credentials[1],
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
    def check_headroom():
        mem = dict(line.split(":", 1) for line in Path("/proc/meminfo").read_text().splitlines())
        if int(mem["MemAvailable"].split()[0]) < 1536 * 1024:
            raise RuntimeError("insufficient free memory for isolated XFS acceptance (requires 1.5 GiB)")
        free = os.statvfs(str(REMOTE_PARENT))
        if free.f_bavail * free.f_frsize < 4 * 1024**3:
            raise RuntimeError("insufficient free disk for isolated XFS acceptance (requires 4 GiB)")

    def setup_xfs_mount(self):
        if os.geteuid() != 0:
            raise RuntimeError("isolated XFS loopback acceptance requires root privileges")
        for binary in ("/usr/sbin/mkfs.xfs", "/usr/sbin/xfs_quota", "/usr/sbin/xfs_info", "/usr/sbin/filefrag"):
            if not Path(binary).is_file():
                raise RuntimeError(f"missing required XFS host tool: {binary}")
        self.xfs_mount.mkdir(mode=0o755, exist_ok=False)
        self.ext4_work.mkdir(mode=0o755, exist_ok=False)
        execute(["truncate", "-s", "12G", str(self.xfs_img)], timeout=10)
        execute(["/usr/sbin/mkfs.xfs", "-q", "-f", "-m", "reflink=1,crc=1", str(self.xfs_img)], timeout=30)
        execute(["mount", "-o", "loop,prjquota", str(self.xfs_img), str(self.xfs_mount)], timeout=20)
        self.xfs_mounted = True
        Path(self.context["work_dir"]).mkdir(mode=0o755, parents=True, exist_ok=True)

    def prepare(self):
        self.setup_xfs_mount()
        super().prepare()

    def health(self, name):
        if name == "ext4_agent":
            url = f"http://127.0.0.1:{self.ports[6]}/livez"
            try:
                with urllib.request.urlopen(url, timeout=2) as response:
                    return response.status == 200
            except (urllib.error.URLError, TimeoutError):
                return False
        return super().health(name)

    def control(self, action, service):
        if service != "ext4_agent":
            return super().control(action, service)
        if action not in {"start", "stop", "kill"}:
            raise ValueError("unsupported service action")
        proc = self.processes.get("ext4_agent")
        if action in {"stop", "kill"}:
            if proc is not None and proc.poll() is None:
                os.killpg(proc.pid, signal.SIGKILL if action == "kill" else signal.SIGTERM)
                proc.wait(timeout=10)
            self.manifest()
            return
        if proc is not None and proc.poll() is None:
            return
        ext4_env = dict(self.env)
        ext4_env.update({
            "GP_AGENT_NODE_NAME": self.context["ext4_node_name"],
            "GP_AGENT_WORK_DIR": self.context["ext4_work_dir"],
            "GP_AGENT_ADDR": f"127.0.0.1:{self.ports[6]}",
        })
        log_dir = self.root / "private-logs"
        log_dir.mkdir(mode=0o700, exist_ok=True)
        log = (log_dir / "ext4_agent.log").open("ab")
        self.log_files.append(log)
        self.processes["ext4_agent"] = subprocess.Popen(
            [str(self.root / "bin" / "node-agent")],
            env=ext4_env,
            cwd=self.root,
            stdout=log,
            stderr=subprocess.STDOUT,
            start_new_session=True,
        )
        self.manifest()
        def started():
            status = self.processes["ext4_agent"].poll()
            if status is not None:
                raise RuntimeError(f"ext4_agent exited with code {status}")
            return self.health("ext4_agent")
        wait_for(started, seconds=45, description="ext4_agent health")
        print("service ext4_agent: ready", flush=True)

    def _cleanup_node_containers(self, node_uid):
        if not node_uid:
            return
        if not re.fullmatch(r"[A-Za-z0-9_-]+", node_uid):
            raise RuntimeError("invalid Node UID for cleanup")
        found = execute(["docker", "ps", "-aq", "--no-trunc", "--filter", "label=gamepanel.io/node-uid=" + node_uid]).stdout.splitlines()
        for container in found:
            if container in self.before:
                self.cleanup_errors.append("refused to remove a pre-existing container")
                continue
            self.fallback_workloads.append(container)
            execute(["docker", "rm", "-f", "-v", container], timeout=20)

    def cleanup(self):
        for name in ("ext4_agent", "agent", "controller", "apiserver"):
            try:
                self.control("stop", name)
            except Exception as exc:
                self.cleanup_errors.append(self.scrub(str(exc)))
        for uid_key in ("node_uid", "ext4_node_uid"):
            self._cleanup_node_containers(self.context.get(uid_key))
        labeled = execute(["docker", "ps", "-aq", "--no-trunc", "--filter", "label=" + LABEL + "=" + self.run_id]).stdout.splitlines()
        for container in labeled:
            if container not in self.before:
                execute(["docker", "rm", "-f", "-v", container], check=False, timeout=20)
        for name in reversed(self.containers):
            try:
                if self.owned_container(name):
                    execute(["docker", "rm", "-f", "-v", name], timeout=30)
            except Exception as exc:
                self.cleanup_errors.append(self.scrub(str(exc)))
        for log in self.log_files:
            log.close()
        if self.xfs_mount.exists():
            mounted = execute(["findmnt", "-n", "-o", "SOURCE", str(self.xfs_mount)], check=False).stdout.strip()
            if mounted or self.xfs_mounted:
                um = execute(["umount", str(self.xfs_mount)], check=False, timeout=30)
                if um.returncode != 0:
                    time.sleep(1.0)
                    um = execute(["umount", str(self.xfs_mount)], check=False, timeout=30)
                    if um.returncode != 0:
                        self.cleanup_errors.append("failed to unmount XFS loopback mount: " + um.stderr.strip())
                leftover = execute(["findmnt", "-n", "-o", "SOURCE", str(self.xfs_mount)], check=False).stdout.strip()
                if leftover:
                    self.cleanup_errors.append("XFS mount still active after umount: " + leftover)
        if self.xfs_img.exists():
            loops = execute(["losetup", "-j", str(self.xfs_img)], check=False).stdout.splitlines()
            for line in loops:
                dev = line.split(":", 1)[0].strip()
                if dev:
                    execute(["losetup", "-d", dev], check=False)
            rem_loops = execute(["losetup", "-j", str(self.xfs_img)], check=False).stdout.strip()
            if rem_loops:
                self.cleanup_errors.append("loop device still attached: " + rem_loops)
        after = self.snapshot_containers()
        missing = {key: value for key, value in self.before.items() if key not in after}
        if missing:
            self.cleanup_errors.append("pre-existing running containers changed: " + json.dumps(missing))
        for relative in ("data", "ext4-work", "xfs-mnt", "xfs.img"):
            target = (self.root / relative).resolve()
            if target.parent != self.root or target.name != relative:
                raise RuntimeError("unsafe cleanup target")
            if target.is_dir():
                if not execute(["findmnt", "-n", str(target)], check=False).stdout.strip():
                    shutil.rmtree(target)
            elif target.exists():
                target.unlink()
        self.manifest()


def remote_run(root, git_revision=""):
    os.umask(0o077)
    for key in ["HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"]:
        os.environ.pop(key, None)
    os.environ["NO_PROXY"] = "127.0.0.1,localhost"
    os.environ["no_proxy"] = "127.0.0.1,localhost"
    def interrupted(signum, frame):
        raise RuntimeError(f"XFS acceptance supervisor interrupted by signal {signum}")
    for sig in [signal.SIGINT, signal.SIGTERM, signal.SIGHUP]:
        signal.signal(sig, interrupted)
    started = time.time()
    environment = None
    report = {"started_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), "git_revision": git_revision, "checks": []}
    try:
        environment = XFSEnvironment(root)
        report.update(run_id=environment.run_id, host=environment.host)
        report["builds"] = {name: hashlib.sha256((environment.root / "bin" / name).read_bytes()).hexdigest() for name in BINARIES}
        report["scripts"] = {name: hashlib.sha256((environment.root / name).read_bytes()).hexdigest() for name in SCRIPTS}
        environment.prepare()
        import xfs_scenarios
        result = xfs_scenarios.run(environment.context, environment.control)
        if not isinstance(result, dict) or not isinstance(result.get("checks"), list):
            raise RuntimeError("xfs_scenarios module did not return a check list")
        report.update(result)
    except Exception as exc:
        message = environment.scrub(str(exc)) if environment else str(exc)
        report["checks"].append({"name": "xfs acceptance execution", "status": "blocked", "evidence": message})
        print("xfs acceptance execution blocked: " + message, flush=True)
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
        lines = ["# Real Linux XFS + FICLONE CoW Acceptance", "", "Run: " + root.name, "", "Host: " + str(report.get("host", {}).get("hostname", "unavailable")), "", "| Check | Result |", "| --- | --- |"]
        for item in report["checks"]:
            lines.append("| " + str(item.get("name", "")).replace("|", "\\|") + " | " + str(item.get("status", "")) + " |")
        lines += ["", "Selected evidence and exact observations are in report.json.", "", "Cleanup: " + json.dumps(report.get("cleanup", {}), ensure_ascii=False), ""]
        (root / "report.md").write_text("\n".join(lines))
        print("XFS Acceptance summary: " + json.dumps(report["summary"]), flush=True)
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
