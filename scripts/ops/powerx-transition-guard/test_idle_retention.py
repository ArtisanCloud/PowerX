import json
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

import idle_retention as idle
import capacity


class IdleRetentionTests(unittest.TestCase):
    def test_worker_failure_or_stale_receipt_is_not_hidden_by_healthy_core(self):
        self.assertTrue(capacity.idle_retention_failed({'state': 'stale'}))
        self.assertTrue(capacity.idle_retention_failed({'state': 'receipt_unavailable'}))
        self.assertTrue(capacity.idle_retention_failed({'state': 'reported', 'databases': {'prod': {'state': 'stopped'}}}))
        self.assertFalse(capacity.idle_retention_failed({'state': 'not_installed'}))
        self.assertFalse(capacity.idle_retention_failed({'state': 'reported', 'databases': {'prod': {'state': 'preview_only'}}}))

    def test_new_worker_failure_alerts_once_without_changing_owned_schedules(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            prior = {'owned_pauses': {'prod-backup': 'preserved'}, 'guard_kind': 'monitor_only_no_schedule_changes_or_deletion',
                     'severity': 'critical', 'idle_retention_failed': False, 'notification_at': 10**12}
            (root/'capacity-latest.json').write_text(json.dumps(prior))
            failed = {'state': 'reported', 'databases': {'prod': {'state': 'stopped'}}}
            with patch.object(capacity, 'STATE', root), patch.object(capacity, 'filesystem', return_value={'available_bytes': 30*idle.GIB, 'used_ratio': .5}), \
                    patch.object(capacity, 'runtime_health', return_value=({'healthy': True}, 1)), patch.object(capacity, 'retention_status', return_value={}), \
                    patch.object(capacity, 'idle_retention_status', return_value=failed), patch.object(capacity, 'send_personal', return_value={'status': 'api_accepted'}) as send:
                first = capacity.observe(True)
                second = capacity.observe(True)
            self.assertTrue(first['idle_retention_failed'])
            self.assertEqual(first['severity'], 'critical')
            self.assertEqual(send.call_count, 1)
            self.assertEqual(second['owned_pauses'], prior['owned_pauses'])

    def test_configuration_cannot_relax_approved_seven_days(self):
        class Config:
            def lstat(self): return SimpleNamespace(st_uid=0, st_mode=0o100640)
            def is_symlink(self): return False
            def read_text(self): return json.dumps(value)
        value = {'version': 1, 'retention_days': 7, 'apply': True,
                 'targets': {k: {'source_commit': 'a'*40, 'native_last_success_at': 'old'} for k in idle.TARGETS}}
        self.assertEqual(idle.configuration(Config())['retention_days'], 7)
        value['retention_days'] = 1
        with self.assertRaisesRegex(RuntimeError, 'invalid'):
            idle.configuration(Config())

    def test_source_upgrade_or_native_recovery_stops_temporary_mutations(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root/'backend').mkdir()
            (root/'manifest.txt').write_text('commit='+'a'*40+'\n')
            health = root/'storage/storage-governance/cleanup-health.json'
            health.parent.mkdir(parents=True)
            health.write_text(json.dumps({'state': 'failed', 'last_success_at': 'old'}))
            with patch.object(idle, 'TARGETS', {'dev': ('powerx_dev', str(root))}):
                binding = {'source_commit': 'a'*40, 'native_last_success_at': 'old'}
                idle.runtime_guard('dev', binding)
                (root/'manifest.txt').write_text('commit='+'b'*40+'\n')
                with self.assertRaisesRegex(RuntimeError, 'runtime_changed'):
                    idle.runtime_guard('dev', binding)
                (root/'manifest.txt').write_text('commit='+'a'*40+'\n')
                health.write_text(json.dumps({'state': 'completed', 'last_success_at': 'new'}))
                with self.assertRaisesRegex(RuntimeError, 'native_cleanup_changed'):
                    idle.runtime_guard('dev', binding)

    def test_unknown_apply_commit_keeps_original_cursor_and_pending_identity(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(idle, 'ARCHIVES', Path(directory)):
            run = Path(directory)/'powerx_pro/apply-1'
            run.mkdir(parents=True)
            run.joinpath('progress.json').write_text(json.dumps({'mode': 'apply', 'cutoff': 'cutoff', 'state': 'commit_unknown', 'cursor': 20}))
            sweep = {'last_run': 'apply-1', 'cursor': 10, 'highwater': 30, 'cutoff': 'cutoff', 'pairs': 2}
            with self.assertRaisesRegex(RuntimeError, 'unconfirmed'):
                idle.reconcile('powerx_pro', 'apply', sweep)
            self.assertEqual(sweep['cursor'], 10)
            self.assertEqual(sweep['last_run'], 'apply-1')

    def test_preview_receipt_cannot_be_used_as_apply_confirmation(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(idle, 'ARCHIVES', Path(directory)):
            run = Path(directory)/'powerx_pro/apply-1'
            run.mkdir(parents=True)
            run.joinpath('progress.json').write_text(json.dumps({'mode': 'preview', 'cutoff': 'cutoff', 'state': 'complete', 'cursor': 20}))
            sweep = {'last_run': 'apply-1', 'cursor': 10, 'highwater': 30, 'cutoff': 'cutoff', 'pairs': 2}
            with self.assertRaisesRegex(RuntimeError, 'binding_mismatch'):
                idle.reconcile('powerx_pro', 'apply', sweep)

    def test_readonly_scan_does_not_advance_delete_cursor(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(idle, 'ARCHIVES', Path(directory)):
            run = Path(directory)/'powerx_pro/preview-1'
            run.mkdir(parents=True)
            run.joinpath('progress.json').write_text(json.dumps({'mode': 'preview', 'cutoff': 'cutoff', 'state': 'complete', 'cursor': 30, 'pairs': 8, 'at': 'now'}))
            preview = {'last_run': 'preview-1', 'cursor': 0, 'highwater': 30, 'cutoff': 'cutoff', 'pairs': 0}
            apply = {'cursor': 5, 'pairs': 2}
            idle.reconcile('powerx_pro', 'preview', preview)
            self.assertEqual(preview['pairs'], 8)
            self.assertEqual(apply, {'cursor': 5, 'pairs': 2})

    def test_preview_receipts_bounded_but_apply_archives_preserved(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(idle, 'ARCHIVES', Path(directory)):
            root = Path(directory)/'powerx_pro'
            root.mkdir()
            for number in range(5):
                p = root/('preview-'+str(number)); p.mkdir(); (p/'progress.json').write_text('{}')
            applied = root/'apply-1'; applied.mkdir(); (applied/'recover.copy.gz').write_bytes(b'protected')
            idle.prune_previews('powerx_pro', keep=2)
            self.assertEqual(len(list(root.glob('preview-*'))), 2)
            self.assertEqual((applied/'recover.copy.gz').read_bytes(), b'protected')

    def test_unknown_files_prevent_preview_cleanup(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(idle, 'ARCHIVES', Path(directory)):
            root = Path(directory)/'powerx_pro'
            for name in ['preview-1', 'preview-2']:
                p = root/name; p.mkdir(parents=True)
            (root/'preview-1/unknown.copy.gz').write_bytes(b'keep')
            with self.assertRaisesRegex(RuntimeError, 'unexpected_files'):
                idle.prune_previews('powerx_pro', keep=1)
            self.assertTrue((root/'preview-1/unknown.copy.gz').is_file())

    def test_low_space_never_calls_apply_even_when_policy_enabled(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory); archives = root/'archives'; archives.mkdir()
            calls = []
            def execute(database, mode, sweep, available):
                calls.append(mode)
                run = archives/database/'preview-1'; run.mkdir(parents=True)
                sweep['last_run'] = 'preview-1'; sweep['last_pairs'] = 0
                (run/'progress.json').write_text(json.dumps({'mode': mode, 'cutoff': sweep['cutoff'], 'state': 'complete', 'cursor': 10, 'pairs': 3, 'at': 'now'}))
                return ['fixture-preview'], run
            with patch.object(idle, 'STATE', root), patch.object(idle, 'ARCHIVES', archives), \
                    patch.object(idle, 'runtime_guard'), patch.object(idle, 'metadata', return_value={'highwater': 10, 'relation_bytes': 1000, 'dead_rows': 4}), \
                    patch.object(idle.os, 'statvfs', return_value=SimpleNamespace(f_bavail=5*idle.GIB, f_frsize=1)), \
                    patch.object(idle, 'execute', side_effect=execute), patch.object(idle.subprocess, 'run', return_value=SimpleNamespace(returncode=0)):
                report = {}
                idle.maintain('prod', {'apply': True, 'targets': {'prod': {}}}, report)
            self.assertEqual(calls, ['preview'])
            self.assertEqual(report['deleted_pairs_this_run'], 0)
            self.assertEqual(report['mode'], 'preview')


if __name__ == '__main__':
    unittest.main()
