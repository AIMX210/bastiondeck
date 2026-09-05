#!/usr/bin/env python3
"""Black-box smoke test for the bastiondeck daemon binary.

Unlike the Go unit/integration tests (which run in-process with a fake SSH
fleet), this script builds the real binary, boots it on an ephemeral port and
exercises the public HTTP surface end to end:

  health -> status(setupRequired) -> first-run setup -> login (cookie) ->
  /auth/me -> snippet CRUD + render -> settings -> doctor ->
  audit hash-chain verify -> encrypted backup export/inspect -> logout

It deliberately avoids SSH-dependent paths (no sshd in CI). Exits non-zero
on the first failed assertion.
"""
from __future__ import annotations
import base64
import http.cookiejar
import json
import os
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PASS = "smoke-passphrase-123"
FAILS: list[str] = []


def free_port() -> int:
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    p = s.getsockname()[1]
    s.close()
    return p


def check(name: str, cond: bool, detail: str = "") -> None:
    mark = "PASS" if cond else "FAIL"
    print(f"  [{mark}] {name}{(' — ' + detail) if detail else ''}")
    if not cond:
        FAILS.append(name)


class Client:
    def __init__(self, base: str):
        self.base = base
        self.jar = http.cookiejar.CookieJar()
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.jar))

    def call(self, method: str, path: str, body=None, raw=False):
        data = None
        headers = {}
        if body is not None:
            data = json.dumps(body).encode()
            headers["Content-Type"] = "application/json"
        if method not in ("GET", "HEAD"):
            headers["X-BDK-CSRF"] = "1"
        req = urllib.request.Request(self.base + path, data=data, headers=headers, method=method)
        try:
            with self.opener.open(req, timeout=10) as resp:
                text = resp.read().decode()
                return resp.status, (text if raw else (json.loads(text) if text else {}))
        except urllib.error.HTTPError as e:  # type: ignore[attr-defined]
            text = e.read().decode()
            try:
                return e.code, json.loads(text)
            except json.JSONDecodeError:
                return e.code, {"raw": text}


def main() -> int:
    port = free_port()
    base = f"http://127.0.0.1:{port}"
    data_dir = tempfile.mkdtemp(prefix="bdk-smoke-")
    bin_path = os.path.join(ROOT, "bin", "bastiondeck")
    print("== building daemon ==")
    rc = subprocess.run(["go", "build", "-o", bin_path, "./cmd/bastiondeck"], cwd=ROOT).returncode
    if rc != 0:
        print("build failed")
        return 2

    env = dict(os.environ, BDK_LISTEN=f"127.0.0.1:{port}", BDK_DATA_DIR=data_dir)
    proc = subprocess.Popen([bin_path], cwd=ROOT, env=env,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    try:
        # Wait for health.
        ok = False
        for _ in range(50):
            try:
                with urllib.request.urlopen(base + "/api/healthz", timeout=1) as r:
                    if r.status == 200:
                        ok = True
                        break
            except OSError:
                time.sleep(0.1)
        check("daemon boots and /api/healthz responds", ok)
        if not ok:
            return 1

        c = Client(base)
        st, body = c.call("GET", "/api/status")
        check("status 200", st == 200)
        check("fresh instance requires setup", body.get("data", {}).get("setupRequired") is True)

        st, body = c.call("POST", "/api/setup",
                          {"username": "owner", "password": PASS, "displayName": "Smoke"})
        check("first setup succeeds", st == 200, str(body))

        # Second setup must be rejected (anti account-takeover).
        st, body = c.call("POST", "/api/setup", {"username": "x", "password": PASS * 2})
        check("second setup rejected", st >= 400)

        st, body = c.call("GET", "/api/auth/me")
        me = body.get("data", {})
        check("session authenticated after setup", st == 200 and me.get("user", {}).get("role") == "owner")

        # CSRF: write without custom header must be blocked for cookie sessions.
        req = urllib.request.Request(base + "/api/snippets", data=b"{}",
                                     headers={"Content-Type": "application/json"}, method="POST")
        try:
            urllib.request.urlopen(req, timeout=5)
            csrf_ok = False
        except urllib.error.HTTPError as e:  # type: ignore[attr-defined]
            csrf_ok = e.code == 403
        check("CSRF header enforced", csrf_ok)

        st, body = c.call("POST", "/api/snippets",
                          {"title": "smoke", "body": "echo ${WHO}", "tags": ["t"]})
        snip = body.get("data", {}).get("snippet", {})
        check("snippet created", st in (200, 201) and bool(snip.get("id")))

        st, body = c.call("POST", "/api/snippets/render",
                          {"body": "echo ${WHO}", "vars": {"WHO": "ok"}})
        check("snippet render substitutes", body.get("data", {}).get("rendered") == "echo ok", str(body))

        st, body = c.call("GET", "/api/settings")
        check("settings expose defaults", st == 200 and "exec.defaultTimeoutSec" in body.get("data", {}).get("settings", {}))

        st, body = c.call("GET", "/api/doctor")
        check("doctor runs", st == 200 and "checks" in body.get("data", {}))

        st, body = c.call("POST", "/api/audit/verify")
        chain = body.get("data", {}).get("chain", {})
        check("audit hash-chain verifies", chain.get("ok") is True, str(chain))

        st, body = c.call("POST", "/api/backup/export", {"passphrase": "backup-pass-1"})
        blob = body.get("data", {}).get("backupBase64", "")
        check("encrypted backup exported", st == 200 and len(blob) > 0)
        st, body = c.call("POST", "/api/backup/inspect",
                          {"backupBase64": blob, "passphrase": "backup-pass-1"})
        check("backup inspect roundtrip", st == 200 and body.get("data", {}).get("report", {}).get("version") == 1)
        st, _ = c.call("POST", "/api/backup/inspect",
                       {"backupBase64": blob, "passphrase": "wrong-pass-9"})
        check("backup wrong password rejected", st == 422)

        c.call("POST", "/api/auth/logout")
        st, _ = c.call("GET", "/api/auth/me")
        check("logout clears session", st in (401, 403))
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()
        shutil.rmtree(data_dir, ignore_errors=True)

    print()
    if FAILS:
        print(f"SMOKE FAILED: {FAILS}")
        return 1
    print("SMOKE OK")
    return 0


if __name__ == "__main__":
    sys.exit(main())
