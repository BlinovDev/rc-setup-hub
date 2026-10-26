"""Production-only host gates; installed root-owned beside the deployment controller.

No caller-selected filesystem paths or root execution of release binaries.
Staging deliberately does not call this module.
"""
import hashlib
import json
import os
from pathlib import Path
import shlex
import subprocess
import tarfile
import tempfile
import time
from urllib.parse import unquote, urlsplit

MEDIA = Path('/var/lib/international-drift-hub/media')
MAINTENANCE = Path('/run/rc-hub-production-maintenance')
KEY = Path('/etc/international-drift-hub/backup.key')
BACKUPS = Path('/var/backups/international-drift-hub')
UNIT = 'international-drift-hub.service'
COMPOSE = ['docker', 'compose', '-p', 'international-drift-hub', '-f',
           '/srv/international-drift-hub/ops/postgres.compose.yml',
           'exec', '-T', 'postgres']


def digest_file(path):
    digest = hashlib.sha256()
    with Path(path).open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def run(*args, **kwargs):
    return subprocess.run(args, check=True, timeout=kwargs.pop('timeout', 180), **kwargs)


def media_path(env_text):
    values = [shlex.split(line.split('=', 1)[1]) for line in env_text.splitlines()
              if line.startswith('MEDIA_DIR=')]
    if values != [[str(MEDIA)]]:
        raise RuntimeError('Production MEDIA_DIR must use the dedicated provisioned directory')
    if MEDIA.is_symlink() or not MEDIA.is_dir():
        raise RuntimeError('Production private media directory is not provisioned')
    return MEDIA


def sandbox(c, env_file):
    return ['--property=User='+c['user'], '--property=Group='+c['user'],
            '--property=EnvironmentFile='+str(env_file), '--property=NoNewPrivileges=yes',
            '--property=ProtectSystem=strict', '--property=ProtectHome=yes',
            '--property=PrivateTmp=yes', '--property=CapabilityBoundingSet=',
            '--property=UMask=0077', '--property=ReadWritePaths='+str(MEDIA)]


def preflight(release, c, env_text):
    media_path(env_text)
    # Identical writable mount and runtime identity to the candidate; no DB needed.
    run('systemd-run', '--quiet', '--wait', '--collect', '--pipe',
        *sandbox(c, c['env']), str(release/'international-drift-hub'), '--check-media')


