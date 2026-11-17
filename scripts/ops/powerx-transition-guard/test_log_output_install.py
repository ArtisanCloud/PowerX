import hashlib
import json
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch
import install_log_output_health as installer


class InstallTests(unittest.TestCase):
    def fixture(self, root):
        root = root.resolve()
        source = root / 'package'; source.mkdir()
        target = root / 'target'; target.mkdir(mode=0o755)
        old = b'print("original monitor")\n'
        (target / 'capacity.py').write_bytes(old); (target / 'capacity.py').chmod(0o755)
        for name in installer.NAMES: (source / name).write_text('print("new validated monitor")\n')
        manifest = {'target': str(target), 'original_capacity_sha256': hashlib.sha256(old).hexdigest(),
                    'files': {name: installer.digest(source / name) for name in installer.NAMES}}
        (source / 'manifest.json').write_text(json.dumps(manifest))
        original_stat = Path.lstat
        def owned_stat(path, *args, **kwargs):
            value = original_stat(path, *args, **kwargs)
            if path in (target, target / 'capacity.py'):
                return SimpleNamespace(st_mode=value.st_mode, st_uid=0, st_nlink=value.st_nlink)
            return value
        return source, target, old, owned_stat

    def test_changed_deployed_script_refuses_overwrite(self):
        with tempfile.TemporaryDirectory() as directory:
            source, target, _, owned = self.fixture(Path(directory))
            (target / 'capacity.py').write_text('print("other operator change")\n')
            with patch.object(installer, '__file__', str(source / 'install.py')), patch.object(installer, 'TARGET', target), patch.object(Path, 'lstat', owned):
                with self.assertRaisesRegex(RuntimeError, 'changed_since_preparation'):
                    installer.run(True)
            self.assertIn('other operator change', (target / 'capacity.py').read_text())
            self.assertFalse((target / 'log_output_health.py').exists())

    def test_dry_run_is_readonly_then_install_keeps_backup_and_is_idempotent(self):
        with tempfile.TemporaryDirectory() as directory:
            source, target, old, owned = self.fixture(Path(directory))
            with patch.object(installer, '__file__', str(source / 'install.py')), patch.object(installer, 'TARGET', target), patch.object(Path, 'lstat', owned), patch.object(installer.os, 'geteuid', return_value=0):
                self.assertEqual(installer.run(False)['state'], 'ready')
                self.assertEqual((target / 'capacity.py').read_bytes(), old)
                self.assertFalse((target / 'log_output_health.py').exists())
                report = installer.run(True)
                self.assertEqual(report['state'], 'installed')
                self.assertEqual((Path(report['backup']) / 'capacity.py').read_bytes(), old)
                self.assertEqual(installer.run(True)['state'], 'already_installed')

    def test_dependency_rolls_back_if_monitor_replacement_fails(self):
        with tempfile.TemporaryDirectory() as directory:
            source, target, old, owned = self.fixture(Path(directory))
            replace = installer.os.replace
            def fail_capacity(a, b):
                if b == target / 'capacity.py': raise OSError('fixture disk failure')
                return replace(a, b)
            with patch.object(installer, '__file__', str(source / 'install.py')), patch.object(installer, 'TARGET', target), patch.object(Path, 'lstat', owned), patch.object(installer.os, 'geteuid', return_value=0), patch.object(installer.os, 'replace', side_effect=fail_capacity):
                with self.assertRaises(OSError): installer.run(True)
            self.assertEqual((target / 'capacity.py').read_bytes(), old)
            self.assertFalse((target / 'log_output_health.py').exists())


if __name__ == '__main__': unittest.main()
