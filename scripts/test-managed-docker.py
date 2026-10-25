#!/usr/bin/env python3
"""Disposable CI VM only: real installer, optional migration, reset and recovery."""
import http.client
from http.cookies import SimpleCookie
import importlib.util
import json
import os
from pathlib import Path
import ssl
import socket
import subprocess
import sys
import tempfile
import time
from urllib.parse import urlsplit

ROOT = Path(__file__).resolve().parents[1]
if os.geteuid() != 0 or os.environ.get('VELORAY_MANAGED_DOCKER_TEST') != '1':
    raise SystemExit('Run as root on a disposable VM with VELORAY_MANAGED_DOCKER_TEST=1')
if Path('/etc/veloray').exists() or Path('/opt/veloray/current').exists():
    raise SystemExit('Refusing to reuse an existing installation')
spec = importlib.util.spec_from_file_location('docker_manager', ROOT / 'deploy/docker/manage.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
env = dict(os.environ, VELORAY_PUBLIC_HOST='127.0.0.1', VELORAY_PANEL_PORT='25443', VELORAY_SUBSCRIPTION_PORT='100', VELORAY_TLS_MODE='local', VELORAY_ADMIN_USERNAME='admin', VELORAY_ADMIN_PASSWORD='managed-docker-test-2026', VELORAY_INSTALL_COMMIT=subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT).decode().strip())
cookies = SimpleCookie()
panel_port = 25443


def request(method, path, body=None, expected=200):
    connection = http.client.HTTPSConnection('127.0.0.1', panel_port, timeout=30, context=ssl._create_unverified_context())
    headers = {'Origin': f'https://127.0.0.1:{panel_port}', 'Sec-Fetch-Site': 'same-origin', 'Cookie': '; '.join(f'{k}={v.value}' for k, v in cookies.items())}
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


def change_panel_port(target):
    global panel_port
    job = request('PUT', '/api/panel-endpoint', {'port': target}, 202)
    deadline = time.monotonic() + 120
    while time.monotonic() < deadline:
        subprocess.run(['/usr/local/bin/veloray', 'host-jobs'], check=True)
        state = m.sql(f"SELECT state FROM vr_host_jobs WHERE id={int(job['id'])};")
        assert state != 'failed', m.sql(f"SELECT error FROM vr_host_jobs WHERE id={int(job['id'])};")
        if state == 'succeeded':
            panel_port = target
            return
        time.sleep(0.25)
    raise AssertionError('Panel port job did not complete')


def fixture():
    node = next(n for n in request('GET', '/api/nodes') if n['is_local'])
    inbound = request('POST', '/api/inbounds', {'name': 'managed-release', 'node': node['id'], 'core': 'xray', 'protocol': 'vless', 'transport': 'raw', 'security': 'none', 'flow': '', 'listen': '127.0.0.1', 'port': 101, 'enabled': True}, 201)
    return request('POST', '/api/clients', {'name': 'preserved-release-client', 'inbound': inbound['id'], 'traffic_limit_bytes': 10485760, 'enabled': True}, 201), node