def coherent_backup(kind='predeploy'):
    """Fail closed: restart/reopen only after an encrypted matched snapshot exists."""
    if kind not in ('predeploy', 'daily'):
        raise ValueError('Invalid snapshot kind')
    if not KEY.is_file() or KEY.stat().st_mode & 0o077:
        raise RuntimeError('Private production backup encryption key is not provisioned')
    BACKUPS.mkdir(mode=0o700, parents=True, exist_ok=True)
    stamp = kind+'-'+time.strftime('%Y%m%dT%H%M%SZ', time.gmtime())
    destination = BACKUPS/(stamp+'.tar.enc')
    was_active = subprocess.run(['systemctl', 'is-active', '--quiet', UNIT]).returncode == 0
    MAINTENANCE.touch(mode=0o644)
    # nginx rejects new API/auth/admin requests; allow the documented body deadline.
    time.sleep(45)
    run('systemctl', 'stop', UNIT)
    # On ANY backup error leave production stopped and its maintenance gate closed.
    with tempfile.TemporaryDirectory(prefix='.snapshot-', dir=BACKUPS) as scratch:
        scratch = Path(scratch)
        db = scratch/'database.dump'
        with db.open('xb') as output:
            run(*COMPOSE, 'sh', '-c',
                'exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --format=custom --no-owner --no-acl', stdout=output)
        with db.open('rb') as source:
            run(*COMPOSE, 'pg_restore', '--list', stdin=source, stdout=subprocess.DEVNULL)
        with tarfile.open(scratch/'media.tar', 'w') as archive:
            def exclude_upload(info):
                return None if any(part.startswith('.upload-') for part in Path(info.name).parts) else info
            archive.add(MEDIA, arcname='media', filter=exclude_upload)
        state = Path('/srv/international-drift-hub/environments/production/active.json')
        schema = run(*COMPOSE, 'sh', '-c',
                     'exec psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "SELECT max(version_id) FROM goose_db_version WHERE is_applied"',
                     capture_output=True, text=True).stdout.strip()
        versions = run('dpkg-query', '-W', '-f=${Package}=${Version}\n', 'imagemagick',
                       'libheif1', 'libheif-plugin-libde265', 'libde265-0', capture_output=True, text=True).stdout
        metadata = dict(snapshot=stamp, environment='production', schema=schema,
                        active=json.loads(state.read_text()) if state.exists() else None,
                        decoder_versions=versions,
                        sha256={p.name: digest_file(p)
                                for p in [db, scratch/'media.tar']})
        (scratch/'manifest.json').write_text(json.dumps(metadata, indent=2)+'\n')
        with tarfile.open(scratch/'snapshot.tar', 'w') as archive:
            for name in ['database.dump', 'media.tar', 'manifest.json']:
                archive.add(scratch/name, arcname=name)
        encrypted = scratch/'snapshot.tar.enc'
        run('openssl', 'enc', '-aes-256-cbc', '-salt', '-pbkdf2', '-iter', '200000',
            '-pass', 'file:'+str(KEY), '-in', str(scratch/'snapshot.tar'), '-out', str(encrypted))
        # Verify decryptability and matched archive checksums BEFORE reopening.
        verified = scratch/'verified.tar'
        run('openssl', 'enc', '-d', '-aes-256-cbc', '-pbkdf2', '-iter', '200000',
            '-pass', 'file:'+str(KEY), '-in', str(encrypted), '-out', str(verified))
        with tarfile.open(verified) as archive:
            for name, digest in metadata['sha256'].items():
                with archive.extractfile(name) as source:
                    verified_digest = hashlib.sha256()
                    for chunk in iter(lambda: source.read(1024 * 1024), b''):
                        verified_digest.update(chunk)
                    if verified_digest.hexdigest() != digest:
                        raise RuntimeError('Production snapshot verification failed')
        digest = digest_file(encrypted)
        encrypted.rename(destination)
        destination.with_suffix(destination.suffix+'.sha256').write_text(digest+'  '+destination.name+'\n')
    if was_active:
        run('systemctl', 'start', UNIT)
    MAINTENANCE.unlink()
    # Only prune complete sets after a newer verified set exists.
    for prefix, keep, age in [('daily-', 7, None), ('predeploy-', None, 30*86400)]:
        files = sorted(BACKUPS.glob(prefix+'*.tar.enc'), reverse=True)
        for index, path in enumerate(files):
            if (keep is not None and index >= keep) or (age is not None and time.time()-path.stat().st_mtime > age):
                path.unlink()
                path.with_suffix(path.suffix+'.sha256').unlink(missing_ok=True)
    print('Verified encrypted production DB+media snapshot: '+destination.name)
    return destination


def candidate_diagnostics(candidate, since, env_text):
    try:
        result = subprocess.run(['journalctl', '--unit='+candidate, '--since='+since,
                             '--no-pager', '-o', 'cat', '-n', '50'],
                                capture_output=True, text=True, timeout=15)
    except (OSError, subprocess.TimeoutExpired):
        print('Candidate journal unavailable; inspect the candidate unit journal on the host.', file=__import__('sys').stderr)
        return
    text = result.stdout
    # Never echo environment values, including a DB password embedded in a URL.
    secrets = []
    for line in env_text.splitlines():
        if '=' not in line or line.startswith('#'):
            continue
        raw = line.split('=', 1)[1]
        secrets.append(raw.strip('\"\''))
        try:
            secrets.extend(shlex.split(raw))
        except ValueError:
            pass
        if line.startswith('DATABASE_URL='):
            try:
                password = urlsplit(raw.strip('\"\'')).password
                if password:
                    secrets.extend([password, unquote(password)])
            except ValueError:
                pass
    for value in sorted(set(secrets), key=len, reverse=True):
        if value:
            text = text.replace(value, '<redacted>')
    print('Candidate startup journal (environment values redacted):\n'+text, file=__import__('sys').stderr)


if __name__ == '__main__':
    import fcntl
    os.umask(0o077)
    if os.geteuid() != 0:
        raise SystemExit('Root is required for the production snapshot job')
    with open('/run/lock/rc-hub-production.lock', 'w') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        coherent_backup('daily')
