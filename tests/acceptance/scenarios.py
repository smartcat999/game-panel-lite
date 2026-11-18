"""Real local Docker acceptance scenarios; Python 3.12 standard library only.

The isolated runner owns processes/fixture containers and calls run(context,
control).  This module never starts Docker, changes database state directly, or
cleans containers itself.  Only recorded API-created UIDs may be inspected/exec'd
or deleted through the API.  Failed product semantics remain failed checks.
"""

from __future__ import annotations

import copy
from decimal import Decimal
import errno
import hashlib
import http.client
import io
import json
import os
from pathlib import Path
import re
import secrets
import socket
import subprocess
import time
from urllib.parse import parse_qsl, quote, urlencode, urlsplit, urlunsplit
import zipfile


CORE = "/apis/core.gamepanel.io/v1"
IAM = "/apis/iam.gamepanel.io/v1"
MIB = 1024 * 1024
SMALL = {"cpu": "250m", "memory": "64Mi", "storage": "32Mi"}
GROWN = {"cpu": "500m", "memory": "96Mi", "storage": "32Mi"}
DIMENSIONS = ("cpu", "mem", "storage", "instances", "backups", "backup_storage")
LABEL = "acceptance.gamepanel.io/run"

# The workload is deliberately idle.  No CPU stress, large allocation, or large
# HTTP response is necessary to verify limits and published port execution.
WORKLOAD = r'''
import http.server, socket, sys, threading
payload = ("gp-acceptance:" + sys.argv[1]).encode()
class Echo(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)
    def log_message(self, *args):
        pass
def udp():
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
        sock.bind(("0.0.0.0", 8081))
        while True:
            data, peer = sock.recvfrom(256)
            sock.sendto(data, peer)
threading.Thread(target=udp, daemon=True).start()
http.server.HTTPServer(("0.0.0.0", 8080), Echo).serve_forever()
'''

INSPECT_FIELDS = {
    "id": ".Id", "running": ".State.Running", "state": ".State.Status",
    "exit_code": ".State.ExitCode", "nano_cpus": ".HostConfig.NanoCpus",
    "memory": ".HostConfig.Memory", "memory_swap": ".HostConfig.MemorySwap",
    "pids_limit": ".HostConfig.PidsLimit", "ports": ".HostConfig.PortBindings",
    "storage_opt": '(index .HostConfig "StorageOpt")', "working_dir": ".Config.WorkingDir",
}
INSPECT_FORMAT = "{" + ",".join(
    json.dumps(key) + ":{{json " + value + "}}"
    for key, value in INSPECT_FIELDS.items()
) + ',"uid":{{json (index .Config.Labels "gamepanel.io/instance-uid")}}' + \
    ',"node_uid":{{json (index .Config.Labels "gamepanel.io/node-uid")}}' + \
    ',"attempt_id":{{json (index .Config.Labels "gamepanel.io/attempt-id")}}' + \
    ',"epoch":{{json (index .Config.Labels "gamepanel.io/epoch")}}' + \
    ',"revision":{{json (index .Config.Labels "gamepanel.io/resource-revision")}}' + \
    ',"generation":{{json (index .Config.Labels "gamepanel.io/generation")}}}'


class ScenarioError(Exception):
    """Only locally authored, non-sensitive details are attached to errors."""

    def __init__(self, message, evidence=None):
        super().__init__(message)
        self.evidence = evidence or {}


class HTTPFailure(ScenarioError):
    def __init__(self, method, path, status):
        super().__init__("HTTP request failed", {
            "method": method, "path": path, "http_status": status,
        })
        self.status = status


class Blocked(ScenarioError):
    pass


def _segment(value):
    if not isinstance(value, str) or not re.fullmatch(r"[A-Za-z0-9_.-]+", value):
        raise ScenarioError("Unsafe or missing identity segment")
    return value


def _quantity(value, cpu=False):
    value = str(value)
    if cpu:
        return int(value[:-1]) if value.endswith("m") else int(Decimal(value) * 1000)
    for suffix, multiplier in (("Gi", 1024 ** 3), ("Mi", MIB), ("Ki", 1024)):
        if value.endswith(suffix):
            return int(value[:-len(suffix)]) * multiplier
    return int(value)


def _budget(resources):
    return [_quantity(resources["cpu"], cpu=True), _quantity(resources["memory"]),
            _quantity(resources["storage"]), 1, 0, 0]


def _rc(obj):
    return obj.get("status", {}).get("resourceControl") or {}


def _identity(obj):
    meta, control = obj.get("metadata", {}), _rc(obj)
    attempt = control.get("currentAttempt") or {}
    return {"uid": meta.get("uid"), "resource_version": meta.get("resourceVersion"),
            "generation": meta.get("generation"), "allocation_id": control.get("allocationID"),
            "attempt_id": attempt.get("id"), "node_uid": attempt.get("nodeUID"),
            "epoch": attempt.get("epoch"), "stage": control.get("stage"),
            "desired_revision": control.get("desiredRevision"),
            "approved_revision": control.get("approvedRevision"),
            "observed_revision": control.get("observedRevision"),
            "observed_attempt_id": control.get("observedAttemptID"),
            "observed_epoch": control.get("observedEpoch"),
            "change_phase": (control.get("change") or {}).get("phase"),
            "phase": obj.get("status", {}).get("phase")}


def _stable_identity(obj):
    item = _identity(obj)
    return {key: item[key] for key in ("uid", "allocation_id", "attempt_id", "node_uid", "epoch")}


def _error(exc):
    # Do not stringify unknown exceptions: URLs, headers, subprocess arguments,
    # response bodies, and context may contain credentials.
    if isinstance(exc, ScenarioError):
        return {"error": str(exc), **exc.evidence}
    return {"error_type": type(exc).__name__}


