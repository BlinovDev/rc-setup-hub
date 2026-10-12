import contextlib
import importlib.util
import io
import hashlib
import json
import shutil
import tarfile
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch, Mock

sys.path.insert(0, str(Path(__file__).parent))
import staging_media as media
spec = importlib.util.spec_from_file_location('controller', Path(__file__).with_name('rc-hub-control.py'))
controller = importlib.util.module_from_spec(spec)
spec.loader.exec_module(controller)


class MediaTests(unittest.TestCase):
    def test_missing_wrong_and_symlink_media_are_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)/'media'
            root.mkdir()
            with patch.object(media, 'MEDIA', root):
                for text in ['', 'MEDIA_DIR=/\n', 'MEDIA_DIR=relative\n', 'MEDIA_DIR=/production/media\n']:
                    with self.assertRaises(RuntimeError):
                        media.media_path(text)
                self.assertEqual(media.media_path('MEDIA_DIR='+str(root)), root)
                root.rmdir()
                root.symlink_to(Path(temp))
                with self.assertRaises(RuntimeError):
                    media.media_path('MEDIA_DIR='+str(root))

    def test_preflight_uses_runtime_identity_and_writable_sandbox(self):
        c = dict(user='idh-staging', env='/private/staging.env')
        with patch.object(media, 'media_path'), patch.object(media, 'run') as run:
            media.preflight(Path('/release'), c, 'MEDIA_DIR=configured')
        args = run.call_args.args
        self.assertIn('--property=User=idh-staging', args)
        self.assertIn('--property=ProtectSystem=strict', args)
        self.assertIn('--property=ReadWritePaths='+str(media.MEDIA), args)
        self.assertEqual(args[-2:], ('/release/international-drift-hub', '--check-media'))

    def test_failed_backup_keeps_staging_stopped_and_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            key = root/'key'; key.write_text('test-only-key'); key.chmod(0o600)
            flag = root/'maintenance'
            calls = []
            def fail_dump(*args, **kwargs):
                calls.append(args)
                if 'pg_dump' in ' '.join(args):
                    raise subprocess.CalledProcessError(1, 'pg_dump')
                return Mock(stdout='')
            with patch.multiple(media, KEY=key, BACKUPS=root/'backups', MAINTENANCE=flag), \
                 patch.object(media.time, 'sleep'), patch.object(media, 'run', side_effect=fail_dump), \
                 patch.object(media.subprocess, 'run', return_value=Mock(returncode=0)):
                with self.assertRaises(subprocess.CalledProcessError):
                    media.coherent_backup()
            self.assertTrue(flag.exists())
            self.assertIn(('systemctl', 'stop', media.UNIT), calls)
            self.assertNotIn(('systemctl', 'start', media.UNIT), calls)
            self.assertEqual(list((root/'backups').glob('*.tar.enc')), [])

    def test_successful_snapshot_pairs_components_and_excludes_abandoned_uploads(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            key = root/'key'; key.write_text('test-only-key'); key.chmod(0o600)
            private = root/'media'; private.mkdir()
            (private/'retained-photo').write_bytes(b'processed-test-image')
            abandoned = private/'.upload-abandoned'; abandoned.mkdir()
            (abandoned/'raw').write_bytes(b'do-not-back-up')
            flag = root/'maintenance'
            calls = []
            def commands(*args, **kwargs):
                calls.append(args)
                if 'pg_dump' in ' '.join(args):
                    kwargs['stdout'].write(b'test-custom-dump')
                if args[0] == 'openssl':
                    shutil.copyfile(args[args.index('-in')+1], args[args.index('-out')+1])
                return Mock(stdout='6' if 'psql' in ' '.join(args) else 'test-decoder-versions')
            with patch.multiple(media, KEY=key, MEDIA=private, BACKUPS=root/'backups', MAINTENANCE=flag), \
                 patch.object(media.time, 'sleep'), patch.object(media, 'run', side_effect=commands), \
                 patch.object(media.subprocess, 'run', return_value=Mock(returncode=0)):
                destination = media.coherent_backup()
            self.assertFalse(flag.exists())
            self.assertIn(('systemctl', 'start', media.UNIT), calls)
            with tarfile.open(destination) as archive:
                manifest = json.load(archive.extractfile('manifest.json'))
                self.assertEqual(manifest['environment'], 'staging')
                self.assertEqual(manifest['schema'], '6')
                for name, digest in manifest['sha256'].items():
                    self.assertEqual(hashlib.sha256(archive.extractfile(name).read()).hexdigest(), digest)
                with tarfile.open(fileobj=archive.extractfile('media.tar')) as images:
                    self.assertIn('media/retained-photo', images.getnames())
                    self.assertFalse(any('.upload-' in name for name in images.getnames()))
            crypto = [args for args in calls if args[0] == 'openssl']
            self.assertEqual(len(crypto), 2)
            self.assertTrue(all('-pbkdf2' in args and '200000' in args for args in crypto))

    def test_missing_backup_key_does_not_interrupt_service(self):
        with tempfile.TemporaryDirectory() as temp, patch.object(media, 'KEY', Path(temp)/'absent'), \
             patch.object(media, 'run') as run:
            with self.assertRaisesRegex(RuntimeError, 'encryption key'):
                media.coherent_backup()
            run.assert_not_called()

    def test_unavailable_journal_does_not_mask_original_failure(self):
        with patch.object(media.subprocess, 'run', side_effect=subprocess.TimeoutExpired('journalctl', 15)), \
             contextlib.redirect_stderr(io.StringIO()) as output:
            media.candidate_diagnostics('candidate', 'now', '')
        self.assertIn('journal unavailable', output.getvalue())

    def test_journal_preserves_cause_but_redacts_credentials(self):
        env = 'SESSION_SECRET=secret-test-value\nDATABASE_URL=postgres://app:p%40ssword@localhost/db\n'
        journal = 'photo storage initialization failed: MEDIA_DIR must be absolute; secret-test-value p@ssword p%40ssword\n'
        output = io.StringIO()
        with patch.object(media.subprocess, 'run', return_value=Mock(stdout=journal)), contextlib.redirect_stderr(output):
            media.candidate_diagnostics('candidate.service', '2026-10-12 16:15:00 UTC', env)
        text = output.getvalue()
        self.assertIn('photo storage initialization failed', text)
        for credential in ['secret-test-value', 'p@ssword', 'p%40ssword']:
            self.assertNotIn(credential, text)


class ControllerTests(unittest.TestCase):
    def candidate_failure(self, environment):
        with tempfile.TemporaryDirectory() as temp:
            base = Path(temp)
            release = base/'release'; release.mkdir()
            env = base/'backend.env'; env.write_text('HTTP_ADDR=127.0.0.1:18081\n')
            c = dict(controller.SETTINGS[environment], env=str(env), backend=base/'backend', frontend=base/'frontend')
            real_exists = Path.exists
            events = []
            commands = []
            def exists(path):
                return str(path).startswith('/etc/letsencrypt/live/') or real_exists(path)
            def run(*args, **kwargs):
                commands.append(args)
                if args[-1] == 'up': events.append('migration')
                return Mock()
            # Candidate env normally lives in /run; redirect only that path for the test.
            original_path = controller.Path
            def path(value):
                if str(value) == '/var/backups':
                    return base
                if str(value).startswith('/run/'):
                    return base/'candidate.env'
                return original_path(value)
            with patch.object(Path, 'exists', exists), patch.object(controller, 'BASE', base), \
                 patch.object(controller, 'Path', side_effect=path), \
                 patch.object(controller, 'validate_release'), patch.object(controller, 'production_gate') as gate, \
                 patch.object(controller, 'run', side_effect=run), patch.object(controller, 'probe', side_effect=RuntimeError('unhealthy')), \
                 patch.object(controller.time, 'sleep'), patch.object(controller.subprocess, 'run') as cleanup, \
                 patch.object(media, 'preflight', side_effect=lambda *a: events.append('preflight')) as preflight, \
                 patch.object(media, 'coherent_backup', side_effect=lambda: events.append('backup')) as backup, \
                 patch.object(media, 'candidate_diagnostics') as diagnostics, patch.object(controller, 'link') as link:
                with self.assertRaisesRegex(RuntimeError, 'Candidate health failed'):
                    controller.deploy(environment, ['a'*40, 'b'*40], release, c)
                link.assert_not_called()
                self.assertFalse((base/'candidate.env').exists())
                if environment == 'staging':
                    self.assertEqual(events[:3], ['preflight', 'backup', 'migration'])
                    diagnostics.assert_called_once()
                    candidate = next(args for args in commands if any(arg.startswith('--unit=') for arg in args))
                    self.assertIn('--property=ReadWritePaths='+str(media.MEDIA), candidate)
                    gate.assert_not_called()
                    self.assertEqual(cleanup.call_args.kwargs['stderr'], subprocess.DEVNULL)
                else:
                    preflight.assert_not_called(); backup.assert_not_called(); diagnostics.assert_not_called()
                    gate.assert_called_once()
                    self.assertEqual(events, ['migration'])

    def test_staging_failure_keeps_release_and_collects_journal(self):
        self.candidate_failure('staging')

    def test_production_does_not_use_staging_hooks(self):
        self.candidate_failure('production')


if __name__ == '__main__':
    unittest.main()
