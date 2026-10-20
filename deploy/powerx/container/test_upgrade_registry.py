import contextlib
import io
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import upgrade_registry


class UpgradeRegistryTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.output = self.root / 'backend/cmd/database/upgrade_payloads.json'
        for name, value in [('ROOT', self.root), ('OUTPUT', self.output)]:
            patcher = mock.patch.object(upgrade_registry, name, value)
            patcher.start()
            self.addCleanup(patcher.stop)
        for path in upgrade_registry.COMMON:
            self.write(path, 'original payload\n')

    def write(self, path, content):
        target = self.root / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content)

    def checksums(self):
        return {item['name']: item['checksum'] for item in upgrade_registry.payloads()}

    def run_registry(self, *args):
        with mock.patch('sys.argv', ['upgrade_registry.py', *args]):
            return upgrade_registry.main()

    def test_baseline_migration_dependencies_change_only_core_identity(self):
        # 此清单独立于生成器 glob，防止新增迁移依赖漏入载荷清单。
        for path in [
            'backend/pkg/corex/iam/authorizationbaseline/bootstrap.go',
            'backend/pkg/corex/db/persistence/repository/authorization/facts_repo.go',
            'backend/pkg/corex/iam/rootrole/root_role.go',
            'backend/pkg/corex/iam/const.go',
        ]:
            with self.subTest(path=path):
                self.write(path, 'original source\n')
                before = self.checksums()
                self.write(path, 'changed migration source\n')
                after = self.checksums()
                self.assertNotEqual(before['core'], after['core'])
                for group in ['agent', 'knowledge', 'catalog']:
                    self.assertEqual(before[group], after[group])

    def test_test_only_changes_preserve_migration_identity(self):
        before = self.checksums()
        self.write('backend/pkg/corex/iam/rootrole/root_role_test.go', 'test source\n')
        self.write('backend/pkg/corex/db/database/authorization_baseline_postgres_test.go', 'test source\n')
        self.assertEqual(before, self.checksums())

    def test_check_rejects_stale_payload_until_explicit_regeneration(self):
        self.assertEqual(0, self.run_registry())
        original = self.output.read_bytes()
        self.write('backend/pkg/corex/iam/rootrole/root_role.go', 'new migration dependency\n')
        error = io.StringIO()
        with contextlib.redirect_stderr(error):
            self.assertEqual(1, self.run_registry('--check'))
        self.assertIn('UPGRADE_PAYLOAD_CHANGED', error.getvalue())
        self.assertEqual(original, self.output.read_bytes(), 'check must not rewrite the manifest')
        self.assertEqual(0, self.run_registry())
        regenerated = self.output.read_bytes()
        self.assertNotEqual(original, regenerated)
        self.assertEqual(0, self.run_registry('--check'))
        self.assertEqual(0, self.run_registry())
        self.assertEqual(regenerated, self.output.read_bytes(), 'regeneration must be deterministic')


if __name__ == '__main__':
    unittest.main()
