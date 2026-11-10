"""只读前置检查的边界验收；HTTP 仅为自有临时 fixture，不表示 OSS 已通过。"""
import contextlib
import copy
import importlib.util
import io
import json
import os
from pathlib import Path
import socket
import stat
import subprocess
import sys
import tempfile
import threading
import unittest
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

SCRIPT = Path(__file__).with_name('check-storage-archive-readiness.py')
spec = importlib.util.spec_from_file_location('archive_readiness', SCRIPT)
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
TARGET = str(uuid.uuid4())
TOKEN = 'owned-test-session-not-a-real-secret'


def fixtures():
    target = {'id': TARGET, 'name': 'owned target', 'driver': 's3', 'revision': 1,
              'endpoint': 'https://s3.oss-cn-hangzhou.aliyuncs.com', 'bucket': 'owned-bucket',
              'prefix': 'acceptance/owned', 'region': 'cn-hangzhou', 'credential_ref': 'owned',
              'verification_valid': True, 'force_path_style': False}
    control = {'authority': 'database', 'ready': True, 'state': 'applied', 'desired_revision': 1, 'applied_revision': 1, 'snapshot_revision': 1}
    return [{'http_status': 200, 'data': {**control, 'targets': [target]}},
            {'http_status': 200, 'data': {'store': {'configured': True, 'ready': True, 'revision': 1, 'rotation_required': False},
                                       'items': [{'ref': 'owned', 'origin': 'sealed', 'state': 'active', 'revision': 1}]}},
            {'http_status': 200, 'data': {**control, 'policies': []}}]


@contextlib.contextmanager
def server(responses=None, redirect=False, response_override=None):
    calls = []
    data = responses or fixtures()

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass

        def do_GET(self):
            calls.append((self.command, self.path))
            if redirect:
                self.send_response(302)
                self.send_header('Location', '/must-not-follow')
                self.end_headers()
                return
            if self.headers.get('Authorization') != 'Bearer ' + TOKEN:
                self.send_response(403)
                self.end_headers()
                self.wfile.write(b'private error must not escape')
                return
            suffix = self.path.removeprefix('/custom/v2')
            if suffix not in m.PATHS:
                self.send_response(404)
                self.end_headers()
                return
            row = data[m.PATHS.index(suffix)]
            self.send_response(row['http_status'])
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            raw = response_override if response_override is not None else json.dumps({'data': row.get('data', {})}).encode()
            self.wfile.write(raw)

        def do_POST(self):
            calls.append(('POST', self.path))
            self.send_response(405)
            self.end_headers()

    httpd = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    httpd.daemon_threads = True
    port = httpd.server_port
    thread = threading.Thread(target=httpd.serve_forever)
    thread.start()
    print(json.dumps({'owned_http_fixture': {'pid': os.getpid(), 'command': 'python -m unittest scripts/ops/test_storage_archive_readiness.py', 'cwd': os.getcwd(), 'port': port}}), flush=True)
    try:
        yield f'http://127.0.0.1:{port}/custom/v2', calls
    finally:
        httpd.shutdown()
        httpd.server_close()
        thread.join(timeout=3)
        assert not thread.is_alive()
        with socket.socket() as s:
            s.settimeout(.2)
            assert s.connect_ex(('127.0.0.1', port)) != 0


