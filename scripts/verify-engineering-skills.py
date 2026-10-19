#!/usr/bin/env python3
"""Offline validation of this foundation's JSON-quoted YAML subset; not a Codex runtime test."""
import hashlib
import json
from pathlib import Path
import re
import sys

EXPECTED = {'rc-implement', 'rc-architect', 'rc-regression', 'rc-blast-radius',
            'rc-review', 'rc-thermo-review'}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def local_file(root, path):
    target = root/path
    require(not target.is_symlink(), 'Symlink is not a vendored file: '+str(path))
    try:
        target.resolve().relative_to(root.resolve())
    except ValueError:
        raise ValueError('Path escapes repository: '+str(path))
    require(target.is_file(), 'Missing file: '+str(path))
    return target


def verify(root):
    root = Path(root)
    skills = root/'.agents/skills'
    found = {p.parent.name for p in skills.glob('*/SKILL.md')}
    require(found == EXPECTED, 'Skill inventory mismatch: '+str(sorted(found)))
    require({p.name for p in skills.iterdir() if p.is_dir()} == EXPECTED,
            'Unexpected skill directory or missing SKILL.md')
    for name in sorted(found):
        path = local_file(root, Path('.agents/skills')/name/'SKILL.md')
        text = path.read_text(encoding='utf-8')
        parts = text.split('---\n', 2)
        require(len(parts) == 3 and parts[0] == '', 'Invalid YAML frontmatter: '+name)
        fields = {}
        for line in parts[1].splitlines():
            key, separator, value = line.partition(': ')
            require(separator and key not in fields, 'Invalid/duplicate frontmatter field: '+name)
            # JSON double-quoted scalars are valid YAML; no third-party YAML dependency.
            fields[key] = json.loads(value)
        require(set(fields) == {'name', 'description', 'license'}, 'Unsupported fields: '+name)
        require(fields['name'] == name and len(name) <= 64, 'Invalid name: '+name)
        require(isinstance(fields['description'], str) and 0 < len(fields['description']) <= 1024,
                'Invalid description: '+name)
        require(fields['license'] == 'MIT' and parts[2].strip(), 'Missing license/body: '+name)
        for link in re.findall(r'\]\(([^\s)]+)\)', parts[2]):
            if '://' not in link and not link.startswith('#'):
                local_file(root, path.parent.relative_to(root)/link.split('#')[0])
    manifest = json.loads(local_file(root, Path('AI/skills-sources.json')).read_text())
    require(manifest['schema_version'] == 1, 'Unknown provenance schema')
    require({s['id'] for s in manifest['sources']} == {'ponytail', 'pstack', 'cursor-plugins'},
            'Missing upstream provenance')
    for source in manifest['sources']:
        require(re.fullmatch('[0-9a-f]{40}', source['revision']), 'Unpinned upstream revision')
        require(source['license'] == 'MIT', 'Review changed upstream license')
        license_file = local_file(root, Path(source['license_file']))
        require(hashlib.sha256(license_file.read_bytes()).hexdigest() == source['license_sha256'],
                'License checksum mismatch: '+source['id'])
        require(source['files'], 'Missing upstream input files')
        for item in source['files']:
            require(re.fullmatch('[0-9a-f]{64}', item['sha256']), 'Invalid upstream input checksum')
            require(not Path(item['path']).is_absolute() and '..' not in Path(item['path']).parts,
                    'Invalid upstream input path')
    return sorted(found)


if __name__ == '__main__':
    try:
        names = verify(Path(__file__).resolve().parents[1])
    except (ValueError, KeyError, OSError, TypeError) as error:
        sys.exit('FAIL: '+str(error))
    print('PASS: skill structure, local links, pinned provenance and license bytes')
    print('\n'.join(names))
    print('Codex discovery/invocation and Cloud behavior require the smoke procedure in AI/engineering-skills.md.')
