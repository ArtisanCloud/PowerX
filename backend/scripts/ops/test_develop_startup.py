import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

HELPER = Path(__file__).with_name('start-develop-web-admin.sh')


class DevelopWebStartupTest(unittest.TestCase):
    def launch(self, content, overrides=None):
        with tempfile.TemporaryDirectory(prefix='powerx-dev-start-') as directory:
            root = Path(directory)
            config = root / 'config.yaml'
            config.write_text(content)
            entry = root / 'web-admin/.output/server/index.mjs'
            entry.parent.mkdir(parents=True)
            entry.write_text("import os,json; print(json.dumps({k:os.environ[k] for k in ['PORT','POWERX_BACKEND','NUXT_PUBLIC_POWERX_CORE_BASE','NUXT_PUBLIC_WS_ORIGIN','NUXT_PUBLIC_WS_PATH']}))")
            env = {k: v for k, v in os.environ.items() if not k.startswith(('POWERX_', 'NUXT_', 'NODE_BIN'))}
            env.update(POWERX_CONFIG=str(config), POWERX_LINKS_ROOT=str(root), NODE_BIN=sys.executable)
            env.update(overrides or {})
            return subprocess.run(['bash', str(HELPER)], env=env, capture_output=True, text=True, timeout=5)

    def test_direct_server_port_and_comments_do_not_change_digits(self):
        result = self.launch('server: # configuration 2026\n  # port 9999\n  grpc:\n    port: 9000\n  port: 8181 # release 2\nweb_admin_port: 3101 # release 3\n')
        self.assertEqual(0, result.returncode, result.stderr)
        data = json.loads(result.stdout)
        self.assertEqual('http://127.0.0.1:8181', data['POWERX_BACKEND'])
        self.assertEqual('3101', data['PORT'])
        self.assertEqual('ws://127.0.0.1:8181', data['NUXT_PUBLIC_WS_ORIGIN'])

    def test_missing_optional_ports_use_development_defaults(self):
        result = self.launch('server:\n  grpc:\n    port: 9000\nother:\n  port: 9292\n')
        self.assertEqual(0, result.returncode, result.stderr)
        data = json.loads(result.stdout)
        self.assertEqual('http://127.0.0.1:8081', data['POWERX_BACKEND'])
        self.assertEqual('3001', data['PORT'])

    def test_public_origin_overrides_survive_while_ssr_uses_loopback(self):
        result = self.launch('server:\n    port: 8182\nweb_admin_port: 3102\n', {
            'POWERX_PUBLIC_BASE_URL': 'https://dev.example.invalid',
            'POWERX_PUBLIC_WS_ORIGIN': 'wss://dev.example.invalid',
        })
        self.assertEqual(0, result.returncode, result.stderr)
        data = json.loads(result.stdout)
        self.assertEqual('http://127.0.0.1:8182', data['POWERX_BACKEND'])
        self.assertEqual('https://dev.example.invalid', data['NUXT_PUBLIC_POWERX_CORE_BASE'])
        self.assertEqual('wss://dev.example.invalid', data['NUXT_PUBLIC_WS_ORIGIN'])
        self.assertEqual('/api/ws', data['NUXT_PUBLIC_WS_PATH'])

    def test_invalid_configured_port_stops_before_node(self):
        for value in ('0', '65536', 'invalid'):
            result = self.launch(f'server:\n  port: {value}\nweb_admin_port: 3101\n')
            self.assertNotEqual(0, result.returncode)
            self.assertEqual('', result.stdout)
            self.assertIn('POWERX_DEV_WEB_PORT_INVALID', result.stderr)


if __name__ == '__main__':
    unittest.main()
