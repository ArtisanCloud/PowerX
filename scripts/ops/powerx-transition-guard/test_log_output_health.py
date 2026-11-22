import json
import os
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch
import log_output_health as health
import capacity


def config(console=False, file=False, loki=False):
    return {'log': {'console': console, 'file': {'enable': file}, 'loki': {'enable': loki, 'url': 'http://127.0.0.1:3100'}}}


class LogOutputTests(unittest.TestCase):
    def inspect(self, cfg, **mocks):
        with patch.object(health, 'properties', return_value={'MainPID': '7', 'ActiveState': 'active', 'StandardOutput': 'journal'}), \
                patch.object(health, 'process_config', return_value=(Path('/etc/powerx/config.yaml'), Path('/opt/core'), cfg, (1000, 1000, [1000]))):
            return health.check_service('powerx-backend.service')

    def test_no_console_no_file_and_dead_loki_is_critical_without_secret_output(self):
        with patch.object(health, 'loki_ready', side_effect=OSError('secret-api-key')):
            row = self.inspect(config(loki=True))
        self.assertFalse(row['healthy'])
        self.assertEqual(row['state'], 'critical')
        self.assertEqual(row['reason'], 'no_available_log_output')
        self.assertNotIn('secret-api-key', json.dumps(row))

    def test_disabled_loki_is_not_probed_and_working_journal_is_valid(self):
        with patch.object(health, 'console_ready', return_value=(True, 'journal_route_ready_not_delivery_receipt')), patch.object(health, 'loki_ready') as loki:
            row = self.inspect(config(console=True))
        self.assertTrue(row['healthy'])
        self.assertFalse(row['degraded'])
        loki.assert_not_called()

    def test_failed_enabled_sink_with_fallback_is_degraded_not_no_output(self):
        with patch.object(health, 'console_ready', return_value=(True, 'ready')), patch.object(health, 'loki_ready', return_value=(False, 'unreachable', 0)):
            row = self.inspect(config(console=True, loki=True))
        self.assertTrue(row['healthy'])
        self.assertTrue(row['degraded'])

    def test_file_permissions_use_core_identity_and_both_log_paths(self):
        with patch.object(health, 'file_ready', return_value=True) as ready:
            row = self.inspect(config(file=True))
        self.assertTrue(row['healthy'])
        self.assertEqual(ready.call_count, 2)
        self.assertEqual(ready.call_args.args[1], (1000, 1000, [1000]))
        with tempfile.NamedTemporaryFile() as f, patch.object(health.os, 'geteuid', return_value=0), patch.object(health.subprocess, 'run', return_value=SimpleNamespace(returncode=1)) as run:
            self.assertFalse(health.file_ready(f.name, (1000, 1000, [1000, 27])))
            self.assertEqual(run.call_args.kwargs['user'], 1000)
            self.assertEqual(run.call_args.kwargs['extra_groups'], [1000, 27])

    def test_process_configuration_is_from_live_args_and_detects_unsaved_runtime_change(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            proc = root / 'proc' / '7'; proc.mkdir(parents=True)
            cwd = root / 'core'; cwd.mkdir()
            cfg = cwd / 'config.yaml'; cfg.write_text('log: {}\n')
            os.utime(cfg, (900, 900))
            (proc / 'cwd').symlink_to(cwd)
            (proc / 'environ').write_bytes(b'POWERX_CONFIG=/wrong/config\0SECRET=never-output\0')
            (proc / 'cmdline').write_bytes(b'powerx\0--config\0config.yaml\0')
            (proc / 'stat').write_text('7 (core worker) ' + ' '.join(['S'] + ['0'] * 18 + ['5000'] + ['0'] * 5))
            (proc / 'status').write_text('Uid:\t1000 1000 1000 1000\nGid:\t1000 1000 1000 1000\nGroups:\t1000 27\n')
            (root / 'proc' / 'uptime').write_text('100 0\n')
            def mapped(*args):
                path = Path(*args)
                return root / path.relative_to('/') if path == Path('/proc') or str(path).startswith('/proc/') else path
            with patch.object(health, 'Path', side_effect=mapped), patch.object(health.time, 'time', return_value=1000), patch.object(health.os, 'sysconf', return_value=100):
                path, _, _, identity = health.process_config(7)
                self.assertEqual(path, cfg.resolve())
                self.assertEqual(identity, (1000, 1000, [1000, 27]))
                os.utime(cfg, (1000, 1000))
                with self.assertRaisesRegex(RuntimeError, 'configuration_changed_since_process_start'):
                    health.process_config(7)

    def test_changed_config_is_not_reported_healthy(self):
        with patch.object(health, 'properties', return_value={'MainPID': '7', 'ActiveState': 'active'}), patch.object(health, 'process_config', side_effect=RuntimeError('configuration_changed_since_process_start')):
            row = health.check_service('powerx-backend.service')
        self.assertFalse(row['healthy'])
        self.assertEqual(row['state'], 'unknown')
        self.assertIn('configuration_changed', row['reason'])

    def test_same_loki_endpoint_is_probed_once_for_two_services(self):
        with patch.object(health, 'properties', return_value={'MainPID': '7', 'ActiveState': 'active'}), \
                patch.object(health, 'process_config', return_value=(Path('/etc/core'), Path('/opt/core'), config(loki=True), (1, 1, []))), \
                patch.object(health, 'loki_ready', return_value=(True, 'accepted', 204)) as probe:
            result = health.check_services()
        self.assertTrue(result['healthy'])
        self.assertEqual(probe.call_count, 1)

    def test_journal_route_and_persistent_storage_are_required(self):
        ok, reason = health.console_ready({'StandardOutput': 'null'}, 7)
        self.assertFalse(ok)
        self.assertEqual(reason, 'stdout_not_verified_journal')
        with patch.object(health, 'properties', return_value={'ActiveState': 'inactive'}):
            self.assertFalse(health.console_ready({'StandardOutput': 'journal'}, 7)[0])

    def test_all_outputs_disabled_and_invalid_boolean_are_not_success(self):
        self.assertFalse(self.inspect(config())['healthy'])
        value = config(); value['log']['console'] = 'true'
        self.assertEqual(self.inspect(value)['reason'], 'log_config_incomplete')

    def test_runtime_cache_cannot_hide_logging_failure_or_restart(self):
        old = {'core_status': 200, 'dev_core_status': 200, 'core_seconds': .1, 'dev_core_seconds': .1,
               'log_outputs': {'version': 1, 'healthy': True, 'fingerprint': 'old', 'services': [{'pid': 7}]}}
        bad = {'version': 1, 'healthy': False, 'fingerprint': 'bad', 'services': [{'pid': 7, 'sinks': {}}]}
        with patch.object(health, 'check_services', return_value=bad), patch.object(capacity.urllib.request, 'urlopen') as request:
            result, _ = capacity.runtime_health({'runtime_health': old, 'health_checked_at': health.time.time()})
            request.assert_not_called()
        self.assertFalse(result['healthy'])
        self.assertIsNone(result['loki_push_status'])
        self.assertTrue(health.changed({'runtime_health': old}, result))
        bad['services'][0]['pid'] = 8
        with patch.object(health, 'check_services', return_value=bad), patch.object(capacity.urllib.request, 'urlopen', side_effect=OSError()) as request:
            capacity.runtime_health({'runtime_health': old, 'health_checked_at': health.time.time()})
            self.assertEqual(request.call_count, 2)

    def test_new_logging_failure_alerts_once_even_if_disk_is_already_critical(self):
        def output(fingerprint, healthy):
            return {'healthy': healthy, 'core_status': 200, 'dev_core_status': 200,
                    'core_seconds': 0.1, 'dev_core_seconds': 0.1,
                    'log_outputs': {'fingerprint': fingerprint, 'healthy': healthy, 'services': [
                {'unit': 'powerx-dev-backend.service', 'healthy': healthy, 'reason': 'no_available_log_output'}]}}
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            prior = {'severity': 'critical', 'guard_kind': 'monitor_only_no_schedule_changes_or_deletion',
                     'filesystem': {'available_bytes': 7 * capacity.GIB, 'used_ratio': .9},
                     'runtime_health': output('old', True), 'notification': {'status': 'api_accepted'}}
            (root / 'capacity-latest.json').write_text(json.dumps(prior))
            with patch.object(capacity, 'STATE', root), patch.object(capacity, 'filesystem', return_value={'available_bytes': 7 * capacity.GIB, 'used_ratio': .9}), \
                    patch.object(capacity, 'runtime_health', return_value=(output('bad', False), 123)) as probe, \
                    patch.object(capacity, 'retention_status', return_value={}), patch.object(capacity, 'idle_retention_status', return_value={'state': 'not_installed'}), \
                    patch.object(capacity, 'send_personal', return_value={'status': 'api_accepted'}) as send:
                capacity.observe(True)
                capacity.observe(True)
                self.assertEqual(send.call_count, 1)
                self.assertIn('无可用日志出口', send.call_args.args[0])
                probe.return_value = (output('recovered', True), 124)
                capacity.observe(True)
                probe.return_value = (output('flapped', False), 125)
                capacity.observe(True)
                self.assertEqual(send.call_count, 1)



if __name__ == '__main__': unittest.main()