class ReadinessTests(unittest.TestCase):
    def test_candidate_configuration_never_means_acceptance_or_permission(self):
        r = m.evaluate(fixtures(), TARGET)
        self.assertTrue(r['configuration_ready'])
        for key in ['t155_passed', 'authorizes_transfer', 'authorizes_configuration_change', 'retrieval_performed', 'provider_protocol_verified', 'independence_and_lifetime_verified']:
            self.assertFalse(r[key], key)

    def test_selection_is_explicit_even_for_one_target(self):
        self.assertIn('TARGET_SELECTION_REQUIRED', m.evaluate(fixtures())['issues'])
        self.assertIn('TARGET_SELECTION_NOT_UNIQUE', m.evaluate(fixtures(), str(uuid.uuid4()))['issues'])
        f = fixtures(); f[0]['data']['targets'] *= 2
        self.assertIn('TARGET_SELECTION_NOT_UNIQUE', m.evaluate(f, TARGET)['issues'])

    def test_empty_legacy_cannot_pass(self):
        f = fixtures(); f[0]['data'].update(authority='legacy', targets=[])
        r = m.evaluate(f)
        self.assertIn('TARGET_NOT_CONFIGURED', r['issues'])
        self.assertIn('TARGET_CONTROL_NOT_APPLIED', r['issues'])
        self.assertFalse(r['configuration_ready'])

    def test_publication_worker_and_snapshot_mismatch_block(self):
        for patch in [{'ready': False}, {'state': 'pending'}, {'snapshot_revision': 2}, {'applied_revision': 0}, {'desired_revision': True}, {'problem': 'STORAGE_CONTROL_SCHEMA_NOT_READY'}]:
            f = fixtures(); f[0]['data'].update(patch)
            self.assertFalse(m.evaluate(f, TARGET)['configuration_ready'], patch)

    def test_credentials_rotation_revocation_recovery_and_legacy_block(self):
        for patch in [{'state': 'revoked'}, {'state': 'recovery_required'}, {'origin': 'legacy'}, {'revision': 0}]:
            f = fixtures(); f[1]['data']['items'][0].update(patch)
            self.assertIn('TARGET_CREDENTIAL_NOT_ACTIVE', m.evaluate(f, TARGET)['issues'])
        f = fixtures(); f[1]['data']['store']['rotation_required'] = True
        self.assertIn('CREDENTIAL_ROTATION_REQUIRED', m.evaluate(f, TARGET)['issues'])
        f[1]['data']['store']['ready'] = False
        self.assertIn('CREDENTIAL_STORE_NOT_READY', m.evaluate(f, TARGET)['issues'])

    def test_loopback_and_unverified_target_are_not_long_term(self):
        for endpoint in ['http://127.0.0.1:9000', 'https://localhost:9000', 'https://[::1]:9000']:
            f = fixtures(); f[0]['data']['targets'][0]['endpoint'] = endpoint
            self.assertIn('TARGET_LONG_TERM_ENDPOINT_UNCONFIRMED', m.evaluate(f, TARGET)['issues'])
        f = fixtures(); f[0]['data']['targets'][0]['verification_valid'] = False
        self.assertIn('TARGET_NODE_VERIFICATION_REQUIRED', m.evaluate(f, TARGET)['issues'])

    def test_oss_uses_s3_endpoint_and_virtual_host_mode(self):
        f = fixtures(); f[0]['data']['targets'][0].update(endpoint='https://oss-cn-hangzhou.aliyuncs.com', force_path_style=True)
        issues = m.evaluate(f, TARGET)['issues']
        self.assertIn('OSS_S3_ENDPOINT_REVIEW_REQUIRED', issues)
        self.assertIn('OSS_VIRTUAL_HOST_REQUIRED', issues)

    def test_endpoint_with_embedded_secret_never_echoed(self):
        for value in ['https://private-user:private-password@example.test', 'https://example.test?secret=hidden', 'https://example.test/#private-token']:
            f = fixtures(); f[0]['data']['targets'][0]['endpoint'] = value
            encoded = json.dumps(m.evaluate(f, TARGET))
            self.assertNotIn(value, encoded)
            self.assertNotIn('private-password', encoded)
            self.assertNotIn('secret=hidden', encoded)

    def test_extra_secret_fields_and_errors_are_not_reported(self):
        f = fixtures(); f[0]['data']['targets'][0]['access_key'] = 'private-access-value'
        f[1]['data']['store']['secret_key'] = 'private-secret-value'
        f[2] = {'http_status': 503, 'private_error': 'do not disclose'}
        text = json.dumps(m.evaluate(f, TARGET))
        for secret in ['private-access-value', 'private-secret-value', 'do not disclose']:
            self.assertNotIn(secret, text)

    def test_source_inventory_is_bounded(self):
        f = fixtures(); f[0]['data']['targets'] *= 129
        self.assertIn('SOURCE_INVENTORY_INVALID', m.evaluate(f, TARGET)['issues'])

    def test_url_validation_and_configured_prefix(self):
        self.assertEqual('http://127.0.0.1:8077/custom/v2', m.core_url('http://127.0.0.1:8077/custom/v2/'))
        for value in ['http://example.test/api/v1', 'https://user:password@example.test/api/v1', 'https://example.test/api/v1?token=secret', 'https://example.test/api/../v1', 'https://example.test/api/v1#token', 'https://example.test:wrong/api/v1', 'https://example.test/\napi/v1']:
            with self.assertRaises(m.CheckError):
                m.core_url(value)

    def test_private_session_file_and_unsafe_forms(self):
        with tempfile.TemporaryDirectory() as directory:
            p = Path(directory) / 'session'; p.write_text(TOKEN); p.chmod(0o600)
            self.assertEqual(TOKEN, m.read_token(p))
            p.chmod(0o644)
            with self.assertRaises(m.CheckError): m.read_token(p)
            p.chmod(0o600)
            link = Path(directory) / 'link'; link.symlink_to(p)
            with self.assertRaises(m.CheckError): m.read_token(link)
            link.unlink(); os.link(p, link)
            with self.assertRaises(m.CheckError): m.read_token(p)
            link.unlink(); p.write_bytes(b'x' * (m.MAX_TOKEN + 1))
            with self.assertRaises(m.CheckError): m.read_token(p)
            p.unlink(); os.mkfifo(p, 0o600)
            with self.assertRaises(m.CheckError): m.read_token(p)

    def test_stdin_bound_and_token_control_characters(self):
        self.assertEqual(TOKEN, m.read_token(source=io.BytesIO((TOKEN + '\n').encode())))
        for value in [b'', b'abc\r\nHeader: injected', b'abc\x00def', b'\xff', b'a' * (m.MAX_TOKEN + 1)]:
            with self.assertRaises(m.CheckError): m.read_token(source=io.BytesIO(value))

    def test_report_is_private_and_never_overwrites(self):
        with tempfile.TemporaryDirectory() as directory:
            p = Path(directory) / 'report'
            m.write_report(p, {'safe': True})
            self.assertEqual(0o600, stat.S_IMODE(p.stat().st_mode))
            with self.assertRaises(m.CheckError): m.write_report(p, {'replaced': True})
            self.assertEqual({'safe': True}, json.loads(p.read_text()))
            link = Path(directory) / 'link'; link.symlink_to(p)
            with self.assertRaises(m.CheckError): m.write_report(link, {})

    def test_real_cli_get_only_and_custom_prefix(self):
        with server() as (base, calls), tempfile.TemporaryDirectory() as directory:
            out = Path(directory) / 'report.json'
            p = subprocess.run([sys.executable, str(SCRIPT), '--core-url', base, '--token-stdin', '--target-id', TARGET, '--output', str(out)], input=TOKEN+'\n', text=True, capture_output=True, timeout=10)
            self.assertEqual(0, p.returncode, p.stderr)
            self.assertEqual([('GET', '/custom/v2'+path) for path in m.PATHS], calls)
            self.assertNotIn(TOKEN, p.stdout+p.stderr+out.read_text())
            self.assertFalse(json.loads(out.read_text())['t155_passed'])

    def test_reflected_session_is_not_written_to_report(self):
        f = fixtures(); f[0]['data']['targets'][0]['name'] = TOKEN
        with server(f) as (base, calls), tempfile.TemporaryDirectory() as directory:
            out = Path(directory) / 'report.json'
            p = subprocess.run([sys.executable, str(SCRIPT), '--core-url', base, '--token-stdin', '--target-id', TARGET, '--output', str(out)], input=TOKEN, text=True, capture_output=True, timeout=10)
            self.assertEqual(2, p.returncode)
            self.assertFalse(out.exists())
            self.assertNotIn(TOKEN, p.stdout+p.stderr)

    def test_legacy_cli_returns_pending_not_passed(self):
        f = fixtures(); f[0]['data'].update(authority='legacy', targets=[])
        with server(f) as (base, calls), tempfile.TemporaryDirectory() as directory:
            out = Path(directory) / 'report.json'
            p = subprocess.run([sys.executable, str(SCRIPT), '--core-url', base, '--token-stdin', '--output', str(out)], input=TOKEN, text=True, capture_output=True, timeout=10)
            self.assertEqual(3, p.returncode)
            self.assertFalse(json.loads(out.read_text())['configuration_ready'])
            self.assertTrue(all(method=='GET' for method,_ in calls))

    def test_redirect_and_auth_errors_do_not_leak_or_follow(self):
        with server(redirect=True) as (base, calls):
            r = m.fetch(base, m.PATHS[0], TOKEN)
            self.assertEqual(302, r['http_status'])
            self.assertEqual(1, len(calls))
        with server() as (base, calls):
            r = m.fetch(base, m.PATHS[0], 'wrong')
            self.assertEqual('ROOT_SESSION_REQUIRED', r['problem'])
            self.assertNotIn('private', json.dumps(r))

    def test_duplicate_and_oversized_responses_are_rejected(self):
        with server(response_override=b'{"data":{},"data":{}}') as (base, _):
            self.assertEqual('RESPONSE_INVALID', m.fetch(base, m.PATHS[0], TOKEN)['problem'])
        with server(response_override=b'x' * (m.MAX_RESPONSE + 1)) as (base, _):
            self.assertEqual('RESPONSE_TOO_LARGE', m.fetch(base, m.PATHS[0], TOKEN)['problem'])


if __name__ == '__main__':
    unittest.main()
