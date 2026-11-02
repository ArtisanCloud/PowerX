import json
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest

import run_receiver


class SupervisorTest(unittest.TestCase):
    def test_deadline_terminates_only_owned_worker_and_marks_commit_unknown(self):
        unrelated = subprocess.Popen([sys.executable, '-c', 'import time; time.sleep(30)'], start_new_session=True)
        try:
            with tempfile.TemporaryDirectory() as directory:
                started = time.monotonic()
                code, report = run_receiver.supervise({'state_directory': directory}, [sys.executable, '-c', 'import time; time.sleep(30)'], timeout=.1)
                self.assertEqual(code, 2)
                self.assertEqual(report['status'], 'unconfirmed')
                self.assertTrue(report['source_results_unknown'])
                self.assertNotIn('source_deleted', report)
                self.assertLess(time.monotonic() - started, 5)
                self.assertIsNone(unrelated.poll())
                self.assertTrue(json.loads((Path(directory) / 'supervisor.json').read_text())['timed_out'])
        finally:
            unrelated.terminate()
            unrelated.wait(timeout=5)

    def test_completed_worker_preserves_its_actual_result(self):
        with tempfile.TemporaryDirectory() as directory:
            code, result = run_receiver.supervise({'state_directory': directory}, [sys.executable, '-c', 'print(\'{"status":"ok","source_deleted":1}\')'], timeout=5)
            self.assertEqual(code, 0)
            self.assertEqual(result['source_deleted'], 1)
            self.assertFalse(json.loads((Path(directory) / 'supervisor.json').read_text())['timed_out'])


if __name__ == '__main__':
    unittest.main()