def install_panel(mode):
    result = subprocess.run(['bash', 'install.sh', '--mode', mode], cwd=ROOT, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    console = result.stdout.decode()
    print(console, flush=True)
    if result.returncode:
        # Synthetic disposable CI data only; production details stay private.
        logs = sorted(Path('/var/log/veloray').glob('install-*.log'))
        if logs:
            print('\n'.join(logs[-1].read_text().splitlines()[-100:]), flush=True)
        result.check_returncode()
    if mode == 'docker':
        assert 'Panel: https://127.0.0.1:25443' in console, console
        assert 'Management: sudo veloray' in console, console
        assert 'Reading package lists' not in console and 'Pulling fs layer' not in console, console


mode = sys.argv[1] if len(sys.argv) > 1 else 'fresh'
original_unprivileged_port = subprocess.check_output(['sysctl', '-n', 'net.ipv4.ip_unprivileged_port_start']).decode().strip()
try:
    # Host networking must work with the ordinary protected-port policy; do
    # not accidentally pass because a CI host permits every unprivileged bind.
    subprocess.run(['sysctl', '-w', 'net.ipv4.ip_unprivileged_port_start=1024'], stdout=subprocess.DEVNULL, check=True)
    previous = None
    if mode in ('migration', 'retained'):
        install_panel('native')
        login(env['VELORAY_ADMIN_PASSWORD'])
        previous, node = fixture()
        # Preserve administrator-selected TLS file paths as well as plain listeners.
        certificate, key = Path('/usr/local/etc/xray/ci.crt'), Path('/usr/local/etc/xray/ci.key')
        subprocess.run(['openssl', 'req', '-x509', '-newkey', 'ec', '-pkeyopt', 'ec_paramgen_curve:P-256', '-nodes', '-days', '2', '-keyout', str(key), '-out', str(certificate), '-subj', '/CN=127.0.0.1', '-addext', 'subjectAltName=IP:127.0.0.1'], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        import grp
        os.chown(key, 0, grp.getgrnam('nogroup').gr_gid)
        key.chmod(0o640)
        tls = request('POST', '/api/inbounds', {'name': 'preserved-tls', 'node': node['id'], 'core': 'xray', 'protocol': 'trojan', 'transport': 'raw', 'security': 'tls', 'flow': '', 'listen': '127.0.0.1', 'port': 25447, 'tls_server_name': '127.0.0.1', 'tls_cert_file': str(certificate), 'tls_key_file': str(key), 'enabled': True}, 201)
        request('POST', '/api/clients', {'name': 'preserved-tls-client', 'inbound': tls['id'], 'enabled': True}, 201)
        agent_env = m.read_env('/etc/veloray-node/agent.env')
        connection = http.client.HTTPConnection('127.0.0.1', 9191, timeout=10)
        connection.request('GET', '/xray/stats', headers={'Authorization': 'Bearer ' + agent_env['VELORAY_AGENT_TOKEN']})
        ledger = json.loads(connection.getresponse().read())['ledger_id']
        connection.close()
        request('POST', '/api/inbounds', {'name':'unused-tls','node':node['id'],'core':'xray','protocol':'trojan','transport':'raw','security':'tls','flow':'','listen':'127.0.0.1','port':25448,'tls_server_name':'127.0.0.1','tls_cert_file':'/missing/unused.crt','tls_key_file':'/missing/unused.key','enabled':False}, 201)
        saved = certificate.with_suffix('.preserved')
        certificate.rename(saved)
        try:
            check = subprocess.run(['python3', 'deploy/docker/manage.py', 'preflight'], cwd=ROOT, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            assert check.returncode != 0 and str(certificate) in check.stdout.decode()
            assert 'Saved panel address' in check.stdout.decode()
            login(env['VELORAY_ADMIN_PASSWORD'])
        finally:
            saved.rename(certificate)
        if mode == 'retained':
            subprocess.run(['/usr/local/bin/veloray', 'uninstall'], input=b'REMOVE\n', check=True)
            assert not Path('/usr/local/bin/veloray').exists()
    install_panel('docker')
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
    # Exercise both protected listeners, including the actual subscription
    # response. Panel health alone would miss a failed low-port proxy bind.
    subscription = urlsplit(previous['subscription_url'])
    assert subscription.port == 100, subscription
    connection = http.client.HTTPSConnection('127.0.0.1', 100, timeout=10, context=ssl._create_unverified_context())
    connection.request('GET', subscription.path + '?format=raw')
    response = connection.getresponse()
    assert response.status == 200 and b'vless://' in response.read()
    connection.close()
    with socket.create_connection(('127.0.0.1', 101), timeout=10):
        pass
    manager = request('GET', f'/api/nodes/{node["id"]}/core-manager')
    assert 'cores' in manager
    # The UI job is processed by the root host worker, without a socket mount.
    subprocess.run(['systemctl', 'stop', 'veloray-docker-control.timer', 'veloray-docker-control.service'], check=True)
    change_panel_port(25446)
    login(env['VELORAY_ADMIN_PASSWORD'])
    assert request('GET', '/api/panel-endpoint')['job']['state'] == 'succeeded'
    change_panel_port(25443)
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
    subprocess.run(['/usr/local/bin/veloray', 'uninstall'], input=b'REMOVE\n', check=True)
    with tempfile.TemporaryDirectory(prefix='veloray-retained-backup-') as tmp:
        subprocess.run(['/usr/local/bin/veloray', 'backup', str(Path(tmp) / 'removed.tar.gz')], check=True)
        assert (Path(tmp) / 'removed.tar.gz').is_file()
    install_panel('docker')
    login(password)
    assert request('GET', '/api/clients/' + str(previous['id']))['credential'] == previous['credential']
    subprocess.run(['/usr/local/bin/veloray', 'uninstall', '--purge'], input=b'PURGE\n', check=True)
    install_panel('docker')
    login(env['VELORAY_ADMIN_PASSWORD'])
    assert request('GET', '/api/clients') == [], 'Archived native users returned after explicit purge'
    print('Managed Docker installer, migration/identity, custom ports, backup/restore, reset, restart, retained reinstall and fresh purge/reinstall passed.')
finally:
    for timer in ('veloray-docker-control.timer', 'veloray-docker-certificates.timer'):
        subprocess.run(['systemctl', 'disable', '--now', timer], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
    subprocess.run(['systemctl', 'stop', 'veloray-docker-control.service', 'veloray-docker-certificates.service'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
    if Path('/etc/veloray/docker/.env').exists():
        with __import__('contextlib').suppress(Exception):
            m.compose('down', '--volumes', '--remove-orphans')
    subprocess.run(['systemctl', 'disable', '--now', 'veloray-docker-control.timer'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
    subprocess.run(['sysctl', '-w', 'net.ipv4.ip_unprivileged_port_start=' + original_unprivileged_port], stdout=subprocess.DEVNULL, check=False)
