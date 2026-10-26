#!/usr/bin/env python3
"""Root-only Docker lifecycle. Containers never receive the Docker socket."""
import argparse
import contextlib
import datetime
import fcntl
import errno
import hashlib
import ipaddress
import json
import os
import platform
from pathlib import Path
import re
import secrets
import shutil
import socket
import ssl
import stat
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.parse
import urllib.request

CONFIG = Path('/etc/veloray/docker')
CURRENT = Path('/opt/veloray/docker-current')
STATE = Path('/var/lib/veloray/docker')
BACKUPS = Path('/var/lib/veloray/backups')
NGINX = Path('/etc/nginx')
REPO = 'ParsaZali/VeloRay'
SERVICES = ['proxy', 'subscriptions', 'panel', 'agent']
DATA = {'agent': '/var/lib/veloray-agent', 'proxy': '/data'}
RESERVED = {80, 5432, 8610, 8611, 8612, 9191, 10085, 10086, 10087}
INSTALL_LOG = None
INSTALL_STEP = None


def say(en, fa=None, color=None):
    message = fa if fa and os.environ.get('VELORAY_INSTALL_LANG', os.environ.get('VELORAY_LANGUAGE')) == 'fa' else en
    if color and sys.stdout.isatty() and not os.environ.get('NO_COLOR'):
        message = f'\033[{color}m{message}\033[0m'
    print(message, flush=True)


def run(args, **kwargs):
    if INSTALL_LOG is not None and os.environ.get('VELORAY_VERBOSE') != '1':
        # Captured SQL/secret values stay in their pipe, never in the log.
        kwargs.setdefault('stdout', INSTALL_LOG)
        kwargs.setdefault('stderr', INSTALL_LOG)
    return subprocess.run([str(x) for x in args], check=True, **kwargs)


@contextlib.contextmanager
def installation_output():
    global INSTALL_LOG
    path = os.environ.get('VELORAY_INSTALL_LOG')
    if not path:
        yield
        return
    descriptor = os.open(path, os.O_WRONLY | os.O_APPEND | os.O_NOFOLLOW)
    info = os.fstat(descriptor)
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or info.st_mode & 0o077:
        os.close(descriptor)
        raise ValueError('The installation log must be a private regular file owned by the installer')
    previous = INSTALL_LOG
    with os.fdopen(descriptor, 'ab', buffering=0) as stream:
        INSTALL_LOG = stream
        try:
            yield
        finally:
            INSTALL_LOG = previous


def install_step(en, fa):
    global INSTALL_STEP
    if INSTALL_LOG is not None:
        INSTALL_STEP = fa if os.environ.get('VELORAY_INSTALL_LANG') == 'fa' else en
        say('  • ' + en, '  • ' + fa)


def output(args, **kwargs):
    return run(args, stdout=subprocess.PIPE, **kwargs).stdout


def read_env(path):
    """Read values as data, never source operator settings as shell commands."""
    result = {}
    for line in Path(path).read_text().splitlines():
        if not line.strip() or line.lstrip().startswith('#'):
            continue
        key, sep, value = line.partition('=')
        if not sep or not re.fullmatch('[A-Z][A-Z0-9_]*', key):
            raise ValueError('Invalid deployment environment')
        if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
            value = value[1:-1]
        result[key] = value
    return result


def atomic(path, content, mode=0o600):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, name = tempfile.mkstemp(prefix='.veloray-', dir=path.parent)
    try:
        with os.fdopen(fd, 'wb') as stream:
            stream.write(content.encode() if isinstance(content, str) else content)
            stream.flush()
            os.fsync(stream.fileno())
        os.chmod(name, mode)
        os.replace(name, path)
    finally:
        if os.path.exists(name):
            os.unlink(name)


def write_env(values):
    for key, value in values.items():
        if not re.fullmatch('[A-Z][A-Z0-9_]*', key) or any(x in str(value) for x in ('\n', '\r', '\0', '$', '`')):
            raise ValueError('Invalid deployment setting')
    atomic(CONFIG / '.env', ''.join(f'{k}={v}\n' for k, v in values.items()))


def compose(*args, source=None, **kwargs):
    source = Path(source or CURRENT).resolve()
    cmd = ['docker', 'compose', '--project-name', 'veloray', '--env-file', CONFIG / '.env', '-f', source / 'deploy/docker/compose.yaml']
    if (CONFIG / 'override.json').exists():
        cmd += ['-f', CONFIG / 'override.json']
    return run(cmd + list(args), **kwargs)


def captured(*args, **kwargs):
    return compose(*args, stdout=subprocess.PIPE, **kwargs).stdout


def cid(service):
    value = captured('ps', '-aq', service).decode().strip()
    if not re.fullmatch('[a-f0-9]{12,64}', value):
        raise ValueError(f'{service} container was not found')
    return value


def host(value):
    try:
        return str(ipaddress.ip_address(value))
    except ValueError:
        pass
    if len(value) > 253 or not value or any(not re.fullmatch('[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?', part) for part in value.split('.')):
        raise ValueError('Enter a hostname or IP without scheme, path or port')
    return value.lower()


def port(value, excluded=(), check=True):
    number = int(value)
    if not 1 <= number <= 65535 or number in RESERVED or number in excluded:
        raise ValueError('Invalid or reserved port')
    if check:
        for family, address in ((socket.AF_INET, '0.0.0.0'), (socket.AF_INET6, '::')):
            for kind in (socket.SOCK_STREAM, socket.SOCK_DGRAM):
                try:
                    with socket.socket(family, kind) as listener:
                        if kind == socket.SOCK_STREAM:
                            listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
                        if family == socket.AF_INET6:
                            listener.setsockopt(socket.IPPROTO_IPV6, socket.IPV6_V6ONLY, 1)
                        listener.bind((address, number))
                except OSError as exc:
                    if family == socket.AF_INET6 and exc.errno in (errno.EAFNOSUPPORT, errno.EPROTONOSUPPORT, errno.EADDRNOTAVAIL):
                        continue
                    raise
    return number


def random_port(excluded=()):
    for _ in range(300):
        try:
            return port(1024 + secrets.randbelow(64512), excluded)
        except (OSError, ValueError):
            pass
    raise ValueError('No free port was found')


def origin(values):
    name = values['VELORAY_DOMAIN']
    if ':' in name:
        name = f'[{name}]'
    number = int(values.get('VELORAY_PANEL_PORT', 443))
    return f'https://{name}' + (f':{number}' if number != 443 else '')


def configure():
    name = os.environ.get('VELORAY_PUBLIC_HOST', '')
    if not name:
        if not sys.stdin.isatty():
            raise ValueError('Set VELORAY_PUBLIC_HOST for unattended installation')
        say('Do you have a domain pointing to this server?', 'دامنه‌ای که به این سرور متصل باشد دارید؟')
        domain = input('[Y/n]: ').strip().lower() not in ('n', 'no')
        name = input('Domain: ' if domain else 'Public IP: ').strip()
    name = host(name)
    values = {'VELORAY_DOMAIN': name, 'VELORAY_LANGUAGE': os.environ.get('VELORAY_INSTALL_LANG', 'en')}
    for key, label in (('VELORAY_PANEL_PORT', 'Panel port'), ('VELORAY_SUBSCRIPTION_PORT', 'Subscription port')):
        value = os.environ.get(key, '')
        if not value and sys.stdin.isatty():
            value = input(f'{label} [Enter = random]: ').strip()
        values[key] = str(port(value, [int(values['VELORAY_PANEL_PORT'])] if 'VELORAY_PANEL_PORT' in values else []) if value else random_port([int(values['VELORAY_PANEL_PORT'])] if 'VELORAY_PANEL_PORT' in values else []))
    try:
        ipaddress.ip_address(name)
        tls = 'local'
    except ValueError:
        tls = os.environ.get('VELORAY_TLS_MODE', 'auto')
    if tls not in ('auto', 'local'):
        raise ValueError('TLS mode must be auto or local')
    values['VELORAY_TLS_MODE'] = tls
    values['VELORAY_ADMIN_USERNAME'] = os.environ.get('VELORAY_ADMIN_USERNAME') or 'vr_' + secrets.token_hex(4)
    if not re.fullmatch('[a-zA-Z0-9_][a-zA-Z0-9_.-]{2,63}', values['VELORAY_ADMIN_USERNAME']):
        raise ValueError('Invalid administrator username')
    values['VELORAY_DATABASE_PORT'] = str(random_port([int(values['VELORAY_PANEL_PORT']), int(values['VELORAY_SUBSCRIPTION_PORT'])]))
    values['VELORAY_TEMPLATE_ROOT'] = '/etc/veloray/templates'
    values['VELORAY_PUBLIC_URL'] = origin(values)
    return values


def digest_file(path):
    with Path(path).open('rb') as stream:
        digest = hashlib.sha256()
        for chunk in iter(lambda: stream.read(1024 * 1024), b''):
            digest.update(chunk)
        return digest.hexdigest()


def private_file(name, value):
    if len(value) < 16 or any(c in value for c in '\n\r\0'):
        raise ValueError(f'Invalid {name}')
    atomic(CONFIG / 'secrets' / name, value + '\n', 0o444)


def keys(values, native=None):
    secret_dir = CONFIG / 'secrets'
    secret_dir.mkdir(mode=0o700)
    mapping = {'database_password': 'POSTGRES_PASSWORD', 'secret_key': 'VELORAY_SECRET_KEY', 'field_key': 'VELORAY_FIELD_KEY', 'agent_token': 'VELORAY_LOCAL_NODE_TOKEN'}
    for name in (*mapping, 'admin_password'):
        value = native.get(mapping.get(name, ''), '') if native else ''
        if name == 'admin_password':
            value = os.environ.get('VELORAY_ADMIN_PASSWORD', '')
        private_file(name, value or ('Vr!' + secrets.token_hex(24) if name == 'admin_password' else secrets.token_urlsafe(48)))
    templates = CONFIG / 'templates'
    shutil.copytree(CURRENT.resolve() / 'examples/subscription', templates)
    certificates = CONFIG / 'certificates'
    certificates.mkdir(mode=0o755)
    values.update(VELORAY_SECRET_DIRECTORY=str(secret_dir), VELORAY_TEMPLATE_DIRECTORY=str(templates), VELORAY_CERTIFICATE_DIRECTORY=str(certificates), VELORAY_HOST_CONTROL='true')


