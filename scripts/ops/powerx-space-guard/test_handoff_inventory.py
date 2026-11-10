import contextlib
import datetime as dt
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

import handoff_inventory as inventory


class FakeHost:
    def __init__(self, units=None, processes=(), problems=()):
        self.units = units or {}
        self.rows = list(processes)
        self.problems = list(problems)

    def unit(self, name):
        return dict(self.units.get(name, {'state': 'not_found'}), name=name)

    def processes(self):
        return self.rows, self.problems


class HandoffInventoryTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.root = Path(self.temporary.name).resolve()
        self.now = dt.datetime(2026, 11, 9, 0, 0, tzinfo=dt.timezone.utc)
        self.directory = inventory.PROGRAM_DIRECTORY
        binding = {'protocol': 2, 'node_id': 'owned-test-node', 'core_uid': os.geteuid(),
                   'control_file': '/var/lib/powerx/archive/custody-control.json', 'sources': ['/var/log/owned']}
        for field, name in [('guard', 'guard.py'), ('helper', 'audit_custody.py'), ('policy', 'custody_policy.py')]:
            raw = (Path(inventory.__file__).parent / name).read_bytes()
            self.write(self.directory + '/' + name, raw, 0o755)
            binding[field + '_script'] = self.directory + '/' + name
            binding[field + '_sha256'] = hashlib.sha256(raw).hexdigest()
        self.binding = binding
        self.write_binding()
        self.control = {'protocol': 2, 'node_id': 'owned-test-node', 'published_at': self.now.isoformat(),
                        'valid_until': (self.now + dt.timedelta(minutes=3)).isoformat(),
                        'sources': [{'source_id': 'owned', 'directory': '/var/log/owned',
                                     'executor': 'powerx', 'archive_enabled': False}]}
        self.write_control()
        (self.root / 'var/log/owned').mkdir(parents=True)
        self.fs = inventory.Filesystem(self.root, snapshot=True)

    def tearDown(self):
        self.temporary.cleanup()

    def write(self, name, value, mode=0o600):
        path = self.root / name.lstrip('/')
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(value if isinstance(value, bytes) else value.encode())
        path.chmod(mode)
        return path

    def write_binding(self):
        self.write('/etc/powerx-space-guard/core-binding.json', json.dumps(self.binding), 0o644)

    def write_control(self):
        self.write(self.binding['control_file'], json.dumps(self.control))

    def report(self, host=None):
        return inventory.inspect(self.fs, host or FakeHost(), owner=os.geteuid(), now=self.now)

    def codes(self, report):
        return {item['code'] for item in report['problems']}

    def test_inventory_is_advisory_even_with_verified_binding_and_quiet_known_units(self):
        before = {str(p): (p.read_bytes(), p.stat().st_mtime_ns) for p in self.root.rglob('*') if p.is_file()}
        result = self.report()
        self.assertEqual('verified', result['binding_state'])
        self.assertEqual('verified', result['control_state'])
        self.assertFalse(result['sources'][0]['archive_enabled'])
        self.assertEqual('empty_or_absent', result['sources'][0]['quarantine'])
        self.assertFalse(result['authorizes_switch'])
        self.assertEqual('requires_review', result['assessment'])
        self.assertEqual(before, {str(p): (p.read_bytes(), p.stat().st_mtime_ns) for p in self.root.rglob('*') if p.is_file()})

    def test_active_or_enabled_inactive_timer_is_not_confused_with_stopped(self):
        for state in ['active', 'scheduled']:
            with self.subTest(state=state):
                result = self.report(FakeHost({'powerx-space-guard.timer': {'state': state}}))
                self.assertIn('legacy_schedule_requires_review', self.codes(result))

    def test_custom_service_and_timer_and_cron_are_discovered_without_secrets(self):
        secret = 'DO_NOT_EXPORT_PRIVATE_VALUE'
        self.write('/etc/systemd/system/custom-audit.service', '[Service]\nEnvironment=KEY=' + secret + '\nExecStart=/usr/bin/python3 /opt/powerx-ops/space-guard/guard.py\n', 0o644)
        self.write('/etc/systemd/system/custom-audit.timer', '[Timer]\nUnit=custom-audit.service\n', 0o644)
        self.write('/etc/cron.d/custom', '# disabled guard.py ' + secret + '\n*/5 * * * * root /usr/bin/python3 /opt/powerx-ops/space-guard/audit_custody.py --key=' + secret + '\n', 0o644)
        result = self.report()
        names = {u['name'] for u in result['units']}
        self.assertIn('custom-audit.service', names)
        self.assertIn('custom-audit.timer', names)
        self.assertEqual([{'line': 2, 'roles': ['audit_custody']}], result['cron'][0]['matches'])
        self.assertNotIn(secret, json.dumps(result))
        self.assertIn('legacy_cron_requires_review', self.codes(result))

    def test_nonempty_or_symlink_quarantine_requires_reconciliation_and_stays_untouched(self):
        pending = self.write('/var/log/owned/.powerx-audit-custody/unconfirmed.json', 'unknown business content')
        result = self.report()
        self.assertIn('legacy_quarantine_requires_reconciliation', self.codes(result))
        self.assertEqual('unknown business content', pending.read_text())
        self.assertNotIn('unknown business content', json.dumps(result))
        pending.unlink()
        pending.parent.rmdir()
        pending.parent.symlink_to(self.root / 'etc', target_is_directory=True)
        self.assertIn('legacy_quarantine_unknown', self.codes(self.report()))

    def test_unit_override_is_discovered_without_exporting_environment(self):
        self.write('/etc/systemd/system/override-audit.service.d/10-exec.conf', '[Service]\nEnvironment=TOKEN=PRIVATE_OVERRIDE_VALUE\nExecStart=/usr/bin/python3 /opt/powerx-ops/space-guard/audit_custody.py\n', 0o644)
        result = self.report()
        self.assertIn('override-audit.service', {u['name'] for u in result['units']})
        self.assertNotIn('PRIVATE_OVERRIDE_VALUE', json.dumps(result))

    def test_unit_symlink_inside_snapshot_is_read_but_outside_escape_is_not(self):
        self.write('/unitfiles/link-audit.service', '[Service]\nExecStart=python3 /opt/powerx-ops/space-guard/guard.py\n', 0o644)
        directory = self.root / 'etc/systemd/system'
        directory.mkdir(parents=True)
        (directory / 'link-audit.service').symlink_to('../../../unitfiles/link-audit.service')
        result = self.report()
        self.assertIn('link-audit.service', {u['name'] for u in result['units']})
        (directory / 'outside-audit.service').symlink_to('/etc/passwd')
        result = self.report()
        self.assertIn('unit_file_requires_review', self.codes(result))

    def test_normal_large_system_unit_directory_does_not_hide_late_custom_executor(self):
        for number in range(257):
            self.write('/usr/lib/systemd/system/base-%03d.service' % number, '[Service]\nExecStart=/usr/bin/true\n', 0o644)
        self.write('/usr/lib/systemd/system/z-custom.service', '[Service]\nExecStart=python3 /opt/powerx-ops/space-guard/guard.py\n', 0o644)
        result = self.report()
        self.assertIn('z-custom.service', {u['name'] for u in result['units']})
        self.assertNotIn('unit_scan_incomplete', self.codes(result))

    def test_real_dev_null_mask_is_not_read_as_unit_content_or_applied_to_binding(self):
        path = self.root / 'masked.service'
        path.symlink_to('/dev/null')
        live_fs = inventory.Filesystem(Path('/'))
        self.assertTrue(live_fs.masked_unit(str(path), os.geteuid()))
        with self.assertRaises(inventory.InspectionError):
            self.fs.read('/masked.service')
        with self.assertRaises(inventory.InspectionError):
            live_fs.read(str(path))

    def test_expired_control_keeps_binding_distinct_from_publication(self):
        self.control['valid_until'] = '2020-01-01T00:00:00+00:00'
        self.write_control()
        result = self.report()
        self.assertEqual('verified', result['binding_state'])
        self.assertEqual('unavailable', result['control_state'])
        self.assertIn('control_expired', self.codes(result))
        self.assertFalse(result['authorizes_switch'])

    def test_protocol_hash_or_source_drift_is_visible(self):
        for change, code in [('program', 'binding_program_mismatch'), ('node', 'control_identity_mismatch'), ('source', 'bound_source_missing_from_core')]:
            with self.subTest(change=change):
                if change == 'program':
                    self.write(self.directory + '/guard.py', 'CONTROL_PROTOCOL = 1\n', 0o755)
                elif change == 'node':
                    self.control['node_id'] = 'different'
                    self.write_control()
                else:
                    self.control['node_id'] = 'owned-test-node'
                    self.control['sources'] = []
                    self.write_control()
                self.assertIn(code, self.codes(self.report()))
                # 下一子例恢复固定程序；不通过改写绑定来掩盖漂移。
                if change == 'program':
                    self.write(self.directory + '/guard.py', (Path(inventory.__file__).parent / 'guard.py').read_bytes(), 0o755)

    def test_symlink_hardlink_unsafe_mode_and_untrusted_owner_are_rejected(self):
        path = self.root / self.binding['control_file'].lstrip('/')
        original = path.read_bytes()
        other = self.write('/control-backup.json', original)
        path.unlink()
        path.symlink_to(other)
        self.assertIn('path_link_requires_review', self.codes(self.report()))
        path.unlink()
        os.link(other, path)
        self.assertIn('file_identity_invalid', self.codes(self.report()))
        path.unlink()
        self.write_control()
        path.chmod(0o644)
        self.assertIn('file_identity_invalid', self.codes(self.report()))
        path.chmod(0o600)
        self.binding['core_uid'] = os.geteuid() + 1
        self.write_binding()
        self.assertIn('file_identity_invalid', self.codes(self.report()))

    def test_scan_errors_and_active_processes_are_not_zero_work(self):
        result = self.report(FakeHost(processes=[{'pid': 999, 'roles': ['file_retention'], 'start_ticks': 12}], problems=['process_scan_incomplete']))
        self.assertIn('process_scan_incomplete', self.codes(result))
        self.assertIn('active_executor_requires_review', self.codes(result))

    def test_report_comparison_detects_change_without_authorizing_switch(self):
        first, second = self.report(), self.report()
        same = inventory.compare(first, second)
        self.assertTrue(same['same_basis'])
        self.assertFalse(same['authorizes_switch'])
        self.write('/etc/cron.d/late', '* * * * * root python3 guard.py\n', 0o644)
        different = inventory.compare(first, self.report())
        self.assertFalse(different['same_basis'])
        self.assertIn('cron', different['changed_sections'])
        with self.assertRaises(inventory.InspectionError):
            inventory.compare({'schema': inventory.SCHEMA, 'authorizes_switch': True}, second)

    def test_bounded_read_and_snapshot_escape_fail_closed(self):
        self.write('/oversized', b'x' * 257)
        with self.assertRaises(inventory.InspectionError):
            self.fs.read('/oversized', limit=256)
        with self.assertRaises(inventory.InspectionError):
            self.fs.read('/etc/../oversized')
        (self.root / 'escape').symlink_to(Path('/etc'), target_is_directory=True)
        with self.assertRaises(inventory.InspectionError):
            self.fs.read('/escape/passwd')

    def test_cli_snapshot_never_calls_host_and_output_is_new_private_file(self):
        output = self.root / 'report.json'
        args = ['handoff_inventory.py', '--root', str(self.root), '--output', str(output)]
        with patch.object(sys, 'argv', args), patch.object(inventory, 'Host', side_effect=AssertionError('must not query live host')), contextlib.redirect_stdout(io.StringIO()):
            inventory.main()
        result = json.loads(output.read_text())
        self.assertEqual('snapshot', result['mode'])
        self.assertFalse(result['authorizes_switch'])
        self.assertEqual(0o600, output.stat().st_mode & 0o777)
        with patch.object(sys, 'argv', args), contextlib.redirect_stdout(io.StringIO()), self.assertRaises(FileExistsError):
            inventory.main()
        self.assertEqual(result, json.loads(output.read_text()))

    def test_apply_is_not_an_option(self):
        with patch.object(sys, 'argv', ['handoff_inventory.py', '--apply']), contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit) as raised:
            inventory.main()
        self.assertEqual(2, raised.exception.code)

    def test_native_process_reader_redacts_arguments_and_tracks_pid_start_time(self):
        secret = b'PRIVATE_PROCESS_PASSWORD'
        self.write('/proc/321/cmdline', b'/usr/bin/python3\0/opt/powerx-ops/space-guard/guard.py\0--password\0' + secret)
        rest = ['S', '1'] + ['0'] * 20
        rest[19] = '8812'
        self.write('/proc/321/stat', '321 (python3) ' + ' '.join(rest))
        real_path = Path
        with patch.object(inventory, 'Path', side_effect=lambda p: self.root / 'proc' if p == '/proc' else real_path(p)):
            rows, problems = inventory.Host().processes()
        self.assertFalse(problems)
        self.assertEqual(321, rows[0]['pid'])
        self.assertEqual(8812, rows[0]['start_ticks'])
        self.assertNotIn(secret.decode(), json.dumps(rows))
        self.assertNotIn('--password', json.dumps(rows))

    def test_systemd_failure_is_unknown_and_only_read_command_is_used(self):
        result = subprocess.CompletedProcess([], 1, stdout=b'LoadState=not-found\n', stderr=b'PRIVATE_DBUS_MESSAGE')
        with patch.object(Path, 'is_file', return_value=True), patch.object(inventory.subprocess, 'run', return_value=result) as run:
            state = inventory.Host().unit('powerx-space-guard.timer')
        self.assertEqual('unknown', state['state'])
        self.assertNotIn('PRIVATE_DBUS_MESSAGE', json.dumps(state))
        self.assertEqual('show', run.call_args.args[0][1])
        self.assertNotIn('shell', run.call_args.kwargs)

    def test_systemd_inactive_enabled_timer_remains_scheduled(self):
        result = subprocess.CompletedProcess([], 0, stdout=b'LoadState=loaded\nActiveState=inactive\nUnitFileState=enabled\nMainPID=0\n', stderr=b'')
        with patch.object(Path, 'is_file', return_value=True), patch.object(inventory.subprocess, 'run', return_value=result):
            state = inventory.Host().unit('powerx-space-guard.timer')
        self.assertEqual('scheduled', state['state'])

    def test_invalid_systemd_configuration_is_unknown_even_if_inactive(self):
        result = subprocess.CompletedProcess([], 0, stdout=b'LoadState=bad-setting\nActiveState=inactive\nUnitFileState=disabled\n', stderr=b'')
        with patch.object(Path, 'is_file', return_value=True), patch.object(inventory.subprocess, 'run', return_value=result):
            state = inventory.Host().unit('powerx-space-guard.service')
        self.assertEqual('unknown', state['state'])

    def test_failed_unit_is_never_reported_as_idle(self):
        result = subprocess.CompletedProcess([], 0, stdout=b'LoadState=loaded\nActiveState=failed\nUnitFileState=static\nResult=exit-code\nExecMainStatus=1\n', stderr=b'')
        with patch.object(Path, 'is_file', return_value=True), patch.object(inventory.subprocess, 'run', return_value=result):
            state = inventory.Host().unit('powerx-space-guard.service')
        self.assertEqual('failed', state['state'])
        self.assertEqual('1', state['properties']['ExecMainStatus'])
        report = self.report(FakeHost({'powerx-space-guard.service': state}))
        self.assertIn('legacy_service_failed', self.codes(report))
        self.assertEqual(1, report['summary']['failed_units'])

    def test_guard_capacity_alarm_is_distinct_from_runtime_error_and_redacted(self):
        self.write('/var/lib/powerx-space-guard/latest.json', json.dumps({'status': 'critical', 'finished_at': self.now.isoformat(), 'errors': [], 'after': {'available_bytes': 9 * 1024 ** 3, 'used_ratio': 0.88}, 'private_business_note': 'PRIVATE_RUN_VALUE'}))
        result = self.report()
        self.assertEqual('critical', result['guard_run']['status'])
        self.assertEqual(0, result['guard_run']['error_count'])
        self.assertIn('legacy_run_critical', self.codes(result))
        self.assertNotIn('PRIVATE_RUN_VALUE', json.dumps(result))

    def test_stale_guard_report_is_not_a_current_capacity_claim(self):
        self.write('/var/lib/powerx-space-guard/latest.json', json.dumps({'status': 'critical', 'finished_at': (self.now - dt.timedelta(minutes=20)).isoformat(), 'errors': []}))
        result = self.report()
        self.assertFalse(result['guard_run']['fresh'])
        self.assertIn('legacy_run_stale', self.codes(result))
        self.assertNotIn('legacy_run_critical', self.codes(result))
        for malformed in [[], None, {'status': 'invalid'}]:
            self.write('/var/lib/powerx-space-guard/latest.json', json.dumps(malformed))
            self.assertEqual('unknown', self.report()['guard_run']['state'])

    def test_go_rfc3339_z_and_nanosecond_publication_is_accepted_without_naive_times(self):
        self.control['published_at'] = '2026-11-09T00:00:00.000000000Z'
        self.control['valid_until'] = '2026-11-09T00:03:00.000000000Z'
        self.write_control()
        self.assertEqual('verified', self.report()['control_state'])
        for invalid in ['2026-11-09T00:03:00', 'not-a-date', None]:
            with self.assertRaises(inventory.InspectionError):
                inventory.timestamp(invalid)

    def test_cli_subprocess_reads_snapshot_and_rejects_existing_output(self):
        output = self.root / 'subprocess-report.json'
        command = [sys.executable, str(Path(inventory.__file__).resolve()), '--root', str(self.root), '--output', str(output)]
        result = subprocess.run(command, capture_output=True, text=True, timeout=5)
        self.assertEqual(0, result.returncode, result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual('snapshot', report['mode'])
        self.assertFalse(report['authorizes_switch'])
        self.assertTrue(report['read_only'])
        self.assertEqual(report, json.loads(output.read_text()))
        second = subprocess.run(command, capture_output=True, text=True, timeout=5)
        self.assertEqual(2, second.returncode)
        self.assertEqual('FileExistsError', json.loads(second.stdout)['reason'])
        self.assertEqual(report, json.loads(output.read_text()))


if __name__ == '__main__':
    unittest.main()
