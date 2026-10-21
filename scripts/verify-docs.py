#!/usr/bin/env python3
"""Offline checks for maintained Markdown/navigation, not semantic or Cloud proof."""
import argparse
from collections import Counter
import os
from pathlib import Path
import re
import subprocess
from urllib.parse import unquote, urlsplit

REQUIRED = ('AI/workflow.md', 'AI/product.md', 'AI/architecture.md',
            'AI/development.md', 'AI/api.md', 'AI/testing.md',
            'AI/deployment.md', 'AI/context-smoke.md')
SKIP_DIRS = {'.git', 'node_modules', 'dist', 'coverage', 'artifacts', '__pycache__'}


def markdown_files(root):
    for directory, dirs, names in os.walk(root):
        dirs[:] = sorted(d for d in dirs if d not in SKIP_DIRS)
        for name in sorted(names):
            path = Path(directory)/name
            if name.endswith('.md'):
                rel = path.relative_to(root)
                # Immutable historical text can contain paths from its old location.
                if 'archive' not in rel.parts or rel == Path('AI/archive/README.md'):
                    yield path


def prose(text):
    return re.sub(r'(?ms)^(`{3,}|~{3,})[^\n]*\n.*?^\1\s*$', '', text)


def destinations(text):
    text = prose(text)
    inline = re.findall(r'\]\(([^\s)]+)(?:\s+"[^"]*")?\)', text)
    references = re.findall(r'(?m)^\s*\[[^\]]+\]:\s*(\S+)', text)
    return [value.strip('<>') for value in inline + references]


def anchors(text):
    counts, found = Counter(), set()
    for heading in re.findall(r'(?m)^#{1,6}\s+(.+?)\s*#*$', prose(text)):
        heading = re.sub(r'\[([^\]]+)\]\([^)]*\)', r'\1', heading)
        slug = re.sub(r'[^\w\-\s]', '', heading.lower()).replace(' ', '-')
        number = counts[slug]
        found.add(slug + (f'-{number}' if number else ''))
        counts[slug] += 1
    return found


def validate_links(root, paths):
    root = root.resolve()
    errors = []
    for path in paths:
        for link in destinations(path.read_text(encoding='utf-8')):
            url = urlsplit(link)
            if url.scheme or url.netloc:
                continue  # External URLs are deliberately not fetched.
            target = (root/unquote(url.path).lstrip('/') if url.path.startswith('/')
                      else path.parent/unquote(url.path) if url.path else path)
            target = target.resolve()
            label = f'{path.relative_to(root)}: {link}'
            if not target.is_relative_to(root):
                errors.append(label+' escapes repository')
            elif not target.exists():
                errors.append(label+' is missing')
            elif url.fragment and target.suffix == '.md' and unquote(url.fragment) not in anchors(target.read_text(encoding='utf-8')):
                errors.append(label+' has missing heading anchor')
    return errors


def verify_router(root):
    errors = []
    router = root/'AGENTS.md'
    if not router.is_file():
        return ['Missing AGENTS.md']
    links = destinations(router.read_text(encoding='utf-8'))
    for name in REQUIRED:
        if name not in links:
            errors.append('AGENTS.md must route to '+name)
    if any(link.startswith('AI/archive/') for link in links):
        errors.append('AGENTS.md must not load archive files directly')
    for name in ('.github/ISSUE_TEMPLATE/change.md', '.github/pull_request_template.md'):
        if not (root/name).is_file():
            errors.append('Missing template: '+name)
    return errors


def core(path):
    return (path in {'README.md', 'AGENTS.md', 'backend/README.md', 'backend/AGENTS.md'}
            or path.startswith('AI/') and path.count('/') == 1 and path.endswith('.md'))


def statistics(root, base):
    def git(*args):
        return subprocess.check_output(['git', *args], cwd=root)
    sha = git('rev-parse', '--verify', base+'^{commit}').decode().strip()
    before = {p: git('show', sha+':'+p) for p in git('ls-tree', '-r', '--name-only', sha).decode().splitlines() if core(p)}
    after = {str(p.relative_to(root)): p.read_bytes() for p in markdown_files(root) if core(str(p.relative_to(root)))}
    def size(values):
        return {'files': len(values), 'lines': sum(len(v.splitlines()) for v in values), 'bytes': sum(len(v) for v in values)}
    print('Active core = root README/AGENTS, AI/*.md, backend README/AGENTS if present; archives/skills excluded.')
    print('Before:', size(list(before.values())))
    print('After: ', size(list(after.values())))
    # The former router mandated these four; the new router requires only two.
    old_entry = ('AGENTS.md', 'AI/main.md', 'AI/product.md', 'AI/workflow.md')
    new_entry = ('AGENTS.md', 'AI/workflow.md')
    print('Default context before:', size([before[p] for p in old_entry]))
    print('Default context after: ', size([after[p] for p in new_entry]))
    print('Historical archive (not default context):', size([p.read_bytes() for p in (root/'AI/archive').glob('*.md')]))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--stats', metavar='BASE_REF', help='Compare active core/default context against a Git revision')
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    files = list(markdown_files(root))
    errors = verify_router(root) + validate_links(root, files)
    if errors:
        raise SystemExit('\n'.join(errors))
    print(f'PASS: {len(files)} maintained Markdown files, local links/heading anchors, task routing and templates')
    print('Not checked: remote URL availability, historical archive bodies, semantic policy equivalence or Codex Cloud behavior.')
    if args.stats:
        statistics(root, args.stats)


if __name__ == '__main__':
    main()
