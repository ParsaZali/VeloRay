"""Real Compose browser/API/core/database smoke checks with synthetic credentials."""
import http.client
from http.cookies import SimpleCookie
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import ssl
import subprocess
import threading
import time
from urllib.parse import urlsplit

project = os.environ["VELORAY_TEST_COMPOSE_PROJECT"]
assert project.startswith("veloray-ci-")
compose = ["docker", "compose", "--project-name", project, "--env-file", os.environ["VELORAY_TEST_COMPOSE_ENV"], "-f", "deploy/docker/compose.yaml"]
cookies = SimpleCookie()


def command(*args, data=None):
    return subprocess.check_output(compose + list(args), input=data, stderr=subprocess.STDOUT).decode()


def request(method, path, body=None, authenticated=True, status=200):
    connection = http.client.HTTPSConnection("localhost", 443, timeout=10, context=ssl._create_unverified_context())
    headers = {"Origin": "https://localhost", "Sec-Fetch-Site": "same-origin"}
    if authenticated:
        headers["Cookie"] = "; ".join(f"{key}={item.value}" for key, item in cookies.items())
        if "csrftoken" in cookies:
            headers["X-CSRFToken"] = cookies["csrftoken"].value
    if body is not None:
        headers["Content-Type"] = "application/json"
    connection.request(method, path, json.dumps(body) if body is not None else None, headers)
    response = connection.getresponse()
    raw = response.read()
    if authenticated:
        for name, value in response.getheaders():
            if name.lower() == "set-cookie":
                cookies.load(value)
    assert response.status == status, f"unexpected HTTP status {response.status}, expected {status}"
    connection.close()
    try:
        return json.loads(raw)
    except ValueError:
        return raw


deadline = time.monotonic() + 90
while True:
    try:
        assert request("GET", "/api/health")["status"] == "ok"
        break
    except (OSError, AssertionError):
        if time.monotonic() > deadline:
            raise
        time.sleep(1)
request("GET", "/api/auth/csrf")
password = (Path(os.environ["VELORAY_SECRET_DIRECTORY"]) / "admin_password").read_text().strip()
request("POST", "/api/auth/login", {"username": "admin", "password": password})
assert request("GET", "/api/auth/me")["is_superuser"]
node = next(row for row in request("GET", "/api/nodes") if row["is_local"])
inbound = request("POST", "/api/inbounds", {"name": "docker-test", "node": node["id"], "core": "xray", "protocol": "vless", "transport": "raw", "security": "none", "flow": "", "listen": "127.0.0.1", "port": 18443, "enabled": True}, status=201)
client = request("POST", "/api/clients", {"name": "docker-test", "inbound": inbound["id"], "traffic_limit_bytes": 1048576, "enabled": True}, status=201)
assert b"vless://" in request("GET", urlsplit(client["subscription_url"]).path + "?format=raw", authenticated=False)
# The independent subscription process must not expose administration endpoints.
connection = http.client.HTTPConnection("127.0.0.1", 8612, timeout=3)
connection.request("GET", "/api/nodes")
assert connection.getresponse().status == 404
connection.close()


class Payload(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Length", "65536")
        self.end_headers()
        self.wfile.write(b"x" * 65536)

    def log_message(self, *args):
        pass


target = ThreadingHTTPServer(("127.0.0.1", 18080), Payload)
threading.Thread(target=target.serve_forever, daemon=True).start()
config = {"inbounds": [{"listen": "127.0.0.1", "port": 18888, "protocol": "http", "settings": {}}], "outbounds": [{"protocol": "vless", "settings": {"vnext": [{"address": "127.0.0.1", "port": 18443, "users": [{"id": client["credential"], "encryption": "none"}]}]}}]}
command("exec", "-T", "agent", "sh", "-c", "umask 077; cat >/tmp/smoke-client.json", data=json.dumps(config).encode())
command("exec", "-T", "-d", "agent", "xray", "run", "-c", "/tmp/smoke-client.json")
deadline = time.monotonic() + 15
while True:
    try:
        proxy = http.client.HTTPConnection("127.0.0.1", 18888, timeout=3)
        proxy.request("GET", "http://127.0.0.1:18080/check")
        response = proxy.getresponse()
        assert response.status == 200 and len(response.read()) == 65536
        proxy.close()
        break
    except OSError:
        if time.monotonic() > deadline:
            raise
        time.sleep(0.2)
target.shutdown()
deadline = time.monotonic() + 35
while True:
    before = request("GET", f"/api/clients/{client['id']}")
    if before["used_traffic_bytes"] > 0:
        break
    assert time.monotonic() < deadline, "native traffic was not accounted"
    time.sleep(1)
ledger = command("exec", "-T", "agent", "jq", "-r", ".id", "/var/lib/veloray-agent/state.json").strip()
command("restart", "agent", "panel", "subscriptions")
deadline = time.monotonic() + 60
while True:
    try:
        after = request("GET", f"/api/clients/{client['id']}")
        assert after["used_traffic_bytes"] >= before["used_traffic_bytes"]
        assert after["traffic_limit_bytes"] == before["traffic_limit_bytes"]
        assert after["subscription_token"] == before["subscription_token"]
        assert request("GET", "/api/auth/me")["username"] == "admin"
        assert command("exec", "-T", "agent", "jq", "-r", ".id", "/var/lib/veloray-agent/state.json").strip() == ledger
        break
    except (OSError, AssertionError, subprocess.CalledProcessError):
        if time.monotonic() > deadline:
            raise
        time.sleep(1)
# Repeated bootstrap must not add another administrator or reset the password.
assert len(request("GET", "/api/admin/users")) == 1
request("GET", "/api/auth/csrf")
request("POST", "/api/auth/login", {"username": "admin", "password": password})
assert request("GET", "/api/auth/me")["is_superuser"]
