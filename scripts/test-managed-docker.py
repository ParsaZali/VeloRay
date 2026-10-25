#!/usr/bin/env python3
"""Disposable CI VM only: real installer, optional migration, reset and recovery."""
import http.client
from http.cookies import SimpleCookie
import importlib.util
import json
import os
from pathlib import Path
import ssl
import subprocess
import sys
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
if os.geteuid() != 0 or os.environ.get('VELORAY_MANAGED_DOCKER_TEST') != '1':
    raise SystemExit('Run as root on a disposable VM with VELORAY_MANAGED_DOCKER_TEST=1')
if Path('/etc/veloray').exists() or Path('/opt/veloray/current').exists():
    raise SystemExit('Refusing to reuse an existing installation')
spec = importlib.util.spec_from_file_location('docker_manager', ROOT / 'deploy/docker/manage.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
env = dict(os.environ, VELORAY_PUBLIC_HOST='127.0.0.1', VELORAY_PANEL_PORT='25443', VELORAY_SUBSCRIPTION_PORT='25444', VELORAY_TLS_MODE='local', VELORAY_ADMIN_USERNAME='admin', VELORAY_ADMIN_PASSWORD='managed-docker-test-2026', VELORAY_INSTALL_COMMIT=subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT).decode().strip())
cookies = SimpleCookie()


def request(method, path, body=None, expected=200):
    connection = http.client.HTTPSConnection('127.0.0.1', 25443, timeout=30, context=ssl._create_unverified_context())
    headers = {'Origin': 'https://127.0.0.1:25443', 'Sec-Fetch-Site': 'same-origin', 'Cookie': '; '.join(f'{k}={v.value}' for k, v in cookies.items())}
    if 'csrftoken' in cookies:
        headers['X-CSRFToken'] = cookies['csrftoken'].value
    if body is not None:
        headers['Content-Type'] = 'application/json'
    try:
        connection.request(method, path, json.dumps(body) if body is not None else None, headers)
        response = connection.getresponse()
        raw = response.read()
        for key, value in response.getheaders():
            if key.lower() == 'set-cookie':
                cookies.load(value)
        assert response.status == expected, (path, response.status, expected, raw[:300])
        return json.loads(raw) if raw else None
    finally:
        connection.close()


def login(password):
    cookies.clear()
    request('GET', '/api/auth/csrf')
    request('POST', '/api/auth/login', {'username': 'admin', 'password': password})
    assert request('GET', '/api/auth/me')['is_superuser']


def fixture():
    node = next(n for n in request('GET', '/api/nodes') if n['is_local'])
    inbound = request('POST', '/api/inbounds', {'name': 'managed-release', 'node': node['id'], 'core': 'xray', 'protocol': 'vless', 'transport': 'raw', 'security': 'none', 'flow': '', 'listen': '127.0.0.1', 'port': 25445, 'enabled': True}, 201)
    return request('POST', '/api/clients', {'name': 'preserved-release-client', 'inbound': inbound['id'], 'traffic_limit_bytes': 10485760, 'enabled': True}, 201), node


mode = sys.argv[1] if len(sys.argv) > 1 else 'fresh'
try:
    previous = None
    if mode == 'migration':
        subprocess.run(['bash', 'install.sh', '--mode', 'native'], cwd=ROOT, env=env, check=True)
        login(env['VELORAY_ADMIN_PASSWORD'])
        previous, node = fixture()
        agent_env = m.read_env('/etc/veloray-node/agent.env')
        connection = http.client.HTTPConnection('127.0.0.1', 9191, timeout=10)
        connection.request('GET', '/xray/stats', headers={'Authorization': 'Bearer ' + agent_env['VELORAY_AGENT_TOKEN']})
        ledger = json.loads(connection.getresponse().read())['ledger_id']
        connection.close()
    subprocess.run(['bash', 'install.sh', '--mode', 'docker'], cwd=ROOT, env=env, check=True)
    subprocess.run(['/usr/local/bin/veloray', 'doctor'], check=True)
    login(env['VELORAY_ADMIN_PASSWORD'])
    deployment = request('GET', '/api/deployment')
    assert deployment['mode'] == 'docker' and deployment['managed_updates']
    if previous:
        client = request('GET', '/api/clients/' + str(previous['id']))
        assert client['credential'] == previous['credential']
        assert client['subscription_url'] == previous['subscription_url']
        assert client['traffic_limit_bytes'] == previous['traffic_limit_bytes']
        token = (Path('/etc/veloray/docker/secrets/agent_token')).read_text().strip()
        connection = http.client.HTTPConnection('127.0.0.1', 9191, timeout=10)
        connection.request('GET', '/xray/stats', headers={'Authorization': 'Bearer ' + token})
        assert json.loads(connection.getresponse().read())['ledger_id'] == ledger
        connection.close()
    else:
        previous, node = fixture()
    manager = request('GET', f'/api/nodes/{node["id"]}/core-manager')
    assert 'cores' in manager
    # The UI job is processed by the root host worker, without a socket mount.
    subprocess.run(['systemctl', 'stop', 'veloray-docker-control.timer'], check=True)
    request('PUT', '/api/panel-endpoint', {'port': 25446}, 202)
    # Exercise its worker without changing the fixture's public origin: inspect
    # and remove this synthetic queued job, then use the CLI port round trip.
    m.sql("UPDATE vr_host_jobs SET state='failed',error='Synthetic CI job; testing CLI port round trip',updated_at=now() WHERE state='queued';")
    subprocess.run(['/usr/local/bin/veloray', 'port', '25446'], check=True)
    subprocess.run(['/usr/local/bin/veloray', 'port', '25443'], check=True)
    login(env['VELORAY_ADMIN_PASSWORD'])
    with tempfile.TemporaryDirectory(prefix='veloray-managed-test-') as tmp:
        archive = Path(tmp) / 'recovery.tar.gz'
        subprocess.run(['/usr/local/bin/veloray', 'backup', archive], check=True)
        # Recovery must restore both database identity and agent ledger together.
        request('PATCH', '/api/clients/' + str(previous['id']), {'traffic_limit_bytes': 20971520})
        subprocess.run(['/usr/local/bin/veloray', 'restore', archive], input=b'RESTORE\n', check=True)
        login(env['VELORAY_ADMIN_PASSWORD'])
        restored = request('GET', '/api/clients/' + str(previous['id']))
        assert restored['credential'] == previous['credential']
        assert restored['traffic_limit_bytes'] == previous['traffic_limit_bytes']
    subprocess.run(['/usr/local/bin/veloray', 'reset'], stdin=subprocess.DEVNULL, check=True)
    password = Path('/etc/veloray/docker/secrets/last_reset_password').read_text().strip()
    assert password != env['VELORAY_ADMIN_PASSWORD']
    login(password)
    subprocess.run(['/usr/local/bin/veloray', 'restart'], check=True)
    login(password)
    assert request('GET', '/api/clients/' + str(previous['id']))['credential'] == previous['credential']
    print('Managed Docker installer, migration/identity, custom ports, backup/restore, password reset and durable restart passed.')
finally:
    if Path('/etc/veloray/docker/.env').exists():
        with __import__('contextlib').suppress(Exception):
            m.compose('down', '--volumes', '--remove-orphans')
    subprocess.run(['systemctl', 'disable', '--now', 'veloray-docker-control.timer'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
