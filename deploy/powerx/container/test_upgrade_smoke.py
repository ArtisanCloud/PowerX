import io
import json
import unittest
import urllib.error
from unittest.mock import patch

from upgrade_smoke import Acceptance, APIResponseError


class Response(io.BytesIO):
    def __init__(self, payload, status=200):
        super().__init__(json.dumps(payload).encode())
        self.status = status
        self.headers = {'X-Request-ID': 'test-request'}


class UpgradeReadinessTests(unittest.TestCase):
    def acceptance(self):
        return Acceptance({'http': [], 'checks': {}})

    def gateway_error(self, status=502):
        return urllib.error.HTTPError(
            'http://127.0.0.1:18080/api/v1/admin/setup/status',
            status, 'gateway restart', {}, io.BytesIO(b'<html>Bad Gateway</html>'))

    def test_html_gateway_error_preserves_status_for_retry(self):
        acceptance = self.acceptance()
        with patch('upgrade_smoke.urllib.request.urlopen', side_effect=self.gateway_error()):
            with self.assertRaises(APIResponseError) as raised:
                acceptance.api('/api/v1/admin/setup/status')
        self.assertEqual(raised.exception.status, 502)
        self.assertEqual(acceptance.report['http'][0]['status'], 502)
        self.assertNotIn('Bad Gateway', json.dumps(acceptance.report))

    def test_waits_through_core_restart_and_setup_only_status(self):
        acceptance = self.acceptance()
        replies = [self.gateway_error(),
                   Response({'data': {'configured': True, 'requires_login': False,
                                      'restart_required': True}}),
                   Response({'data': {'configured': True, 'requires_login': True,
                                      'restart_required': False}})]
        with patch('upgrade_smoke.urllib.request.urlopen', side_effect=replies), \
                patch('upgrade_smoke.time.sleep') as sleep:
            acceptance.wait_installed()
        self.assertEqual(sleep.call_count, 2)
        self.assertEqual([r['status'] for r in acceptance.report['http']], [502, 200, 200])

    def test_forbidden_status_fails_without_readiness_retry(self):
        with patch('upgrade_smoke.urllib.request.urlopen', side_effect=self.gateway_error(403)), \
                patch('upgrade_smoke.time.sleep') as sleep:
            with self.assertRaises(APIResponseError):
                self.acceptance().wait_installed()
        sleep.assert_not_called()

    def test_invalid_success_json_fails_and_keeps_http_record(self):
        acceptance = self.acceptance()
        response = Response({})
        response.seek(0)
        response.write(b'<html>not Core JSON</html>')
        response.truncate()
        response.seek(0)
        with patch('upgrade_smoke.urllib.request.urlopen', return_value=response):
            with self.assertRaisesRegex(RuntimeError, 'UPGRADE_CI_HTTP_JSON_INVALID'):
                acceptance.api('/api/v1/admin/setup/status')
        self.assertEqual(acceptance.report['http'][0]['status'], 200)

    def test_readiness_timeout_is_bounded(self):
        with patch('upgrade_smoke.urllib.request.urlopen', side_effect=lambda *a, **k: self.gateway_error()), \
                patch('upgrade_smoke.time.sleep') as sleep:
            # Returning HTTPError from urlopen is treated like the caught
            # error response, with its actual 502 status.
            with self.assertRaisesRegex(RuntimeError, 'UPGRADE_CI_INSTALLED_STATE_FAILED'):
                self.acceptance().wait_installed()
        self.assertEqual(sleep.call_count, 150)


if __name__ == '__main__':
    unittest.main()