def revision(source):
    value = os.environ.get('VELORAY_INSTALL_COMMIT', '')
    if not value:
        value = output(['git', '-C', source, 'rev-parse', 'HEAD'], stderr=subprocess.DEVNULL).decode().strip()
        if output(['git', '-C', source, 'status', '--porcelain']).strip():
            raise ValueError('Use a clean source revision with matching runtime bundles')
    if not re.fullmatch('[a-f0-9]{40}', value):
        raise ValueError('An immutable source revision is required')
    return value


def stage(source, sha):
    destination = Path('/opt/veloray/releases') / f'docker-{sha}'
    run(['bash', Path(source) / 'scripts/verify-runtime.sh'])
    if not destination.exists():
        destination.parent.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(prefix='.docker-', dir=destination.parent) as tmp:
            tree = Path(tmp) / 'source'
            shutil.copytree(source, tree, ignore=shutil.ignore_patterns('.git', 'node_modules', 'test-results', 'playwright-report', '__pycache__', '.env', 'secrets', 'bin', 'dist'))
            os.replace(tree, destination)
    return destination


def link(source):
    CURRENT.parent.mkdir(parents=True, exist_ok=True)
    temporary = CURRENT.with_name('.docker-current-' + secrets.token_hex(4))
    temporary.symlink_to(source)
    os.replace(temporary, CURRENT)


def deployment_values(values, source, sha):
    values.update(VELORAY_SOURCE_DIRECTORY=str(source), VELORAY_APP_IMAGE=f'veloray:{sha}', VELORAY_PROXY_IMAGE=f'veloray-proxy:{sha}', VELORAY_INSTALLED_COMMIT=sha)
    return values


def health():
    rows = captured('ps', '--format', 'json').decode().strip()
    try:
        parsed = json.loads(rows)
        rows = parsed if isinstance(parsed, list) else [parsed]
    except json.JSONDecodeError:
        rows = [json.loads(line) for line in rows.splitlines() if line.strip()]
    healthy = {row['Service'] for row in rows if row.get('State') == 'running' and row.get('Health', 'healthy') in ('healthy', '')}
    if not set(SERVICES + ['db']).issubset(healthy):
        raise ValueError('Some containers are stopped or unhealthy; run sudo veloray logs')
    values = read_env(CONFIG / '.env')
    name = values['VELORAY_DOMAIN']
    number = int(values.get('VELORAY_PANEL_PORT', 443))
    # Verify the local origin, independent of DNS/CDN. HTTP/auth/SQL health still
    # must pass; only trust-chain verification is skipped for local certificates.
    import http.client
    class LocalHTTPS(http.client.HTTPSConnection):
        def connect(self):
            self.sock = self._context.wrap_socket(socket.create_connection(('127.0.0.1', number), self.timeout), server_hostname=name)
    conn = LocalHTTPS(name, number, context=ssl._create_unverified_context(), timeout=10)
    try:
        conn.request('GET', '/api/health', headers={'Host': name})
        response = conn.getresponse()
        if response.status != 200 or json.loads(response.read(8192)).get('status') != 'ok':
            raise ValueError('Local HTTPS health check failed')
    finally:
        conn.close()


def up():
    try:
        compose('up', '-d', '--no-build', '--wait', '--wait-timeout', '240')
    except subprocess.SubprocessError:
        # The disposable CI fixture contains synthetic accounts only. Capture
        # its startup failure before migration rollback removes the container;
        # production core output stays out of installation logs.
        if os.environ.get('VELORAY_MANAGED_DOCKER_TEST') == '1':
            with contextlib.suppress(Exception):
                compose('logs', '--tail', '20', 'agent')
        raise
    for attempt in range(40):
        try:
            health()
            return
        except (OSError, ValueError):
            if attempt == 39:
                raise
            time.sleep(1)


def install_tools():
    wrapper = '#!/usr/bin/env bash\nset -Eeuo pipefail\nexec python3 /opt/veloray/docker-current/deploy/docker/manage.py "$@"\n'
    atomic('/usr/local/bin/veloray', wrapper, 0o755)
    atomic('/usr/local/bin/velorayctl', wrapper, 0o755)
    STATE.mkdir(parents=True, exist_ok=True)
    unit = '''[Unit]
Description=VeloRay Docker management worker
After=docker.service
Requires=docker.service
[Service]
Type=oneshot
ExecStart=/usr/local/bin/veloray host-jobs
TimeoutStartSec=90min
UMask=0077
'''
    timer = '''[Unit]
Description=Process approved VeloRay server operations
[Timer]
OnBootSec=30s
OnUnitInactiveSec=5s
[Install]
WantedBy=timers.target
'''
    atomic('/etc/systemd/system/veloray-docker-control.service', unit, 0o644)
    atomic('/etc/systemd/system/veloray-docker-control.timer', timer, 0o644)
    run(['systemctl', 'daemon-reload'])
    run(['systemctl', 'enable', '--now', 'veloray-docker-control.timer'])
    atomic('/etc/systemd/system/veloray-docker-certificates.service', '[Unit]\nDescription=Refresh VeloRay VPN TLS certificates from Caddy\nAfter=docker.service\n[Service]\nType=oneshot\nUMask=0077\nExecStart=/usr/local/bin/veloray certificate-sync\n', 0o644)
    atomic('/etc/systemd/system/veloray-docker-certificates.timer', '[Unit]\nDescription=Refresh renewed VeloRay certificates\n[Timer]\nOnBootSec=2min\nOnUnitInactiveSec=5min\nRandomizedDelaySec=30s\n[Install]\nWantedBy=timers.target\n', 0o644)
    run(['systemctl', 'daemon-reload'])
    run(['systemctl', 'enable', '--now', 'veloray-docker-certificates.timer'])


def schedule(kind, action):
    if kind not in ('backup', 'auto-update') or action not in ('enable', 'disable', 'status'):
        raise ValueError('Invalid schedule')
    name = 'veloray-' + kind
    if action == 'status':
        subprocess.run(['systemctl', 'list-timers', name + '.timer', '--no-pager'], check=False)
        return
    if action == 'disable':
        run(['systemctl', 'disable', '--now', name + '.timer'])
        return
    command = 'backup' if kind == 'backup' else 'auto-update run'
    atomic('/etc/systemd/system/' + name + '.service', f'[Unit]\nDescription=VeloRay Docker {kind}\nAfter=docker.service\n[Service]\nType=oneshot\nUMask=0077\nTimeoutStartSec=90min\nExecStart=/usr/local/bin/veloray {command}\n', 0o644)
    atomic('/etc/systemd/system/' + name + '.timer', '[Unit]\nDescription=VeloRay daily maintenance\n[Timer]\nOnCalendar=daily\nRandomizedDelaySec=1h\nPersistent=true\n[Install]\nWantedBy=timers.target\n', 0o644)
    run(['systemctl', 'daemon-reload'])
    run(['systemctl', 'enable', '--now', name + '.timer'])


def install(source):
    install_step('Verify VeloRay files', 'بررسی فایل‌های VeloRay')
    sha = revision(source)
    source = stage(source, sha)
    if (CONFIG / 'migration-pending.json').exists():
        raise ValueError('A native migration was interrupted. Run sudo python3 /opt/veloray/docker-current/deploy/docker/manage.py recover-migration before reinstalling')
    if (CONFIG / '.env').exists():
        say('Existing Docker data and keys will be preserved.', 'اطلاعات و کلیدهای نصب قبلی حفظ می‌شوند.')
        if (CONFIG / 'installing').exists():
            install_step('Build application images', 'آماده‌سازی ایمیج‌ها')
            compose('build', 'panel', 'proxy')
            install_step('Start services and verify HTTPS', 'راه‌اندازی سرویس‌ها و بررسی HTTPS')
            up()
            install_tools()
            (CONFIG / 'installing').unlink()
            (STATE / 'purged').unlink(missing_ok=True)
            return show_credentials(read_env(CONFIG / '.env'))
        return upgrade(source, sha)
    native_path = Path('/etc/veloray/veloray.env')
    native = native_environment_for_install(native_path)
    if native:
        return migrate(source, sha, native)
    if captured_project_exists():
        raise ValueError('A Docker project named veloray already exists without its recovery configuration; restore /etc/veloray/docker first')
    values = configure()
    nginx_plan(values['VELORAY_TLS_MODE'])
    CONFIG.mkdir(parents=True, mode=0o700, exist_ok=True)
    CONFIG.chmod(0o700)
    link(source)
    keys(values)
    write_env(deployment_values(values, source, sha))
    atomic(CONFIG / 'installing', sha)
    compose('config', '--quiet')
    install_step('Build application images', 'آماده‌سازی ایمیج‌ها')
    compose('build', 'panel', 'proxy')
    install_step('Start services and verify HTTPS', 'راه‌اندازی سرویس‌ها و بررسی HTTPS')
    up()
    install_tools()
    sync_certificate()
    (CONFIG / 'installing').unlink()
    (STATE / 'purged').unlink(missing_ok=True)
    show_credentials(values)


def native_environment_for_install(path):
    if not path.is_file():
        return None
    if (STATE / 'native-migration.json').exists():
        if (STATE / 'purged').exists():
            return None
        raise ValueError('Docker configuration is missing after a completed native migration. Restore /etc/veloray/docker; the archived native database will not be imported again')
    return read_env(path)


