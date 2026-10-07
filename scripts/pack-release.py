#!/usr/bin/env python3
"""Package CI outputs for RC Setup Hub deployment."""
import hashlib
import json
from pathlib import Path
import re
import sys
import tarfile

if len(sys.argv) != 6:
    sys.exit('Usage: pack-release.py ENV BACKEND_SHA FRONTEND_SHA PAYLOAD_DIRECTORY OUTPUT_TAR')
environment, backend_sha, frontend_sha, folder, output = sys.argv[1:]
if environment not in ('staging', 'production') or any(not re.fullmatch('[0-9a-f]{40}', sha) for sha in (backend_sha, frontend_sha)):
    sys.exit('Invalid environment or SHAs')
root = Path(folder).resolve()
output = Path(output).resolve()
if root == output or root in output.parents:
    sys.exit('Output archive must be outside the payload directory')
files = []
for path in sorted(root.rglob('*')):
    if path.is_symlink():
        sys.exit('Symlinks are not permitted')
    if path.is_file() and path != root/'manifest.json':
        name = str(path.relative_to(root))
        if name not in ('international-drift-hub','international-drift-hub-migrate') and not name.startswith('dist/'):
            sys.exit('Unexpected payload file: '+name)
        files.append(path)
manifest = dict(environment=environment, backend_sha=backend_sha, frontend_sha=frontend_sha, sha256={str(path.relative_to(root)): hashlib.sha256(path.read_bytes()).hexdigest() for path in files})
(root/'manifest.json').write_text(json.dumps(manifest, sort_keys=True, indent=2)+'\n')
with tarfile.open(output, 'w', format=tarfile.USTAR_FORMAT) as archive:
    for path in sorted([*files, root/'manifest.json']):
        info = archive.gettarinfo(str(path), arcname=str(path.relative_to(root)))
        info.uid = info.gid = info.mtime = 0
        info.uname = info.gname = ''
        info.mode = 0o755 if path.parent == root and path.name != 'manifest.json' else 0o644
        with path.open('rb') as data:
            archive.addfile(info, data)
print(output)
