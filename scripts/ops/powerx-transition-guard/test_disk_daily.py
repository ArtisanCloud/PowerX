import datetime as dt
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import disk_daily


class DailyReportTests(unittest.TestCase):
    def test_beijing_date_and_duplicate_delivery_are_independent_of_test_message(self):
        now = dt.datetime(2026, 11, 9, 16, 5, tzinfo=dt.timezone.utc)
        with tempfile.TemporaryDirectory() as root, patch.object(disk_daily, 'STATE', Path(root)), \
                patch.object(disk_daily, 'guard_status', return_value={'state': 'unavailable'}), \
                patch.object(disk_daily, 'send_personal', return_value={'status': 'api_accepted', 'message_id': 'fixture'}) as sender:
            disk_daily.run(True, True, now)
            report = disk_daily.run(True, False, now)
            self.assertEqual(report['report_date'], '2026-11-10')
            self.assertEqual(disk_daily.run(True, False, now)['status'], 'already_accepted')
            self.assertEqual(sender.call_count, 2)
            self.assertNotEqual(sender.call_args_list[0].args[1], sender.call_args_list[1].args[1])

    def test_failed_delivery_can_retry_same_event_without_marking_success(self):
        now = dt.datetime(2026, 11, 9, 16, 5, tzinfo=dt.timezone.utc)
        with tempfile.TemporaryDirectory() as root, patch.object(disk_daily, 'STATE', Path(root)), \
                patch.object(disk_daily, 'guard_status', return_value={'state': 'unavailable'}), \
                patch.object(disk_daily, 'send_personal', side_effect=[OSError('fixture'), {'status': 'api_accepted'}]) as sender:
            with self.assertRaises(OSError):
                disk_daily.run(True, False, now)
            self.assertFalse(Path(root, 'disk-daily-latest.json').exists())
            disk_daily.run(True, False, now)
            self.assertEqual(sender.call_args_list[0].args[1], sender.call_args_list[1].args[1])

    def test_stale_protection_status_is_not_reported_as_current_health(self):
        now = dt.datetime(2026, 11, 9, 16, 5, tzinfo=dt.timezone.utc)
        with tempfile.TemporaryDirectory() as root:
            path = Path(root, 'guard.json')
            path.write_text(json.dumps({'at': (now - dt.timedelta(minutes=11)).isoformat(), 'severity': 'ok'}))
            with patch.object(disk_daily, 'GUARD', path):
                report = disk_daily.collect(now)
            self.assertEqual(report['capacity_guard']['state'], 'stale')
            self.assertIn('状态过期', report['text'])


if __name__ == '__main__':
    unittest.main()