def show_credentials(values, migrated=False):
    say('\n  ✓ Installation complete', '\n  ✓ نصب کامل شد', '32')
    say('  Panel: ' + origin(values), '  آدرس پنل: ' + origin(values))
    if not migrated:
        say('  Username: ' + values['VELORAY_ADMIN_USERNAME'], '  نام کاربری: ' + values['VELORAY_ADMIN_USERNAME'])
        # Only the interactive operator sees the generated credential; never tee
        # it into the installation log or a CI artifact.
        if sys.stdout.isatty():
            password = (CONFIG / 'secrets/admin_password').read_text().strip()
            say('  Password: ' + password, '  رمز عبور: ' + password)
        else:
            say('  Password file (root only): /etc/veloray/docker/secrets/admin_password', '  فایل رمز (مخصوص root): /etc/veloray/docker/secrets/admin_password')
    else:
        say('Existing accounts, credentials and subscription tokens were preserved.', 'حساب‌ها، رمزها و شناسه‌های اشتراک قبلی حفظ شدند.')
    say('  Management: sudo veloray', '  مدیریت: sudo veloray')
    say('  Reset password: sudo veloray reset\n', '  تغییر رمز: sudo veloray reset\n')
    if values.get('VELORAY_TLS_MODE') == 'local':
        say('HTTPS uses a local certificate. Set a domain using sudo veloray domain HOST for public TLS.', 'گواهی HTTPS محلی است؛ برای گواهی معتبر دامنه را با sudo veloray domain HOST تنظیم کنید.', '33')


def backup(path=None):
    BACKUPS.mkdir(mode=0o700, parents=True, exist_ok=True)
    path = Path(path or BACKUPS / ('docker-' + time.strftime('%Y%m%dT%H%M%SZ', time.gmtime()) + '-' + secrets.token_hex(3) + '.tar.gz')).resolve()
    if path.exists():
        raise ValueError('Backup already exists; choose another filename')
    # A retained uninstall removes the containers, including PostgreSQL.
    # Recreate the verified previous deployment before taking its coordinated
    # snapshot so reinstalling a newer revision succeeds on the first attempt.
    up()
    compose('stop', '-t', '30', *SERVICES)
    try:
        with tempfile.TemporaryDirectory(prefix='veloray-docker-backup-') as tmp:
            root = Path(tmp)
            shutil.copytree(CONFIG, root / 'configuration')
            with (root / 'database.dump').open('xb') as stream:
                compose('exec', '-T', 'db', 'pg_dump', '-U', 'veloray', '-p', read_env(CONFIG / '.env').get('VELORAY_DATABASE_PORT', '5432'), '-Fc', '--no-owner', '--no-acl', 'veloray', stdout=stream)
            for service, directory in DATA.items():
                run(['docker', 'cp', '-a', f'{cid(service)}:{directory}/.', root / service])
            run(['docker', 'cp', '-a', f'{cid("proxy")}:/config/.', root / 'proxy-config'])
            # Unix sockets are ephemeral. Only regular files, directories and
            # safe volume-local links are included by tarfile's filter below.
            manifest = {'format': 'veloray-docker-1', 'commit': read_env(CONFIG / '.env')['VELORAY_INSTALLED_COMMIT'], 'files': {}}
            for item in root.rglob('*'):
                if item.is_file() and not item.is_symlink():
                    manifest['files'][str(item.relative_to(root))] = digest_file(item)
            (root / 'manifest.json').write_text(json.dumps(manifest))
            with path.open('xb') as stream:
                with tarfile.open(fileobj=stream, mode='w:gz') as archive:
                    archive.add(root, arcname='.', filter=lambda member: member if member.isfile() or member.isdir() or member.issym() else None)
    except BaseException:
        path.unlink(missing_ok=True)
        raise
    finally:
        up()
    say(f'✓ Backup: {path}', f'✓ پشتیبان: {path}', '32')
    return path


def verify_backup(path, directory):
    with tarfile.open(path, 'r:gz') as archive:
        members = archive.getmembers()
        if len(members) > 200000 or sum(x.size for x in members) > 100 * 1024**3:
            raise ValueError('Backup is too large')
        seen = set()
        for member in members:
            name = Path(member.name)
            if name.is_absolute() or '..' in name.parts or str(name) in seen or not (member.isfile() or member.isdir() or member.issym()):
                raise ValueError('Unsafe backup member')
            seen.add(str(name))
            if member.issym():
                if not (str(name).startswith('agent/cores/') or str(name).startswith('agent/assets/xray/')):
                    raise ValueError('Links are allowed only in managed agent slots')
                # Agent core slots use absolute links inside their managed tree.
                target = Path(member.linkname)
                allowed = str(target).startswith('/var/lib/veloray-agent/cores/') and '..' not in target.parts
                if target.is_absolute() and not allowed or not target.is_absolute() and '..' in target.parts:
                    raise ValueError('Unsafe backup link')
        # Extract regular files/directories first, then links. No archive entry
        # can cause a later write through a symlink into the host filesystem.
        for member in members:
            if not member.issym():
                if any(parent.is_symlink() for parent in (directory / member.name).parents if parent != directory.parent):
                    raise ValueError('Backup writes through a link')
                archive.extract(member, directory, set_attrs=False)
                if member.isfile():
                    (directory / member.name).chmod(member.mode & 0o777)
        for member in members:
            if member.issym():
                destination = directory / member.name
                if destination.exists() or destination.is_symlink():
                    raise ValueError('Duplicate backup link')
                destination.parent.mkdir(parents=True, exist_ok=True)
                destination.symlink_to(member.linkname)
    manifest = json.loads((directory / 'manifest.json').read_text())
    if manifest.get('format') != 'veloray-docker-1' or not re.fullmatch('[a-f0-9]{40}', manifest.get('commit', '')) or not isinstance(manifest.get('files'), dict):
        raise ValueError('Not a VeloRay Docker recovery backup')
    actual = {str(p.relative_to(directory)) for p in directory.rglob('*') if p.is_file() and not p.is_symlink() and p != directory / 'manifest.json'}
    if actual != set(manifest['files']):
        raise ValueError('Backup manifest does not cover all files')
    for name, digest in manifest['files'].items():
        file = directory / name
        if not file.is_relative_to(directory) or file.is_symlink() or digest_file(file) != digest:
            raise ValueError('Backup checksum mismatch')
    for name in ('secret_key', 'field_key', 'agent_token', 'database_password', 'admin_password'):
        if not (directory / 'configuration/secrets' / name).is_file():
            raise ValueError('Recovery keys are missing')
    validate_recovery_configuration(directory / 'configuration')
    return manifest


def validate_recovery_configuration(directory):
    values = read_env(directory / '.env')
    host(values['VELORAY_DOMAIN'])
    panel = port(values['VELORAY_PANEL_PORT'], check=False)
    port(values['VELORAY_SUBSCRIPTION_PORT'], [panel], check=False)
    database = int(values['VELORAY_DATABASE_PORT'])
    if not 1024 <= database <= 65535 or database in RESERVED or database in (panel, int(values['VELORAY_SUBSCRIPTION_PORT'])):
        raise ValueError('Invalid recovery database port')
    for key, relative in (('VELORAY_SECRET_DIRECTORY', 'secrets'), ('VELORAY_TEMPLATE_DIRECTORY', 'templates'), ('VELORAY_CERTIFICATE_DIRECTORY', 'certificates')):
        if values.get(key) != str(CONFIG / relative):
            raise ValueError('Recovery configuration points outside the managed data directory')
    for key, value in values.items():
        if not re.fullmatch('VELORAY_[A-Z0-9_]+', key) or any(c in value for c in '\n\r\0$`'):
            raise ValueError('Invalid recovery environment')
    for name in ('migration-pending.json', 'installing'):
        if (directory / name).exists():
            raise ValueError('Backup contains an unfinished installation')
    override_path = directory / 'override.json'
    if not override_path.exists():
        return
    override = json.loads(override_path.read_text())
    if set(override) != {'services'} or set(override['services']) != {'agent'}:
        raise ValueError('Recovery override may configure only the agent')
    agent = override['services']['agent']
    if set(agent) - {'environment', 'volumes', 'cap_add', 'devices'}:
        raise ValueError('Recovery contains an unsupported container override')
    permitted_env = {'VELORAY_XRAY_CONFIG': '/usr/local/etc/xray/config.json', 'VELORAY_SINGBOX_CONFIG': '/usr/local/etc/sing-box/config.json'}
    if any(permitted_env.get(key) != value for key, value in agent.get('environment', {}).items()):
        raise ValueError('Recovery changes a runtime executable or command')
    if agent.get('cap_add', []) not in ([], ['NET_ADMIN']) or agent.get('devices', []) not in ([], ['/dev/net/tun:/dev/net/tun']):
        raise ValueError('Recovery grants unsupported host capabilities')
    for mount in agent.get('volumes', []):
        if not isinstance(mount, dict) or set(mount) - {'type', 'source', 'target', 'read_only'} or mount.get('type') != 'bind':
            raise ValueError('Unsupported recovery mount')
        source = Path(mount.get('source', ''))
        target = Path(mount.get('target', ''))
        if not source.is_relative_to(CONFIG / 'native') or '..' in source.parts or not target.is_absolute() or '..' in target.parts:
            raise ValueError('Recovery mount escapes managed data')
        relative = source.relative_to(CONFIG)
        local_source = directory / relative
        if local_source.is_symlink() or not local_source.exists():
            raise ValueError('Recovery mount source is missing or linked')
        if relative.as_posix() in ('native/xray', 'native/sing-box'):
            expected = '/usr/local/etc/' + ('xray' if relative.name == 'xray' else 'sing-box')
            if str(target) != expected or not local_source.is_dir():
                raise ValueError('Invalid core configuration mount')
        elif not relative.is_relative_to(Path('native/certificates')) or not mount.get('read_only') or not local_source.is_file() or (target.parts[1] in ('bin', 'sbin', 'usr', 'proc', 'sys', 'dev', 'run', 'var', 'tmp') and not (target.is_relative_to(Path('/usr/local/etc/xray')) or target.is_relative_to(Path('/usr/local/etc/sing-box')))):
            raise ValueError('Recovery contains an unsafe certificate mount')


