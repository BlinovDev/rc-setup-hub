#!/usr/bin/python3
"""Root-owned controller. No caller-controlled executable runs as root."""
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.request

# Root-owned installed modules; separate modules fence production and staging storage.
sys.path.insert(0, "/usr/local/lib/rc-hub")
import staging_media
import production_media

BASE = Path('/srv/international-drift-hub')
SETTINGS = {
    'production': dict(user='international-drift-hub', unit='international-drift-hub.service', port=18080, domain='international-drift-hub.com', env='/etc/international-drift-hub/backend.env', backend=BASE/'backend', frontend=Path('/var/www/international-drift-hub'), project='international-drift-hub', compose=BASE/'ops/postgres.compose.yml'),
    'staging': dict(user='idh-staging', unit='international-drift-hub-staging.service', port=18081, domain='staging.international-drift-hub.com', env='/etc/international-drift-hub-staging/backend.env', backend=BASE/'environments/staging/backend', frontend=Path('/var/www/international-drift-hub-staging'), project='international-drift-hub-staging', compose=BASE/'environments/staging/postgres.compose.yml'),
}

def run(*args, **kw):
    return subprocess.run(args, check=True, timeout=kw.pop('timeout', 180), **kw)

def probe(url, expected=None):
    request = urllib.request.Request(url, headers={'User-Agent': 'curl/8.5.0'})
    with urllib.request.urlopen(request, timeout=5) as response:
        body = response.read(2*1024*1024)
        if response.status != 200:
            raise RuntimeError('HTTP smoke failed')
        if expected and json.loads(body).get('status') != expected:
            raise RuntimeError('Backend health response failed')

def production_gate(shas):
    # Fail closed if GitHub cannot be reached or either SHA is outside main.
    for repo, sha in zip(('rc-setup-hub', 'rc-setup-hub-fe'), shas):
        cache = BASE/'ops/git-cache'/f'{repo}.git'
        if not cache.exists():
            cache.parent.mkdir(parents=True, exist_ok=True)
            run('git', 'init', '--bare', str(cache), stdout=subprocess.DEVNULL)
        run('git', '--git-dir='+str(cache), 'fetch', '--no-tags', 'https://github.com/BlinovDev/'+repo+'.git', '+refs/heads/main:refs/heads/verified-main', stdout=subprocess.DEVNULL)
        run('git', '--git-dir='+str(cache), 'merge-base', '--is-ancestor', sha, 'refs/heads/verified-main')

def validate_release(folder, environment, shas):
    manifest = json.loads((folder/'manifest.json').read_text())
    if (manifest.get('environment'), manifest.get('backend_sha'), manifest.get('frontend_sha')) != (environment, *shas):
        raise RuntimeError('Manifest environment/SHAs mismatch')
    hashes = manifest.get('sha256', {})
    files = {str(p.relative_to(folder)) for p in folder.rglob('*') if p.is_file() and str(p.relative_to(folder)) not in ('manifest.json', '.upload-sha256')}
    if set(hashes) != files:
        raise RuntimeError('Manifest must hash every payload file')
    for name, digest in hashes.items():
        if not re.fullmatch(r'[0-9a-f]{64}', digest) or hashlib.sha256((folder/name).read_bytes()).hexdigest() != digest:
            raise RuntimeError('Artifact checksum mismatch')
    for name in ('international-drift-hub', 'international-drift-hub-migrate'):
        header = (folder/name).read_bytes()[:20]
        if header[:6] != b'\x7fELF\x02\x01' or header[18:20] != b'\xb7\x00':
            raise RuntimeError('Backend binaries must be Linux ARM64 ELF')
    if not (folder/'dist/index.html').is_file():
        raise RuntimeError('Frontend index missing')
    origin = 'https://'+SETTINGS[environment]['domain']
    if not any(origin.encode() in p.read_bytes() for p in (folder/'dist').rglob('*.js')):
        raise RuntimeError('Frontend build lacks correct VITE_API_URL origin')

