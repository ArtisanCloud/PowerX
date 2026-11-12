import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest

import yaml

spec = importlib.util.spec_from_file_location('develop_storage', Path(__file__).with_name('configure-develop-storage.py'))
storage = importlib.util.module_from_spec(spec)
spec.loader.exec_module(storage)


class DevelopStoragePlanTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.config = self.root / 'config.yaml'
        self.original = b'# preserved comment\ndeployment:\n  env: dev\ndatabase:\n  dsn: fixture-private-connection\n'
        self.config.write_bytes(self.original)
        self.config.chmod(0o600)
        self.media = self.root / 'storage'
        self.media.mkdir()
        self.backups = self.root / 'private'
        self.backups.mkdir(mode=0o700)

    def plan(self):
        return storage.make_plan(self.config, self.media, 'host-fixture', 'fs-fixture', 'pg-fixture')

    def test_explicit_apply_preserves_other_configuration_and_never_enables_deletion(self):
        plan = self.plan()
        self.assertEqual(self.original, self.config.read_bytes())
        result = storage.apply_plan(plan, self.backups / 'config.before.yaml')
        self.assertTrue(result['restart_required'])
        self.assertFalse(result['automatic_cleanup_enabled'])
        self.assertEqual(self.original, (self.backups / 'config.before.yaml').read_bytes())
        after = yaml.safe_load(self.config.read_bytes())
        self.assertEqual(yaml.safe_load(self.original), {k: v for k, v in after.items() if k != 'storage_governance'})
        self.assertIn(b'# preserved comment', self.config.read_bytes())
        settings = after['storage_governance']
        for key in ('allow_enforce', 'manage_native_retention', 'admission_enabled', 'advanced_operations_enabled'):
            self.assertFalse(settings[key])
        self.assertTrue(settings['sampling_enabled'])
        self.assertEqual(0o700, (self.media/'storage-governance').stat().st_mode & 0o777)

    def test_concurrent_config_change_is_rejected_before_backup_or_write(self):
        plan = self.plan()
        self.config.write_bytes(self.original + b'web_admin_port: 3101\n')
        with self.assertRaisesRegex(ValueError, 'STORAGE_CONFIG_CHANGED_REPLAN_REQUIRED'):
            storage.apply_plan(plan, self.backups / 'before.yaml')
        self.assertFalse((self.backups/'before.yaml').exists())
        self.assertFalse((self.media/'storage-governance').exists())

    def test_plan_cannot_enable_execution_even_with_recomputed_seal(self):
        plan = self.plan()
        plan['settings']['allow_enforce'] = True
        plan['seal'] = storage.sha(json.dumps({k: v for k, v in plan.items() if k != 'seal'}, sort_keys=True).encode())
        with self.assertRaisesRegex(ValueError, 'STORAGE_PLAN_SETTINGS_INVALID'):
            storage.apply_plan(plan, self.backups/'before.yaml')
        self.assertEqual(self.original, self.config.read_bytes())

    def test_existing_runtime_and_configuration_are_not_reset(self):
        (self.media/'storage-governance').mkdir()
        with self.assertRaisesRegex(ValueError, 'STORAGE_EXISTING_RUNTIME_REVIEW_REQUIRED'):
            self.plan()
        (self.media/'storage-governance').rmdir()
        self.config.write_bytes(self.original+b'storage_governance: {}\n')
        with self.assertRaisesRegex(ValueError, 'STORAGE_EXISTING_CONFIGURATION_REVIEW_REQUIRED'):
            self.plan()

    def test_production_and_symlink_targets_are_rejected(self):
        self.config.write_bytes(self.original.replace(b'env: dev', b'env: prod'))
        with self.assertRaisesRegex(ValueError, 'STORAGE_DEV_ENVIRONMENT_REQUIRED'):
            self.plan()
        self.config.write_bytes(self.original)
        linked = self.root/'linked.yaml'
        linked.symlink_to(self.config)
        with self.assertRaisesRegex(ValueError, 'STORAGE_CONFIG_PATH_INVALID'):
            storage.make_plan(linked, self.media, 'host-fixture', 'fs-fixture', 'pg-fixture')


if __name__ == '__main__':
    unittest.main()
