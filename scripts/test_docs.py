"""Behavioral regressions for offline docs checking using disposable files."""
import importlib.util
import contextlib
import io
from unittest.mock import patch
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('verify_docs', Path(__file__).with_name('verify-docs.py'))
checker = importlib.util.module_from_spec(spec)
spec.loader.exec_module(checker)


class DocsTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name).resolve()
        self.doc = self.root/'AGENTS.md'
        self.target = self.root/'target.md'
        self.target.write_text('# Current rules\n## Repeated\n## Repeated\n')

    def check(self, text):
        self.doc.write_text(text)
        return checker.validate_links(self.root, [self.doc])

    def test_local_reference_anchor_and_external_links(self):
        self.assertEqual(self.check('[rules](target.md#current-rules)\n[repeat](target.md#repeated-1)\n[remote](https://example.invalid/not-fetched)\n[ref]: target.md#current-rules\n'), [])

    def test_missing_file_and_heading_fail(self):
        self.assertIn('is missing', self.check('[lost](lost.md)')[0])
        self.assertIn('missing heading', self.check('[lost](target.md#old-rules)')[0])

    def test_escape_including_symlink_fails(self):
        self.assertIn('escapes', self.check('[escape](../outside.md)')[0])
        (self.root/'escape.md').symlink_to(self.root.parent/'outside.md')
        self.assertIn('escapes', self.check('[escape](escape.md)')[0])

    def test_code_examples_are_not_links(self):
        self.assertEqual(self.check('```md\n[example](missing.md)\n```\n[rules](target.md)\n'), [])

    def test_router_requires_topics_templates_and_no_direct_archive(self):
        self.doc.write_text('\n'.join(f'[topic]({p})' for p in checker.REQUIRED))
        for p in ('.github/ISSUE_TEMPLATE/change.md', '.github/pull_request_template.md'):
            target = self.root/p
            target.parent.mkdir(parents=True, exist_ok=True)
            target.touch()
        self.assertEqual(checker.verify_router(self.root), [])
        anchored = self.doc.read_text().replace('AI/architecture.md)', 'AI/architecture.md#package-boundaries)')
        self.doc.write_text(anchored)
        self.assertEqual(checker.verify_router(self.root), [])
        self.doc.write_text(anchored.replace('AI/architecture.md#package-boundaries', 'https://example.invalid/AI/architecture.md'))
        self.assertTrue(any('AI/architecture.md' in e for e in checker.verify_router(self.root)))
        self.doc.write_text(anchored+'\n[history](AI/archive/poc.md)')
        self.assertTrue(any('archive' in e for e in checker.verify_router(self.root)))
        self.doc.write_text('No routes')
        self.assertTrue(any('AI/testing.md' in e for e in checker.verify_router(self.root)))

    def test_statistics_uses_same_current_entry_set_for_both_revisions(self):
        current = {'AGENTS.md': b'router\n', 'AI/workflow.md': b'workflow\n'}
        old = {**current, 'AI/main.md': b'legacy index\n'*10, 'AI/product.md': b'optional product\n'*10}
        for name, contents in old.items():
            path = self.root/name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(contents)
        def git_output(args, **kwargs):
            if args[1] == 'rev-parse': return b'abc\n'
            if args[1] == 'ls-tree': return ('\n'.join(old)+'\n').encode()
            return old[args[-1].split(':', 1)[1]]
        output = io.StringIO()
        with patch.object(checker.subprocess, 'check_output', side_effect=git_output), contextlib.redirect_stdout(output):
            checker.statistics(self.root, 'BASE')
        lines = output.getvalue().splitlines()
        before = next(line for line in lines if line.startswith('Default context before:'))
        after = next(line for line in lines if line.startswith('Default context after:'))
        self.assertEqual(before.split(':', 1)[1].strip(), after.split(':', 1)[1].strip())
        self.assertIn("'files': 2", before)


if __name__ == '__main__':
    unittest.main()
