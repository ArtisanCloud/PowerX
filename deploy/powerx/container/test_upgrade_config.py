import importlib.util
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import yaml
import upgrade_config
import init

ROOT=Path(__file__).resolve().parents[3]


class UpgradeConfigTests(unittest.TestCase):
    def fixture(self, root):
        init.initialize(root/'config',root/'data',ROOT/'backend/etc/config_example.prod.yaml')
        path=root/'config/config.yaml'
        cfg=yaml.safe_load(path.read_text());cfg['install']['status']='installed'
        cfg['storage']['local']['base_path']=str(root/'data/uploads')
        path.write_text(yaml.safe_dump(cfg));path.chmod(0o600)
        return path

    def test_upgrade_preserves_bytes_keys_files_and_mode(self):
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp);path=self.fixture(root)
            before={p:p.read_bytes() for p in (root/'config').iterdir()}
            with patch.dict(os.environ,{'POWERX_UPGRADE_MODE':'1','PUBLIC_ORIGIN':'invalid-for-fresh-init','DEPLOYMENT_ENV':'prod'}):
                init.initialize(root/'config',root/'data')
            self.assertEqual(before,{p:p.read_bytes() for p in (root/'config').iterdir()})
            self.assertEqual(path.stat().st_mode & 0o777,0o600)

    def test_missing_config_never_initializes(self):
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp)
            with patch.dict(os.environ,{'POWERX_UPGRADE_MODE':'1'}):
                with self.assertRaises(upgrade_config.UpgradeConfigError):init.initialize(root/'config',root/'data')
            self.assertFalse((root/'config').exists());self.assertFalse((root/'data').exists())

    def test_invalid_cases_preserve_original(self):
        for case in ['empty','invalid_yaml','not_installed','unsafe_permissions','missing_mount','missing_secret','missing_binding']:
            with self.subTest(case=case),tempfile.TemporaryDirectory() as temp:
                root=Path(temp);path=self.fixture(root)
                cfg=yaml.safe_load(path.read_text())
                if case=='empty':path.write_text('')
                elif case=='invalid_yaml':path.write_text('auth: [broken')
                elif case=='unsafe_permissions':path.chmod(0o644)
                elif case=='missing_mount':(root/'data/runtime').rmdir()
                else:
                    if case=='not_installed':cfg['install']['status']='uninstalled'
                    if case=='missing_secret':cfg['auth']['jwt_secret']=''
                    if case=='missing_binding':cfg['database']['password']=''
                    path.write_text(yaml.safe_dump(cfg))
                original=path.read_bytes()
                with self.assertRaises(upgrade_config.UpgradeConfigError):upgrade_config.validate(path,root/'data')
                self.assertEqual(path.read_bytes(),original)


if __name__=='__main__':unittest.main()
