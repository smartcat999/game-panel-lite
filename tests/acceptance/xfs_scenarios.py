"""Real Linux 6.8 XFS (reflink=1 + prjquota) & FICLONE CoW acceptance + benchmark scenarios."""
from __future__ import annotations

import concurrent.futures
import errno
import fcntl
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import time

import scenarios

CORE = scenarios.CORE
REGISTRY = "/apis/registry.gamepanel.io/v1"
MIB = scenarios.MIB
SMALL = scenarios.SMALL
LABEL = scenarios.LABEL
FICLONE_IOCTL = 0x40049409  # _IOW(0x94, 9, int)
EXTENT_RE = re.compile(
    r"^\s*\d+:\s*(\d+)\.\.\s*(\d+):\s*(\d+)\.\.\s*(\d+):\s*(\d+):\s*(?:(\d+):)?\s*(.*)$"
)


def parse_filefrag(path: Path) -> list[dict]:
    proc = subprocess.run(
        ["/usr/sbin/filefrag", "-v", str(path)],
        capture_output=True, text=True, timeout=15, check=True,
    )
    extents = []
    for line in proc.stdout.splitlines():
        m = EXTENT_RE.match(line)
        if m:
            flags = [f.strip() for f in m.group(7).split(",") if f.strip()]
            extents.append({
                "logical": f"{m.group(1)}..{m.group(2)}",
                "physical": f"{m.group(3)}..{m.group(4)}",
                "length_blocks": int(m.group(5)),
                "flags": flags,
            })
    return extents


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        while chunk := f.read(1024 * 1024):
            h.update(chunk)
    return h.hexdigest()


def build_tar_gz(files: dict[str, bytes]) -> bytes:
    buf = io.BytesIO()
    with gzip.GzipFile(fileobj=buf, mode="wb", mtime=0) as gz:
        with tarfile.open(fileobj=gz, mode="w") as tf:
            for name, content in files.items():
                info = tarfile.TarInfo(name=name)
                info.size = len(content)
                info.mode = 0o644
                tf.addfile(info, io.BytesIO(content))
    return buf.getvalue()


