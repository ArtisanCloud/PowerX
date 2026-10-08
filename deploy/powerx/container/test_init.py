import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import yaml

ROOT=Path(__file__).resolve().parents[3]
spec=importlib.util.spec_from_file_location('container_init',Path(__file__).with_name('init.py'))
module=importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

class InitTests(unittest.TestCase):
    def test_secrets_are_unique_persistent_and_container_addresses_are_used(self):
        with tempfile.TemporaryDirectory() as temp, patch.dict(os.environ,{'PUBLIC_ORIGIN':'https://dev.example.com','DEPLOYMENT_ENV':'dev'}):
            root=Path(temp)
            module.initialize(root/'config',root/'data',ROOT/'backend/etc/config_example.prod.yaml')
            config=yaml.safe_load((root/'config/config.yaml').read_text())
            self.assertEqual(config['database']['host'],'postgres')
            self.assertEqual(config['queue']['redis']['addr'],'redis:6379')
            self.assertEqual(config['deployment']['env'],'dev')
            self.assertEqual(config['server']['write_timeout_seconds'],0)
            self.assertEqual(config['http_security']['web_admin_origins'],['https://dev.example.com'])
            self.assertEqual(config['storage']['local']['public_base_url'],'https://dev.example.com/media')
            self.assertNotEqual(config['auth']['jwt_secret'],config['database']['password'])
            admin=json.loads((root/'config/initial-admin.json').read_text())
            self.assertGreaterEqual(len(admin['password']),24)
            original=(root/'config/config.yaml').read_bytes()
            module.initialize(root/'config',root/'data',ROOT/'backend/etc/config_example.prod.yaml')
            self.assertEqual((root/'config/config.yaml').read_bytes(),original)
            self.assertEqual((root/'config/config.yaml').stat().st_mode & 0o777,0o600)
            self.assertEqual((root/'config').stat().st_mode & 0o777,0o700)

    def test_incomplete_initialization_is_not_overwritten(self):
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp);(root/'config').mkdir();(root/'config/postgres-password').write_text('existing-secret')
            with self.assertRaises(ValueError):module.initialize(root/'config',root/'data',ROOT/'backend/etc/config_example.prod.yaml')
            self.assertEqual((root/'config/postgres-password').read_text(),'existing-secret')

if __name__=='__main__':unittest.main()