def upload(environment, shas, release):
    release.parent.mkdir(parents=True, exist_ok=True)
    # Stream to root-owned storage; never trust caller-owned paths or symlinks.
    with tempfile.TemporaryDirectory(prefix='.upload-', dir=release.parent) as scratch:
        scratch = Path(scratch)
        archive = scratch/'payload.tar'
        total = 0
        with archive.open('wb') as output:
            while chunk := sys.stdin.buffer.read(1024*1024):
                total += len(chunk)
                if total > 100*1024*1024:
                    raise RuntimeError('Upload exceeds 100 MiB')
                output.write(chunk)
        digest = hashlib.sha256(archive.read_bytes()).hexdigest()
        if release.exists():
            if (release/'.upload-sha256').read_text() != digest:
                raise RuntimeError('Immutable release already exists with different bytes')
            print('Identical upload already present')
            return
        tree = scratch/'tree'
        tree.mkdir()
        with tarfile.open(archive) as tar:
            seen = set()
            total_size = 0
            for member in tar:
                name = member.name.removeprefix('./').rstrip('/')
                path = Path(name)
                if not name or path.is_absolute() or '..' in path.parts or name in seen:
                    raise RuntimeError('Unsafe or duplicate archive path')
                seen.add(name)
                allowed = name in ('manifest.json', 'international-drift-hub', 'international-drift-hub-migrate', 'dist') or name.startswith('dist/')
                if not allowed or not (member.isdir() or member.isfile()):
                    raise RuntimeError('Archive contains unsupported entry')
                total_size += member.size
                if total_size > 200*1024*1024 or len(seen) > 10000:
                    raise RuntimeError('Expanded archive too large')
                destination = tree/path
                if member.isdir():
                    destination.mkdir(parents=True, exist_ok=True)
                else:
                    destination.parent.mkdir(parents=True, exist_ok=True)
                    with tar.extractfile(member) as source, destination.open('xb') as output:
                        shutil.copyfileobj(source, output)
                    destination.chmod(0o755 if name in ('international-drift-hub', 'international-drift-hub-migrate') else 0o644)
        validate_release(tree, environment, shas)
        (tree/'.upload-sha256').write_text(digest)
        for directory in [tree, *[p for p in tree.rglob('*') if p.is_dir()]]:
            directory.chmod(0o755)
        tree.rename(release)
    print('Uploaded immutable release '+release.name)

def link(target, current):
    temporary = current.with_name('.'+current.name+'-new')
    temporary.unlink(missing_ok=True)
    temporary.symlink_to(target)
    temporary.replace(current)