class XFSAcceptance(scenarios.Acceptance):
    def __init__(self, context, control):
        super().__init__(context, control)
        self.xfs_mount = Path(context["xfs_mount"])
        self.ext4_work = Path(context["ext4_work_dir"])
        self.ext4_node_name = scenarios._segment(context["ext4_node_name"])
        self.registry_key = hashlib.sha256(str(self.xfs_mount).encode()).hexdigest() + ".json"

    def xfs_cmd(self, subcmd: str) -> str:
        env = dict(os.environ, LC_ALL="C", LANG="C")
        proc = subprocess.run(
            ["/usr/sbin/xfs_quota", "-x", "-D", "/dev/null", "-P", "/dev/null", "-c", subcmd, str(self.xfs_mount)],
            capture_output=True, text=True, timeout=15, check=True, env=env,
        )
        return proc.stdout.strip()

    def load_project_registry(self) -> dict:
        path = self.work_dir / ".volume-quotas" / self.registry_key
        if not path.is_file():
            return {}
        return json.loads(path.read_text())

    def project_id_for(self, uid: str) -> int | None:
        reg = self.load_project_registry()
        for pid, rec in (reg.get("Projects") or {}).items():
            if (rec.get("Intent") or {}).get("InstanceUID") == uid:
                return int(pid)
        return None

    def dump_project_hard_bytes(self, pid: int) -> int:
        out = self.xfs_cmd(f"dump -p -L {pid} -U {pid}")
        lines = [ln.strip() for ln in out.splitlines() if ln.strip()]
        if not lines:
            return 0
        fields = lines[-1].split()
        if len(fields) >= 3 and int(fields[0]) == pid:
            return int(fields[2]) * 512
        raise scenarios.ScenarioError("Unexpected xfs_quota dump output", {"pid": pid, "output": out})

    def report_project_used_bytes(self, pid: int) -> int:
        out = self.xfs_cmd(f"report -p -b -n -N -L {pid} -U {pid}")
        lines = [ln.strip() for ln in out.splitlines() if ln.strip()]
        if not lines:
            return 0
        fields = lines[0].split()
        if len(fields) >= 4 and fields[0] == f"#{pid}":
            return int(fields[1]) * 1024
        raise scenarios.ScenarioError("Unexpected xfs_quota report output", {"pid": pid, "output": out})

    def bootstrap_xfs(self):
        session = self.http("POST", scenarios.IAM + "/sessions", {
            "apiVersion": "iam.gamepanel.io/v1", "kind": "User",
            "identifier": self.context["admin_user"], "password": self.context["admin_password"],
        })
        self.auth = session.get("token")
        if not self.auth:
            raise scenarios.ScenarioError("Login did not issue a session token")
        node = self.wait(
            "XFS Ready node", lambda: self.http("GET", CORE + "/nodes/" + self.node_name),
            lambda n: n.get("status", {}).get("phase") == "Ready"
            and any(r.get("handler") == "docker" for r in n.get("status", {}).get("runtimes", [])),
        )
        self.node_uid = scenarios._segment(node["metadata"]["uid"])
        self.context["node_uid"] = self.node_uid
        self.tenant = self.prefix + "-xfs-tenant"
        tenant = self.http("POST", CORE + "/tenants", {
            "apiVersion": "core.gamepanel.io/v1", "kind": "Tenant",
            "metadata": {"name": self.tenant, "labels": {LABEL: self.nonce}},
            "spec": {
                "type": "Organization", "displayName": self.tenant, "owner": self.context["admin_user"],
                "quota": {"cpu": "4", "memory": "1024Mi", "storage": "512Mi", "instances": "8", "backups": "2", "backupStorage": "32Mi"},
            },
        })
        self.tenant_uid = scenarios._segment(tenant["metadata"]["uid"])
        self.runtime_class = self.prefix + "-docker-xfs"
        self.http("POST", CORE + "/runtimeclasses", {
            "apiVersion": "core.gamepanel.io/v1", "kind": "RuntimeClass",
            "metadata": {"name": self.runtime_class, "labels": {LABEL: self.nonce}},
            "handler": "docker",
            "parameters": {
                "pidsLimit": "64",
                "storageQuotaProvider": "xfs-project-quota",
                "storageQuotaMount": str(self.xfs_mount),
                "storageQuotaProjectIDMin": "20001",
                "storageQuotaProjectIDMax": "20002",
            },
            "scheduling": {"nodeSelector": {"kubernetes.io/hostname": self.node_name}},
        })
        info = subprocess.run(["/usr/sbin/xfs_info", str(self.xfs_mount)], capture_output=True, text=True, check=True).stdout
        state = self.xfs_cmd("state -p")
        ok = ("reflink=1" in info and "Accounting: ON" in state and "Enforcement: ON" in state)
        self.record("xfs.mount_and_provider_ready", ok, {
            "xfs_mount": str(self.xfs_mount), "node_uid": self.node_uid,
            "runtime_class": self.runtime_class, "pool": "20001..20002",
            "reflink_enabled": "reflink=1" in info, "state_summary": state.splitlines()[:4],
        })

    def create_xfs_instance(self, suffix: str, *, runtime_class=None, storage="32Mi", server_artifact=None, addons=None):
        name = self.prefix + "-" + suffix
        res = {"cpu": "250m", "memory": "64Mi", "storage": storage}
        spec = {
            "runtimeClassName": runtime_class or self.runtime_class,
            "state": "Running",
            "image": self.context.get("workload_image", "python:3.12-alpine"),
            "command": ["python", "-u", "-c", scenarios.WORKLOAD],
            "args": [self.nonce],
            "resources": {"requests": res, "limits": dict(res)},
            "ports": [{"name": "http", "protocol": "TCP", "targetPort": 8080, "publishedPort": 0}],
        }
        if server_artifact:
            spec["serverArtifact"] = server_artifact
        if addons:
            spec["addons"] = addons
        obj = self.http("POST", CORE + "/tenants/" + self.tenant + "/instances", {
            "apiVersion": "core.gamepanel.io/v1", "kind": "Instance",
            "metadata": {"name": name, "tenant": self.tenant, "labels": {LABEL: self.nonce}},
            "spec": spec,
        })
        uid = scenarios._segment(obj["metadata"]["uid"])
        work_base = self.ext4_work if runtime_class != self.runtime_class and runtime_class is not None else self.work_dir
        self.records[uid] = {
            "name": name, "uid": uid, "creation": scenarios._identity(obj),
            "recovered_create_ack": False, "work_base": work_base,
        }
        return uid, obj

    def inspect(self, uid):
        if uid not in self.records:
            raise scenarios.ScenarioError("Refusing to inspect an unrecorded instance UID")
        raw = self.command(["inspect", "--format", scenarios.INSPECT_FORMAT, "gp-" + scenarios._segment(uid)], missing=True)
        if raw is None:
            return None
        item = json.loads(raw)
        allowed_nodes = {self.node_uid}
        if self.context.get("ext4_node_uid"):
            allowed_nodes.add(self.context["ext4_node_uid"])
        if item.get("uid") != uid or item.get("node_uid") not in allowed_nodes:
            raise scenarios.ScenarioError("Container ownership differs from this fixture", {"uid": uid})
        return item

    def deleted_sample(self, uid):
        snapshot = self.ledger()
        bound_attempt = self.records[uid].get("bound_attempt_id")
        closed = [
            r for r in snapshot["quota"]
            if r["instance_uid"] == uid and r["state"] == "Closed"
            and not r["allocation_id"].startswith("quota:backup:")
        ]
        closed += [r for r in snapshot["node"] if r["allocation_id"] == bound_attempt and r["state"] == "Closed"]
        obj, item = self.get(uid, absent=True), self.inspect(uid)
        work_base = self.records[uid].get("work_base", self.work_dir)
        workdir_exists = os.path.lexists(work_base / "instances" / scenarios._segment(uid))
        facts = {"uid": uid, "api_absent": obj is None, "container_absent": item is None,
                 "workdir_absent": not workdir_exists, "closed": bool(closed)}
        if (closed or obj is None) and (item is not None or workdir_exists):
            self.deletion_violations[uid] = {**facts, "closed_allocations": [r["allocation_id"] for r in closed]}
        return facts

    def case_edquot_and_pool_saturation(self):
        uid1, _ = self.create_xfs_instance("quota-1")
        uid2, _ = self.create_xfs_instance("quota-2")
        self.wait_resource(uid1, timeout=60)
        self.wait_resource(uid2, timeout=60)
        pid1, pid2 = self.project_id_for(uid1), self.project_id_for(uid2)
        hard1 = self.dump_project_hard_bytes(pid1) if pid1 else 0
        edquot_res = self.exec(uid1, """import errno,json,os
chunk=b'\\x5a'*1048576
written=0; got_errno=0; err_msg=''
fd=os.open('/data/fill.bin', os.O_CREAT|os.O_TRUNC|os.O_WRONLY, 0o644)
try:
 for _ in range(40):
  n=os.write(fd, chunk)
  written+=n
 os.fsync(fd)
except OSError as exc:
 got_errno=exc.errno or 0; err_msg=str(exc)
finally:
 try: os.fsync(fd)
 except OSError: pass
 os.close(fd)
print(json.dumps({'written_bytes':written,'errno':got_errno,'errno_name':errno.errorcode.get(got_errno,''),'quota_enforced':got_errno in (errno.EDQUOT,errno.ENOSPC),'error':err_msg}))
""", timeout=30)
        used_after = self.report_project_used_bytes(pid1)
        ok_stable, latest1 = self.observe_for(
            2.5, lambda: self.get(uid1),
            lambda o: o["status"]["phase"] == "Running",
        )
        t_small = self.http("POST", self.path(uid1) + "/files", {"action": "write", "path": "small-at-full.bin"})
        full_status, _, _ = self.raw_http(
            "PUT", t_small["directURL"], body=b"\x00" * 4096, token="",
            headers={"Content-Type": "application/octet-stream"}, timeout=15,
        )
        t_resp = self.http("POST", self.path(uid1) + "/files", {"action": "write", "path": "oversize.bin"})
        up_status, _, _ = self.raw_http(
            "PUT", t_resp["directURL"], body=b"\x00" * (32 * MIB + 1024), token="",
            headers={"Content-Type": "application/octet-stream"}, timeout=30,
        )
        self.record("xfs.kernel_edquot_and_full_quota_stable",
                    bool(pid1 and pid2 and pid1 != pid2 and hard1 == 32 * MIB
                         and edquot_res.get("quota_enforced") and edquot_res.get("written_bytes") == 32 * MIB
                         and used_after <= 32 * MIB and ok_stable
                         and full_status == 507 and up_status == 413),
                    {"pid1": pid1, "pid2": pid2, "bhard_bytes": hard1, "used_bytes": used_after,
                     "container_write": edquot_res, "phase_at_full_quota": latest1["status"]["phase"],
                     "file_ticket_at_full_status": full_status,
                     "file_ticket_oversize_status": up_status})

        uid3, _ = self.create_xfs_instance("pool-exhaust")
        failed3 = self.wait(
            "3rd XFS instance fails on exhausted 2-ID pool", lambda: self.get(uid3),
            lambda o: o.get("status", {}).get("phase") == "Failed", timeout=45,
        )
        ready_msg = next((c.get("message", "") for c in failed3.get("status", {}).get("conditions", []) if c.get("type") == "Ready"), "")
        self.record("xfs.project_pool_exhaustion_fail_closed",
                    "no free XFS project IDs" in ready_msg,
                    {"uid3": uid3, "phase": failed3["status"]["phase"], "ready_message": ready_msg})
        self.delete(uid3)
        self.wait_deleted(uid3)
        return uid1, uid2, pid1, pid2

    def case_shrink_withheld_and_settled(self, uid1: str, pid1: int):
        self.exec(uid1, """import json,os,pathlib
pathlib.Path('/data/fill.bin').unlink(missing_ok=True)
chunk=b'\\x4b'*1048576
fd=os.open('/data/keep20m.bin', os.O_CREAT|os.O_TRUNC|os.O_WRONLY, 0o644)
for _ in range(20):
 n=0
 while n<len(chunk): n+=os.write(fd, chunk[n:])
os.fsync(fd); os.close(fd)
print(json.dumps({'size':os.path.getsize('/data/keep20m.bin')}))""")
        target = {"cpu": "250m", "memory": "64Mi", "storage": "16Mi"}
        intent = self.update_resources(uid1, target)
        desired = scenarios._rc(intent)["desiredRevision"]
        change_id = (scenarios._rc(intent).get("change") or {}).get("id")
        self.wait("XFS storage shrink approved", lambda: self.get(uid1),
                  lambda o: scenarios._rc(o).get("approvedRevision") == desired
                  and (scenarios._rc(o).get("change") or {}).get("phase") == "Applying")
        ok_held, _ = self.observe_for(
            2.5, lambda: self.get(uid1),
            lambda o: scenarios._rc(o).get("observedRevision", 0) < desired,
        )
        early_hard = self.dump_project_hard_bytes(pid1)
        early_used = self.report_project_used_bytes(pid1)
        snap = self.ledger()
        early_settles = [o for d in ("quota", "node") for o in snap[d + "_ops"]
                         if o["change_plan_id"] == change_id and o["kind"] == "SettleDecrease" and o["state"] == "Committed"]
        self.record("xfs.shrink_withheld_while_charged_above_target",
                    ok_held and early_hard == 32 * MIB and early_used >= 20 * MIB and not early_settles,
                    {"pid": pid1, "xfs_hard_bytes": early_hard, "xfs_used_bytes": early_used, "early_settles": early_settles})

        self.exec(uid1, "import json,pathlib; pathlib.Path('/data/keep20m.bin').unlink(missing_ok=True); print(json.dumps({'removed':True}))")
        self.wait_resource(uid1, revision=desired, timeout=60)
        new_hard = self.dump_project_hard_bytes(pid1)
        settled = self.ledger()
        settles = [o for d in ("quota", "node") for o in settled[d + "_ops"]
                   if o["change_plan_id"] == change_id and o["kind"] == "SettleDecrease" and o["state"] == "Committed"]
        self.record("xfs.shrink_settled_on_disk_after_reclaim",
                    new_hard == 16 * MIB and len(settles) == 2,
                    {"pid": pid1, "xfs_hard_bytes": new_hard, "committed_settles": len(settles)})
        self.conservation("xfs.shrink_account_conservation", settled)

    def case_delete_reclaim_and_reuse(self, uid1: str, uid2: str, pid1: int, pid2: int):
        self.delete(uid1)
        self.delete(uid2)
        self.wait_deleted(uid1, timeout=60)
        self.wait_deleted(uid2, timeout=60)
        reg = self.load_project_registry()
        hard1, used1 = self.dump_project_hard_bytes(pid1), self.report_project_used_bytes(pid1)
        hard2, used2 = self.dump_project_hard_bytes(pid2), self.report_project_used_bytes(pid2)
        self.record("xfs.delete_reclaims_kernel_quota_and_pool",
                    (reg.get("Projects") or {}) == {} and hard1 == 0 and used1 == 0 and hard2 == 0 and used2 == 0,
                    {"remaining_projects": reg.get("Projects"),
                     "pid1": {"id": pid1, "hard": hard1, "used": used1},
                     "pid2": {"id": pid2, "hard": hard2, "used": used2}})
        uid4, _ = self.create_xfs_instance("reclaim-reuse", storage="64Mi")
        self.wait_resource(uid4, timeout=60)
        pid4 = self.project_id_for(uid4)
        self.record("xfs.reclaimed_project_id_reused",
                    pid4 in (20001, 20002),
                    {"reused_uid": uid4, "allocated_project_id": pid4})
        self.delete(uid4)
        self.wait_deleted(uid4, timeout=60)

    def upload_artifact(self, repo: str, pkg: str, ver: str, file_name: str, files: dict[str, bytes]):
        payload = build_tar_gz(files)
        digest = "sha256:" + hashlib.sha256(payload).hexdigest()
        up = self.http("POST", REGISTRY + "/blobuploads", {
            "digest": digest, "sizeBytes": len(payload), "mediaType": "application/gzip", "fileName": file_name,
        })
        st, _, _ = self.raw_http("PUT", up["presignedURL"], body=payload, token="",
                                 headers={"Content-Type": "application/octet-stream"}, timeout=60)
        if not 200 <= st < 300:
            raise scenarios.ScenarioError("S3 presigned blob upload failed", {"status": st, "package": pkg})
        self.http("PUT", REGISTRY + "/blobuploads/" + up["metadata"]["name"], {"digest": digest})
        self.http("POST", REGISTRY + "/artifacts", {
            "apiVersion": "registry.gamepanel.io/v1", "kind": "Artifact",
            "spec": {
                "repository": repo, "package": pkg, "version": ver, "archiveFormat": "tar.gz",
                "descriptor": {"digest": digest, "sizeBytes": len(payload), "archiveFormat": "tar.gz", "fileName": file_name},
            },
        })
        return hashlib.sha256(payload).hexdigest(), digest.split(":", 1)[1]

    def case_ficlone_and_fail_closed(self):
        repo = self.prefix + "-repo"
        self.http("POST", REGISTRY + "/repositories", {
            "apiVersion": "registry.gamepanel.io/v1", "kind": "Repository",
            "metadata": {"name": repo},
            "spec": {"displayName": repo, "mode": "Hosted", "format": "generic", "publicRead": True},
        })
        server_bytes = os.urandom(20 * MIB)
        mod_bytes = os.urandom(4 * MIB)
        server_sha = hashlib.sha256(server_bytes).hexdigest()
        _, server_blob_hex = self.upload_artifact(repo, "srv", "1.0.0", "srv.tar.gz", {"server.bin": server_bytes})
        _, _ = self.upload_artifact(repo, "mod", "1.0.0", "mod.tar.gz", {"mod.bin": mod_bytes})

        s_ref = {"repository": repo, "package": "srv", "version": "1.0.0"}
        addons = [{"repository": repo, "package": "mod", "version": "1.0.0", "installMode": "flat_dir", "targetDir": "addons"}]
        stat_before = os.statvfs(str(self.xfs_mount))
        u_a, _ = self.create_xfs_instance("cow-a", storage="64Mi", server_artifact=s_ref, addons=addons)
        u_b, _ = self.create_xfs_instance("cow-b", storage="64Mi", server_artifact=s_ref, addons=addons)
        self.wait_resource(u_a, timeout=90)
        self.wait_resource(u_b, timeout=90)
        os.sync()
        stat_after = os.statvfs(str(self.xfs_mount))

        cache_srv = self.work_dir / ".blob-cache" / "extracted" / server_blob_hex / "server.bin"
        a_srv = self.work_dir / "instances" / u_a / "server.bin"
        b_srv = self.work_dir / "instances" / u_b / "server.bin"
        ex_c, ex_a, ex_b = parse_filefrag(cache_srv), parse_filefrag(a_srv), parse_filefrag(b_srv)
        phys_c = [(e["physical"], e["length_blocks"]) for e in ex_c]
        phys_a = [(e["physical"], e["length_blocks"]) for e in ex_a]
        phys_b = [(e["physical"], e["length_blocks"]) for e in ex_b]
        shared_flag_ok = all("shared" in e["flags"] for e in ex_a) and all("shared" in e["flags"] for e in ex_b)
        self.record("cow.blobcache_to_workspaces_shared_extents",
                    phys_c == phys_a == phys_b and shared_flag_ok and bool(ex_a)
                    and sha256_file(a_srv) == server_sha and sha256_file(b_srv) == server_sha,
                    {"cache_extents": ex_c, "instance_a_extents": ex_a, "instance_b_extents": ex_b,
                     "fs_free_delta_bytes": (stat_before.f_bavail - stat_after.f_bavail) * stat_after.f_frsize})

        mut = self.exec(u_a, """import hashlib,json,os
fd=os.open('/data/server.bin', os.O_RDWR)
os.lseek(fd, 0, os.SEEK_SET)
os.write(fd, b'\\xaa'*1048576)
os.fsync(fd); os.close(fd)
h=hashlib.sha256(open('/data/server.bin','rb').read()).hexdigest()
print(json.dumps({'mutated_sha256':h}))""", timeout=20)
        self.record("cow.mutation_isolated_from_cache_and_peer",
                    mut["mutated_sha256"] != server_sha
                    and sha256_file(cache_srv) == server_sha and sha256_file(b_srv) == server_sha
                    and (self.work_dir / "instances" / u_a / "addons" / "mod.bin").is_file(),
                    {"original_sha256": server_sha, "cow_a_after": mut["mutated_sha256"],
                     "cache_after": sha256_file(cache_srv), "cow_b_after": sha256_file(b_srv),
                     "cow_a_extents_after_write": parse_filefrag(a_srv)})

        self.service("start", "ext4_agent")
        ext4_node = self.wait(
            "ext4 Ready node", lambda: self.http("GET", CORE + "/nodes/" + self.ext4_node_name),
            lambda n: n.get("status", {}).get("phase") == "Ready",
        )
        self.context["ext4_node_uid"] = scenarios._segment(ext4_node["metadata"]["uid"])
        ext4_rc = self.prefix + "-docker-ext4-reflink"
        self.http("POST", CORE + "/runtimeclasses", {
            "apiVersion": "core.gamepanel.io/v1", "kind": "RuntimeClass",
            "metadata": {"name": ext4_rc, "labels": {LABEL: self.nonce}},
            "handler": "docker", "parameters": {"pidsLimit": "64", "requireReflink": "true"},
            "scheduling": {"nodeSelector": {"kubernetes.io/hostname": self.ext4_node_name}},
        })
        u_fail, _ = self.create_xfs_instance("ext4-reflink-deny", runtime_class=ext4_rc, storage="64Mi", server_artifact=s_ref)
        failed = self.wait("ext4 requireReflink fails closed", lambda: self.get(u_fail),
                           lambda o: o.get("status", {}).get("phase") == "Failed", timeout=60)
        msg = next((c.get("message", "") for c in failed.get("status", {}).get("conditions", []) if c.get("type") == "Ready"), "")
        leaked = (self.ext4_work / "instances" / u_fail / "server.bin").exists()
        self.record("cow.require_reflink_fail_closed_on_ext4",
                    "reflink" in msg.lower() and not leaked,
                    {"uid": u_fail, "phase": failed["status"]["phase"], "message": msg, "leaked_copy": leaked})
        self.delete(u_fail)
        self.delete(u_a)
        self.delete(u_b)
        self.wait_deleted(u_fail, timeout=60)
        self.wait_deleted(u_a, timeout=60)
        self.wait_deleted(u_b, timeout=60)

    def case_benchmark_and_container_xfs_quota(self):
        bench_dir = self.xfs_mount / "bench"
        bench_dir.mkdir(exist_ok=True)
        src = bench_dir / "src-64m.bin"
        src.write_bytes(os.urandom(64 * MIB))
        os.sync()

        def clone_one(i: int):
            dst = bench_dir / f"cow-{i}.bin"
            s = os.open(src, os.O_RDONLY)
            d = os.open(dst, os.O_CREAT | os.O_TRUNC | os.O_WRONLY, 0o644)
            try:
                fcntl.ioctl(d, FICLONE_IOCTL, s)
            finally:
                os.close(d)
                os.close(s)

        def copy_one(i: int):
            dst = bench_dir / f"copy-{i}.bin"
            with src.open("rb") as rf, dst.open("wb") as wf:
                shutil.copyfileobj(rf, wf, 1024 * 1024)
                wf.flush()
                os.fsync(wf.fileno())

        try:
            st0 = os.statvfs(str(self.xfs_mount))
            t0 = time.perf_counter()
            with concurrent.futures.ThreadPoolExecutor(max_workers=16) as pool:
                list(pool.map(clone_one, range(16)))
            os.sync()
            cow_ms = (time.perf_counter() - t0) * 1000
            st1 = os.statvfs(str(self.xfs_mount))
            cow_delta_bytes = (st0.f_bavail - st1.f_bavail) * st1.f_frsize
            ex0, ex15 = parse_filefrag(bench_dir / "cow-0.bin"), parse_filefrag(bench_dir / "cow-15.bin")
            for i in range(16):
                (bench_dir / f"cow-{i}.bin").unlink(missing_ok=True)
            os.sync()

            st2 = os.statvfs(str(self.xfs_mount))
            t1 = time.perf_counter()
            with concurrent.futures.ThreadPoolExecutor(max_workers=16) as pool:
                list(pool.map(copy_one, range(16)))
            os.sync()
            copy_ms = (time.perf_counter() - t1) * 1000
            st3 = os.statvfs(str(self.xfs_mount))
            copy_delta_bytes = (st2.f_bavail - st3.f_bavail) * st3.f_frsize
            for i in range(16):
                (bench_dir / f"copy-{i}.bin").unlink(missing_ok=True)

            speedup = round(copy_ms / max(cow_ms, 0.01), 2)
            self.record("bench.16x64Mi_ficlone_vs_copy",
                        ex0 == ex15 and cow_delta_bytes < 16 * MIB and copy_delta_bytes >= 900 * MIB,
                        {"logical_total_bytes": 16 * 64 * MIB,
                         "ficlone_ms": round(cow_ms, 2),
                         "ficlone_throughput_gib_per_s": round(1.0 / (cow_ms / 1000.0), 2),
                         "ficlone_physical_delta_bytes": cow_delta_bytes,
                         "copy_ms": round(copy_ms, 2),
                         "copy_throughput_gib_per_s": round(1.0 / (copy_ms / 1000.0), 2),
                         "copy_physical_delta_bytes": copy_delta_bytes,
                         "speedup_x": speedup})
        finally:
            shutil.rmtree(bench_dir, ignore_errors=True)

        src_dev = subprocess.run(
            ["findmnt", "-no", "SOURCE", str(self.xfs_mount)],
            capture_output=True, text=True, timeout=10, check=True,
        ).stdout.strip()
        proc = subprocess.run(
            self.docker_argv + [
                "run", "--rm", "--cap-add", "SYS_ADMIN",
                "--device", f"{src_dev}:{src_dev}",
                "--label", f"io.gamepanel.acceptance={self.context['run_id']}",
                "-v", f"{self.xfs_mount}:{self.xfs_mount}",
                "alpine:3.22", "sh", "-c",
                f"apk add --no-cache -q xfsprogs xfsprogs-extra && test -x /usr/sbin/xfs_quota && LC_ALL=C /usr/sbin/xfs_quota -x -D /dev/null -P /dev/null -c 'state -p' {self.xfs_mount}",
            ],
            capture_output=True, text=True, timeout=120, check=False,
        )
        self.record("xfs.alpine322_container_xfs_quota_sys_admin",
                    proc.returncode == 0 and "Enforcement: ON" in proc.stdout,
                    {"device": src_dev, "exit_code": proc.returncode, "stdout": proc.stdout.strip(), "stderr": proc.stderr.strip()})


def run(context: dict, control) -> dict:
    suite = XFSAcceptance(context, control)
    try:
        suite.bootstrap_xfs()
        pair = suite.case("xfs.edquot_and_pool_saturation", suite.case_edquot_and_pool_saturation)
        if pair:
            uid1, uid2, pid1, pid2 = pair
            suite.case("xfs.shrink_withheld_and_settled", lambda: suite.case_shrink_withheld_and_settled(uid1, pid1))
            suite.case("xfs.delete_reclaim_and_reuse", lambda: suite.case_delete_reclaim_and_reuse(uid1, uid2, pid1, pid2))
        suite.case("cow.artifact_ficlone_and_fail_closed", suite.case_ficlone_and_fail_closed)
        suite.case("bench.xfs_ficlone_vs_copy_and_container", suite.case_benchmark_and_container_xfs_quota)
    finally:
        suite.cleanup()
    return {
        "run_id": context["run_id"],
        "checks": suite.checks,
        "tenant": suite.tenant,
        "tenant_uid": suite.tenant_uid,
        "node_uid": suite.node_uid,
        "elapsed_seconds": round(time.monotonic() - suite.started, 3),
        "passed": all(c["status"] == "passed" for c in suite.checks),
    }
