#!/usr/bin/env python3
"""Exercise the real interactive setup prompts without modifying the host."""
import json
import os
import pathlib
import pty
import subprocess

project = pathlib.Path(__file__).resolve().parent.parent
keys = ["VELORAY_PUBLIC_HOST", "VELORAY_PANEL_PORT", "VELORAY_SUBSCRIPTION_PORT", "VELORAY_TLS_MODE", "VELORAY_ACME_EMAIL"]
script = """set -e
source scripts/common.sh
vr_setup_initial
python3 -c 'import json,os;print("SETUP_RESULT="+json.dumps({k:os.environ.get(k) for k in ["VELORAY_PUBLIC_HOST","VELORAY_PANEL_PORT","VELORAY_SUBSCRIPTION_PORT","VELORAY_TLS_MODE"]}))'
"""

def setup(answers):
    master, slave = pty.openpty()
    env = dict(os.environ, NO_COLOR="1")
    for key in keys:
        env.pop(key, None)
    process = subprocess.Popen(["bash", "-c", script], cwd=project, stdin=slave, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, env=env)
    os.close(slave)
    try:
        os.write(master, answers.encode())
        output = process.communicate(timeout=20)[0].decode()
    finally:
        os.close(master)
        if process.poll() is None:
            process.kill()
            process.wait()
    return process.returncode, output

status, output = setup("y\nvpn.example.com\n\n\n\n")
assert status == 0, output
value = json.loads(output.split("SETUP_RESULT=")[-1].strip())
assert value["VELORAY_PUBLIC_HOST"] == "vpn.example.com" and value["VELORAY_TLS_MODE"] == "auto"
ports = [int(value["VELORAY_PANEL_PORT"]), int(value["VELORAY_SUBSCRIPTION_PORT"])]
assert all(1024 <= port <= 65535 for port in ports) and ports[0] != ports[1]
assert not set(ports) & {80,5432,8610,8611,8612,9191,10085,10086,10087}
status, output = setup("n\n203.0.113.8\n23456\n100\n")
assert status == 0, output
value = json.loads(output.split("SETUP_RESULT=")[-1].strip())
assert value["VELORAY_TLS_MODE"] == "local" and value["VELORAY_PANEL_PORT"] == "23456" and value["VELORAY_SUBSCRIPTION_PORT"] == "100"
status, output = setup("n\n203.0.113.8\n8610\n")
assert status != 0 and "Invalid or reserved panel port" in output
print("Interactive setup: domain, IP, random ports, custom non-CDN ports and reserved-port rejection passed.")
