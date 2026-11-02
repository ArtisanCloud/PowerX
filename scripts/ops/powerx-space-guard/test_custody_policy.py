import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import custody_policy as policy


class ControlPolicyTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name).resolve()
        self.logs = self.root / 'logs'
        self.logs.mkdir()
        self.control = self.root / 'control.json'
        self.binding = self.root / 'binding.json'
        base = Path(policy.__file__).resolve().parent
        binding = {'protocol': 2, 'node_id': 'fixture', 'core_uid': os.geteuid(),
                   'control_file': str(self.control), 'sources': [str(self.logs)]}
        for field, name in [('guard', 'guard.py'), ('helper', 'audit_custody.py'), ('policy', 'custody_policy.py')]:
            binding[field + '_script'] = str(base / name)
            binding[field + '_sha256'] = hashlib.sha256((base / name).read_bytes()).hexdigest()
        self.binding.write_text(json.dumps(binding))
        self.binding.chmod(0o644)
        now = dt.datetime.now(dt.timezone.utc)
        self.value = {'protocol': 2, 'node_id': 'fixture', 'published_at': now.isoformat(),
                      'valid_until': (now + dt.timedelta(minutes=3)).isoformat(),
                      'sources': [{'source_id': 'logs', 'directory': str(self.logs), 'executor': 'powerx', 'archive_enabled': True}]}
        self.write_control()
        self.patch = patch.object(policy, 'BINDING', self.binding)
        self.patch.start()

    def write_control(self):
        self.control.write_text(json.dumps(self.value))
        self.control.chmod(0o600)

    def tearDown(self):
        self.patch.stop()
        self.temp.cleanup()

    def test_no_binding_is_disabled_by_default(self):
        self.binding.unlink()
        self.assertFalse(policy.state_for(self.logs)['allow_archive'])

    def test_native_owner_blocks_old_receiver_even_when_ui_archive_is_on(self):
        state = policy.state_for(self.logs)
        self.assertTrue(state['managed'])
        self.assertFalse(state['allow_archive'])
        self.assertEqual(state['reason'], 'managed_by_powerx')

    def test_shutdown_and_expiry_do_not_return_source_to_old_deleter(self):
        self.value['valid_until'] = '2020-01-01T00:00:00+00:00'
        self.write_control()
        state = policy.state_for(self.logs)
        self.assertTrue(state['managed'])
        self.assertFalse(state['allow_archive'])
        self.assertEqual(state['reason'], 'control_expired')

    def test_receiver_requires_explicit_enable(self):
        self.value['sources'][0]['executor'] = 'temporary_receiver'
        self.value['sources'][0]['archive_enabled'] = False
        self.write_control()
        self.assertFalse(policy.state_for(self.logs)['allow_archive'])
        self.value['sources'][0]['archive_enabled'] = True
        self.write_control()
        self.assertTrue(policy.state_for(self.logs)['allow_archive'])

    def test_wrong_identity_permissions_and_unbound_directory_are_blocked(self):
        other = self.root / 'other'
        other.mkdir()
        self.assertFalse(policy.state_for(other)['allow_archive'])
        self.value['node_id'] = 'other-node'
        self.write_control()
        self.assertFalse(policy.state_for(self.logs)['allow_archive'])
        self.value['node_id'] = 'fixture'
        self.write_control()
        self.control.chmod(0o644)
        self.assertFalse(policy.state_for(self.logs)['allow_archive'])


if __name__ == '__main__':
    unittest.main()
