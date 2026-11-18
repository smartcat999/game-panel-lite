#!/usr/bin/env python3
import base64
import datetime
import json
import os
import re
import subprocess
import sys
import urllib.error
import urllib.request


def read_pw() -> str:
    pw = "paladmin123"
    if os.path.exists("/data/rcon.yaml"):
        with open("/data/rcon.yaml", encoding="utf-8", errors="replace") as f:
            m = re.search(r'password:\s*"?([^"\r\n]+)"?', f.read())
            if m:
                pw = m.group(1)
    return pw


def rcon_fallback(line: str) -> None:
    parts = line.split(None, 1)
    op = parts[0].lower() if parts else ""
    fb = line
    if op == "broadcast" and len(parts) == 2:
        fb = "Broadcast " + parts[1].replace(" ", "\u00a0")
    elif op == "shutdown" and len(parts) == 2:
        sp = parts[1].split(None, 1)
        fb = "Shutdown " + sp[0] + (" " + sp[1].replace(" ", "\u00a0") if len(sp) > 1 else "")
    p = subprocess.run(["rcon-cli", fb])
    sys.exit(p.returncode)


def main() -> int:
    line = " ".join(sys.argv[1:]).strip()
    if not line:
        return 0
    if not os.path.exists("/data/Pal/Saved/Config/LinuxServer/PalWorldSettings.ini"):
        rcon_fallback(line)

    parts = line.split(None, 1)
    op = parts[0].lower()
    arg = parts[1].strip() if len(parts) > 1 else ""
    norm_uid = lambda u: ("steam_" + u) if u.isdigit() else u

    method, ep, payload, echo = None, None, None, True
    if op == "save":
        method, ep, payload = "POST", "save", {}
    elif op == "broadcast":
        method, ep, payload = "POST", "announce", {"message": arg}
    elif op == "kickplayer" and arg:
        sp = arg.split(None, 1)
        method, ep, payload = "POST", "kick", {"userid": norm_uid(sp[0]), "message": sp[1] if len(sp) > 1 else "Kicked by admin"}
    elif op == "banplayer" and arg:
        sp = arg.split(None, 1)
        method, ep, payload = "POST", "ban", {"userid": norm_uid(sp[0]), "message": sp[1] if len(sp) > 1 else "Banned by admin"}
    elif op == "unbanplayer" and arg:
        method, ep, payload = "POST", "unban", {"userid": norm_uid(arg.split()[0])}
    elif op == "shutdown":
        sp = arg.split(None, 1)
        sec = int(sp[0]) if sp and sp[0].isdigit() else 60
        method, ep, payload = "POST", "shutdown", {"waittime": max(1, sec), "message": sp[1] if len(sp) > 1 else "Server is shutting down"}
    elif op == "doexit":
        method, ep, payload = "POST", "stop", {}
    elif op == "info":
        method, ep, echo = "GET", "info", False
    elif op == "showplayers":
        method, ep, echo = "GET", "players", False

    if not ep:
        rcon_fallback(line)

    auth = base64.b64encode(("admin:" + read_pw()).encode("utf-8")).decode("ascii")
    hdrs = {"Authorization": "Basic " + auth}
    data = None
    if method == "POST":
        hdrs["Content-Type"] = "application/json; charset=utf-8"
        data = json.dumps(payload or {}, ensure_ascii=False).encode("utf-8")

    req = urllib.request.Request("http://127.0.0.1:8212/v1/api/" + ep, data=data, headers=hdrs, method=method)
    try:
        with urllib.request.urlopen(req, timeout=2.5) as resp:
            body = resp.read().decode("utf-8", "replace").strip()
            if echo:
                ts = datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")
                try:
                    with open("/proc/1/fd/1", "a", encoding="utf-8") as out:
                        out.write(f"[{ts}] [LOG] Executed command: {line}\n")
                except Exception:
                    pass
            print(body or "OK")
            return 0
    except urllib.error.HTTPError as e:
        err_body = e.read().decode("utf-8", "replace").strip()
        sys.stderr.write((err_body or str(e)) + "\n")
        return 1
    except Exception:
        rcon_fallback(line)
        return 0


if __name__ == "__main__":
    raise SystemExit(main())
