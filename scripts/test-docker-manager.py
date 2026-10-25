#!/usr/bin/env python3
"""Offline lifecycle failure and hostile-backup tests. No host changes."""
import importlib.util
import io
import json
import os
import shutil
from pathlib import Path
import subprocess
import tarfile
import tempfile
import threading
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('docker_manager', ROOT / 'deploy/docker/manage.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)


class ManagerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.config = self.root / 'configuration'
        self.config.mkdir()
        self.current = self.root / 'current'
        self.source = self.root / 'release'
        self.source.mkdir()
        self.current.symlink_to(self.source)
        self.patches = [patch.object(m, 'CONFIG', self.config), patch.object(m, 'CURRENT', self.current), patch.object(m, 'BACKUPS', self.root / 'backups'), patch.object(m, 'STATE', self.root/'state')]
        for item in self.patches:
            item.start()
        self.values = {'VELORAY_DOMAIN': 'vpn.example', 'VELORAY_PANEL_PORT': '24443', 'VELORAY_SUBSCRIPTION_PORT': '24444', 'VELORAY_DATABASE_PORT': '24543', 'VELORAY_INSTALLED_COMMIT': 'a' * 40}
        m.write_env(self.values)

    def tearDown(self):
        for item in reversed(self.patches):
            item.stop()
        self.temp.cleanup()

    def test_hosts_ports_and_environment_reject_injection(self):
        for value in ('https://vpn.example', 'vpn.example:443', 'x;id', 'x\nY=z', '../x', 'a..b', ''):
            with self.assertRaises(ValueError):
                m.host(value)
        self.assertEqual(m.host('2001:db8::1'), '2001:db8::1')
        self.assertEqual(m.origin(dict(self.values, VELORAY_DOMAIN='2001:db8::1')), 'https://[2001:db8::1]:24443')
        for value in (0, 80, 8610, 9191, 65536, 24444):
            with self.assertRaises(ValueError):
                m.port(value, [24444], check=False)
        for value in ('$(id)', '`id`', 'hello\nX=y'):
            with self.assertRaises(ValueError):
                m.write_env({'VALUE': value})
        self.assertEqual(m.read_env(self.config / '.env'), self.values)
        self.assertEqual((self.config / '.env').stat().st_mode & 0o777, 0o600)

    def test_foreground_command_waits_for_short_maintenance_and_keeps_exclusion(self):
        path = self.root/'operation.lock'
        with path.open('a') as first, path.open('a') as second:
            m.fcntl.flock(first, m.fcntl.LOCK_EX)
            self.assertFalse(m.acquire_operation_lock(second, background=True))
            release = threading.Timer(0.15, lambda: m.fcntl.flock(first, m.fcntl.LOCK_UN))
            release.start()
            try:
                self.assertTrue(m.acquire_operation_lock(second))
                with self.assertRaises(BlockingIOError):
                    m.fcntl.flock(first, m.fcntl.LOCK_EX | m.fcntl.LOCK_NB)
            finally:
                release.join()

    def test_foreground_lock_wait_is_bounded_and_does_not_interrupt_active_work(self):
        path = self.root/'operation.lock'
        with path.open('a') as first, path.open('a') as second:
            m.fcntl.flock(first, m.fcntl.LOCK_EX)
            with patch.object(m.time, 'monotonic', side_effect=[0, 16]), self.assertRaises(ValueError):
                m.acquire_operation_lock(second)
            with self.assertRaises(BlockingIOError):
                m.fcntl.flock(second, m.fcntl.LOCK_EX | m.fcntl.LOCK_NB)

    def test_random_ports_are_free_and_distinct(self):
        one = m.random_port()
        two = m.random_port([one])
        self.assertNotEqual(one, two)
        self.assertNotIn(one, m.RESERVED)
        self.assertGreaterEqual(one, 1024)

    def test_random_ports_work_on_ipv4_only_hosts(self):
        original = m.socket.socket
        def ipv4_only(family, kind):
            if family == m.socket.AF_INET6:
                raise OSError(m.errno.EAFNOSUPPORT, 'IPv6 disabled')
            return original(family, kind)
        with patch.object(m.socket, 'socket', side_effect=ipv4_only):
            number = m.random_port()
        self.assertGreaterEqual(number, 1024)

    def test_volume_stream_preserves_executable_keys_and_local_links(self):
        source, destination = self.root/'volume-source', self.root/'volume-target'
        source.mkdir(mode=0o700)
        destination.mkdir(mode=0o700)
        (source/'binary-core').write_bytes(b'core-test')
        (source/'binary-core').chmod(0o755)
        (source/'private-state').write_bytes(b'private-test')
        (source/'private-state').chmod(0o600)
        (source/'current').symlink_to('binary-core')
        def extract(*args, **kwargs):
            # Replace only the Docker transport; exercise the exact extractor
            # with its streamed tar, file modes and links on a real filesystem.
            script = args[args.index('-c')+1]
            subprocess.run(['/bin/sh', '-c', script, '--', str(destination)], stdin=kwargs['stdin'], check=True)
        with patch.object(m, 'compose', side_effect=extract):
            m.copy_volume(source, 'agent', str(destination), os.getuid(), os.getgid())
        self.assertEqual((destination/'binary-core').read_bytes(), b'core-test')
        self.assertEqual((destination/'binary-core').stat().st_mode & 0o777, 0o755)
        self.assertEqual((destination/'private-state').stat().st_mode & 0o777, 0o600)
        self.assertTrue((destination/'current').is_symlink())
        self.assertEqual(os.readlink(destination/'current'), 'binary-core')

    def test_purge_never_reimports_archived_native_accounts(self):
        native = self.root/'native.env'
        native.write_text('POSTGRES_DB=veloray\n')
        self.assertEqual(m.native_environment_for_install(native), {'POSTGRES_DB': 'veloray'})
        m.STATE.mkdir()
        (m.STATE/'native-migration.json').write_text('{}')
        with self.assertRaises(ValueError):
            m.native_environment_for_install(native)
        (m.STATE/'purged').write_text('confirmed')
        self.assertIsNone(m.native_environment_for_install(native))

    def test_failed_image_build_keeps_previous_services_and_configuration(self):
        with patch.object(m, 'compose', side_effect=subprocess.CalledProcessError(1, ['docker'])), patch.object(m, 'backup') as backup:
            with self.assertRaises(subprocess.CalledProcessError):
                m.upgrade(self.root / 'new', 'b' * 40)
        backup.assert_not_called()
        self.assertEqual(m.read_env(self.config / '.env'), self.values)
        self.assertEqual(self.current.resolve(), self.source)

    def test_failed_update_restores_coordinated_database_and_ledger_snapshot(self):
        new = self.root / 'new'
        new.mkdir()
        recovery = self.root / 'backup.tar.gz'
        with patch.object(m, 'compose'), patch.object(m, 'backup', return_value=recovery), patch.object(m, 'up', side_effect=ValueError('bad health')), patch.object(m, 'restore') as restore:
            with self.assertRaises(ValueError):
                m.upgrade(new, 'b' * 40)
        restore.assert_called_once_with(recovery, confirmed=True)
        self.assertEqual(m.read_env(self.config / '.env'), self.values)
        self.assertEqual(self.current.resolve(), self.source)

    def test_address_failure_restores_old_origin(self):
        with patch.object(m, 'up', side_effect=[ValueError('TLS failed'), None]) as up:
            with self.assertRaises(ValueError):
                m.set_address('VELORAY_DOMAIN', 'new.example')
        self.assertEqual(up.call_count, 2)
        self.assertEqual(m.read_env(self.config / '.env'), self.values)

    def archive(self, members):
        path = self.root / 'backup.tar.gz'
        path.unlink(missing_ok=True)
        with tarfile.open(path, 'w:gz') as tar:
            for name, value, kind in members:
                member = tarfile.TarInfo(name)
                member.mode = 0o600
                if kind == 'link':
                    member.type = tarfile.SYMTYPE
                    member.linkname = value
                    tar.addfile(member)
                else:
                    raw = value.encode()
                    member.size = len(raw)
                    tar.addfile(member, io.BytesIO(raw))
        return path

    def test_backups_reject_escape_links_duplicates_and_uncovered_content(self):
        cases = [
            [('../outside', 'x', 'file')],
            [('/etc/shadow', 'x', 'file')],
            [('agent/file', '/etc/shadow', 'link')],
            [('agent/file', '../../outside', 'link')],
            [('agent/file', 'x', 'file'), ('agent/file', 'y', 'file')],
            [('manifest.json', json.dumps({'format': 'veloray-docker-1', 'commit': 'a'*40, 'files': {}}), 'file'), ('extra', 'unlisted', 'file')],
        ]
        for members in cases:
            with self.subTest(members=members), tempfile.TemporaryDirectory() as tmp, self.assertRaises(ValueError):
                m.verify_backup(self.archive(members), Path(tmp))
        self.assertFalse((self.root.parent / 'outside').exists())

    def test_native_external_database_preflight_never_stops_services(self):
        native = {k: 's' * 48 for k in ('VELORAY_SECRET_KEY', 'VELORAY_FIELD_KEY', 'VELORAY_LOCAL_NODE_TOKEN', 'POSTGRES_PASSWORD')}
        native['POSTGRES_HOST'] = 'db.example'
        with patch.object(m, 'run') as run, self.assertRaises(ValueError):
            m.migrate_native(self.source, 'b' * 40, native)
        run.assert_not_called()

    def test_unknown_queued_operations_are_marked_failed_without_execution(self):
        row = json.dumps({'id': 4, 'kind': 'shell', 'target': 'rm -rf /', 'previous': ''})
        with patch.object(m, 'sql', side_effect=[row, 'UPDATE 1']) as sql, patch.object(m, 'run') as run, self.assertRaises(ValueError):
            m.host_jobs()
        run.assert_not_called()
        self.assertIn("state='failed'", sql.call_args.args[0])

    def test_stale_update_target_is_not_installed(self):
        row = json.dumps({'id': 4, 'kind': 'panel_update', 'target': 'b' * 40, 'previous': 'c' * 40})
        with patch.object(m, 'sql', side_effect=[row, 'UPDATE 1']), patch.object(m, 'update_source') as update, self.assertRaises(ValueError):
            m.host_jobs()
        update.assert_not_called()

    def test_missing_or_failed_exact_main_ci_prevents_download(self):
        for ci in ([], [{'head_sha': 'b'*40, 'head_branch': 'main', 'event': 'push', 'status': 'completed', 'conclusion': 'failure'}], [{'head_sha': 'b'*40, 'head_branch': 'other', 'event': 'push', 'status': 'completed', 'conclusion': 'success'}]):
            with patch.object(m, 'github_json', side_effect=[{'sha': 'b'*40}, {'workflow_runs': ci}]), patch.object(m, 'run') as run, self.assertRaises(ValueError):
                m.update_source()
            run.assert_not_called()

    def test_recovery_configuration_cannot_mount_host_socket_or_replace_entrypoint(self):
        values = dict(self.values, VELORAY_SECRET_DIRECTORY=str(self.config/'secrets'), VELORAY_TEMPLATE_DIRECTORY=str(self.config/'templates'), VELORAY_CERTIFICATE_DIRECTORY=str(self.config/'certificates'))
        m.write_env(values)
        for agent in ({'privileged': True}, {'entrypoint': ['/bin/sh']}, {'environment': {'VELORAY_XRAY_BINARY': '/bin/sh'}}, {'volumes': [{'type': 'bind', 'source': '/var/run/docker.sock', 'target': '/run/docker.sock'}]}, {'cap_add': ['SYS_ADMIN']}):
            (self.config/'override.json').write_text(json.dumps({'services': {'agent': agent}}))
            with self.subTest(agent=agent), self.assertRaises(ValueError):
                m.validate_recovery_configuration(self.config)

    def test_certificate_import_checks_name_key_pair_and_private_files(self):
        certificate, key, other = self.root/'cert.pem', self.root/'key.pem', self.root/'other.pem'
        subprocess.run(['openssl','req','-x509','-newkey','ec','-pkeyopt','ec_paramgen_curve:P-256','-nodes','-days','2','-keyout',str(key),'-out',str(certificate),'-subj','/CN=vpn.example','-addext','subjectAltName=DNS:vpn.example'], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        subprocess.run(['openssl','genpkey','-algorithm','EC','-pkeyopt','ec_paramgen_curve:P-256','-out',str(other)], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        with self.assertRaises(ValueError):
            m.import_certificate('other.example', certificate, key)
        with self.assertRaises(ValueError):
            m.import_certificate('vpn.example', certificate, other)
        self.assertTrue(m.import_certificate('vpn.example', certificate, key))
        self.assertFalse(m.import_certificate('vpn.example', certificate, key))
        self.assertEqual((self.config/'certificates/vpn.example/privkey.pem').stat().st_mode & 0o777, 0o444)

    def test_certificate_sync_prefers_public_pair_and_restarts_only_on_change(self):
        source = self.root/'caddy'
        for issuer, days in (('local', '3'), ('acme', '2')):
            directory = source/issuer/'vpn.example'
            directory.mkdir(parents=True)
            subprocess.run(['openssl','req','-x509','-newkey','ec','-pkeyopt','ec_paramgen_curve:P-256','-nodes','-days',days,'-keyout',str(directory/'vpn.example.key'),'-out',str(directory/'vpn.example.crt'),'-subj','/CN=vpn.example','-addext','subjectAltName=DNS:vpn.example'], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        m.write_env(dict(self.values, VELORAY_TLS_MODE='auto'))
        original = m.run
        def transport(args, **kwargs):
            if args[:2] == ['docker', 'cp']:
                shutil.copytree(source, args[-1], dirs_exist_ok=True)
                return subprocess.CompletedProcess(args, 0)
            return original(args, **kwargs)
        with patch.object(m,'run',side_effect=transport), patch.object(m,'cid',return_value='synthetic-proxy'), patch.object(m,'compose') as compose, patch.object(m,'up'):
            m.sync_certificate()
            self.assertEqual((self.config/'certificates/vpn.example/fullchain.pem').read_bytes(), (source/'acme/vpn.example/vpn.example.crt').read_bytes())
            compose.assert_called_once_with('restart','agent')
            m.sync_certificate()
            compose.assert_called_once_with('restart','agent')


if __name__ == '__main__':
    unittest.main()
