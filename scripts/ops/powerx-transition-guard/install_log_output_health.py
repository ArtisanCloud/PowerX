#!/usr/bin/env python3
"""Install a pre-validated, hash-pinned monitor update; no service/config changes."""
import argparse
import datetime as dt
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shutil
import stat
import sys

TARGET = Path('/opt/powerx-ops/transition-20261109')
NAMES = ('log_output_health.py', 'capacity.py')


def digest(path): return hashlib.sha256(path.read_bytes()).hexdigest()


def run(apply):
    source = Path(__file__).resolve().parent
    manifest = json.loads((source / 'manifest.json').read_text())
    if manifest.get('target') != str(TARGET) or set(manifest.get('files', {})) != set(NAMES):
        raise RuntimeError('package_manifest_invalid')
    info = TARGET.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022 or TARGET.resolve() != TARGET:
        raise RuntimeError('monitor_directory_identity_invalid')
    for name in NAMES:
        p = source / name
        if p.is_symlink() or not p.is_file() or digest(p) != manifest['files'][name]:
            raise RuntimeError('package_hash_mismatch')
        compile(p.read_bytes(), name, 'exec')
    existing = TARGET / 'capacity.py'
    info = existing.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_nlink != 1 or info.st_mode & 0o022:
        raise RuntimeError('monitor_script_identity_invalid')
    actual = digest(existing)
    if actual == manifest['files']['capacity.py'] and (TARGET / NAMES[0]).is_file() and digest(TARGET / NAMES[0]) == manifest['files'][NAMES[0]]:
        return {'state': 'already_installed', 'core_restarted': False}
    if actual != manifest['original_capacity_sha256']:
        raise RuntimeError('monitor_changed_since_preparation_no_overwrite')
    if (TARGET / NAMES[0]).exists():
        raise RuntimeError('existing_helper_requires_review')
    if not apply: return {'state': 'ready', 'target': str(TARGET), 'core_restart_required_for_monitor': False}
    if os.geteuid() != 0: raise RuntimeError('sudo_required')
    os.umask(0o077)
    with (TARGET / '.log-output-install.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        if digest(existing) != manifest['original_capacity_sha256'] or (TARGET / NAMES[0]).exists():
            raise RuntimeError('concurrent_monitor_update_no_overwrite')
        backup = TARGET / ('log-output-backup-' + dt.datetime.now().strftime('%Y%m%d-%H%M%S-%f'))
        backup.mkdir(mode=0o700)
        shutil.copy2(existing, backup / 'capacity.py')
        (backup / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
        installed = []
        try:
            # Install dependency first. The old timer remains valid until capacity.py changes.
            for name in NAMES:
                tmp = TARGET / ('.' + name + '.log-output-next')
                fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
                with os.fdopen(fd, 'wb') as f:
                    f.write((source / name).read_bytes()); f.flush(); os.fsync(f.fileno())
                tmp.chmod(0o755 if name == 'capacity.py' else 0o644)
                os.replace(tmp, TARGET / name)
                installed.append(name)
        except Exception:
            if 'capacity.py' in installed:
                shutil.copy2(backup / 'capacity.py', TARGET / '.capacity.py.rollback')
                os.replace(TARGET / '.capacity.py.rollback', existing)
            if 'log_output_health.py' in installed: (TARGET / 'log_output_health.py').unlink()
            raise
        directory = os.open(TARGET, os.O_RDONLY | os.O_DIRECTORY)
        try: os.fsync(directory)
        finally: os.close(directory)
    return {'state': 'installed', 'backup': str(backup), 'core_restarted': False,
            'config_changed': False, 'effective': 'next existing timer invocation'}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apply', action='store_true')
    args = parser.parse_args()
    try: print(json.dumps(run(args.apply)))
    except Exception as error:
        print(json.dumps({'state': 'failed', 'reason': str(error) if isinstance(error, RuntimeError) else type(error).__name__}))
        sys.exit(1)