def restore_ownership(directory, user, group):
    for item in [directory, *directory.rglob('*')]:
        os.lchown(item, user, group)



def copy_volume(source, service, directory, user, group):
    # docker cp -a can rewrite ownership to the configured bootstrap user,
    # rather than the source UID. Extract as the final data owner instead;
    # no temporary CHOWN/DAC capability or privileged helper is required.
    with tempfile.TemporaryFile() as stream:
        with tarfile.open(fileobj=stream, mode='w') as archive:
            archive.add(source, arcname='.', filter=lambda member: member if member.isfile() or member.isdir() or member.issym() else None)
        stream.seek(0)
        compose('run', '--rm', '--no-deps', '-T', '--user', f'{user}:{group}', '--entrypoint', '/bin/sh', service, '-c',
                'umask 000; exec /bin/tar -xof - -C "$1"', '--', directory, stdin=stream)


def restore(path, confirmed=False):
    with tempfile.TemporaryDirectory(prefix='veloray-docker-restore-') as tmp:
        root = Path(tmp)
        manifest = verify_backup(path, root)
        if not confirmed and input('Replace current data? Type RESTORE: ') != 'RESTORE':
            return
        source = Path('/opt/veloray/releases') / ('docker-' + manifest['commit'])
        if not source.is_dir():
            raise ValueError('Matching release is missing; download and verify the backup revision first')
        if not confirmed:
            backup()
        compose('stop', '-t', '30', *SERVICES)
        # The current database role remains; restore its password from the saved
        # secret using psql stdin (never SQL interpolation or process arguments).
        values = read_env(root / 'configuration/.env')
        current_values = read_env(CONFIG / '.env')
        password = (root / 'configuration/secrets/database_password').read_text().strip()
        sql = "ALTER ROLE veloray PASSWORD :'password';\n"
        sql = "ALTER ROLE veloray PASSWORD '" + password.replace("'", "''") + "';\n"
        compose('exec', '-T', 'db', 'psql', '-U', 'veloray', '-p', current_values.get('VELORAY_DATABASE_PORT', '5432'), '-d', 'postgres', '-v', 'ON_ERROR_STOP=1', input=sql.encode(), stdout=subprocess.DEVNULL)
        with (root / 'database.dump').open('rb') as stream:
            compose('exec', '-T', 'db', 'pg_restore', '-U', 'veloray', '-p', current_values.get('VELORAY_DATABASE_PORT', '5432'), '-d', 'veloray', '--clean', '--if-exists', '--no-owner', '--no-acl', '--single-transaction', stdin=stream)
        # Copy only this project's fixed data roots, never database physical files.
        for service, directory in DATA.items():
            restore_ownership(root / service, 10002 if service == 'agent' else 10003, 10002 if service == 'agent' else 10001)
            compose('run', '--rm', '--no-deps', '--user', '10002:10002' if service == 'agent' else '10003:10001', '--entrypoint', '/bin/sh', service, '-c', f'find {directory} -mindepth 1 -maxdepth 1 -exec rm -rf -- {{}} +')
            copy_volume(root / service, service, directory, 10002 if service == 'agent' else 10003, 10002 if service == 'agent' else 10001)
        compose('run', '--rm', '--no-deps', '--user', '10003:10001', '--entrypoint', '/bin/sh', 'proxy', '-c', 'find /config -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +')
        restore_ownership(root / 'proxy-config', 10003, 10001)
        copy_volume(root / 'proxy-config', 'proxy', '/config', 10003, 10001)
        for item in CONFIG.iterdir():
            if item.is_dir():
                shutil.rmtree(item)
            else:
                item.unlink()
        shutil.copytree(root / 'configuration', CONFIG, dirs_exist_ok=True)
        templates = CONFIG / 'templates'
        if templates.is_dir():
            for item in [templates, *templates.rglob('*')]:
                item.chmod(0o755 if item.is_dir() else 0o644)
        # Native compatibility config directories are writable only by the agent.
        certificates = CONFIG / 'certificates'
        if certificates.is_dir():
            for item in [certificates, *certificates.rglob('*')]:
                item.chmod(0o755 if item.is_dir() else 0o444)
        for label in ('xray', 'sing-box'):
            if (CONFIG / 'native' / label).is_dir():
                restore_ownership(CONFIG / 'native' / label, 10002, 10002)
        write_env(deployment_values(values, source, manifest['commit']))
        link(source)
        up()
        install_tools()
        (STATE / 'purged').unlink(missing_ok=True)
        say('✓ Recovery completed.', '✓ بازیابی کامل شد.', '32')


def github_json(path):
    req = urllib.request.Request('https://api.github.com/repos/' + REPO + '/' + path, headers={'Accept': 'application/vnd.github+json', 'User-Agent': 'VeloRay-Docker'})
    with urllib.request.urlopen(req, timeout=30) as response:
        return json.loads(response.read(8 * 1024 * 1024))


def update_source(expected=None, check_only=False):
    sha = github_json('commits/main')['sha']
    if not re.fullmatch('[a-f0-9]{40}', sha) or expected and sha != expected:
        raise ValueError('The verified main revision changed; check again')
    runs = github_json('actions/workflows/ci.yml/runs?branch=main&per_page=30')['workflow_runs']
    ci = next((r for r in runs if r['head_sha'] == sha and r['head_branch'] == 'main' and r['event'] == 'push'), None)
    if not ci or ci.get('status') != 'completed' or ci.get('conclusion') != 'success':
        raise ValueError('The exact main revision has not passed CI; the installed version was preserved')
    if read_env(CONFIG / '.env').get('VELORAY_INSTALLED_COMMIT') == sha:
        say('Already up to date.', 'نسخهٔ نصب‌شده به‌روز است.', '32')
        return None, sha
    if check_only:
        say('Verified update available: ' + sha, 'آپدیت تأییدشده موجود است: ' + sha, '33')
        return None, sha
    with tempfile.TemporaryDirectory(prefix='veloray-docker-source-') as tmp:
        archive = Path(tmp) / 'source.tar.gz'
        run(['curl', '--proto', '=https', '--proto-redir', '=https', '-fsSL', '--retry', '2', '--connect-timeout', '15', '--max-time', '1200', f'https://codeload.github.com/{REPO}/tar.gz/{sha}', '-o', archive])
        tree = Path(tmp) / 'source'
        tree.mkdir()
        with tarfile.open(archive) as tar:
            for member in tar.getmembers():
                if not (member.isfile() or member.isdir()) or Path(member.name).is_absolute() or '..' in Path(member.name).parts:
                    raise ValueError('Unsafe source archive')
            tar.extractall(tree)
        roots = list(tree.iterdir())
        if len(roots) != 1 or not roots[0].is_dir():
            raise ValueError('Unexpected source archive')
        run(['sha256sum', '--check', '--quiet', 'SHA256SUMS'], cwd=roots[0])
        return stage(roots[0], sha), sha


def upgrade(source, sha):
    values = read_env(CONFIG / '.env')
    previous = CURRENT.resolve()
    if values.get('VELORAY_INSTALLED_COMMIT') == sha:
        up()
        install_tools()
        show_credentials(values, migrated=True)
        return
    before = dict(values)
    # Build while the previous services remain live. Revisions have unique tags.
    write_env(deployment_values(values, source, sha))
    try:
        install_step('Build application images', 'آماده‌سازی ایمیج‌ها')
        compose('build', 'panel', 'proxy', source=source)
    except BaseException:
        write_env(before)
        raise
    write_env(before)
    install_step('Create recovery backup', 'ساخت پشتیبان بازیابی')
    recovery = backup()
    try:
        write_env(deployment_values(values, source, sha))
        link(source)
        install_step('Start services and verify HTTPS', 'راه‌اندازی سرویس‌ها و بررسی HTTPS')
        up()
        install_tools()
    except BaseException:
        say('Update failed. Restoring the coordinated recovery backup…', 'آپدیت ناموفق بود؛ بازیابی پشتیبان هماهنگ…', '33')
        link(previous)
        write_env(before)
        restore(recovery, confirmed=True)
        raise
    say('✓ Docker update completed.', '✓ آپدیت کانتینرها کامل شد.', '32')
    if INSTALL_LOG is not None:
        show_credentials(values, migrated=True)


def set_address(key, value):
    before = read_env(CONFIG / '.env')
    values = dict(before)
    if key == 'VELORAY_DOMAIN':
        values[key] = host(value)
        try:
            ipaddress.ip_address(values[key])
            values['VELORAY_TLS_MODE'] = 'local'
        except ValueError:
            values['VELORAY_TLS_MODE'] = 'auto'
    else:
        values[key] = str(port(value, [int(values['VELORAY_SUBSCRIPTION_PORT'])]))
    values['VELORAY_PUBLIC_URL'] = origin(values)
    if key == 'VELORAY_DOMAIN':
        nginx_plan(values['VELORAY_TLS_MODE'])
    try:
        write_env(values)
        up()
    except BaseException:
        write_env(before)
        up()
        raise
    print(values['VELORAY_PUBLIC_URL'])
    sync_certificate()


