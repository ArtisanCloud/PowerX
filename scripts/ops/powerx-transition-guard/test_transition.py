import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import capacity
import notify
import retention
import verify_recovery


class TransitionTests(unittest.TestCase):
    def test_upgraded_observation_never_calls_legacy_mutations_and_preserves_ownership(self):
        with tempfile.TemporaryDirectory() as root:
            state = Path(root)
            prior = {'owned_pauses': {'prod-backup': 'preserved-marker'},
                     'pending_verification': True, 'notification_at': 0}
            (state/'capacity-latest.json').write_text(json.dumps(prior))
            fs = {'available_bytes': 7*capacity.GIB, 'used_ratio': .91}
            with patch.object(capacity, 'STATE', state), \
                    patch.object(capacity, 'legacy_runtime', return_value=False) as legacy, \
                    patch.object(capacity, 'filesystem', return_value=fs), \
                    patch.object(capacity, 'runtime_health', return_value=({'healthy': True}, 123)), \
                    patch.object(capacity, 'retention_status', return_value={'service_result': 'exit-code'}), \
                    patch.object(capacity, 'policies') as policies, \
                    patch.object(capacity, 'set_enabled') as mutate, \
                    patch.object(capacity, 'send_personal', return_value={'status': 'api_accepted'}) as send:
                first = capacity.observe(True)
                second = capacity.observe(True)
            legacy.assert_not_called()
            policies.assert_not_called()
            mutate.assert_not_called()
            send.assert_called_once()
            self.assertEqual(first['severity'], 'critical')
            self.assertEqual(second['owned_pauses'], prior['owned_pauses'])
            self.assertTrue(second['pending_verification'])
            self.assertFalse(second['handoff_complete'])
            self.assertFalse(second['automatic_cleanup_active'])
            self.assertEqual(json.loads((state/'capacity-latest.json').read_text())['owned_pauses'], prior['owned_pauses'])

    def test_observation_preview_does_not_send_or_save(self):
        with tempfile.TemporaryDirectory() as root:
            state = Path(root)
            with patch.object(capacity, 'STATE', state), \
                    patch.object(capacity, 'filesystem', return_value={'available_bytes': 30*capacity.GIB, 'used_ratio': .5}), \
                    patch.object(capacity, 'runtime_health', return_value=({'healthy': True}, 123)), \
                    patch.object(capacity, 'retention_status', return_value={'state': 'idle'}), \
                    patch.object(capacity, 'send_personal') as send:
                result = capacity.observe(False)
            self.assertEqual(result['severity'], 'ok')
            send.assert_not_called()
            self.assertFalse((state/'capacity-latest.json').exists())

    def test_peak_headroom_pauses_and_resume_has_hysteresis(self):
        g = capacity.GIB
        self.assertEqual(capacity.decision(16*g, 5*g, False), 'pause')
        self.assertIsNone(capacity.decision(18*g, 5*g, True))
        self.assertEqual(capacity.decision(19*g, 5*g, True), 'resume')

    def test_unknown_commit_or_interrupted_run_is_never_resumed(self):
        for state in ('running', 'commit_unknown', 'stopped_error'):
            with self.assertRaisesRegex(RuntimeError, 'unconfirmed'):
                retention.confirmed_cursor({'state': state, 'cursor': 25})
        self.assertEqual(retention.confirmed_cursor({'state': 'bounded_complete', 'cursor': 25}), 25)

    def test_runtime_probe_failure_is_visible_without_response_payload(self):
        with patch.object(capacity.urllib.request, 'urlopen', side_effect=OSError('fixture')):
            result, checked_at = capacity.runtime_health({})
        self.assertFalse(result['healthy'])
        self.assertEqual(result['core_status'], 0)
        self.assertEqual(result['loki_push_status'], 0)
        self.assertGreater(checked_at, 0)

    def test_archive_symlink_stops_budget_accounting(self):
        with tempfile.TemporaryDirectory() as root:
            Path(root, 'outside').symlink_to('/tmp')
            with self.assertRaisesRegex(RuntimeError, 'symlink'):
                retention.allocated_bytes(Path(root))

    def test_recovery_hash_corruption_is_detected(self):
        with tempfile.TemporaryDirectory() as root:
            path = Path(root, 'dump'); path.write_bytes(b'changed')
            with self.assertRaisesRegex(RuntimeError, 'hash'):
                verify_recovery.sha_file(path, 7, '0'*64)

    def test_notification_refuses_group_recipient(self):
        with tempfile.TemporaryDirectory() as root:
            path = Path(root, 'private.json')
            path.write_text(json.dumps({'mode': 'feishu_app', 'app_id': 'cli_fixture',
                                       'app_secret': 'fixture', 'receive_id_type': 'open_id', 'receive_id': 'oc_group'}))
            path.chmod(0o600)
            real_lstat = path.lstat
            info = real_lstat()
            class RootInfo:
                st_mode = info.st_mode
                st_uid = 0
            with patch.object(Path, 'lstat', return_value=RootInfo()), patch.object(notify, 'request_json') as api:
                with self.assertRaisesRegex(RuntimeError, 'configuration_invalid'):
                    notify.send_personal('test', 'fixture-event', path)
                api.assert_not_called()

    def test_feishu_acceptance_requires_message_id(self):
        with tempfile.TemporaryDirectory() as root:
            path = Path(root, 'private.json')
            path.write_text(json.dumps({'mode': 'feishu_app', 'app_id': 'cli_fixture', 'app_secret': 'fixture',
                                       'receive_id_type': 'open_id', 'receive_id': 'ou_owner'}))
            path.chmod(0o600); info = path.lstat()
            class RootInfo:
                st_mode = info.st_mode
                st_uid = 0
            with patch.object(Path, 'lstat', return_value=RootInfo()), patch.object(notify, 'request_json', side_effect=[{'tenant_access_token':'fixture'}, {'data':{}}]):
                with self.assertRaisesRegex(RuntimeError, 'unconfirmed'):
                    notify.send_personal('test', 'fixture-event', path)


if __name__ == '__main__':
    unittest.main()
