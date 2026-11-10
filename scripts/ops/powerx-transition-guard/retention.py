#!/usr/bin/env python3
"""旧版只读鉴权追踪的有界期限执行器；每批先验证可恢复副本，不重试未知提交。"""
import datetime as dt
import fcntl
import json
import os
from pathlib import Path
import subprocess
import time

GIB = 1024 ** 3
ROOT = Path('/var/lib/powerx-baseline-retention')
BINARY = '/opt/powerx-ops/transition-20261109/baseline-retention'
EXPECTED_COMMIT = 'dc8583939bfb524dbd349fe645652e746deffc74'


def atomic_json(path, value):
    temp = path.with_name(path.name + '.next')
    with temp.open('w') as stream:
        json.dump(value, stream, indent=2); stream.write('\n'); stream.flush(); os.fsync(stream.fileno())
    temp.chmod(0o600); os.replace(temp, path)


def allocated_bytes(root):
    total = 0
    for directory, dirs, files in os.walk(root, followlinks=False):
        for name in dirs + files:
            path = Path(directory) / name
            if path.is_symlink():
                raise RuntimeError('retention_archive_symlink_requires_review')
            total += path.stat().st_blocks * 512
    return total


def confirmed_cursor(progress):
    if progress.get('state') not in ('bounded_complete', 'complete'):
        raise RuntimeError('prior_run_unconfirmed_manual_reconciliation_required')
    return int(progress['cursor'])


def run():
    ROOT.mkdir(exist_ok=True, mode=0o700)
    lock = (ROOT / 'supervisor.lock').open('a')
    try: fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        print(json.dumps({'status': 'skipped', 'reason': 'already_running'})); return
    manifest = Path('/opt/powerx/backend').resolve().parent / 'manifest.txt'
    values = dict(line.split('=', 1) for line in manifest.read_text().splitlines() if '=' in line)
    if values.get('commit') != EXPECTED_COMMIT:
        raise RuntimeError('runtime_upgraded_handoff_required')
    state_path = ROOT / 'supervisor.json'
    state = json.loads(state_path.read_text()) if state_path.exists() else {}
    if state.get('last_run'):
        progress_file = ROOT / state['last_run'] / 'progress.json'
        if not progress_file.exists():
            raise RuntimeError('prior_run_missing_receipt_requires_review')
        progress = json.loads(progress_file.read_text())
        state['cursor'] = confirmed_cursor(progress)
        if progress['state'] == 'complete':
            state['completed_at'] = time.time(); state.pop('cutoff', None); state.pop('highwater', None)
        state.pop('last_run', None)
        atomic_json(state_path, state)
    if time.time() - state.get('completed_at', 0) < 3600:
        print(json.dumps({'status': 'idle', 'reason': 'completed_sweep_backoff'})); return
    capacity = os.statvfs('/')
    available = capacity.f_bavail * capacity.f_frsize
    if available < 14*GIB:
        print(json.dumps({'status': 'paused', 'reason': 'minimum_14_gib_headroom', 'available_bytes': available})); return
    consumed = allocated_bytes(ROOT)
    remaining = 6*GIB - consumed
    if remaining < 128*1024**2:
        raise RuntimeError('rollback_archive_budget_requires_verified_offhost_transfer')
    if 'cutoff' not in state:
        state['cutoff'] = (dt.datetime.now(dt.timezone.utc) - dt.timedelta(days=7, seconds=1)).strftime('%Y-%m-%dT%H:%M:%SZ')
        result = subprocess.run(['psql', '-X', '-A', '-t', '-v', 'ON_ERROR_STOP=1', '-d', 'powerx_pro', '-c',
                                 'SELECT COALESCE(max(id),0) FROM public.capability_registry_invocation_traces'],
                                capture_output=True, text=True, timeout=8)
        if result.returncode: raise RuntimeError('retention_highwater_unavailable')
        state['highwater'] = int(result.stdout.strip()); state['cursor'] = 0
    name = 'run-' + str(time.time_ns())
    state['last_run'] = name; atomic_json(state_path, state)
    args = [BINARY, '-mode', 'apply', '-cutoff', state['cutoff'], '-out', str(ROOT / name),
            '-batch', '500', '-span', '2000', '-start-id', str(state['cursor']),
            '-end-id', str(state['highwater']), '-max-pairs', '20000', '-max-seconds', '100',
            '-min-free', str(14*GIB), '-backup-budget', str(min(remaining, 256*1024**2))]
    result = subprocess.run(args, capture_output=True, text=True, timeout=155)
    progress_file = ROOT / name / 'progress.json'
    if result.returncode or not progress_file.exists():
        raise RuntimeError('cleanup_stopped_inspect_private_receipts_no_automatic_retry')
    progress = json.loads(progress_file.read_text()); state['cursor'] = confirmed_cursor(progress)
    state['last_outcome'] = {'at': dt.datetime.now(dt.timezone.utc).isoformat(), 'pairs': progress['pairs'],
                             'backup_bytes': progress['backup_bytes'], 'run': name,
                             'archive_allocated_bytes_before_run': consumed}
    if progress['state'] == 'complete':
        state['completed_at'] = time.time(); state.pop('cutoff', None); state.pop('highwater', None)
    state.pop('last_run', None); atomic_json(state_path, state)
    print(json.dumps({'status': progress['state'], 'run': name, 'pairs': progress['pairs'],
                      'cursor': progress['cursor'], 'backup_bytes': progress['backup_bytes'],
                      'total_archive_allocated_bytes_before_run': consumed,
                      'at': dt.datetime.now(dt.timezone.utc).isoformat()}))


if __name__ == '__main__':
    try: run()
    except Exception as error:
        print(json.dumps({'status': 'failed', 'error_type': type(error).__name__,
                          'reason': str(error) if isinstance(error, RuntimeError) else 'operation_failed'}))
        raise SystemExit(1)
