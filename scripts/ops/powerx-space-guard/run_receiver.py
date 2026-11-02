#!/usr/bin/env python3
"""Bound the whole receiver process independently of SDK/socket retry behavior."""
import argparse
import fcntl
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import time

from audit_custody import private_json
import guard


def supervise(settings, command, timeout=285):
    directory = Path(settings['state_directory']).resolve()
    directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    st = directory.stat()
    if st.st_uid != os.geteuid() or st.st_mode & 0o077:
        raise RuntimeError('receiver_state_unsafe')
    with (directory / 'supervisor.lock').open('a') as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            return 0, {'status': 'skipped', 'reason': 'receiver_already_running'}
        start, monotonic = time.time(), time.monotonic()
        child = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
        info = {'started_at': start, 'supervisor_pid': os.getpid(), 'child_pid': child.pid, 'timeout_seconds': timeout}
        timed_out = False
        try:
            out, err = child.communicate(timeout=timeout)
        except subprocess.TimeoutExpired:
            timed_out = True
            os.killpg(child.pid, signal.SIGTERM)
            try:
                out, err = child.communicate(timeout=3)
            except subprocess.TimeoutExpired:
                os.killpg(child.pid, signal.SIGKILL)
                out, err = child.communicate(timeout=3)
            # The session is exclusively this worker and its SSH descendants.
            try:
                os.killpg(child.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
        info.update(elapsed_seconds=round(time.monotonic() - monotonic, 3), timed_out=timed_out,
                    child_exit=child.returncode, child_stderr_bytes=len(err), finished_at=time.time())
        guard.atomic_json(directory / 'supervisor.json', info)
        if timed_out:
            # A commit response may have been lost; never fabricate zero deletes.
            report = {'started_at': start, 'finished_at': time.time(), 'status': 'unconfirmed',
                      'reason': 'receiver_run_timeout', 'source_results_unknown': True,
                      'source_commit_unconfirmed': 1, 'worker_group_terminated': True}
            for path in directory.glob('audit-download-*'):
                if path.is_file() and not path.is_symlink() and path.stat().st_uid == os.geteuid():
                    path.unlink()
            guard.atomic_json(directory / 'last-run.json', report)
            return 2, report
        try:
            if len(out) > 1 << 20:
                raise ValueError('oversized worker report')
            report = json.loads(out)
            if not isinstance(report, dict) or 'status' not in report:
                raise ValueError('invalid worker report')
        except (ValueError, TypeError):
            report = {'status': 'failed', 'reason': 'receiver_report_unavailable', 'source_results_unknown': True}
            guard.atomic_json(directory / 'last-run.json', report)
        return child.returncode or (0 if report['status'] == 'ok' else 2), report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--config', required=True)
    args = parser.parse_args()
    os.umask(0o077)
    try:
        settings = private_json(Path(args.config))
        code, report = supervise(settings, [sys.executable, str(Path(__file__).with_name('archive_s3.py')), '--config', args.config])
        print(json.dumps(report))
        return code
    except Exception:
        print(json.dumps({'status': 'failed', 'reason': 'receiver_supervisor_unavailable', 'source_results_unknown': True}))
        return 2


if __name__ == '__main__':
    sys.exit(main())