def deploy(environment, shas, release, c):
    if not release.is_dir():
        raise RuntimeError('Upload the release first')
    validate_release(release, environment, shas)
    env_text = Path(c['env']).read_text()
    if 'REPLACE_ME' in env_text:
        raise RuntimeError('Application secrets/configuration incomplete; root must fill backend.env')
    if not Path('/etc/letsencrypt/live/'+c['domain']+'/fullchain.pem').exists():
        raise RuntimeError('Origin TLS not ready; configure DNS then run rc-hub-enable-staging-tls')
    if environment == 'production':
        production_gate(shas)
    state = BASE/'environments'/environment/'active.json'
    if state.exists() and json.loads(state.read_text()).get('release') == release.name:
        probe('http://127.0.0.1:'+str(c['port'])+'/health', 'ok')
        probe('https://'+c['domain']+'/health', 'ok')
        probe('https://'+c['domain']+'/')
        print('Already deployed; health checks passed')
        return
    run('docker', 'compose', '-p', c['project'], '-f', str(c['compose']), 'up', '-d', '--wait', 'postgres')
    media_hooks = staging_media if environment == 'staging' else production_media
    media_hooks.preflight(release, c, env_text)
    media_hooks.coherent_backup()
    run('systemd-run', '--quiet', '--wait', '--collect', '--pipe', '--property=User='+c['user'], '--property=Group='+c['user'], '--property=EnvironmentFile='+c['env'], '--property=NoNewPrivileges=yes', '--property=ProtectSystem=strict', '--property=ProtectHome=yes', '--property=PrivateTmp=yes', '--property=CapabilityBoundingSet=', '--property=UMask=0077', str(release/'international-drift-hub-migrate'), 'up')
    # Probe candidate on a reserved loopback port before activating it.
    candidate = c['project']+'-candidate.service'
    candidate_port = c['port']+100
    candidate_env = Path('/run/'+c['project']+'-candidate.env')
    candidate_env.write_text(re.sub(r'^HTTP_ADDR=.*$', 'HTTP_ADDR=127.0.0.1:'+str(candidate_port), env_text, flags=re.MULTILINE))
    candidate_since = time.strftime('%Y-%m-%d %H:%M:%S UTC', time.gmtime())
    writable = ['--property=ReadWritePaths='+str(media_hooks.MEDIA)]
    try:
        run('systemd-run', '--quiet', '--collect', '--unit='+candidate, '--property=User='+c['user'], '--property=Group='+c['user'], '--property=EnvironmentFile='+str(candidate_env), '--property=NoNewPrivileges=yes', '--property=ProtectSystem=strict', '--property=ProtectHome=yes', '--property=PrivateTmp=yes', '--property=CapabilityBoundingSet=', *writable, str(release/'international-drift-hub'))
        for attempt in range(20):
            try:
                probe('http://127.0.0.1:'+str(candidate_port)+'/health', 'ok')
                break
            except Exception:
                if attempt == 19:
                    raise RuntimeError('Candidate health failed; active release unchanged')
                time.sleep(1)
    except Exception:
        media_hooks.candidate_diagnostics(candidate, candidate_since, env_text)
        raise
    finally:
        subprocess.run(['systemctl', 'stop', candidate], check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        candidate_env.unlink(missing_ok=True)
    # Link activation does not alter the existing unit or production environment.
    c['backend'].mkdir(parents=True, exist_ok=True)
    link(release, c['backend']/'current')
    run('systemctl', 'restart', c['unit'])
    for attempt in range(20):
        try:
            probe('http://127.0.0.1:'+str(c['port'])+'/health', 'ok')
            break
        except Exception:
            if attempt == 19:
                raise RuntimeError('Active backend health failed; inspect journal. No DB rollback performed.')
            time.sleep(1)
    # Retain old hashed assets, without changing the immutable uploaded release.
    web_release = c['frontend']/'releases'/release.name
    if not web_release.exists():
        shutil.copytree(release/'dist', web_release)
        old_assets = c['frontend']/'current/assets'
        if old_assets.exists():
            for old in old_assets.rglob('*'):
                if old.is_file():
                    target = web_release/'assets'/old.relative_to(old_assets)
                    if not target.exists():
                        target.parent.mkdir(parents=True, exist_ok=True)
                        shutil.copyfile(old, target)
        for path in web_release.rglob('*'):
            path.chmod(0o755 if path.is_dir() else 0o644)
        web_release.chmod(0o755)
    link(web_release, c['frontend']/'current')
    probe('https://'+c['domain']+'/health', 'ok')
    probe('https://'+c['domain']+'/')
    state.write_text(json.dumps(dict(release=release.name, backend_sha=shas[0], frontend_sha=shas[1], deployed_at=time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())))+'\n')
    print('Deployment passed backend and frontend smoke checks')

def main():
    os.umask(0o077)
    if os.geteuid() != 0:
        raise RuntimeError('Controller requires scoped sudo')
    args = sys.argv[1:]
    if len(args) not in (2,4) or args[0] not in ('upload','deploy','status') or args[1] not in SETTINGS:
        raise RuntimeError('Invalid deployment command')
    action, environment = args[:2]
    c = SETTINGS[environment]
    if action == 'status':
        if len(args) != 2:
            raise RuntimeError('Invalid status command')
        run('systemctl', 'is-active', c['unit'])
        probe('http://127.0.0.1:'+str(c['port'])+'/health', 'ok')
        print(environment+' healthy')
        return
    if len(args) != 4 or any(not re.fullmatch('[0-9a-f]{40}', sha) for sha in args[2:]):
        raise RuntimeError('Use full lowercase Git SHAs')
    shas = args[2:]
    release = BASE/'environments'/environment/'releases'/('-'.join(shas))
    with open('/run/lock/rc-hub-'+environment+'.lock', 'w') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        if action == 'upload':
            upload(environment, shas, release)
        else:
            deploy(environment, shas, release, c)

if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        # Avoid exposing application env values in subprocess error messages.
        print('Deployment failed: '+str(error), file=sys.stderr)
        sys.exit(1)
