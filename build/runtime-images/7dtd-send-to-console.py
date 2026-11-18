#!/usr/bin/env python3
import os
import re
import socket
import sys
import time


def main() -> int:
    cmd = " ".join(sys.argv[1:]).strip()
    if not cmd:
        return 0
    port = int(os.environ.get("SEVEN_DTD_TELNET_PORT", "8081"))
    with socket.create_connection(("127.0.0.1", port), timeout=3.0) as sock:
        banner = bytearray()
        deadline = time.monotonic() + 1.5
        sock.settimeout(0.2)
        while time.monotonic() < deadline:
            try:
                chunk = sock.recv(4096)
                if not chunk:
                    break
                banner.extend(chunk)
                if b"Press 'exit' to end session." in banner:
                    break
            except socket.timeout:
                if banner:
                    break
        sock.sendall((cmd + "\r\n").encode("utf-8"))
        out = bytearray()
        end_deadline = time.monotonic() + 3.0
        got_data = False
        while time.monotonic() < end_deadline:
            sock.settimeout(0.25 if got_data else 1.2)
            try:
                chunk = sock.recv(65536)
                if not chunk:
                    break
                out.extend(chunk)
                got_data = True
            except socket.timeout:
                break
        try:
            sock.sendall(b"exit\r\n")
        except OSError:
            pass
    text = out.decode("utf-8", errors="replace")
    inf_log_re = re.compile(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\s+\d+\.\d+\s+INF\s+")
    lines = []
    for raw in text.splitlines():
        line = raw.strip("\r")
        if not line.strip():
            continue
        if inf_log_re.match(line):
            continue
        lines.append(line)
    if lines:
        sys.stdout.write("\n".join(lines) + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
