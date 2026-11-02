import contextlib
import io
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

import bind_core


class BindingPreviewTest(unittest.TestCase):
    def test_preview_is_source_bound_and_does_not_write_or_enable(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary).resolve()
            logs = root / 'logs'
            logs.mkdir()
            control = root / 'custody-control.json'
            control.write_text(json.dumps({'protocol': 2, 'node_id': 'test-node', 'sources': [
                {'directory': str(logs), 'executor': 'powerx', 'archive_enabled': False}]}))
            control.chmod(0o600)
            destination = root / 'binding.json'
            args = ['bind_core.py', '--control-file', str(control), '--node-id', 'test-node',
                    '--core-uid', str(os.geteuid()), '--source-root', str(logs),
                    '--binding-file', str(destination)]
            output = io.StringIO()
            with patch.object(sys, 'argv', args), contextlib.redirect_stdout(output):
                bind_core.main()
            result = json.loads(output.getvalue())
            self.assertFalse(result['applied'])
            self.assertEqual([str(logs)], result['binding']['sources'])
            self.assertFalse(destination.exists())
            self.assertFalse(json.loads(control.read_text())['sources'][0]['archive_enabled'])
            (logs / '.powerx-audit-custody').mkdir()
            (logs / '.powerx-audit-custody' / 'unconfirmed').write_text('canary')
            with patch.object(sys, 'argv', args), self.assertRaisesRegex(RuntimeError, 'quarantine_requires'):
                bind_core.main()
            self.assertFalse(destination.exists())


if __name__ == '__main__':
    unittest.main()