class Acceptance:
    def __init__(self, context, control):
        self.context, self.control = context, control
        self.started = time.monotonic()
        self.end = self.started + 600
        self.checks = []
        self.step_latencies = []
        self.records = {}  # UID -> selected immutable creation facts
        self.deletion_violations = {}
        self.stopped_services = set()
        self.auth = None
        self.tenant = self.tenant_uid = self.runtime_class = None
        self.node_uid = None
        self.nonce = secrets.token_hex(6)
        self.prefix = "acc-" + self.nonce
        self.echo = "gp-acceptance:" + self.nonce
        url = urlsplit(context["api_url"])
        if (url.scheme != "http" or url.hostname != "127.0.0.1" or not url.port
                or url.username or url.password or url.query or url.fragment or url.path not in ("", "/")):
            raise ScenarioError("api_url must be a plain local 127.0.0.1 HTTP endpoint")
        self.port = url.port
        self.node_name = _segment(context["node_name"])
        self.pg_container = _segment(context["pg_container"])
        self.pg_user = _segment(context.get("pg_user", "acceptance"))
        self.pg_db = _segment(context.get("pg_db", "acceptance"))
        self.s3_container = "gpc-" + _segment(context["run_id"]) + "-s3"
        self.s3_bucket = _segment(context.get("s3_bucket", "acceptance-backups"))
        self.work_dir = Path(context["work_dir"])
        if not self.work_dir.is_absolute() or self.work_dir == Path("/"):
            raise ScenarioError("work_dir must be an isolated absolute host directory")
        docker_host = os.environ.get("DOCKER_HOST", "unix:///var/run/docker.sock")
        if not docker_host.startswith("unix:///"):
            raise ScenarioError("Only a local Docker Unix socket is permitted")
        self.docker_argv = ["docker", "--host", docker_host]

    def record(self, name, passed, evidence, duration_ms=None):
        entry = {"name": name, "status": "passed" if passed else "failed", "evidence": evidence}
        if duration_ms is not None:
            entry["duration_ms"] = round(duration_ms, 1)
        self.checks.append(entry)
        print(name + ": " + ("passed" if passed else "failed"), flush=True)

    def case(self, name, function):
        if time.monotonic() > self.end - 70:
            self.checks.append({"name": name, "status": "blocked",
                                "evidence": {"reason": "Overall deadline reserves time for API cleanup"}})
            return None
        t0 = time.monotonic()
        try:
            return function()
        except Exception as exc:
            self.checks.append({"name": name, "status": "blocked" if isinstance(exc, Blocked) else "failed",
                                "evidence": _error(exc)})
            print(name + ": " + self.checks[-1]["status"], flush=True)
            return None
        finally:
            self.step_latencies.append({"case": name, "duration_ms": round((time.monotonic() - t0) * 1000, 1)})

    def raw_http(self, method, url_or_path, body=None, token=None, headers=None, timeout=15):
        is_abs = url_or_path.startswith(("http://", "https://"))
        if is_abs:
            parsed = urlsplit(url_or_path)
            if parsed.scheme != "http" or parsed.hostname != "127.0.0.1" or not parsed.port:
                raise ScenarioError("Acceptance raw_http permits loopback http://127.0.0.1 URLs only")
            host, port = parsed.hostname, parsed.port
            target = (parsed.path or "/") + ("?" + parsed.query if parsed.query else "")
        else:
            host, port, target = "127.0.0.1", self.port, url_or_path
        req_headers = {"Accept": "application/json"}
        if headers:
            req_headers.update(headers)
        if token is not None:
            if token:
                req_headers["Authorization"] = "Bearer " + token
        elif not is_abs and self.auth:
            req_headers["Authorization"] = "Bearer " + self.auth
        if isinstance(body, (bytes, bytearray)):
            payload = bytes(body)
        elif body is not None:
            payload = json.dumps(body).encode()
            req_headers.setdefault("Content-Type", "application/json")
        else:
            payload = None
        conn = http.client.HTTPConnection(host, port, timeout=timeout)
        try:
            if payload is not None and len(payload) > MIB:
                conn.putrequest(method, target)
                for key, value in req_headers.items():
                    conn.putheader(key, value)
                conn.putheader("Content-Length", str(len(payload)))
                conn.endheaders()
                chunk_size = 256 * 1024
                for offset in range(0, len(payload), chunk_size):
                    try:
                        conn.send(payload[offset:offset + chunk_size])
                    except OSError:
                        break
            else:
                conn.request(method, target, payload, req_headers)
            response = conn.getresponse()
            raw = response.read(2 * MIB + 1)
            if len(raw) > 2 * MIB:
                raise ScenarioError("HTTP response exceeded bounded reader")
            return response.status, dict(response.getheaders()), raw
        except (OSError, http.client.HTTPException) as exc:
            raise ScenarioError("Local HTTP transport unavailable",
                                {"error_type": type(exc).__name__, "method": method}) from None
        finally:
            conn.close()

    def http(self, method, path, body=None, absent=False):
        conn = http.client.HTTPConnection("127.0.0.1", self.port, timeout=5)
        headers = {"Accept": "application/json", "Content-Type": "application/json"}
        if self.auth:
            headers["Authorization"] = "Bearer " + self.auth
        try:
            conn.request(method, path, None if body is None else json.dumps(body).encode(), headers)
            response = conn.getresponse()
            raw = response.read(2 * MIB + 1)
            if absent and response.status == 404:
                return None
            if not 200 <= response.status < 300:
                raise HTTPFailure(method, path, response.status)
            if len(raw) > 2 * MIB:
                raise ScenarioError("API response exceeded bounded reader")
            if path in ("/readyz", "/livez"):
                return {"http_status": response.status}
            return json.loads(raw) if raw else {}
        except (OSError, http.client.HTTPException) as exc:
            raise ScenarioError("Local API transport unavailable", {"error_type": type(exc).__name__,
                                                                    "method": method, "path": path}) from None
        finally:
            conn.close()

    def wait(self, description, sample, predicate=bool, timeout=60, cleanup=False):
        deadline = min(time.monotonic() + timeout, self.end if cleanup else self.end - 65)
        last_error = None
        while time.monotonic() < deadline:
            try:
                value = sample()
                if predicate(value):
                    return value
            except (ScenarioError, OSError, ValueError) as exc:
                last_error = _error(exc)
            time.sleep(min(0.25, max(0, deadline - time.monotonic())))
        raise ScenarioError("Deadline waiting for " + description, {"last_error": last_error})

    def observe_for(self, seconds, sample, predicate):
        deadline = min(time.monotonic() + seconds, self.end - 65)
        value, ok = None, True
        while time.monotonic() < deadline:
            value = sample()
            ok = bool(predicate(value)) and ok
            time.sleep(min(0.25, max(0, deadline - time.monotonic())))
        return ok, value

    def service(self, action, service):
        # Mark before stopping so cleanup also retries a start after an unknown outcome.
        if action in ("stop", "kill"):
            self.stopped_services.add(service)
        try:
            self.control(action, service)
        except Exception as exc:
            raise ScenarioError("Runner service control failed", {"action": action, "service": service,
                                                                  "error_type": type(exc).__name__}) from None
        if action == "start":
            self.stopped_services.discard(service)

    def command(self, args, timeout=8, missing=False):
        try:
            proc = subprocess.run(self.docker_argv + args, capture_output=True, text=True,
                                  timeout=timeout, check=False)
        except (OSError, subprocess.TimeoutExpired) as exc:
            raise ScenarioError("Local Docker command unavailable", {"error_type": type(exc).__name__,
                                                                    "operation": args[0]}) from None
        if proc.returncode:
            if missing and ("No such object" in proc.stderr or "No such container" in proc.stderr):
                return None
            raise ScenarioError("Local Docker command failed", {"operation": args[0],
                                                               "return_code": proc.returncode})
        if len(proc.stdout) > 2 * MIB:
            raise ScenarioError("Docker output exceeded bounded evidence size")
        return proc.stdout.strip()

    def inspect(self, uid):
        if uid not in self.records:
            raise ScenarioError("Refusing to inspect an unrecorded instance UID")
        raw = self.command(["inspect", "--format", INSPECT_FORMAT, "gp-" + _segment(uid)], missing=True)
        if raw is None:
            return None
        item = json.loads(raw)
        if item.get("uid") != uid or item.get("node_uid") != self.node_uid:
            raise ScenarioError("Container ownership differs from this fixture", {"uid": uid})
        return item

    def exec(self, uid, script, *args, timeout=8):
        item = self.inspect(uid)
        if not item or not item["running"]:
            raise ScenarioError("Recorded workload container is not running", {"uid": uid})
        raw = self.command(["exec", "gp-" + _segment(uid), "python", "-c", script, *args], timeout=timeout)
        return json.loads(raw)

    def path(self, uid):
        if uid not in self.records:
            raise ScenarioError("Unrecorded instance UID")
        return CORE + "/tenants/" + quote(self.tenant, safe="") + "/instances/" + self.records[uid]["name"]

    def get(self, uid, absent=False):
        obj = self.http("GET", self.path(uid), absent=absent)
        if obj is not None and obj.get("metadata", {}).get("uid") != uid:
            raise ScenarioError("Instance name now identifies a different UID", {"expected_uid": uid})
        if obj is not None and _rc(obj).get("approvedRevision", 0) > 0:
            attempt = _rc(obj).get("currentAttempt") or {}
            if attempt.get("id"):
                self.records[uid]["bound_attempt_id"] = attempt["id"]
        return obj

    def bootstrap(self):
        session = self.http("POST", IAM + "/sessions", {
            "apiVersion": "iam.gamepanel.io/v1", "kind": "User",
            "identifier": self.context["admin_user"], "password": self.context["admin_password"],
        })
        self.auth = session.get("token")
        if not self.auth:
            raise ScenarioError("Login did not issue a session token")
        node_path = CORE + "/nodes/" + self.node_name
        node = self.wait("agent registered Docker Ready node", lambda: self.http("GET", node_path),
                         lambda n: n.get("status", {}).get("phase") == "Ready" and
                         any(r.get("handler") == "docker" for r in n.get("status", {}).get("runtimes", [])) and
                         n.get("metadata", {}).get("labels", {}).get("kubernetes.io/hostname") == self.node_name)
        self.node_uid = _segment(node["metadata"]["uid"])
        self.context["node_uid"] = self.node_uid
        self.tenant = self.prefix + "-tenant"
        tenant = self.http("POST", CORE + "/tenants", {
            "apiVersion": "core.gamepanel.io/v1", "kind": "Tenant",
            "metadata": {"name": self.tenant, "labels": {LABEL: self.nonce}},
            "spec": {"type": "Organization", "displayName": self.tenant,
                     "owner": self.context["admin_user"],
                     "quota": {"cpu": "2", "memory": "384Mi", "storage": "96Mi",
                               "instances": "3", "backups": "2", "backupStorage": "32Mi"}},
        })
        self.tenant_uid = _segment(tenant["metadata"]["uid"])
        self.runtime_class = self.prefix + "-docker"
        self.http("POST", CORE + "/runtimeclasses", {
            "apiVersion": "core.gamepanel.io/v1", "kind": "RuntimeClass",
            "metadata": {"name": self.runtime_class, "labels": {LABEL: self.nonce}},
            "handler": "docker", "parameters": {"pidsLimit": "64"},
            "scheduling": {"nodeSelector": {"kubernetes.io/hostname": self.node_name}},
        })
        self.record("bootstrap.real_session_tenant_runtime_node", True, {
            "tenant": self.tenant, "tenant_uid": self.tenant_uid, "node_uid": self.node_uid,
            "node_phase": node["status"]["phase"], "runtime": "docker", "finite_instances": 3,
        })

    def create(self, suffix, ports=None, resources=None):
        name = self.prefix + "-" + suffix
        res = dict(resources or SMALL)
        payload = {"apiVersion": "core.gamepanel.io/v1", "kind": "Instance",
                   "metadata": {"name": name, "tenant": self.tenant, "labels": {LABEL: self.nonce}},
                   "spec": {"runtimeClassName": self.runtime_class, "state": "Running",
                            "nodeSelector": {"kubernetes.io/hostname": self.node_name},
                            "image": self.context.get("workload_image", "python:3.12-alpine"),
                            "command": ["python", "-u", "-c", WORKLOAD], "args": [self.nonce],
                            "resources": {"requests": res, "limits": dict(res)},
                            "ports": ports if ports is not None else [
                                {"name": "http", "protocol": "TCP", "targetPort": 8080, "publishedPort": 0},
                                {"name": "udp", "protocol": "UDP", "targetPort": 8081, "publishedPort": 0}]}}
        collection = CORE + "/tenants/" + self.tenant + "/instances"
        recovered_ack = False
        try:
            obj = self.http("POST", collection, payload)
        except ScenarioError:
            recovered_ack = True
            # Recover an unknown create acknowledgement by the unique run label,
            # then record its UID. Never retry POST with a replacement identity.
            obj = self.http("GET", collection + "/" + name, absent=True)
            if obj is None or obj.get("metadata", {}).get("labels", {}).get(LABEL) != self.nonce:
                raise ScenarioError("Create acknowledgement unknown; no verified fixture object recovered") from None
        uid = _segment(obj["metadata"]["uid"])
        if obj["metadata"].get("tenant") != self.tenant or obj["metadata"].get("labels", {}).get(LABEL) != self.nonce:
            raise ScenarioError("Create response is outside this fixture")
        self.records[uid] = {"name": name, "uid": uid, "creation": _identity(obj),
                             "recovered_create_ack": recovered_ack}
        return uid, obj

    def wait_resource(self, uid, state="Running", revision=None, timeout=60):
        def sample():
            obj = self.get(uid)
            c, a = _rc(obj), _rc(obj).get("currentAttempt") or {}
            rev = c.get("approvedRevision", 0)
            matched = (c.get("observedRevision") == rev and c.get("observedAttemptID") == a.get("id")
                       and c.get("observedEpoch") == a.get("epoch"))
            ready = (c.get("stage") == "Active" and a.get("phase") == "Bound" and rev > 0
                     and a.get("nodeUID") == self.node_uid and matched and not c.get("change")
                     and (revision is None or rev == revision)
                     and obj.get("spec", {}).get("state") == state
                     and obj.get("status", {}).get("phase") == state
                     and obj.get("status", {}).get("observedGeneration", 0) >= obj["metadata"].get("generation", 1))
            item = self.inspect(uid) if ready else None
            exact = (item is not None and item["running"] == (state == "Running")
                     and item["attempt_id"] == a.get("id") and int(item["epoch"]) == a.get("epoch")
                     and 0 < int(item["revision"]) <= rev
                     and 0 < int(item["generation"]) <= obj["metadata"].get("generation"))
            return (obj, item) if ready and exact else None
        return self.wait("Active approved/observed/changeClosed and real Docker " + state, sample, timeout=timeout)

    def sql(self, query):
        if not query.lstrip().startswith("SELECT "):
            raise ScenarioError("Acceptance SQL is SELECT-only")
        return json.loads(self.command([
            "exec", "-e", "PGOPTIONS=-c default_transaction_read_only=on -c statement_timeout=4000",
            self.pg_container, "psql", "-X", "-q", "-A", "-t", "-v", "ON_ERROR_STOP=1",
            "-h", "/var/run/postgresql", "-U", self.pg_user, "-d", self.pg_db, "-c", query,
        ]))

    def ledger(self):
        tenant, node = _segment(self.tenant_uid), _segment(self.node_uid)
        account_fields = ",".join("allocated_" + d for d in DIMENSIONS)
        current_fields = ",".join("current_" + d for d in DIMENSIONS)
        # All JSON projections explicitly select non-sensitive accounting fields.
        queries = {
            "quota_account": f"SELECT {account_fields} FROM quota_accounts WHERE tenant_uid='{tenant}'",
            "node_account": f"SELECT {account_fields} FROM node_accounts WHERE node_uid='{node}'",
            "quota": f"SELECT allocation_id,instance_uid,state,{current_fields},version,resource_revision,change_plan_id,change_revision FROM quota_allocations WHERE tenant_uid='{tenant}' ORDER BY allocation_id",
            "node": f"SELECT allocation_id,instance_uid,state,{current_fields},version,resource_revision,change_plan_id,change_revision,assigned_ports FROM node_reservations WHERE node_uid='{node}' ORDER BY allocation_id",
            "ports": f"SELECT protocol,published_port,attempt_id FROM node_ports WHERE node_uid='{node}' ORDER BY protocol,published_port",
            "quota_ops": f"SELECT o.operation_id,o.allocation_id,o.kind,o.state,o.change_plan_id,o.command->>'resourceRevision' AS revision FROM quota_operations o JOIN quota_allocations a USING(allocation_id) WHERE a.tenant_uid='{tenant}' ORDER BY o.operation_id",
            "node_ops": f"SELECT o.operation_id,o.allocation_id,o.kind,o.state,o.change_plan_id,o.command->>'resourceRevision' AS revision FROM node_operations o JOIN node_reservations a USING(allocation_id) WHERE a.node_uid='{node}' ORDER BY o.operation_id",
        }
        members = ["'" + key + "',COALESCE((SELECT json_agg(x) FROM (" + query + ") x),'[]'::json)"
                   for key, query in queries.items()]
        return self.sql("SELECT json_build_object(" + ",".join(members) + ")")

    @staticmethod
    def holding(row, account=False):
        return [int(row[("allocated_" if account else "current_") + d]) for d in DIMENSIONS]

    def conservation(self, name, snapshot=None):
        snapshot = snapshot or self.ledger()
        errors = []
        for domain in ("quota", "node"):
            accounts = snapshot[domain + "_account"]
            sums = [sum(self.holding(row)[i] for row in snapshot[domain]) for i in range(6)]
            actual = self.holding(accounts[0], account=True) if len(accounts) == 1 else [0] * 6
            if len(accounts) != 1:
                errors.append(domain + " account is missing or ambiguous")
            if actual != sums:
                errors.append(domain + " account differs from persisted current holdings")
            if any(row["state"] in ("Pending", "Closed") and any(self.holding(row)) for row in snapshot[domain]):
                errors.append(domain + " Pending/Closed holding is nonzero")
            seen = set()
            for op in snapshot[domain + "_ops"]:
                key = (op["allocation_id"], op["kind"], op["change_plan_id"])
                if op["kind"] in ("Grant", "Increase", "SettleDecrease") and key in seen:
                    errors.append(domain + " duplicate grant/change operation")
                seen.add(key)
            for row in snapshot[domain]:
                if row["state"] in ("Active", "Closing") and any(self.holding(row)):
                    grants = [o for o in snapshot[domain + "_ops"] if o["allocation_id"] == row["allocation_id"]
                              and o["kind"] == "Grant" and o["state"] == "Committed"]
                    if len(grants) != 1:
                        errors.append(domain + " current holding does not have exactly one committed Grant")
        active_attempts = {r["allocation_id"]: r for r in snapshot["node"] if r["state"] in ("Active", "Closing")}
        actual_ports = {(p["protocol"], p["published_port"], p["attempt_id"]) for p in snapshot["ports"]}
        assigned_ports = {(p["protocol"], p["publishedPort"], r["allocation_id"])
                          for r in active_attempts.values() for p in (r["assigned_ports"] or [])}
        if actual_ports != assigned_ports:
            errors.append("Node port ownership differs from persisted assigned ports")
        for port in snapshot["ports"]:
            if port["attempt_id"] not in active_attempts:
                errors.append("Port owner has no current node holding")
        self.record(name, not errors, {"errors": errors, "ledger": snapshot})
        return snapshot

    def cgroup(self, uid):
        return self.exec(uid, '''import json,pathlib
root=pathlib.Path('/sys/fs/cgroup')
print(json.dumps({name:(root/name).read_text().strip() if (root/name).is_file() else None
                  for name in ('cpu.max','memory.max','memory.swap.max')}))''')

    def limits(self, uid, resources, name):
        item, group = self.inspect(uid), self.cgroup(uid)
        expected = _budget(resources)
        cpu = str(group.get("cpu.max") or "").split()
        cpu_ok = len(cpu) == 2 and cpu[0] != "max" and int(cpu[0]) * 1000 == expected[0] * int(cpu[1])
        ok = (item["nano_cpus"] == expected[0] * 1_000_000 and item["memory"] == expected[1]
              and cpu_ok and group.get("memory.max") == str(expected[1]))
        self.record(name, ok, {"uid": uid, "expected": resources, "docker": item, "cgroup_v2": group})

    def markers(self, uid, write=False):
        return self.exec(uid, '''import json,pathlib,sys
value=sys.argv[1]; write=sys.argv[2]=='write'
paths={'data':pathlib.Path('/data/acceptance-marker'),'tmp':pathlib.Path('/tmp/acceptance-marker')}
if write:
    for path in paths.values(): path.write_text(value)
print(json.dumps({key:path.is_file() and path.read_text()==value for key,path in paths.items()}))''',
                         self.nonce, "write" if write else "read")

    def probe(self, uid, timeout=10):
        obj = self.get(uid)
        approved = _rc(obj).get("approvedPorts") or []
        endpoint = obj.get("status", {}).get("endpoint", {}).get("ports", [])
        results = []
        for port in approved:
            protocol, published = port["protocol"], port["publishedPort"]
            def one():
                if protocol == "TCP":
                    conn = http.client.HTTPConnection("127.0.0.1", published, timeout=1)
                    try:
                        conn.request("GET", "/acceptance")
                        response = conn.getresponse()
                        return response.status == 200 and response.read(256) == self.echo.encode()
                    finally:
                        conn.close()
                with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
                    sock.settimeout(1)
                    message = self.nonce.encode()
                    sock.sendto(message, ("127.0.0.1", published))
                    return sock.recvfrom(256)[0] == message
            try:
                self.wait("loopback " + protocol + " echo", one, timeout=timeout)
                passed = True
            except ScenarioError:
                passed = False
            results.append({"name": port["name"], "protocol": protocol, "target_port": port["targetPort"],
                            "published_port": published, "echo": passed})
        canonical = lambda rows: sorted((p["name"], p["protocol"], p["targetPort"], p.get("publishedPort", 0)) for p in rows)
        return bool(results) and all(r["echo"] for r in results) and canonical(approved) == canonical(endpoint), {
            "uid": uid, "approved_ports": approved, "endpoint_ports": endpoint, "probes": results}

    def create_running(self):
        uid, intent = self.create("primary")
        initial = _rc(intent)
        if self.records[uid]["recovered_create_ack"]:
            self.checks.append({"name": "create.persisted_intent", "status": "blocked", "evidence": {
                "reason": "Create acknowledgement was recovered; initial response could not be observed",
                "recovered": _identity(intent)}})
        else:
            self.record("create.persisted_intent", bool(initial.get("allocationID")) and
                        initial.get("stage") == "PendingQuota" and initial.get("approvedRevision", 0) == 0 and
                        initial.get("observedRevision", 0) == 0 and not initial.get("currentAttempt"), _identity(intent))
        obj, item = self.wait_resource(uid)
        self.record("create.active_approved_real_running", _stable_identity(intent)["allocation_id"] == _rc(obj)["allocationID"],
                    {"intent": _identity(intent), "active": _identity(obj), "docker": item})
        self.case("create.cgroup_probe", lambda: self.limits(uid, SMALL, "create.real_cpu_memory_cgroups"))
        def echo_check():
            passed, evidence = self.probe(uid)
            self.record("create.approved_tcp_udp_loopback_echo", passed, evidence)
        self.case("create.echo_probe", echo_check)
        self.case("create.write_markers", lambda: self.markers(uid, write=True))
        self.case("create.ledger_probe", lambda: self.conservation("create.account_conservation_and_single_grants"))
        return uid

    def state(self, uid, target):
        for _ in range(8):
            obj = self.get(uid)
            body = {"apiVersion": "core.gamepanel.io/v1", "kind": "InstanceState",
                    "metadata": copy.deepcopy(obj["metadata"]), "state": target}
            try:
                return self.http("PUT", self.path(uid) + "/state", body)
            except HTTPFailure as exc:
                if exc.status != 409:
                    raise
        raise ScenarioError("Fresh UID/RV state updates repeatedly conflicted", {"uid": uid})

    def stop_start(self, uid):
        before, before_docker = self.wait_resource(uid)
        saved = self.ledger()
        self.state(uid, "Stopped")
        stopped, stopped_docker = self.wait_resource(uid, "Stopped")
        stopped_ledger = self.ledger()
        same = lambda a, b: all(a[k] == b[k] for k in ("quota_account", "node_account", "ports"))
        self.record("state.stopped_holds_resources_and_ports", same(saved, stopped_ledger), {
            "before": _identity(before), "stopped": _identity(stopped), "docker": stopped_docker,
            "before_ledger": saved, "stopped_ledger": stopped_ledger})
        self.state(uid, "Running")
        after, after_docker = self.wait_resource(uid)
        self.record("state.container_immutable_id_preserved", before_docker["id"] == stopped_docker["id"] == after_docker["id"],
                    {"before_id": before_docker["id"], "stopped_id": stopped_docker["id"],
                     "after_id": after_docker["id"], "before": _identity(before), "after": _identity(after)})
        markers = self.markers(uid)
        self.record("state.data_marker_preserved", markers["data"], {"uid": uid, "marker_present": markers["data"]})
        self.record("state.writable_tmp_marker_preserved", markers["tmp"], {"uid": uid, "marker_present": markers["tmp"]})
        self.record("state.start_holds_same_binding", _stable_identity(before) == _stable_identity(after),
                    {"before": _stable_identity(before), "after": _stable_identity(after)})
        passed, evidence = self.probe(uid)
        self.record("state.resumed_approved_echo", passed, evidence)
        self.conservation("state.account_conservation")

    def update_resources(self, uid, target):
        for _ in range(8):
            obj = self.get(uid)
            obj["spec"]["resources"] = {"requests": dict(target), "limits": dict(target)}
            try:
                return self.http("PUT", self.path(uid), obj)
            except HTTPFailure as exc:
                if exc.status != 409:
                    raise
        raise ScenarioError("Fresh UID/RV resource updates repeatedly conflicted", {"uid": uid})

    def resize(self, uid, target, name, stopped=False):
        state = "Stopped" if stopped else "Running"
        before, item_before = self.wait_resource(uid, state)
        old = _rc(before)["approvedResources"]
        saved_ports = self.ledger()["ports"]
        self.service("stop", "agent")
        try:
            intent = self.update_resources(uid, target)
            desired = _rc(intent)["desiredRevision"]
            change_id = (_rc(intent).get("change") or {}).get("id")
            approved = self.wait("both-domain resource approval", lambda: self.get(uid),
                                 lambda o: _rc(o).get("approvedRevision") == desired and
                                 (_rc(o).get("change") or {}).get("phase") == "Applying")
            budgeted = self.ledger()
            budget_runtime = self.inspect(uid)
            budget_cgroup = self.cgroup(uid) if not stopped else None
            old_amount, new_amount = _budget(old), _budget(target)
            held = [max(a, b) for a, b in zip(old_amount, new_amount)]
            relevant = [r for domain in ("quota", "node") for r in budgeted[domain]
                        if r["instance_uid"] == uid and r["state"] == "Active"]
            ops = [o for domain in ("quota", "node") for o in budgeted[domain + "_ops"]
                   if o["change_plan_id"] == change_id]
            increases = [o for o in ops if o["kind"] == "Increase" and o["state"] == "Committed"]
            has_growth = any(b > a for a, b in zip(old_amount, new_amount))
            ok = (len(relevant) == 2 and all(self.holding(r) == held for r in relevant)
                  and budget_runtime is not None and budget_runtime["id"] == item_before["id"]
                  and budget_runtime["nano_cpus"] == old_amount[0] * 1_000_000
                  and budget_runtime["memory"] == old_amount[1]
                  and (not has_growth or len(increases) == 2)
                  and _rc(approved).get("observedRevision", 0) < desired
                  and not any(o["kind"] == "SettleDecrease" and o["state"] == "Committed" for o in ops))
            self.record(name + ".both_budgets_before_execution", ok, {
                "before": _identity(before), "intent": _identity(intent), "approved": _identity(approved),
                "expected_current_before_observation": held, "ledger": budgeted,
                "docker_before_execution": budget_runtime, "cgroup_before_execution": budget_cgroup})
        finally:
            self.service("start", "agent")
        after, item_after = self.wait_resource(uid, state, revision=desired)
        settled = self.ledger()
        rows = [r for domain in ("quota", "node") for r in settled[domain]
                if r["instance_uid"] == uid and r["state"] == "Active"]
        settles = [o for domain in ("quota", "node") for o in settled[domain + "_ops"]
                   if o["change_plan_id"] == change_id and o["kind"] == "SettleDecrease" and o["state"] == "Committed"]
        self.record(name + ".matching_observation_settles_both_domains", len(rows) == 2 and
                    all(self.holding(r) == _budget(target) for r in rows) and len(settles) == 2,
                    {"after": _identity(after), "change_id": change_id, "ledger": settled})
        self.record(name + ".ports_retained", saved_ports == settled["ports"], {"before": saved_ports, "after": settled["ports"]})
        self.record(name + ".binding_preserved", _stable_identity(before) == _stable_identity(after),
                    {"before": _stable_identity(before), "after": _stable_identity(after)})
        self.record(name + ".docker_limits", item_after["nano_cpus"] == _budget(target)[0] * 1_000_000 and
                    item_after["memory"] == _budget(target)[1], {"docker": item_after, "expected": target})
        if not stopped:
            self.limits(uid, target, name + ".real_cgroup_limits")
            markers = self.markers(uid)
            self.record(name + ".data_marker_preserved", markers["data"], {"uid": uid, "markers": markers})
        self.conservation(name + ".account_conservation", settled)
        return after

    def stopped_resize(self, uid):
        self.state(uid, "Stopped")
        self.wait_resource(uid, "Stopped")
        self.resize(uid, GROWN, "resize.stopped", stopped=True)
        self.state(uid, "Running")
        self.wait_resource(uid)
        self.limits(uid, GROWN, "resize.stopped.real_cgroup_after_start")
        markers = self.markers(uid)
        self.record("resize.stopped.data_marker_preserved", markers["data"], {"uid": uid, "markers": markers})
        self.resize(uid, SMALL, "resize.after_stopped_shrink")

    def disk(self, uid):
        before, _ = self.wait_resource(uid)
        if _quantity(_rc(before)["approvedResources"]["storage"]) != 32 * MIB:
            raise Blocked("Disk negative case requires the original 32Mi budget")
        ticket_resp = self.http("POST", self.path(uid) + "/files", {"action": "write", "path": "oversize.bin"})
        oversize = b"\x00" * (32 * MIB + 1024)
        up_status, _, _ = self.raw_http(
            "PUT", ticket_resp["directURL"], body=oversize, token="",
            headers={"Content-Type": "application/octet-stream"}, timeout=30,
        )
        self.record("disk.file_ticket_hard_quota_32Mi",
                    up_status == 413 and ticket_resp.get("maxBytes") == 32 * MIB,
                    {"uid": uid, "budget_bytes": 32 * MIB, "ticket_max_bytes": ticket_resp.get("maxBytes"),
                     "oversize_bytes": len(oversize), "http_status": up_status,
                     "quota_provider": "hostpath",
                     "note": "FileTicket PUT enforces HTTP 413 MaxBytes; kernel EDQUOT requires XFS pquota via xfs-project-quota"})
        written = self.exec(uid, '''import json,os,pathlib
path=pathlib.Path('/data/acceptance-20m.bin')
chunk=b'\\x5a'*(1024*1024)
with path.open('wb',buffering=0) as stream:
    for _ in range(20): stream.write(chunk)
    os.fsync(stream.fileno())
stat=path.stat()
print(json.dumps({'size':stat.st_size,'blocks_bytes':stat.st_blocks*512}))''', timeout=20)
        if written["size"] <= 16 * MIB:
            raise Blocked("Existing file is not larger than the 16Mi shrink target", written)
        target = dict(_rc(self.get(uid))["approvedResources"])
        target = {k: target[k] for k in ("cpu", "memory", "storage")}
        target["storage"] = "16Mi"
        intent = self.update_resources(uid, target)
        desired = _rc(intent)["desiredRevision"]
        change_id = (_rc(intent).get("change") or {}).get("id")
        self.wait("storage shrink approved before execution", lambda: self.get(uid),
                  lambda o: _rc(o).get("approvedRevision") == desired and
                  (_rc(o).get("change") or {}).get("phase") == "Applying")
        ok_held, held_obj = self.observe_for(
            2.5, lambda: self.get(uid),
            lambda o: _rc(o).get("observedRevision", 0) < desired and
            (_rc(o).get("change") or {}).get("phase") == "Applying",
        )
        held_snap = self.ledger()
        early_settles = [o for d in ("quota", "node") for o in held_snap[d + "_ops"]
                         if o["change_plan_id"] == change_id and o["kind"] == "SettleDecrease" and o["state"] == "Committed"]
        self.record("disk.shrink_withheld_above_actual_usage",
                    ok_held and not early_settles and written["size"] > 16 * MIB,
                    {"identity": _identity(held_obj), "real_file": written,
                     "target_storage": "16Mi", "early_settles": early_settles})
        # Remove only the known file inside this recorded UID's mounted /data, then verify shrink settles.
        self.exec(uid, "import json,pathlib; pathlib.Path('/data/acceptance-20m.bin').unlink(missing_ok=True); print(json.dumps({'removed':True}))")
        after, _ = self.wait_resource(uid, revision=desired)
        settled = self.ledger()
        rows = [r for d in ("quota", "node") for r in settled[d] if r["instance_uid"] == uid and r["state"] == "Active"]
        settles = [o for d in ("quota", "node") for o in settled[d + "_ops"]
                   if o["change_plan_id"] == change_id and o["kind"] == "SettleDecrease" and o["state"] == "Committed"]
        self.record("disk.shrink_settles_after_reclaim",
                    len(rows) == 2 and all(self.holding(r) == _budget(target) for r in rows) and len(settles) == 2,
                    {"identity": _identity(after), "change_id": change_id, "settled_ledger": settled})
        self.conservation("disk.shrink_account_conservation", settled)
        self.resize(uid, SMALL, "disk.restore_32Mi")

    def _file_ticket(self, uid, action, path):
        return self.http("POST", self.path(uid) + "/files", {"action": action, "path": path})

    def s3_object_exists(self, key):
        url = f"http://127.0.0.1:8888/buckets/{self.s3_bucket}/{key.lstrip('/')}"
        proc = subprocess.run(
            self.docker_argv + ["exec", self.s3_container, "wget", "-Y", "off", "-qO", "/dev/null", url],
            capture_output=True, text=True, timeout=8, check=False,
        )
        return proc.returncode == 0

    def isolation(self, uid):
        tenant_b = self.prefix + "-tenant-b"
        member = self.prefix + "-member"
        member_pw = "M3mb3r!" + self.nonce
        role_name = self.prefix + "-reader"
        bind_name = self.prefix + "-bind-reader"
        rogue_node = self.prefix + "-rogue"
        inst_name = self.records[uid]["name"]
        created_binding = False
        created_role = False
        created_member = False
        created_tenant_b = False
        try:
            self.http("POST", CORE + "/tenants", {
                "apiVersion": "core.gamepanel.io/v1", "kind": "Tenant",
                "metadata": {"name": tenant_b, "labels": {LABEL: self.nonce}},
                "spec": {"type": "Organization", "displayName": tenant_b,
                         "owner": self.context["admin_user"],
                         "quota": {"cpu": "1", "memory": "128Mi", "storage": "32Mi",
                                   "instances": "1", "backups": "0", "backupStorage": "0"}},
            })
            created_tenant_b = True
            self.http("POST", IAM + "/users", {
                "apiVersion": "iam.gamepanel.io/v1", "kind": "User",
                "metadata": {"name": member, "labels": {LABEL: self.nonce}},
                "spec": {"email": member + "@example.invalid", "displayName": member, "password": member_pw},
            })
            created_member = True
            self.http("POST", IAM + "/tenants/" + self.tenant + "/tenantroles", {
                "apiVersion": "iam.gamepanel.io/v1", "kind": "TenantRole",
                "metadata": {"name": role_name, "tenant": self.tenant, "labels": {LABEL: self.nonce}},
                "rules": [{"apiGroups": ["core.gamepanel.io"], "resources": ["instances", "instances/*"], "verbs": ["get", "list"]}],
            })
            created_role = True
            self.http("POST", IAM + "/tenants/" + self.tenant + "/tenantrolebindings", {
                "apiVersion": "iam.gamepanel.io/v1", "kind": "TenantRoleBinding",
                "metadata": {"name": bind_name, "tenant": self.tenant, "labels": {LABEL: self.nonce}},
                "roleRef": {"name": role_name},
                "subjects": [{"kind": "User", "name": member}],
            })
            created_binding = True

            msess = self.http("POST", IAM + "/sessions", {
                "apiVersion": "iam.gamepanel.io/v1", "kind": "User",
                "identifier": member, "password": member_pw,
            })
            member_tok = msess.get("token")
            if not member_tok:
                raise ScenarioError("Member login did not issue a session token")
            # Wait for apiserver's 2s RBAC snapshot cache to pick up the new TenantRoleBinding.
            self.wait("member TenantRoleBinding RBAC convergence",
                      lambda: self.raw_http("GET", CORE + "/tenants/" + self.tenant + "/instances", token=member_tok)[0],
                      lambda st: st == 200, timeout=15)

            t_st, _, t_raw = self.raw_http("GET", CORE + "/tenants", token=member_tok)
            visible_tenants = [item["metadata"]["name"] for item in (json.loads(t_raw).get("items") or [])] if t_st == 200 else []
            del_st, _, _ = self.raw_http("DELETE", self.path(uid), token=member_tok)
            wr_st, _, _ = self.raw_http("POST", self.path(uid) + "/files", {"action": "write", "path": "forbidden.txt"}, token=member_tok)
            cross_st, _, _ = self.raw_http("GET", CORE + "/tenants/" + tenant_b + "/instances", token=member_tok)
            mint_st, _, _ = self.raw_http("POST", IAM + "/sessions", {"apiVersion": "iam.gamepanel.io/v1", "kind": "Node", "identifier": self.node_name}, token=member_tok)
            self.record("security.tenant_rbac_isolation",
                        t_st == 200 and tenant_b not in visible_tenants and self.tenant in visible_tenants
                        and del_st == 403 and wr_st == 403 and cross_st == 403 and mint_st == 403,
                        {"member": member, "tenant_b": tenant_b, "tenants_status": t_st,
                         "visible_tenants": visible_tenants, "delete_status": del_st,
                         "write_ticket_status": wr_st, "cross_tenant_status": cross_st,
                         "node_session_mint_status": mint_st})

            nsess = self.http("POST", IAM + "/sessions", {
                "apiVersion": "iam.gamepanel.io/v1", "kind": "Node", "identifier": rogue_node,
            })
            rogue_tok = nsess.get("token")
            if not rogue_tok:
                raise ScenarioError("Admin node session mint did not issue a token")
            nr_node_st, _, _ = self.raw_http(
                "PUT", CORE + "/nodes/" + self.node_name + "/status",
                {"apiVersion": "core.gamepanel.io/v1", "kind": "Node", "metadata": {"name": self.node_name}},
                token=rogue_tok,
            )
            nr_inst_st, _, _ = self.raw_http("GET", self.path(uid), token=rogue_tok)
            nr_user_st, _, _ = self.raw_http("GET", IAM + "/users", token=rogue_tok)
            self.record("security.node_restriction_isolation",
                        nr_node_st == 403 and nr_inst_st == 403 and nr_user_st == 403,
                        {"rogue_node": rogue_node, "other_node_status": nr_node_st,
                         "unscheduled_instance_status": nr_inst_st, "iam_users_status": nr_user_st,
                         "target_instance": inst_name})

            w_tkt = self._file_ticket(uid, "write", "ticket-probe.txt")
            p_st, _, _ = self.raw_http("PUT", w_tkt["directURL"], body=b"ok", token="", headers={"Content-Type": "application/octet-stream"})
            r_tkt = self._file_ticket(uid, "read", "ticket-probe.txt")
            direct = r_tkt["directURL"]
            ok_st, _, ok_raw = self.raw_http("GET", direct, token="")

            sp = urlsplit(direct)
            qs = parse_qsl(sp.query, keep_blank_values=True)
            no_tkt_url = urlunsplit((sp.scheme, sp.netloc, sp.path, urlencode([(k, v) for k, v in qs if k != "ticket"]), ""))
            bad_tkt_qs = [(k, (v[:-1] + ("A" if not v.endswith("A") else "B")) if k == "ticket" else v) for k, v in qs]
            bad_tkt_url = urlunsplit((sp.scheme, sp.netloc, sp.path, urlencode(bad_tkt_qs), ""))
            trav_qs = [(k, "../../etc/passwd" if k == "path" else v) for k, v in qs]
            trav_url = urlunsplit((sp.scheme, sp.netloc, sp.path, urlencode(trav_qs), ""))

            no_tkt_st, _, _ = self.raw_http("GET", no_tkt_url, token="")
            bad_tkt_st, _, _ = self.raw_http("GET", bad_tkt_url, token="")
            wrong_method_st, _, _ = self.raw_http("PUT", direct, body=b"evil", token="")
            api_trav_st, _, _ = self.raw_http("POST", self.path(uid) + "/files", {"action": "read", "path": "../../etc/passwd"})
            direct_trav_st, _, _ = self.raw_http("GET", trav_url, token="")
            d_tkt = self._file_ticket(uid, "delete", "ticket-probe.txt")
            clean_probe_st, _, _ = self.raw_http("DELETE", d_tkt["directURL"], token="")
            self.record("security.file_ticket_isolation",
                        p_st == 200 and ok_st == 200 and ok_raw == b"ok" and no_tkt_st == 401
                        and bad_tkt_st == 401 and wrong_method_st == 403
                        and 400 <= api_trav_st < 500 and direct_trav_st in (400, 403)
                        and clean_probe_st in (200, 204),
                        {"write_status": p_st, "valid_read_status": ok_st, "missing_ticket_status": no_tkt_st,
                         "tampered_ticket_status": bad_tkt_st, "wrong_method_status": wrong_method_st,
                         "api_traversal_status": api_trav_st, "direct_traversal_status": direct_trav_st,
                         "cleanup_status": clean_probe_st})
        finally:
            if created_binding:
                self.http("DELETE", IAM + "/tenants/" + self.tenant + "/tenantrolebindings/" + bind_name, absent=True)
            if created_role:
                self.http("DELETE", IAM + "/tenants/" + self.tenant + "/tenantroles/" + role_name, absent=True)
            if created_member:
                self.http("DELETE", IAM + "/users/" + member, absent=True)
                self.wait("isolation member user and personal tenant deleted",
                          lambda: self.http("GET", IAM + "/users/" + member, absent=True) is None
                          and self.http("GET", CORE + "/tenants/" + member, absent=True) is None,
                          cleanup=True, timeout=30)
            if created_tenant_b:
                self.http("DELETE", CORE + "/tenants/" + tenant_b, absent=True)
                self.wait("isolation tenant_b deleted",
                          lambda: self.http("GET", CORE + "/tenants/" + tenant_b, absent=True) is None,
                          cleanup=True, timeout=30)

    def quota_denial(self):
        saved = self.ledger()
        uid, intent = self.create("over-cpu", resources={"cpu": "2", "memory": "64Mi", "storage": "16Mi"})
        ok, paused = self.observe_for(
            2.5, lambda: self.get(uid),
            lambda o: _rc(o).get("stage") == "PendingQuota" and _rc(o).get("approvedRevision", 0) == 0,
        )
        now = self.ledger()
        own_rows = [r for d in ("quota", "node") for r in now[d] if r["instance_uid"] == uid]
        self.record("capacity.quota_overcommit_denied",
                    ok and self.inspect(uid) is None
                    and all(r["state"] == "Pending" and not any(self.holding(r)) for r in own_rows)
                    and all(saved[k] == now[k] for k in ("quota_account", "node_account", "ports")),
                    {"intent": _identity(intent), "observed": _identity(paused), "own_rows": own_rows})
        self.delete(uid)
        self.wait_deleted(uid)
        self.conservation("capacity.quota_denial_conservation")

    def s3_backup_restore(self, uid):
        unit = b"GP-S3-" + self.nonce.encode() + b"\n"
        payload = (unit * ((256 * 1024 // len(unit)) + 1))[:256 * 1024]
        expected_sha = hashlib.sha256(payload).hexdigest()

        wt = self._file_ticket(uid, "write", "save/world.dat")
        w_st, _, _ = self.raw_http("PUT", wt["directURL"], body=payload, token="", headers={"Content-Type": "application/octet-stream"})
        if w_st != 200:
            raise ScenarioError("Failed to upload test save payload via FileTicket", {"http_status": w_st})

        t_backup = time.monotonic()
        bresp = self.http("POST", self.path(uid) + "/files", {"action": "backup", "path": "pre-upgrade.tar.gz"})
        backup_path = bresp["path"]
        completed = self.wait(
            "S3 backup Completed",
            lambda: next((b for b in (self.get(uid).get("status", {}).get("backups") or [])
                          if b.get("path") == backup_path and b.get("phase") == "Completed"), None),
            timeout=60,
        )
        backup_rto_ms = round((time.monotonic() - t_backup) * 1000, 1)
        bak_ledger = self.ledger()
        dl = self.http("POST", self.path(uid) + "/files", {"action": "download_backup", "path": backup_path})
        dl_st, _, dl_raw = self.raw_http("GET", dl["directURL"], token="")
        dl_sha = hashlib.sha256(dl_raw).hexdigest()
        acct = bak_ledger["quota_account"][0] if bak_ledger["quota_account"] else {}
        self.record("recovery.s3_backup_and_presigned_sha256",
                    dl_st == 200 and completed["sizeBytes"] > 0 and len(dl_raw) == completed["sizeBytes"]
                    and dl_sha == completed.get("sha256") and acct.get("allocated_backups") == 1
                    and acct.get("allocated_backup_storage") == completed["sizeBytes"],
                    {"uid": uid, "backup_path": backup_path, "size_bytes": completed["sizeBytes"],
                     "sha256": completed.get("sha256"), "download_status": dl_st,
                     "quota_account": acct, "backup_rto_ms": backup_rto_ms},
                    duration_ms=backup_rto_ms)

        ct = self._file_ticket(uid, "write", "save/world.dat")
        c_st, _, _ = self.raw_http("PUT", ct["directURL"], body=b"CORRUPTED", token="", headers={"Content-Type": "application/octet-stream"})
        if c_st != 200:
            raise ScenarioError("Failed to write corrupted payload before restore", {"http_status": c_st})

        t_restore = time.monotonic()
        self.http("POST", self.path(uid) + "/files", {"action": "restore", "path": backup_path})
        restored = self.wait(
            "S3 restore Succeeded",
            lambda: next((b for b in (self.get(uid).get("status", {}).get("backups") or [])
                          if b.get("path") == backup_path and b.get("phase") == "Completed"
                          and b.get("lastRestoreResult") == "Succeeded"), None),
            timeout=60,
        )
        self.wait_resource(uid)
        restore_rto_ms = round((time.monotonic() - t_restore) * 1000, 1)
        rt = self._file_ticket(uid, "read", "save/world.dat")
        r_st, _, r_raw = self.raw_http("GET", rt["directURL"], token="")
        probe_ok, probe_ev = self.probe(uid)
        restored_sha = hashlib.sha256(r_raw).hexdigest() if r_st == 200 else ""
        self.record("recovery.s3_restore_zero_loss",
                    r_st == 200 and len(r_raw) == len(payload) and restored_sha == expected_sha and probe_ok,
                    {"uid": uid, "backup_path": backup_path, "expected_bytes": len(payload),
                     "restored_bytes": len(r_raw), "sha256_match": restored_sha == expected_sha,
                     "rpo_bytes_lost": 0 if restored_sha == expected_sha else -1,
                     "last_restore_result": restored.get("lastRestoreResult"),
                     "probe": probe_ev, "restore_rto_ms": restore_rto_ms},
                    duration_ms=restore_rto_ms)

        self.http("POST", self.path(uid) + "/files", {"action": "delete_backup", "path": backup_path})
        self.wait("backup deleted and quota refunded",
                  lambda: (not (self.get(uid).get("status", {}).get("backups") or [])
                           and self.ledger()["quota_account"][0]["allocated_backups"] == 0
                           and self.ledger()["quota_account"][0]["allocated_backup_storage"] == 0),
                  timeout=45)
        dt = self._file_ticket(uid, "delete", "save/world.dat")
        self.raw_http("DELETE", dt["directURL"], token="")
        zero_snap = self.ledger()
        self.record("recovery.backup_delete_refunds_quota",
                    zero_snap["quota_account"][0]["allocated_backups"] == 0
                    and zero_snap["quota_account"][0]["allocated_backup_storage"] == 0,
                    {"quota_account": zero_snap["quota_account"][0]})
        self.conservation("recovery.backup_account_conservation", zero_snap)

    def direct_upload_backup_restore(self, uid):
        unit = b"GP-ZIP-" + self.nonce.encode() + b"\n"
        world_bytes = (unit * ((64 * 1024 // len(unit)) + 1))[:64 * 1024]
        expected_sha = hashlib.sha256(world_bytes).hexdigest()
        props_bytes = ("motd=imported-" + self.nonce + "\n").encode()
        buf = io.BytesIO()
        with zipfile.ZipFile(buf, "w", compression=zipfile.ZIP_DEFLATED) as zf:
            zf.writestr("exports/save/world.dat", world_bytes)
            zf.writestr("exports/save/server.properties", props_bytes)
            zf.writestr("exports/save/.gp-injected/evil.txt", b"SHOULD_BE_STRIPPED\n")
        zip_bytes = buf.getvalue()
        zip_sha = hashlib.sha256(zip_bytes).hexdigest()
        reserve_bytes = len(zip_bytes) + 4096

        t_upload = time.monotonic()
        init_resp = self.http("POST", self.path(uid) + "/files", {
            "action": "upload_backup", "path": "imported-world.zip", "sizeBytes": reserve_bytes,
        })
        zip_key = init_resp["path"]
        put_zip_st, _, _ = self.raw_http(
            "PUT", init_resp["directURL"], body=zip_bytes, token="", headers={"Content-Type": "application/zip"},
        )
        if put_zip_st != 200:
            raise ScenarioError("Failed direct PUT upload of ZIP backup to S3", {"http_status": put_zip_st})
        self.http("POST", self.path(uid) + "/files", {
            "action": "complete_upload_backup", "operationID": init_resp["operationID"], "path": zip_key,
        })
        completed = self.wait(
            "DirectUpload ZIP backup Completed",
            lambda: next((b for b in (self.get(uid).get("status", {}).get("backups") or [])
                          if b.get("path") == zip_key and b.get("phase") == "Completed"), None),
            timeout=60,
        )
        upload_rto_ms = round((time.monotonic() - t_upload) * 1000, 1)
        dl = self.http("POST", self.path(uid) + "/files", {"action": "download_backup", "path": zip_key})
        dl_st, _, dl_raw = self.raw_http("GET", dl["directURL"], token="")
        dl_sha = hashlib.sha256(dl_raw).hexdigest() if dl_st == 200 else ""
        snap = self.ledger()
        acct = snap["quota_account"][0] if snap["quota_account"] else {}
        self.record("recovery.direct_upload_zip_and_quota",
                    dl_st == 200 and len(dl_raw) == len(zip_bytes) and dl_sha == zip_sha
                    and completed.get("imported") is True and completed["sizeBytes"] == len(zip_bytes)
                    and acct.get("allocated_backups") == 1
                    and acct.get("allocated_backup_storage") == len(zip_bytes),
                    {"uid": uid, "path": zip_key, "reserved_bytes": reserve_bytes, "actual_bytes": len(zip_bytes),
                     "zip_sha256": zip_sha, "download_sha256": dl_sha, "completed": completed,
                     "quota_account": acct, "upload_rto_ms": upload_rto_ms},
                    duration_ms=upload_rto_ms)
        self.conservation("recovery.direct_upload_conservation", snap)

        def inspect_once():
            resp = self.http("POST", self.path(uid) + "/files", {
                "action": "inspect_backup", "path": zip_key,
                "sourcePath": "exports/save", "targetPath": "mapped-restore",
            })
            return resp if resp.get("phase") == "Completed" else None

        insp = self.wait("ZIP inspect_backup Completed", inspect_once, timeout=45)
        self.record("recovery.backup_inspect_zip_and_reserved_strip",
                    insp.get("archiveFormat") == "zip"
                    and "exports/save" in (insp.get("archiveDirectories") or [])
                    and insp.get("fileCount") == 2
                    and sorted(insp.get("sourceEntries") or []) == ["server.properties", "world.dat"],
                    {"uid": uid, "path": zip_key, "inspection": insp})

        bad_restore_st, _, _ = self.raw_http("POST", self.path(uid) + "/files", {
            "action": "restore", "path": zip_key, "sourcePath": "exports/save", "targetPath": ".gp-evil",
        })
        bad_read_st, _, _ = self.raw_http("POST", self.path(uid) + "/files", {
            "action": "read", "path": "mapped-restore/.gp-injected/evil.txt",
        })
        st = self._file_ticket(uid, "write", "mapped-restore/stale.txt")
        s_st, _, _ = self.raw_http("PUT", st["directURL"], body=b"STALE", token="", headers={"Content-Type": "application/octet-stream"})
        if s_st != 200:
            raise ScenarioError("Failed to seed stale file before mapped restore", {"http_status": s_st})

        t_mapped = time.monotonic()
        self.http("POST", self.path(uid) + "/files", {
            "action": "restore", "path": zip_key,
            "sourcePath": "exports/save", "targetPath": "mapped-restore", "replaceTarget": True,
        })
        restored = self.wait(
            "Mapped ZIP restore Succeeded",
            lambda: next((b for b in (self.get(uid).get("status", {}).get("backups") or [])
                          if b.get("path") == zip_key and b.get("phase") == "Completed"
                          and b.get("lastRestoreResult") == "Succeeded"), None),
            timeout=60,
        )
        self.wait_resource(uid)
        mapped_rto_ms = round((time.monotonic() - t_mapped) * 1000, 1)
        rt = self._file_ticket(uid, "read", "mapped-restore/world.dat")
        r_st, _, r_raw = self.raw_http("GET", rt["directURL"], token="")
        restored_sha = hashlib.sha256(r_raw).hexdigest() if r_st == 200 else ""
        dir_entries = self.exec(uid, "import json,os; print(json.dumps(sorted(os.listdir('/data/mapped-restore'))))")
        self.record("recovery.mapped_restore_and_reserved_guard",
                    bad_restore_st == 400 and bad_read_st == 400
                    and r_st == 200 and restored_sha == expected_sha
                    and dir_entries == ["server.properties", "world.dat"],
                    {"uid": uid, "bad_restore_status": bad_restore_st, "bad_read_status": bad_read_st,
                     "read_status": r_st, "sha256_match": restored_sha == expected_sha,
                     "dir_entries": dir_entries, "last_restore_result": restored.get("lastRestoreResult"),
                     "mapped_rto_ms": mapped_rto_ms},
                    duration_ms=mapped_rto_ms)

        zip_in_s3 = self.s3_object_exists(zip_key)
        over_init = self.http("POST", self.path(uid) + "/files", {
            "action": "upload_backup", "path": "oversize.zip", "sizeBytes": 1024,
        })
        over_key, over_op = over_init["path"], over_init["operationID"]
        put_over_st, _, _ = self.raw_http(
            "PUT", over_init["directURL"], body=b"X" * 2048, token="", headers={"Content-Type": "application/zip"},
        )
        comp_over_st, _, _ = self.raw_http("POST", self.path(uid) + "/files", {
            "action": "complete_upload_backup", "operationID": over_op, "path": over_key,
        })

        def check_oversize_cleanup():
            if any(b.get("path") == over_key for b in (self.get(uid).get("status", {}).get("backups") or [])):
                return None
            if self.s3_object_exists(over_key):
                return None
            cur = self.ledger()
            qa = cur["quota_account"][0]
            if qa["allocated_backups"] != 1 or qa["allocated_backup_storage"] != len(zip_bytes):
                return None
            return cur

        oversize_snap = self.wait("oversize upload S3 cleanup and quota refund", check_oversize_cleanup, timeout=45)
        existed_after = self.s3_object_exists(over_key)
        dl_over_st, _, _ = self.raw_http("POST", self.path(uid) + "/files", {"action": "download_backup", "path": over_key})
        self.record("recovery.oversize_upload_s3_cleanup_and_refund",
                    zip_in_s3 and put_over_st == 200 and comp_over_st in (200, 409)
                    and not existed_after and dl_over_st == 409,
                    {"uid": uid, "over_key": over_key, "zip_in_s3": zip_in_s3,
                     "put_status": put_over_st, "complete_status": comp_over_st,
                     "existed_after": existed_after, "download_deleted_status": dl_over_st,
                     "quota_account": oversize_snap["quota_account"][0]})

        self.http("POST", self.path(uid) + "/files", {"action": "delete_backup", "path": zip_key})
        self.wait("direct upload backup deleted and quota refunded",
                  lambda: (not (self.get(uid).get("status", {}).get("backups") or [])
                           and self.ledger()["quota_account"][0]["allocated_backups"] == 0
                           and self.ledger()["quota_account"][0]["allocated_backup_storage"] == 0),
                  timeout=45)
        dt = self._file_ticket(uid, "delete", "mapped-restore")
        self.raw_http("DELETE", dt["directURL"], token="")
        self.conservation("recovery.direct_upload_final_conservation")

    def restarts(self, uid, service):
        obj, item = self.wait_resource(uid)
        saved = self.ledger()
        self.service("stop", service)
        try:
            self.service("start", service)
            self.wait("API recovery after " + service, lambda: self.http("GET", "/readyz"), timeout=60)
            after, after_item = self.wait_resource(uid)
        finally:
            if service in self.stopped_services:
                self.service("start", service)
        current = self.ledger()
        same_holding = all(saved[k] == current[k] for k in ("quota_account", "node_account", "ports"))
        self.record("restart." + service + ".original_identity_and_current", _stable_identity(obj) == _stable_identity(after)
                    and item["id"] == after_item["id"] and same_holding,
                    {"before": _identity(obj), "after": _identity(after), "before_id": item["id"],
                     "after_id": after_item["id"], "before_ledger": saved, "after_ledger": current})
        self.conservation("restart." + service + ".single_grants_and_conservation", current)

    def controller_interruption(self):
        saved = self.ledger()
        self.service("stop", "controller")
        try:
            uid, intent = self.create("controller-pause")
            def unallocated(obj):
                c = _rc(obj)
                return c.get("stage") == "PendingQuota" and c.get("approvedRevision", 0) == 0 and not c.get("currentAttempt")
            ok, held = self.observe_for(1.5, lambda: self.get(uid), unallocated)
            paused = self.ledger()
            own_rows = [r for domain in ("quota", "node") for r in paused[domain] if r["instance_uid"] == uid]
            self.record("interruption.controller.intent_without_early_grant", ok and
                        all(not any(self.holding(r)) for r in own_rows) and
                        all(saved[k] == paused[k] for k in ("quota_account", "node_account", "ports")),
                        {"intent": _identity(intent), "paused": _identity(held), "ledger": paused})
        finally:
            self.service("start", "controller")
        active, _ = self.wait_resource(uid)
        self.record("interruption.controller.recovers_original_ids", intent["metadata"]["uid"] == active["metadata"]["uid"]
                    and _rc(intent)["allocationID"] == _rc(active)["allocationID"],
                    {"intent": _identity(intent), "active": _identity(active)})
        self.delete(uid)
        self.wait_deleted(uid)
        self.record("interruption.controller.created_workload_cleaned", True, {"uid": uid})

    def delete(self, uid):
        # The HTTP handler has no delete-preconditions payload; it resolves the
        # current UID internally. Still verify our recorded UID immediately first.
        obj = self.get(uid, absent=True)
        if obj is not None:
            self.http("DELETE", self.path(uid), {"metadata": copy.deepcopy(obj["metadata"])})

    def deleted_sample(self, uid):
        # Read financial closure before physical artifacts: a later artifact
        # observation cannot falsely precede a legitimate earlier Destroy.
        snapshot = self.ledger()
        bound_attempt = self.records[uid].get("bound_attempt_id")
        closed = [
            r for r in snapshot["quota"]
            if r["instance_uid"] == uid and r["state"] == "Closed"
            and not r["allocation_id"].startswith("quota:backup:")
        ]
        closed += [r for r in snapshot["node"] if r["allocation_id"] == bound_attempt and r["state"] == "Closed"]
        obj, item = self.get(uid, absent=True), self.inspect(uid)
        workdir_exists = os.path.lexists(self.work_dir / "instances" / _segment(uid))
        facts = {"uid": uid, "api_absent": obj is None, "container_absent": item is None,
                 "workdir_absent": not workdir_exists}
        if (closed or obj is None) and (item is not None or workdir_exists):
            self.deletion_violations[uid] = {**facts, "closed_allocations": [r["allocation_id"] for r in closed]}
        return facts

    def wait_deleted(self, uid, cleanup=False, timeout=60):
        return self.wait("API404, Docker Destroy and UID workdir removal", lambda: self.deleted_sample(uid),
                         lambda s: s["api_absent"] and s["container_absent"] and s["workdir_absent"],
                         timeout=timeout, cleanup=cleanup)

    def port_contention(self, owner_uid):
        owner, _ = self.wait_resource(owner_uid)
        tcp = next(p for p in _rc(owner)["approvedPorts"] if p["protocol"] == "TCP")
        owner_attempt = _rc(owner)["currentAttempt"]["id"]
        uid, intent = self.create("port-waiter", [{"name": "contended-http", "protocol": "TCP", "targetPort": 8080,
                                                   "publishedPort": tcp["publishedPort"]}])
        pending = self.wait("contended port remains Scheduling", lambda: self.get(uid),
                            lambda o: _rc(o).get("stage") == "Scheduling" and _rc(o).get("approvedRevision", 0) == 0)
        ok, pending = self.observe_for(2, lambda: self.get(uid),
                                      lambda o: _rc(o).get("stage") == "Scheduling" and _rc(o).get("approvedRevision", 0) == 0)
        snapshot = self.ledger()
        owners = [p for p in snapshot["ports"] if p["protocol"] == "TCP" and p["published_port"] == tcp["publishedPort"]]
        self.record("ports.contention_does_not_steal", ok and self.inspect(uid) is None and len(owners) == 1
                    and owners[0]["attempt_id"] == owner_attempt,
                    {"waiting": _identity(pending), "port_owner": owners, "published_port": tcp["publishedPort"]})
        self.delete(owner_uid)
        cleanup = self.wait_deleted(owner_uid)
        snapshot = self.ledger()
        old_rows = [r for domain in ("quota", "node") for r in snapshot[domain] if r["instance_uid"] == owner_uid]
        self.record("ports.owner_real_cleanup_before_closed", all(r["state"] == "Closed" and not any(self.holding(r)) for r in old_rows)
                    and bool(old_rows), {"cleanup": cleanup, "old_holdings": old_rows})
        active, _ = self.wait_resource(uid)
        self.record("ports.waiter_recovers_original_instance_allocation", _rc(intent)["allocationID"] == _rc(active)["allocationID"]
                    and intent["metadata"]["uid"] == active["metadata"]["uid"],
                    {"intent": _identity(intent), "recovered": _identity(active)})
        passed, evidence = self.probe(uid)
        self.record("ports.recovered_fixed_port_real_echo", passed, evidence)
        return uid

    def agent_delete_interruption(self, uid):
        before, item = self.wait_resource(uid)
        saved = self.ledger()
        self.service("stop", "agent")
        try:
            self.delete(uid)
            deleting = self.wait("Delete intent remains awaiting actual agent cleanup", lambda: self.get(uid),
                                 lambda o: _rc(o).get("stage") == "Deleting")
            def retained(_):
                now = self.ledger()
                same = all(saved[k] == now[k] for k in ("quota_account", "node_account", "ports"))
                rows = [r for d in ("quota", "node") for r in now[d] if r["instance_uid"] == uid and any(self.holding(r))]
                docker = self.inspect(uid)
                return {"ok": same and len(rows) == 2 and all(r["state"] in ("Active", "Closing") for r in rows)
                        and bool(docker and docker["running"] and docker["id"] == item["id"]), "ledger": now}
            ok, held = self.observe_for(2, lambda: retained(None), lambda s: s["ok"])
            self.record("interruption.agent_delete.no_refund_before_destroy", ok,
                        {"before": _identity(before), "deleting": _identity(deleting), "paused": held})
        finally:
            self.service("start", "agent")
        cleanup = self.wait_deleted(uid)
        snapshot = self.ledger()
        rows = [r for domain in ("quota", "node") for r in snapshot[domain] if r["instance_uid"] == uid]
        self.record("interruption.agent_delete.destroy_workdir_then_closed", bool(rows) and
                    all(r["state"] == "Closed" and not any(self.holding(r)) for r in rows),
                    {"cleanup": cleanup, "closed_holdings": rows})

    def agent_delete_existing(self):
        for uid in reversed(list(self.records)):
            obj = self.get(uid, absent=True)
            if (obj is not None and not obj["metadata"].get("deletionTimestamp")
                    and _rc(obj).get("stage") == "Active" and obj["spec"].get("state") == "Running"):
                return self.agent_delete_interruption(uid)
        raise Blocked("No recorded Running instance is available for the independent delete interruption")

    def duplicate_target(self):
        ports, sockets = [], []
        try:
            for _ in range(128):
                if len(ports) == 2:
                    break
                port = 35000 + secrets.randbelow(25000)
                if port in ports:
                    continue
                sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
                try:
                    sock.bind(("0.0.0.0", port))
                    sock.listen(1)
                except OSError:
                    sock.close()
                    continue
                ports.append(port)
                sockets.append(sock)
        finally:
            for sock in sockets:
                sock.close()
        if len(ports) != 2:
            raise Blocked("Could not probe two unused high TCP host ports")
        claimed = {(p["protocol"], p["published_port"]) for p in self.ledger()["ports"]}
        if any(("TCP", p) in claimed for p in ports):
            raise Blocked("Free host probe overlaps a held fixture node port")
        uid, _ = self.create("duplicate-target", [{"name": "http-" + str(i), "protocol": "TCP",
                                                "targetPort": 8080, "publishedPort": port}
                                               for i, port in enumerate(ports)])
        obj, item = self.wait_resource(uid)
        passed, evidence = self.probe(uid, timeout=3)
        evidence.update({"identity": _identity(obj), "docker_port_bindings": item["ports"],
                         "probed_free_high_ports": ports})
        self.record("ports.same_tcp_target_multiple_published_ports", passed, evidence)
        self.delete(uid)
        self.wait_deleted(uid)

    def cleanup(self):
        # Restore only fixture services controlled here. Never destroy containers
        # or remove host directories to manufacture successful finalization.
        for service in ("postgres", "redis", "apiserver", "controller", "agent"):
            if service in self.stopped_services:
                try:
                    self.service("start", service)
                except Exception as exc:
                    self.checks.append({"name": "cleanup.restart_" + service, "status": "failed", "evidence": _error(exc)})
        requested = []
        for uid in self.records:
            try:
                self.delete(uid)
                requested.append(uid)
            except Exception as exc:
                self.checks.append({"name": "cleanup.api_delete", "status": "failed", "evidence": {"uid": uid, **_error(exc)}})
        if requested:
            try:
                facts = self.wait("all recorded fixture instances actually cleaned", lambda: [self.deleted_sample(uid) for uid in requested],
                                  lambda rows: all(r["api_absent"] and r["container_absent"] and r["workdir_absent"] for r in rows),
                                  cleanup=True, timeout=60)
                self.record("cleanup.api404_docker_absent_uid_workdirs_absent", True, {"instances": facts})
            except Exception as exc:
                self.checks.append({"name": "cleanup.api404_docker_absent_uid_workdirs_absent", "status": "failed", "evidence": _error(exc)})
        self.record("cleanup.no_closed_or_404_before_real_cleanup", not self.deletion_violations,
                    {"violations": list(self.deletion_violations.values())})
        if self.tenant_uid and self.node_uid:
            try:
                snapshot = self.conservation("cleanup.final_account_conservation")
                accounts = snapshot["quota_account"] + snapshot["node_account"]
                rows = snapshot["quota"] + snapshot["node"]
                self.record("cleanup.zero_current_accounts_ports", not snapshot["ports"] and
                            all(not any(self.holding(a, account=True)) for a in accounts) and
                            all(r["state"] == "Closed" and not any(self.holding(r)) for r in rows),
                            {"ledger": snapshot})
            except Exception as exc:
                self.checks.append({"name": "cleanup.zero_current_accounts_ports", "status": "failed", "evidence": _error(exc)})


def run(context: dict, control) -> dict:
    """Run against the already-started isolated fixture; return safe check evidence.

    Required context: run_id, api_url, node_name, work_dir, pg_container,
    admin_user, admin_password. Optional pg_user/pg_db default to 'acceptance';
    workload_image defaults to 'python:3.12-alpine'. Adds node_uid in place for
    the runner's exact-node final cleanup. control(action, service) must return
    after the requested local fixture process/container action is complete.
    """
    suite = Acceptance(context, control)
    try:
        suite.bootstrap()  # Fatal bootstrap errors still enter the cleanup path.
        primary = suite.case("create.core", suite.create_running)
        if primary:
            suite.case("state.stop_start", lambda: suite.stop_start(primary))
            suite.case("resize.running_grow", lambda: suite.resize(primary, GROWN, "resize.running_grow"))
            suite.case("resize.running_shrink", lambda: suite.resize(primary, SMALL, "resize.running_shrink"))
            suite.case("resize.stopped_case", lambda: suite.stopped_resize(primary))
            suite.case("disk.negative_case", lambda: suite.disk(primary))
            suite.case("restart.apiserver", lambda: suite.restarts(primary, "apiserver"))
            suite.case("restart.postgres", lambda: suite.restarts(primary, "postgres"))
            suite.case("security.isolation", lambda: suite.isolation(primary))
            suite.case("capacity.quota_denial", suite.quota_denial)
            suite.case("recovery.s3_backup_restore", lambda: suite.s3_backup_restore(primary))
            suite.case("recovery.direct_upload_backup_restore", lambda: suite.direct_upload_backup_restore(primary))
        else:
            for name in ("state.stop_start", "resize.running_grow", "resize.running_shrink",
                         "resize.stopped_case", "disk.negative_case", "restart.apiserver",
                         "restart.postgres", "security.isolation", "capacity.quota_denial",
                         "recovery.s3_backup_restore", "recovery.direct_upload_backup_restore",
                         "ports.contention_recovery"):
                suite.checks.append({"name": name, "status": "blocked", "evidence": {
                    "reason": "Primary instance did not reach verified real Docker Running"}})
        suite.case("interruption.controller", suite.controller_interruption)
        waiter = suite.case("ports.contention_recovery", lambda: suite.port_contention(primary)) if primary else None
        if waiter:
            suite.case("interruption.agent_delete", lambda: suite.agent_delete_interruption(waiter))
        else:
            # A port case failure must not suppress an independent real delete test.
            suite.case("interruption.agent_delete", suite.agent_delete_existing)
        suite.case("ports.optional_duplicate_target", suite.duplicate_target)
    finally:
        suite.cleanup()
    durations = sorted(s["duration_ms"] for s in suite.step_latencies)
    p95_idx = min(len(durations) - 1, max(0, int(0.95 * (len(durations) - 1)))) if durations else 0
    return {"run_id": context["run_id"], "checks": suite.checks,
            "tenant": suite.tenant, "tenant_uid": suite.tenant_uid, "node_uid": suite.node_uid,
            "instances": list(suite.records.values()), "elapsed_seconds": round(time.monotonic() - suite.started, 3),
            "latencies": {"steps": suite.step_latencies,
                          "max_duration_ms": durations[-1] if durations else 0.0,
                          "p95_duration_ms": durations[p95_idx] if durations else 0.0},
            "limits": {"direct_disk_write_bytes": 20 * MIB,
                       "file_ticket_oversize_probe_bytes": 32 * MIB + 1024,
                       "largest_instance_memory_bytes": 96 * MIB,
                       "cpu_stress_seconds": 0},
            "passed": all(c["status"] == "passed" for c in suite.checks)}
