import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

import yaml

spec = importlib.util.spec_from_file_location('production_storage', Path(__file__).with_name('configure-production-storage.py'))
storage = importlib.util.module_from_spec(spec)
spec.loader.exec_module(storage)


class ProductionStoragePlanTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.config = self.root/'config.yaml'
        self.original = b'# preserved\ndeployment:\n  env: prod\ndatabase:\n  dsn: host=127.0.0.1 dbname=powerx_pro user=fixture\n'
        self.config.write_bytes(self.original)
        self.config.chmod(0o600)
        self.media = self.root/'storage'
        self.media.mkdir()
        self.backups = self.root/'private'
        self.backups.mkdir(mode=0o700)

    def plan(self):
        return storage.make_plan(self.config, self.media, 'host-fixture', 'fs-fixture', 'pg-fixture', expected_database='powerx_pro')

    def test_apply_only_adds_observation_and_preserves_original(self):
        result = storage.apply_plan(self.plan(), self.backups/'before.yaml')
        self.assertFalse(result['automatic_cleanup_enabled'])
        self.assertFalse(result['legacy_handoff'])
        self.assertTrue(result['restart_required'])
        after = yaml.safe_load(self.config.read_bytes())
        settings = after.pop('storage_governance')
        self.assertEqual(after, yaml.safe_load(self.original))
        self.assertEqual((self.backups/'before.yaml').read_bytes(), self.original)
        self.assertIn(b'# preserved', self.config.read_bytes())
        self.assertTrue(settings['sampling_enabled'])
        self.assertTrue(settings['postgres_enabled'])
        for key in ['admission_enabled', 'allow_enforce', 'advanced_operations_enabled', 'manage_native_retention']:
            self.assertFalse(settings[key])
        self.assertEqual((self.media/'storage-governance').stat().st_mode & 0o777, 0o700)

    def test_dev_wrong_database_remote_and_url_override_are_rejected(self):
        inputs = [
            (self.original.replace(b'env: prod', b'env: dev'), 'STORAGE_PROD_ENVIRONMENT_REQUIRED'),
            (self.original.replace(b'powerx_pro', b'powerx_dev'), 'STORAGE_PROD_DATABASE_SCOPE_MISMATCH'),
            (self.original.replace(b'127.0.0.1', b'10.1.2.3'), 'STORAGE_PROD_DATABASE_SCOPE_MISMATCH'),
            (b'deployment: {env: prod}\ndatabase: {dsn: "postgres://fixture@127.0.0.1/powerx_pro?host=10.1.2.3"}\n', 'STORAGE_PROD_DATABASE_SCOPE_MISMATCH'),
        ]
        for content, code in inputs:
            self.config.write_bytes(content)
            with self.assertRaisesRegex(ValueError, code):
                self.plan()
        self.assertFalse((self.media/'storage-governance').exists())

    def test_missing_confirmation_and_mysql_are_rejected(self):
        with self.assertRaisesRegex(ValueError, 'DATABASE_SCOPE'):
            storage.make_plan(self.config, self.media, 'host-fixture', 'fs-fixture', 'pg-fixture')
        self.config.write_bytes(self.original+b'  driver: mysql\n')
        with self.assertRaisesRegex(ValueError, 'POSTGRES_REQUIRED'):
            self.plan()

    def test_changed_configuration_is_not_overwritten(self):
        plan = self.plan()
        changed = self.original+b'web_admin_port: 3000\n'
        self.config.write_bytes(changed)
        with self.assertRaisesRegex(ValueError, 'CONFIG_CHANGED'):
            storage.apply_plan(plan, self.backups/'before.yaml')
        self.assertEqual(self.config.read_bytes(), changed)
        self.assertFalse((self.backups/'before.yaml').exists())

    def test_existing_configuration_runtime_and_symlink_require_review(self):
        self.config.write_bytes(self.original+b'storage_governance: {}\n')
        with self.assertRaisesRegex(ValueError, 'EXISTING_CONFIGURATION'):
            self.plan()
        self.config.write_bytes(self.original)
        runtime = self.media/'storage-governance'
        runtime.mkdir()
        with self.assertRaisesRegex(ValueError, 'EXISTING_RUNTIME'):
            self.plan()
        runtime.rmdir()
        target = self.root/'target.yaml'
        self.config.rename(target)
        self.config.symlink_to(target)
        with self.assertRaisesRegex(ValueError, 'CONFIG_PATH'):
            self.plan()

    def test_resealed_plan_cannot_enable_cleanup(self):
        plan = self.plan()
        plan['settings']['allow_enforce'] = True
        plan['seal'] = storage.sha(json.dumps({k:v for k,v in plan.items() if k!='seal'}, sort_keys=True).encode())
        with self.assertRaisesRegex(ValueError, 'SETTINGS_INVALID'):
            storage.apply_plan(plan, self.backups/'before.yaml')
        self.assertEqual(self.config.read_bytes(), self.original)


if __name__ == '__main__':
    unittest.main()