def import_certificate(name, certificate, key):
    name = host(name)
    check = '-checkhost'
    with contextlib.suppress(ValueError):
        ipaddress.ip_address(name)
        check = '-checkip'
    match = output(['openssl', 'x509', '-in', certificate, '-noout', check, name], stderr=subprocess.DEVNULL, env=dict(os.environ, LC_ALL='C')).decode().strip()
    if not match.endswith(' does match certificate'):
        raise ValueError('Certificate does not cover this hostname or IP')
    run(['openssl', 'x509', '-in', certificate, '-noout', '-checkend', '3600'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    public = output(['openssl', 'x509', '-in', certificate, '-pubkey', '-noout'])
    private_public = output(['openssl', 'pkey', '-in', key, '-pubout'], stderr=subprocess.DEVNULL)
    if public != private_public:
        raise ValueError('Certificate and private key do not match')
    directory = CONFIG / 'certificates' / name
    directory.mkdir(parents=True, exist_ok=True, mode=0o755)
    directory.chmod(0o755)
    cert_raw, key_raw = Path(certificate).read_bytes(), Path(key).read_bytes()
    changed = not (directory / 'fullchain.pem').exists() or (directory / 'fullchain.pem').read_bytes() != cert_raw or not (directory / 'privkey.pem').exists() or (directory / 'privkey.pem').read_bytes() != key_raw
    if changed:
        atomic(directory / 'privkey.pem', key_raw, 0o444)
        atomic(directory / 'fullchain.pem', cert_raw, 0o444)
    return changed


def sync_certificate():
    values = read_env(CONFIG / '.env')
    name = values['VELORAY_DOMAIN']
    with tempfile.TemporaryDirectory(prefix='veloray-caddy-certificate-') as tmp:
        root = Path(tmp)
        run(['docker', 'cp', f'{cid("proxy")}:/data/.', root])
        candidates = []
        for certificate in root.rglob('*.crt'):
            if certificate.stem != name or certificate.stat().st_size > 1024 * 1024:
                continue
            key = certificate.with_suffix('.key')
            if not key.is_file() or key.stat().st_size > 65536:
                continue
            try:
                date = output(['openssl', 'x509', '-in', certificate, '-noout', '-enddate'], stderr=subprocess.DEVNULL, env=dict(os.environ, LC_ALL='C')).decode().strip().split('=', 1)[1]
                expires = datetime.datetime.strptime(date, '%b %d %H:%M:%S %Y %Z')
                is_local = 'local' in certificate.relative_to(root).parts
                preferred = is_local if values.get('VELORAY_TLS_MODE') == 'local' else not is_local
                candidates.append((preferred, expires, certificate, key))
            except (ValueError, IndexError, subprocess.SubprocessError):
                continue
        for _, _, certificate, key in sorted(candidates, key=lambda item: (item[0], item[1]), reverse=True):
            try:
                changed = import_certificate(name, certificate, key)
            except (ValueError, subprocess.CalledProcessError):
                continue
            if changed:
                # Certificate files are mounted only into the agent. Restarting
                # the agent makes every core pick up the renewed key pair.
                compose('restart', 'agent')
                up()
            return
    raise ValueError('No matching Caddy certificate is ready; retry sudo veloray certificate-sync after HTTPS issuance')


def sql(statement):
    values = read_env(CONFIG / '.env')
    return captured('exec', '-T', 'db', 'psql', '-U', 'veloray', '-p', values.get('VELORAY_DATABASE_PORT', '5432'), '-d', 'veloray', '-v', 'ON_ERROR_STOP=1', '-tA', input=statement.encode()).decode().strip()


def host_jobs():
    # Fixed SQL and two allowlisted operations only. No queued command/URL/path
    # reaches a shell. The root controller alone owns lifecycle permissions.
    row = sql("UPDATE vr_host_jobs SET state='running',updated_at=now() WHERE id=(SELECT id FROM vr_host_jobs WHERE state IN ('queued','running') ORDER BY id LIMIT 1) RETURNING json_build_object('id',id,'kind',kind,'target',target,'previous',previous);")
    if not row or row.startswith('UPDATE '):
        return
    job = json.loads(row.splitlines()[0])
    try:
        values = read_env(CONFIG / '.env')
        if job['kind'] == 'panel_update' and re.fullmatch('[a-f0-9]{40}', job['target']) and values['VELORAY_INSTALLED_COMMIT'] in (job['previous'], job['target']):
            if values['VELORAY_INSTALLED_COMMIT'] != job['target']:
                source, sha = update_source(job['target'])
                if source:
                    upgrade(source, sha)
            else:
                up()
        elif job['kind'] == 'panel_port' and job['target'].isdigit() and (origin(values) == job['previous'] or int(values['VELORAY_PANEL_PORT']) == int(job['target'])):
            if int(values['VELORAY_PANEL_PORT']) != int(job['target']):
                set_address('VELORAY_PANEL_PORT', job['target'])
            else:
                up()
        else:
            raise ValueError('Unsupported or stale server operation')
        sql(f"UPDATE vr_host_jobs SET state='succeeded',error='',updated_at=now() WHERE id={int(job['id'])};")
    except Exception:
        sql(f"UPDATE vr_host_jobs SET state='failed',error='Server operation failed. Run sudo veloray doctor and inspect the private management log.',updated_at=now() WHERE id={int(job['id'])};")
        raise


def unchanged_nginx_welcome(path):
    if path.name != 'default' or path.resolve() != NGINX / 'sites-available/default':
        return False
    try:
        records = output(['dpkg-query', '-W', '-f=${Conffiles}', 'nginx-common']).decode().splitlines()
        expected = next(line.split()[1] for line in records if line.split() and line.split()[0] == '/etc/nginx/sites-available/default')
        return hashlib.md5(path.read_bytes(), usedforsecurity=False).hexdigest() == expected
    except (OSError, StopIteration, subprocess.SubprocessError):
        return False


def native_tls_mode(native):
    name = host(native.get('VELORAY_PUBLIC_HOST') or urllib.parse.urlsplit(native['VELORAY_PUBLIC_URL']).hostname)
    try:
        ipaddress.ip_address(name)
        return 'local'
    except ValueError:
        mode = os.environ.get('VELORAY_TLS_MODE') or native.get('VELORAY_TLS_MODE', 'auto')
        if mode not in ('auto', 'local'):
            raise ValueError('TLS mode must be auto or local')
        return mode


def nginx_plan(tls_mode):
    """Only the canonical native VeloRay link may be detached. Never edit another site."""
    site = NGINX / 'sites-enabled/veloray'
    target = None
    if os.path.lexists(site):
        if not site.is_symlink() or site.resolve() != NGINX / 'sites-available/veloray':
            raise ValueError('Unrecognized nginx configuration at ' + str(site) + '; preserve it and move the native VeloRay listener manually before migration')
        target = os.readlink(site)
    active = subprocess.run(['systemctl', 'is-active', '--quiet', 'nginx'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0
    if target is not None or (active and tls_mode != 'local'):
        run(['nginx', '-t'])
    # Internal TLS with redirects disabled binds only the chosen panel/sub ports.
    # This also leaves sites in conf.d/custom includes and nginx service state alone.
    if tls_mode == 'local':
        return {'nginx': False, 'nginx_reload': active, 'nginx_site': target}
    foreign = [str(p) for p in (NGINX / 'sites-enabled').glob('*') if p != site and not unchanged_nginx_welcome(p)]
    foreign += [str(p) for p in (NGINX / 'conf.d').glob('*.conf')]
    if active:
        # Detect nonstandard includes and inline server blocks too, rather than
        # assuming sites-enabled is the only source of an operator's applications.
        dump = output(['nginx', '-T'], stderr=subprocess.DEVNULL).decode()
        known = {str(NGINX / name) for name in ('nginx.conf', 'mime.types', 'sites-enabled/veloray', 'sites-available/veloray', 'sites-enabled/default', 'sites-available/default')}
        for path in re.findall(r'^# configuration file (.+):$', dump, re.MULTILINE):
            if path not in known and not path.startswith(str(NGINX / 'modules-enabled') + '/'):
                foreign.append(path)
        main = NGINX / 'nginx.conf'
        if main.is_file() and re.search(r'^\s*server\s*\{', main.read_text(), re.MULTILINE):
            foreign.append(str(main))
    if foreign:
        paths = ', '.join(json.dumps(p) for p in sorted(set(foreign)))
        raise ValueError('Automatic HTTPS cannot take ports 80/443 from existing nginx applications: ' + paths + '. Keep nginx running. Use an IP with local TLS, or explicitly set VELORAY_TLS_MODE=local for a domain with a local certificate. No nginx site or service was changed.')
    return {'nginx': active, 'nginx_reload': False, 'nginx_site': None}


def detach_native_nginx(state):
    target = state.get('nginx_site')
    if target is not None:
        site = NGINX / 'sites-enabled/veloray'
        if not site.is_symlink() or os.readlink(site) != target:
            raise ValueError('The native nginx link changed during migration; refusing to remove it')
        site.unlink()
        run(['nginx', '-t'])
        if state.get('nginx_reload'):
            run(['systemctl', 'reload', 'nginx'])
    if state.get('nginx'):
        run(['systemctl', 'stop', 'nginx'])


def restore_native_nginx(state):
    target = state.get('nginx_site')
    if target is not None:
        site = NGINX / 'sites-enabled/veloray'
        # Validate journal data before restoring an interrupted migration.
        destination = Path(target) if Path(target).is_absolute() else site.parent / target
        if destination.resolve() != NGINX / 'sites-available/veloray':
            raise ValueError('Invalid nginx recovery link')
        if os.path.lexists(site):
            if not site.is_symlink() or os.readlink(site) != target:
                raise ValueError('An nginx site now exists at the recovery path; preserve it and restore the native VeloRay link manually')
        else:
            site.symlink_to(target)
        run(['nginx', '-t'])
        if state.get('nginx_reload'):
            run(['systemctl', 'reload', 'nginx'])
    if state.get('nginx'):
        if state.get('nginx_enabled'):
            run(['systemctl', 'enable', 'nginx'])
        run(['systemctl', 'start', 'nginx'])


def check_migration_ports(panel_port, sub_port):
    # A graceful nginx reload and systemd stops may take a moment to release
    # old listen sockets. Never terminate an unrelated owner to claim its port.
    for value in (panel_port, sub_port, 8610, 8612, 9191):
        for attempt in range(20):
            try:
                if value in RESERVED:
                    with socket.socket() as listener:
                        listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
                        listener.bind(('127.0.0.1', value))
                else:
                    port(value)
                break
            except OSError as exc:
                if attempt == 19:
                    raise ValueError(f'Port {value} is still used by another service. Choose an unused VeloRay port; other applications were preserved') from exc
                time.sleep(0.25)


def migrate(source, sha, native):
    existed = CONFIG.exists()
    try:
        return migrate_native(source, sha, native)
    except BaseException:
        if (CONFIG / 'recovery-incomplete').exists():
            # Keep the live recovery journal at its documented path so an
            # operator can retry restoration after fixing nginx/systemd.
            raise
        if not existed and CONFIG.exists():
            with contextlib.suppress(Exception):
                compose('down', '--volumes', '--remove-orphans')
            BACKUPS.mkdir(mode=0o700, parents=True, exist_ok=True)
            shutil.move(CONFIG, BACKUPS / ('failed-docker-staging-' + secrets.token_hex(6)))
        raise


def native_query(native, statement):
    environment = dict(os.environ, PGHOST=native.get('POSTGRES_HOST', '127.0.0.1'), PGPORT=native.get('POSTGRES_PORT', '5432'), PGUSER=native.get('POSTGRES_USER', 'veloray'), PGDATABASE=native.get('POSTGRES_DB', 'veloray'), PGPASSWORD=native['POSTGRES_PASSWORD'])
    return output(['psql', '-X', '-v', 'ON_ERROR_STOP=1', '-tA', '-c', statement], env=environment).decode().strip()


def native_certificate_paths(records, configs):
    required = set()
    for record in records:
        data = record['data']
        if data.get('enabled') is True and record.get('node_enabled') is True and data.get('security') == 'tls' and data.get('protocol') not in ('wireguard', 'tun', 'tunnel'):
            required.update(value for key in ('tls_cert_file', 'tls_key_file') if (value := data.get(key)))
    def collect(value):
        if isinstance(value, list):
            for item in value:
                collect(item)
        elif isinstance(value, dict):
            for key, item in value.items():
                if key == 'tlsSettings' and value.get('security') != 'tls':
                    continue
                if key == 'tls' and isinstance(item, dict) and item.get('enabled') is False:
                    continue
                if key in ('certificateFile', 'keyFile', 'certificate_path', 'key_path') and isinstance(item, str) and item:
                    required.add(item)
                else:
                    collect(item)
    for config in configs:
        if config.is_file():
            collect(json.loads(config.read_text()))
    missing = []
    for value in sorted(required):
        path = Path(value)
        if not path.is_absolute() or '..' in path.parts or not path.is_file():
            missing.append(json.dumps(value, ensure_ascii=False))
    if missing:
        raise ValueError('Required VPN TLS files are missing or invalid:\n  ' + '\n  '.join(missing) + '\nRestore these files or correct the active inbound TLS paths, then rerun the installer. No native service has been stopped.')
    return sorted(required)


def check_native_certificates(native):
    if not native.get('POSTGRES_PASSWORD'):
        raise ValueError('The preserved native database password is missing; restore /etc/veloray/veloray.env before migration')
    rows = json.loads(native_query(native, "SELECT COALESCE(json_agg(json_build_object('id',i.id,'data',i.data,'node_enabled',n.data->'enabled')),'[]'::json) FROM vr_inbounds i JOIN vr_nodes n ON n.id=i.node_id WHERE n.data->>'is_local'='true';"))
    return native_certificate_paths(rows, [Path('/usr/local/etc/xray/config.json'), Path('/usr/local/etc/sing-box/config.json')])


def preflight_native():
    if (CONFIG / '.env').is_file():
        return
    native = native_environment_for_install(Path('/etc/veloray/veloray.env'))
    if native:
        install_step('Check native VPN certificates', 'بررسی گواهی‌های اتصال قبلی')
        if native.get('DATABASE_URL') or native.get('POSTGRES_HOST', '127.0.0.1') not in ('127.0.0.1', 'localhost'):
            raise ValueError('External database migration requires an operator-managed dump')
        check_native_certificates(native)
        install_step('Check nginx coexistence', 'بررسی هم‌زیستی با Nginx')
        nginx_plan(native_tls_mode(native))


def native_recovery_backup(source, path, native):
    # Retained native uninstalls remove the management binary. Use the already
    # verified bundled CLI, without reinstalling or invoking the missing CLI.
    architecture = {'x86_64': 'amd64', 'aarch64': 'arm64', 'arm64': 'arm64'}[platform.machine()]
    with tempfile.TemporaryDirectory(prefix='veloray-native-backup-') as directory:
        binary = Path(directory) / 'veloray'
        with tarfile.open(source / f'runtime/linux-{architecture}.tar.xz') as archive:
            with archive.extractfile(f'bin/linux-{architecture}/veloray') as stream, binary.open('xb') as target:
                shutil.copyfileobj(stream, target)
        binary.chmod(0o700)
        run([binary, 'backup', path], env={**os.environ, **native, 'VELORAY_ENV_FILE': '/etc/veloray/veloray.env'})


def migrate_native(source, sha, native):
    # Preflight all dependencies before stopping a working native installation.
    for key in ('VELORAY_SECRET_KEY', 'VELORAY_FIELD_KEY', 'VELORAY_LOCAL_NODE_TOKEN', 'POSTGRES_PASSWORD'):
        if not native.get(key) or len(native[key]) < 16:
            raise ValueError(f'Native migration requires the preserved {key}')
    if native.get('DATABASE_URL') or native.get('POSTGRES_HOST', '127.0.0.1') not in ('127.0.0.1', 'localhost'):
        raise ValueError('External database migration requires an operator-managed dump; the working installation was preserved')
    if captured_project_exists():
        raise ValueError('An unrelated Docker project named veloray already exists')
    name = host(native.get('VELORAY_PUBLIC_HOST') or urllib.parse.urlsplit(native['VELORAY_PUBLIC_URL']).hostname)
    panel_port = port(native.get('VELORAY_PANEL_PORT', '8443'), check=False)
    sub_port = port(native.get('VELORAY_SUBSCRIPTION_PORT', '8444'), [panel_port], check=False)
    pg_env = dict(os.environ, PGHOST=native.get('POSTGRES_HOST', '127.0.0.1'), PGPORT=native.get('POSTGRES_PORT', '5432'), PGUSER=native.get('POSTGRES_USER', 'veloray'), PGDATABASE=native.get('POSTGRES_DB', 'veloray'), PGPASSWORD=native['POSTGRES_PASSWORD'])
    def query(statement):
        return output(['psql', '-X', '-v', 'ON_ERROR_STOP=1', '-tA', '-c', statement], env=pg_env).decode().strip()
    if query("SELECT EXISTS(SELECT 1 FROM vr_users WHERE data->>'is_superuser'='true');") != 't':
        raise ValueError('No native administrator was found; repair that account before migration')
    paths = check_native_certificates(native)
    settings_raw = query('SELECT data FROM vr_settings WHERE id=1;')
    cfg = json.loads(settings_raw) if settings_raw else {}
    sub_port = port(cfg.get('_subscription_endpoint', {}).get('applied_port') or sub_port, [panel_port], check=False)
    # Preserve arbitrary administrator-selected certificate paths as read-only
    # file mounts, rather than mounting the host root or silently dropping TLS.
    tls = native_tls_mode(native)
    nginx_state = nginx_plan(tls)
    values = {'VELORAY_DOMAIN': name, 'VELORAY_PANEL_PORT': str(panel_port), 'VELORAY_SUBSCRIPTION_PORT': str(sub_port), 'VELORAY_TLS_MODE': tls, 'VELORAY_ADMIN_USERNAME': native.get('VELORAY_ADMIN_USERNAME', 'admin'), 'VELORAY_DATABASE_PORT': str(random_port([panel_port, sub_port]))}
    template_root = native.get('VELORAY_TEMPLATE_ROOT', '/etc/veloray/templates')
    if not re.fullmatch(r'/[A-Za-z0-9_./-]+', template_root) or '..' in Path(template_root).parts:
        raise ValueError('Unsupported native template directory; configure a simple absolute path before migration')
    values['VELORAY_TEMPLATE_ROOT'] = template_root
    values['VELORAY_PUBLIC_URL'] = origin(values)
    CONFIG.mkdir(parents=True, mode=0o700, exist_ok=True)
    CONFIG.chmod(0o700)
    link(source)
    keys(values, native)
    write_env(deployment_values(values, source, sha))
    saved_tools = CONFIG / 'native-tools'
    saved_tools.mkdir()
    for name in ('veloray', 'velorayctl'):
        if Path('/usr/local/bin', name).is_file():
            shutil.copy2(Path('/usr/local/bin', name), saved_tools / name)
    atomic(CONFIG / 'migration-pending.json', json.dumps({'active': [], 'enabled': [], 'nginx': False}))
    compose('config', '--quiet')
    install_step('Build application images', 'آماده‌سازی ایمیج‌ها')
    compose('build', 'panel', 'proxy')
    BACKUPS.mkdir(mode=0o700, parents=True, exist_ok=True)
    recovery = BACKUPS / ('native-before-docker-' + time.strftime('%Y%m%dT%H%M%SZ', time.gmtime()) + '.tar.gz')
    units = ['veloray-web.service', 'veloray-agent.service', 'veloray-subscriptions.service', 'xray.service', 'sing-box.service', 'veloray-host-control.timer', 'veloray-subscription-endpoint.timer', 'veloray-auto-update.timer']
    active = [unit for unit in units if subprocess.run(['systemctl', 'is-active', '--quiet', unit], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0]
    enabled = [unit for unit in units if subprocess.run(['systemctl', 'is-enabled', '--quiet', unit], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0]
    say('Migrating native services to Docker. A coordinated recovery backup is created first.', 'انتقال سرویس‌های فعلی به Docker؛ ابتدا پشتیبان هماهنگ ساخته می‌شود.', '33')
    # Journal the exact owned link before detaching it, including crash recovery.
    nginx_state['nginx_enabled'] = nginx_state['nginx'] and subprocess.run(['systemctl', 'is-enabled', '--quiet', 'nginx'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0
    atomic(CONFIG / 'migration-pending.json', json.dumps({'active': active, 'enabled': enabled, **nginx_state}))
    try:
        install_step('Back up native data and transfer services', 'پشتیبان‌گیری و انتقال سرویس‌های قبلی')
        run(['systemctl', 'stop', *active]) if active else None
        detach_native_nginx(nginx_state)
        # Native services and only their nginx listener have now released ports.
        # Reject unrelated owners before copying data or starting a second stack.
        check_migration_ports(panel_port, sub_port)
        native_recovery_backup(source, recovery, native)
        compatibility = CONFIG / 'native'
        compatibility.mkdir(mode=0o700)
        mounts = []
        for value in paths:
            target = compatibility / 'certificates' / value.lstrip('/')
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(value, target)
            target.chmod(0o444)
            mounts.append({'type': 'bind', 'source': str(target), 'target': value, 'read_only': True})
        # File mounts and core configuration retain their original container
        # paths, so rollback journals and saved node patches stay meaningful.
        for value in ('/usr/local/etc/xray', '/usr/local/etc/sing-box'):
            source_dir = Path(value)
            target = compatibility / source_dir.name
            if source_dir.exists():
                shutil.copytree(source_dir, target, symlinks=False)
            else:
                target.mkdir()
            for item in [target, *target.rglob('*')]:
                os.chown(item, 10002, 10002)
                item.chmod(0o700 if item.is_dir() else 0o600)
            mounts.append({'type': 'bind', 'source': str(target), 'target': value})
        templates = Path(native.get('VELORAY_TEMPLATE_ROOT', '/etc/veloray/templates'))
        if templates.is_dir():
            shutil.copytree(templates, CONFIG / 'templates', dirs_exist_ok=True)
            for item in (CONFIG / 'templates').rglob('*'):
                item.chmod(0o755 if item.is_dir() else 0o644)
        override = {'services': {'agent': {'environment': {'VELORAY_XRAY_CONFIG': '/usr/local/etc/xray/config.json', 'VELORAY_SINGBOX_CONFIG': '/usr/local/etc/sing-box/config.json'}, 'volumes': mounts}}}
        atomic(CONFIG / 'override.json', json.dumps(override))
        compose('up', '-d', '--no-build', '--wait', 'db')
        with tempfile.TemporaryDirectory(prefix='veloray-native-copy-') as tmp:
            dump = Path(tmp) / 'database.dump'
            with dump.open('xb') as stream:
                run(['pg_dump', '-Fc', '--no-owner', '--no-acl'], env=pg_env, stdout=stream)
            with dump.open('rb') as stream:
                compose('exec', '-T', 'db', 'pg_restore', '-U', 'veloray', '-p', values['VELORAY_DATABASE_PORT'], '-d', 'veloray', '--no-owner', '--no-acl', '--single-transaction', stdin=stream)
            compose('create', '--no-build', 'agent')
            ledger = Path('/var/lib/veloray-agent')
            if ledger.is_dir():
                copy = Path(tmp) / 'agent'
                shutil.copytree(ledger, copy, symlinks=True)
                # Preserve managed official cores and their rollback history.
                old_slots = Path('/var/lib/veloray-cores')
                if old_slots.is_dir():
                    shutil.copytree(old_slots, copy / 'cores', dirs_exist_ok=True, symlinks=True)
                    for item in (copy / 'cores').rglob('*'):
                        if item.is_symlink():
                            target = os.readlink(item).replace('/var/lib/veloray-cores/', '/var/lib/veloray-agent/cores/')
                            item.unlink()
                            item.symlink_to(target)
                        elif item.suffix == '.json':
                            item.write_text(item.read_text().replace('/var/lib/veloray-cores/', '/var/lib/veloray-agent/cores/'))
                for core, binary in (('xray', '/usr/local/bin/xray'), ('singbox', '/usr/local/bin/sing-box')):
                    slot = copy / 'cores' / core
                    slot.mkdir(parents=True, exist_ok=True)
                    if not (slot / 'current').is_symlink():
                        digest = digest_file(binary)
                        shutil.copy2(binary, slot / ('binary-' + digest), follow_symlinks=True)
                        (slot / 'current').symlink_to('/var/lib/veloray-agent/cores/' + core + '/binary-' + digest)
                        (slot / 'active.json').write_text(json.dumps({'source': 'bundled', 'tag': 'preserved-native', 'sha256': digest}))
                assets = copy / 'assets/xray'
                assets.mkdir(parents=True, exist_ok=True)
                for asset in ('geoip.dat', 'geosite.dat'):
                    original = Path('/usr/local/share/xray') / asset
                    if original.is_file():
                        shutil.copy2(original, assets / asset)
                for item in [copy, *copy.rglob('*')]:
                    if not item.is_symlink():
                        os.chown(item, 10002, 10002)
                copy_volume(copy, 'agent', '/var/lib/veloray-agent', 10002, 10002)
        install_step('Start services and verify HTTPS', 'راه‌اندازی سرویس‌ها و بررسی HTTPS')
        up()
        for unit in enabled:
            run(['systemctl', 'disable', unit])
        if nginx_state['nginx']:
            run(['systemctl', 'disable', 'nginx'])
        install_tools()
        sync_certificate()
        atomic(STATE / 'native-migration.json', json.dumps({'backup': str(recovery), 'source': sha, 'native_enabled': enabled}))
        (CONFIG / 'migration-pending.json').unlink()
    except BaseException:
        for timer in ('veloray-docker-control.timer', 'veloray-docker-certificates.timer'):
            subprocess.run(['systemctl', 'disable', '--now', timer], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
        with contextlib.suppress(Exception):
            compose('down', '--volumes', '--remove-orphans')
        # Native database/configuration were only read, never overwritten. Do
        # not restart two agents against the same traffic listeners.
        for name in ('veloray', 'velorayctl'):
            if (CONFIG / 'native-tools' / name).is_file():
                atomic(Path('/usr/local/bin') / name, (CONFIG / 'native-tools' / name).read_bytes(), 0o755)
        errors = []
        try:
            restore_native_nginx(nginx_state)
        except (ValueError, OSError, subprocess.SubprocessError):
            errors.append('nginx')
        if active:
            if subprocess.run(['systemctl', 'start', *active], check=False).returncode:
                errors.append('native services')
        for unit in enabled:
            if subprocess.run(['systemctl', 'enable', unit], check=False).returncode:
                errors.append(unit)
        if errors:
            atomic(CONFIG / 'recovery-incomplete', '\n'.join(errors))
            raise ValueError('Automatic native recovery needs attention: ' + ', '.join(errors) + '. The recovery journal was preserved. Fix the service/configuration error, then run sudo python3 /opt/veloray/docker-current/deploy/docker/manage.py recover-migration')
        # Failed staging state must not be mistaken for a completed migration on
        # retry. Preserve the staged evidence and keys next to the recovery set.
        failed = BACKUPS / ('failed-docker-migration-' + secrets.token_hex(6))
        shutil.move(CONFIG, failed)
        say(f'Migration failed. Native services restored; preserved configuration: {failed}', f'انتقال ناموفق بود؛ سرویس‌های قبلی بازیابی شدند. تنظیمات محفوظ: {failed}', '31')
        if recovery.is_file():
            say(f'Recovery backup: {recovery}', f'پشتیبان بازیابی: {recovery}')
        raise
    show_credentials(values, migrated=True)


def recover_migration():
    pending = CONFIG / 'migration-pending.json'
    if not pending.is_file():
        raise ValueError('No interrupted native migration was found')
    state = json.loads(pending.read_text())
    compose('down', '--volumes', '--remove-orphans')
    for name in ('veloray', 'velorayctl'):
        saved = CONFIG / 'native-tools' / name
        if saved.is_file():
            atomic(Path('/usr/local/bin') / name, saved.read_bytes(), 0o755)
    for unit in state.get('enabled', []):
        if not re.fullmatch(r'(veloray-[a-z-]+|xray|sing-box)\.(service|timer)', unit):
            raise ValueError('Invalid recovery unit')
        run(['systemctl', 'enable', unit])
    restore_native_nginx(state)
    active = state.get('active', [])
    if any(not re.fullmatch(r'(veloray-[a-z-]+|xray|sing-box)\.(service|timer)', unit) for unit in active):
        raise ValueError('Invalid recovery unit')
    if active:
        run(['systemctl', 'start', *active])
    BACKUPS.mkdir(mode=0o700, parents=True, exist_ok=True)
    shutil.move(CONFIG, BACKUPS / ('interrupted-docker-migration-' + secrets.token_hex(6)))
    say('Native services restored. Rerun the quick installer to retry migration.', 'سرویس‌های قبلی بازیابی شدند؛ برای انتقال دوباره نصب سریع را اجرا کنید.', '32')


def captured_project_exists():
    return bool(output(['docker', 'ps', '-aq', '--filter', 'label=com.docker.compose.project=veloray']).strip() or output(['docker', 'volume', 'ls', '-q', '--filter', 'label=com.docker.compose.project=veloray']).strip())


def menu():
    while True:
        say('\nVeloRay · Docker\n', '\nVeloRay · Docker\n')
        compose('ps')
        print('\n  1  Status                  7  Reset password\n  2  Panel address           8  Change domain / IP\n  3  Restart                 9  Change panel port\n  4  Start                  10  Backup\n  5  Stop                   11  Restore\n  6  Logs                   12  Health check\n                            13  Update VeloRay\n                            14  Uninstall\n  0  Exit\n')
        if not sys.stdin.isatty():
            return
        choice = input('Select [0–14]: ').strip()
        actions = {'1': ['status'], '2': ['url'], '3': ['restart'], '4': ['start'], '5': ['stop'], '6': ['logs'], '7': ['reset'], '8': ['domain', None], '9': ['port', None], '10': ['backup'], '11': ['restore', None], '12': ['doctor'], '13': ['update'], '14': ['uninstall']}
        if choice == '0':
            return
        args = actions.get(choice)
        if args:
            if args[-1] is None:
                args[-1] = input('Value: ').strip()
            subprocess.run(['/usr/local/bin/veloray'] + args, check=False)
        input('Press Enter to return…')


def acquire_operation_lock(lock, background=False):
    deadline = time.monotonic() + 15
    while True:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            return True
        except BlockingIOError:
            if background:
                return False
            if time.monotonic() >= deadline:
                raise ValueError('Another VeloRay operation is running; retry when it finishes')
            time.sleep(0.1)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', nargs='?', default='menu')
    parser.add_argument('values', nargs='*')
    parser.add_argument('--source', type=Path)
    parser.add_argument('--purge', action='store_true')
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise ValueError('Run with sudo')
    os.umask(0o077)
    command, values = args.command, args.values
    if (CONFIG / '.env').is_file():
        os.environ['VELORAY_LANGUAGE'] = read_env(CONFIG / '.env').get('VELORAY_LANGUAGE', 'en')
    if command == 'version':
        return print((CURRENT / 'VERSION').read_text().strip())
    if command == 'help':
        print('VeloRay Docker management\n  status | doctor | url | ports | inbounds\n  start | stop | restart | logs [SERVICE]\n  reset [USERNAME] | admin-list | passkeys-reset [USERNAME]\n  domain HOST | port PORT | ssl\n  backup [FILE] | restore FILE\n  update | auto-update enable|disable|status|check|run\n  backup-schedule daily|off|status\n  certificate-sync | certificate-import HOST CERTIFICATE KEY\n  uninstall [--purge]\n')
        return
    if command == 'menu':
        return menu()
    if command in ('status', 'url', 'logs', 'admin-list', 'inbounds', 'audit', 'doctor'):
        # Read-only commands and a log follower must not block a queued update.
        lock_context = contextlib.nullcontext()
    else:
        lock_context = open('/run/lock/veloray-docker.lock', 'a')
    # A single lock covers install, migration, CLI, timers and panel requests.
    with lock_context as lock:
        if lock and not acquire_operation_lock(lock, background=command in ('host-jobs', 'certificate-sync')):
            return
        if command == 'recover-migration':
            return recover_migration()
        if command == 'preflight':
            return preflight_native()
        if command == 'install':
            return install(args.source.resolve())
        if not (CONFIG / '.env').is_file():
            raise ValueError('Docker installation was not found')
        if command == 'ports':
            return run(['ss', '-ltnup'])
        if command == 'language' and values in (['fa'], ['en']):
            settings = read_env(CONFIG / '.env')
            settings['VELORAY_LANGUAGE'] = values[0]
            return write_env(settings)
        if command == 'status':
            return compose('ps')
        if command == 'url':
            return print(origin(read_env(CONFIG / '.env')))
        if command in ('doctor', 'repair'):
            if command == 'repair':
                compose('restart', 'agent')
                up()
            health()
            say('✓ All containers and HTTPS are healthy.', '✓ کانتینرها و HTTPS سالم هستند.', '32')
        elif command in ('start', 'restart'):
            if command == 'restart':
                compose('restart')
            up()
            install_tools()
        elif command == 'stop':
            return compose('stop', '-t', '30')
        elif command == 'logs':
            service = (values or ['panel'])[0]
            service = {'web': 'panel', 'xray': 'agent', 'singbox': 'agent'}.get(service, service)
            if service not in SERVICES + ['db']:
                raise ValueError('Choose panel, agent, subscriptions, proxy or db')
            return compose('logs', '--tail', '100', '-f', service)
        elif command in ('reset', 'admin-reset', 'passkeys-reset', 'admin-list', 'inbounds', 'inbound-port', 'audit'):
            action = {'reset': 'reset-password', 'admin-reset': 'reset-password', 'passkeys-reset': 'reset-passkeys', 'inbounds': 'host-inbounds', 'inbound-port': 'host-inbound-port', 'audit': 'host-audit'}.get(command, command)
            if command in ('reset', 'admin-reset'):
                import getpass
                username = captured('exec', '-T', 'panel', 'veloray', 'admin-username', *values).decode().strip()
                password = getpass.getpass('New password [Enter = random]: ') if sys.stdin.isatty() else ''
                generated = not password
                password = password or 'Vr!' + secrets.token_hex(24)
                compose('exec', '-T', '--user', '0:0', 'panel', 'env', '-u', 'VELORAY_ADMIN_PASSWORD_FILE', '-u', 'VELORAY_ADMIN_PASSWORD', 'VELORAY_ADMIN_USERNAME=' + username, 'veloray', 'reset-password', input=(password + '\n').encode())
                if generated:
                    if sys.stdout.isatty():
                        print('Password: ' + password)
                    else:
                        private_file('last_reset_password', password)
                        print('Password file: /etc/veloray/docker/secrets/last_reset_password')
            else:
                return compose('exec', '-T', '--user', '0:0', 'panel', 'veloray', action, *values)
        elif command == 'backup':
            return backup(values[0] if values else None)
        elif command == 'restore' and len(values) == 1:
            return restore(Path(values[0]).resolve())
        elif command in ('domain', 'port') and len(values) == 1:
            return set_address('VELORAY_DOMAIN' if command == 'domain' else 'VELORAY_PANEL_PORT', values[0])
        elif command == 'ssl':
            return set_address('VELORAY_DOMAIN', read_env(CONFIG / '.env')['VELORAY_DOMAIN'])
        elif command == 'certificate-sync':
            return sync_certificate()
        elif command == 'certificate-import' and len(values) == 3:
            if import_certificate(values[0], Path(values[1]), Path(values[2])):
                compose('restart', 'agent')
                up()
        elif command == 'update':
            source, sha = update_source(os.environ.get('VELORAY_EXPECTED_UPDATE_COMMIT'))
            if source:
                return upgrade(source, sha)
        elif command == 'host-jobs':
            return host_jobs()
        elif command == 'auto-update':
            action = values[0] if values else 'status'
            if action in ('enable', 'disable', 'status'):
                return schedule('auto-update', action)
            if action not in ('check', 'run'):
                raise ValueError('Choose enable, disable, status, check or run')
            source, sha = update_source(check_only=action == 'check')
            if source:
                return upgrade(source, sha)
        elif command == 'backup-schedule':
            return schedule('backup', {'daily': 'enable', 'off': 'disable'}.get(values[0] if values else 'status', values[0] if values else 'status'))
        elif command == 'uninstall':
            confirmation = 'PURGE' if args.purge else 'REMOVE'
            if input(('Delete VeloRay containers, database, keys and volumes? Type PURGE: ' if args.purge else 'Remove VeloRay containers; preserve all data? Type REMOVE: ')) != confirmation:
                return
            if args.purge:
                backup()
            compose('down', '--remove-orphans', *(['--volumes'] if args.purge else []))
            run(['systemctl', 'disable', '--now', 'veloray-docker-control.timer'])
            for timer in ('veloray-auto-update.timer', 'veloray-backup.timer', 'veloray-docker-certificates.timer'):
                subprocess.run(['systemctl', 'disable', '--now', timer], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
            if args.purge:
                atomic(STATE / 'purged', time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()))
                shutil.rmtree(CONFIG)
                say('VeloRay application data removed. Recovery backups and Docker itself were preserved.', 'اطلاعات VeloRay حذف شد؛ بکاپ‌های بازیابی و خود Docker حفظ شدند.', '32')
                return
            say('VeloRay containers removed. Database, keys, volumes and backups were preserved. Reinstall to resume.', 'کانتینرها حذف شدند؛ اطلاعات، کلیدها و بکاپ‌ها حفظ شدند. با نصب مجدد ادامه دهید.', '32')
        else:
            raise ValueError('Use sudo veloray --help or sudo veloray for the management menu')


def saved_panel_address():
    for path in (CONFIG / '.env', Path('/etc/veloray/veloray.env')):
        if path.is_file():
            with contextlib.suppress(Exception):
                values = read_env(path)
                address = origin(values) if 'VELORAY_DOMAIN' in values else values['VELORAY_PUBLIC_URL']
                parsed = urllib.parse.urlsplit(address)
                host(parsed.hostname)
                if parsed.scheme not in ('http', 'https') or parsed.username or parsed.password or any(c in address for c in '\r\n\033'):
                    continue
                say('  Saved panel address (availability unverified): ' + address, '  آدرس ثبت‌شده (وضعیت نامشخص): ' + address, '33')
                return


if __name__ == '__main__':
    try:
        with installation_output() if len(sys.argv) > 1 and sys.argv[1] in ('install', 'preflight') else contextlib.nullcontext():
            main()
    except (ValueError, OSError, subprocess.SubprocessError) as exc:
        # Subprocess command arguments may contain sensitive environment values;
        # report only the return code, never repr(CalledProcessError.cmd).
        message = f'Operation failed (exit {exc.returncode}).' if isinstance(exc, subprocess.CalledProcessError) else str(exc)
        prefix = INSTALL_STEP or 'VeloRay'
        if os.environ.get('VELORAY_INSTALL_LOG'):
            with contextlib.suppress(Exception), installation_output():
                INSTALL_LOG.write((prefix + ': ' + message + '\n').encode())
        say('\n  ✗ ' + prefix + ': ' + message, color='31')
        if len(sys.argv) > 1 and sys.argv[1] in ('install', 'preflight'):
            saved_panel_address()
            if os.environ.get('VELORAY_INSTALL_LOG'):
                say('  Details: sudo tail -n 60 ' + os.environ['VELORAY_INSTALL_LOG'], '  جزئیات: sudo tail -n 60 ' + os.environ['VELORAY_INSTALL_LOG'])
        sys.exit(1)
