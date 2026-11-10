import hashlib
import io
import json
import os
import subprocess
from pathlib import Path
import tempfile
import time
import unittest
from unittest.mock import patch

import archive_s3 as receiver


class S3ReadbackTest(unittest.TestCase):
    def test_full_versioned_content_is_checked_instead_of_etag_or_metadata(self):
        class S3:
            def get_object(self, **kwargs):
                return {'Body': io.BytesIO(b'corrupt'), 'VersionId': 'v1', 'ETag': 'looks-valid'}
        with self.assertRaisesRegex(RuntimeError, 'digest_mismatch'):
            receiver.full_readback(S3(), 'bucket', 'key', 'v1', hashlib.sha256(b'correct').hexdigest(), 7)
        with self.assertRaisesRegex(RuntimeError, 'versioned_object_required'):
            receiver.full_readback(S3(), 'bucket', 'key', 'null', '0' * 64, 7)

    def test_public_bucket_policy_is_rejected(self):
        for principal in ['*', {'AWS': '*'}, {'AWS': ['*']}]:
            with self.assertRaisesRegex(RuntimeError, 'public_policy'):
                receiver.no_public_policy({'Statement': [{'Effect': 'Allow', 'Principal': principal}]})
        receiver.no_public_policy({'Statement': [{'Effect': 'Allow', 'Principal': {'AWS': 'arn:aws:iam::123:root'}}]})
        with self.assertRaisesRegex(RuntimeError, 'public_policy'):
            receiver.no_public_policy({'Statement': [{'Effect': 'Allow', 'NotPrincipal': {'AWS': 'arn:aws:iam::123:root'}}]})


class ReceiverPipelineTest(unittest.TestCase):
    def test_disabled_never_loads_credentials_or_contacts_s3(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            root.chmod(0o700)
            with patch.object(receiver, 'rpc', return_value={'archive_enabled': False}), patch.object(receiver, 'build_client') as client:
                result = receiver.run_once({'state_directory': temporary})
            self.assertEqual('disabled', result['status'])
            client.assert_not_called()


    def test_object_manifest_or_offline_failure_never_sends_delete_commit(self):
        for fault in ['object_corrupt', 'manifest_corrupt', 'offline', 'commit_reply_lost', 'batch_deadline', None]:
            with self.subTest(fault=fault), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary).resolve()
                key_file = root / 'key.json'
                key_file.write_text(json.dumps({'key_hex': 'ab' * 32}))
                key_file.chmod(0o600)
                settings = {'state_directory': str(root), 'receipt_key_file': str(key_file),
                            'receiver_id': 'receiver-test', 'bucket': 'test', 'prefix': 'hosts/test'}
                item = {'id': '123', 'size': 7, 'mtime_ns': int((time.time() - 9 * 86400) * 1e9)}
                commits = []
                class S3:
                    def __init__(self):
                        self.objects = {}
                    def head_bucket(self, **kwargs):
                        if fault == 'offline':
                            raise RuntimeError('receiver_offline')
                    def get_bucket_policy(self, **kwargs):
                        return {'Policy': '{}'}
                    def get_bucket_acl(self, **kwargs):
                        return {'Grants': []}
                    def get_bucket_lifecycle_configuration(self, **kwargs):
                        return {'Rules': []}
                    def get_bucket_versioning(self, **kwargs):
                        return {'Status': 'Enabled'}
                    def put_object(self, **kwargs):
                        body = kwargs['Body']
                        self.objects[kwargs['Key']] = body.read() if hasattr(body, 'read') else body
                        return {'VersionId': 'version-1'}
                    def get_object(self, **kwargs):
                        value = self.objects[kwargs['Key']]
                        if fault == 'object_corrupt' and '/objects/' in kwargs['Key'] or fault == 'manifest_corrupt' and '/manifests/' in kwargs['Key']:
                            value = b'x' * len(value)
                        return {'Body': io.BytesIO(value), 'VersionId': 'version-1'}
                s3 = S3()
                def rpc(settings, operation, value=None):
                    if operation == 'manifest':
                        return {'version': 1, 'archive_enabled': True, 'complete': True, 'files': [item]}
                    self.assertEqual(operation, 'confirm')
                    commits.append(value)
                    if fault == 'commit_reply_lost':
                        raise RuntimeError('simulated_lost_reply')
                    return {'state': 'deleted'}
                def download(settings, item, directory):
                    if fault == 'batch_deadline':
                        settings['_deadline'] = time.monotonic() - 1
                        raise subprocess.TimeoutExpired('ssh', .01)
                    p = directory / 'owned-download'
                    p.write_bytes(b'correct')
                    return p, hashlib.sha256(b'correct').hexdigest()
                with patch.object(receiver, 'build_client', return_value=(s3, 'endpoint', 'fingerprint')), patch.object(receiver, 'rpc', side_effect=rpc), patch.object(receiver, 'download', side_effect=download):
                    result = receiver.run_once(settings)
                if fault == 'batch_deadline':
                    self.assertEqual(commits, [])
                    self.assertEqual(result['status'], 'partial')
                    self.assertEqual(result['deferred_files'], 1)
                    self.assertEqual(result['failed_files'], 0)
                elif fault == 'commit_reply_lost':
                    self.assertEqual(len(commits), 1)
                    self.assertEqual(result['source_commit_unconfirmed'], 1)
                    self.assertEqual(result['status'], 'failed')
                elif fault:
                    self.assertEqual(commits, [])
                    self.assertEqual(result['status'], 'failed')
                else:
                    self.assertEqual(len(commits), 1)
                    self.assertEqual(result['source_deleted'], 1)
                    self.assertIn('manifest_version_id', commits[0]['proof']['object'])
                self.assertFalse((root / 'owned-download').exists())


if __name__ == '__main__':
    unittest.main()
