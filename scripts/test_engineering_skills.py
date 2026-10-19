"""Validator regressions use disposable copies, never the source checkout."""
import importlib.util
from pathlib import Path
import shutil
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('verify_skills', Path(__file__).with_name('verify-engineering-skills.py'))
checker = importlib.util.module_from_spec(spec)
spec.loader.exec_module(checker)


class ValidationTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        source = Path(__file__).resolve().parents[1]
        for name in ['.agents', 'AI']:
            shutil.copytree(source/name, self.root/name)

    def test_valid_bundle(self):
        self.assertEqual(len(checker.verify(self.root)), 6)

    def test_missing_skill_fails(self):
        shutil.rmtree(self.root/'.agents/skills/rc-review')
        with self.assertRaisesRegex(ValueError, 'inventory mismatch'):
            checker.verify(self.root)

    def test_unsupported_frontmatter_fails(self):
        path = self.root/'.agents/skills/rc-review/SKILL.md'
        path.write_text(path.read_text().replace('license: "MIT"', 'disable-model-invocation: "true"'))
        with self.assertRaisesRegex(ValueError, 'Unsupported fields'):
            checker.verify(self.root)

    def test_broken_reference_fails(self):
        (self.root/'AI/engineering-skills.md').unlink()
        with self.assertRaisesRegex(ValueError, 'Missing file'):
            checker.verify(self.root)

    def test_modified_license_fails(self):
        path = self.root/'AI/skill-licenses/pstack.txt'
        path.write_text('truncated notice')
        with self.assertRaisesRegex(ValueError, 'License checksum'):
            checker.verify(self.root)

    def test_escaping_reference_fails(self):
        with self.assertRaisesRegex(ValueError, 'escapes repository'):
            checker.local_file(self.root, Path('../outside'))


if __name__ == '__main__':
    unittest.main()
