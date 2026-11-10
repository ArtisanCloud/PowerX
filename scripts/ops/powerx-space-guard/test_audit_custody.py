import copy
import gzip
import hashlib
import hmac
import io
import json
import os
from pathlib import Path
import tempfile
import time
import unittest
from unittest.mock import patch

import audit_custody as custody


class AuditCustodyTest(unittest.TestCase):
    def setUp(self):
        self.control = patch.object(custody.custody_policy, 'state_for', return_value={'managed': False, 'allow_archive': True, 'reason': 'enabled'})
        self.control.start()
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name).resolve()
        self.logs = self.root / 'logs'
        self.logs.mkdir(mode=0o700)
        self.state = self.root / 'state'
        self.rule = patch.object(custody.guard, 'RULES', [('postgres', str(self.logs), r'postgres\.log\.\d+\.gz', custody.guard.GIB)])
        self.rule.start()
        self.opened = patch.object(custody.guard, 'open_inodes', return_value=set())
        self.opened.start()
        self.config = {'key_hex': '12' * 32, 'receiver_id': 'fixture-receiver', 'bucket': 'audit-test', 'prefix': 'hosts/test', 'endpoint_id': 'endpoint-test'}

    def tearDown(self):
        self.control.stop()
        self.opened.stop()
        self.rule.stop()
        self.tmp.cleanup()

    def log(self, age=10):
        path = self.logs / 'postgres.log.2.gz'
        path.write_bytes(gzip.compress(b'non-business custody fixture\n'))
        at = time.time() - age * 86400
        os.utime(path, (at, at))
        item = custody.manifest()['files'][0]
        return path, item

    def proof(self, item, digest):
        return {'version': 1, 'receiver_id': self.config['receiver_id'], 'verified_at': time.time(), 'snapshot': item,
                'object': {'endpoint_id': self.config['endpoint_id'], 'bucket': self.config['bucket'],
                           'key': 'hosts/test/objects/' + digest[:2] + '/' + digest, 'version_id': 'object-version',
                           'sha256': digest, 'bytes': item['size'], 'manifest_key': 'hosts/test/manifests/' + item['id'] + '.json',
                           'manifest_version_id': 'manifest-version', 'manifest_sha256': 'a' * 64}}

    def sign(self, proof):
        return {'proof': proof, 'signature': hmac.new(bytes.fromhex(self.config['key_hex']), custody.canonical(proof), hashlib.sha256).hexdigest()}

    def test_verified_expired_inode_is_removed_and_intent_is_completed(self):
        path, item = self.log()
        proof = self.proof(item, hashlib.sha256(path.read_bytes()).hexdigest())
        result = custody.confirm(self.sign(proof), self.config, self.state)
        self.assertEqual(result['state'], 'deleted')
        self.assertFalse(path.exists())
        record = json.loads((self.state / 'archive-receipts' / (item['id'] + '.json')).read_text())
        self.assertEqual(record['state'], 'deleted')
        self.assertEqual(list((self.logs / '.powerx-audit-custody').iterdir()), [])

    def test_verified_recent_file_is_kept_under_budget(self):
        path, item = self.log(2)
        result = custody.confirm(self.sign(self.proof(item, hashlib.sha256(path.read_bytes()).hexdigest())), self.config, self.state)
        self.assertEqual(result['state'], 'retained')
        self.assertTrue(path.exists())

    def test_ui_disabled_blocks_manifest_read_and_confirm_without_deleting(self):
        path, item = self.log()
        receipt = self.sign(self.proof(item, hashlib.sha256(path.read_bytes()).hexdigest()))
        with patch.object(custody.custody_policy, 'state_for', return_value={'managed': True, 'allow_archive': False, 'reason': 'managed_by_powerx'}):
            self.assertFalse(custody.manifest()['archive_enabled'])
            self.assertEqual(custody.manifest()['files'], [])
            with self.assertRaises(RuntimeError):
                custody.read_file(item, io.BytesIO())
            with self.assertRaises(RuntimeError):
                custody.confirm(receipt, self.config, self.state)
        self.assertTrue(path.exists())

    def test_budget_reclamation_still_requires_proof_and_keeps_latest_day(self):
        path, item = self.log(2)
        with patch.object(custody.guard, 'RULES', [('postgres', str(self.logs), r'postgres\.log\.\d+\.gz', 0)]):
            result = custody.confirm(self.sign(self.proof(item, hashlib.sha256(path.read_bytes()).hexdigest())), self.config, self.state)
        self.assertEqual(result['state'], 'deleted')
        path, item = self.log(0.5)
        with patch.object(custody.guard, 'RULES', [('postgres', str(self.logs), r'postgres\.log\.\d+\.gz', 0)]):
            result = custody.confirm(self.sign(self.proof(item, hashlib.sha256(path.read_bytes()).hexdigest())), self.config, self.state)
        self.assertEqual(result['state'], 'retained')
        self.assertTrue(path.exists())

    def test_invalid_expired_or_other_destination_receipts_never_delete(self):
        path, item = self.log()
        good = self.proof(item, hashlib.sha256(path.read_bytes()).hexdigest())
        variants = []
        for field, value in [('verified_at', time.time() - 121), ('verified_at', time.time() + 12), ('receiver_id', 'other')]:
            p = copy.deepcopy(good)
            p[field] = value
            variants.append(self.sign(p))
        for field, value in [('bucket', 'other'), ('endpoint_id', 'other'), ('key', 'other/path'), ('version_id', 'null'), ('manifest_key', 'unrelated'), ('manifest_version_id', ''), ('manifest_sha256', 'invalid')]:
            p = copy.deepcopy(good)
            p['object'][field] = value
            variants.append(self.sign(p))
        damaged = self.sign(good)
        damaged['signature'] = '0' * 64
        variants.append(damaged)
        for bad in variants:
            with self.subTest(proof=variants.index(bad)):
                with self.assertRaises(RuntimeError):
                    custody.confirm(bad, self.config, self.state)
                self.assertTrue(path.exists())

    def test_digest_corruption_file_change_and_open_handle_fail_closed(self):
        path, item = self.log()
        wrong = self.proof(item, '0' * 64)
        with self.assertRaisesRegex(RuntimeError, 'source_digest'):
            custody.confirm(self.sign(wrong), self.config, self.state)
        correct = self.sign(self.proof(item, hashlib.sha256(path.read_bytes()).hexdigest()))
        with patch.object(custody.guard, 'open_inodes', return_value={(item['dev'], item['ino'])}):
            result = custody.confirm(correct, self.config, self.state)
            self.assertEqual(result['state'], 'retained')
            self.assertTrue(path.exists())
        path.write_bytes(b'replaced audit content')
        os.utime(path, ns=(item['mtime_ns'], item['mtime_ns']))
        with self.assertRaisesRegex(RuntimeError, 'file_changed'):
            custody.confirm(correct, self.config, self.state)
        self.assertTrue(path.exists())

    def test_source_replacement_and_caller_path_cannot_escape(self):
        path, item = self.log()
        output = io.BytesIO()
        bad = copy.deepcopy(item)
        bad['name'] = '../outside.log.gz'
        with self.assertRaises(RuntimeError):
            custody.read_file(bad, output)
        self.assertEqual(output.getvalue(), b'')
        self.logs.rename(self.root / 'original')
        self.logs.mkdir()
        (self.logs / path.name).write_bytes(b'new source')
        with self.assertRaisesRegex(RuntimeError, 'source_replaced'):
            custody.read_file(item, output)
        self.assertTrue((self.root / 'original' / path.name).exists())

    def test_rotation_race_preserves_both_inodes(self):
        path, item = self.log()
        receipt = self.sign(self.proof(item, hashlib.sha256(path.read_bytes()).hexdigest()))
        original_rename = os.rename
        def race(src, dst, **kwargs):
            if src == item['name']:
                original_rename(path, self.logs / 'original-preserved.gz')
                path.write_bytes(b'fresh replacement')
            return original_rename(src, dst, **kwargs)
        with patch.object(custody.os, 'rename', side_effect=race):
            with self.assertRaisesRegex(RuntimeError, 'quarantine_identity_conflict'):
                custody.confirm(receipt, self.config, self.state)
        self.assertEqual(path.read_bytes(), b'fresh replacement')
        self.assertTrue((self.logs / 'original-preserved.gz').exists())

    def test_growing_source_cannot_overrun_the_receivers_reserved_size(self):
        path, item = self.log()
        class Growing(io.BytesIO):
            def write(output, value):
                with path.open('ab') as changed:
                    changed.write(b'concurrent growth')
                return super().write(value)
        output = Growing()
        with self.assertRaises(RuntimeError):
            custody.read_file(item, output)
        self.assertLessEqual(len(output.getvalue()), item['size'])


if __name__ == '__main__':
    unittest.main()
