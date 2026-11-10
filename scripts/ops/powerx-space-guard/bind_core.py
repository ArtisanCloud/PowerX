#!/usr/bin/env python3
"""部署时登记 Core 与临时守护的固定来源；默认仅预览，不启用归档或删除日志。"""
import argparse
import ast
import fcntl
import hashlib
import json
import os
from pathlib import Path
import tempfile

import custody_policy


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--control-file', required=True)
    parser.add_argument('--node-id', required=True)
    parser.add_argument('--core-uid', type=int, required=True)
    parser.add_argument('--source-root', action='append', required=True)
    parser.add_argument('--binding-file', default=str(custody_policy.BINDING))
    parser.add_argument('--state-dir', default='/var/lib/powerx-space-guard')
    parser.add_argument('--apply', action='store_true')
    args = parser.parse_args()
    path = Path(args.control_file).resolve(strict=True)
    control = custody_policy.read_owned_json(path, args.core_uid, private=True)
    if control.get('protocol') != 2 or control.get('node_id') != args.node_id:
        raise RuntimeError('core_control_identity_mismatch')
    directories = [str(Path(p).resolve(strict=True)) for p in args.source_root]
    registered = {s.get('directory') for s in control.get('sources', []) if s.get('executor') == 'powerx'}
    if len(directories) != len(set(directories)) or not set(directories).issubset(registered):
        raise RuntimeError('source_not_registered_in_core')
    for directory in directories:
        quarantine = Path(directory) / '.powerx-audit-custody'
        if quarantine.exists() and (quarantine.is_symlink() or any(quarantine.iterdir())):
            raise RuntimeError('legacy_quarantine_requires_reconciliation')
    base = Path(__file__).resolve().parent
    out = {'protocol': 2, 'node_id': args.node_id, 'core_uid': args.core_uid,
           'control_file': str(path), 'sources': directories}
    for field, name in [('guard', 'guard.py'), ('helper', 'audit_custody.py'), ('policy', 'custody_policy.py')]:
        script = base / name
        tree = ast.parse(script.read_text())
        if not any(isinstance(n, ast.Assign) and any(isinstance(t, ast.Name) and t.id == 'CONTROL_PROTOCOL' for t in n.targets) and isinstance(n.value, ast.Constant) and n.value.value == 2 for n in tree.body):
            raise RuntimeError('control_program_upgrade_required')
        out[field + '_script'] = str(script)
        out[field + '_sha256'] = hashlib.sha256(script.read_bytes()).hexdigest()
    if args.apply:
        if os.geteuid() != 0:
            raise RuntimeError('root_required_for_binding')
        destination = Path(args.binding_file)
        if not destination.is_absolute() or destination.is_symlink():
            raise RuntimeError('binding_path_invalid')
        destination.parent.mkdir(parents=True, exist_ok=True, mode=0o755)
        state = Path(args.state_dir)
        state.mkdir(parents=True, exist_ok=True, mode=0o700)
        with (state / 'guard.lock').open('a') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            fd, temp = tempfile.mkstemp(prefix='.core-binding-', dir=destination.parent)
            try:
                with os.fdopen(fd, 'w') as f:
                    json.dump(out, f, indent=2)
                    f.write('\n')
                    f.flush()
                    os.fsync(f.fileno())
                os.chmod(temp, 0o644)  # 无凭据的 root 只读绑定；Core 非 root 用户可核对。
                os.replace(temp, destination)
            finally:
                if os.path.exists(temp):
                    os.unlink(temp)
    print(json.dumps({'applied': args.apply, 'binding': out}, ensure_ascii=False))


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print(json.dumps({'status': 'failed', 'reason': str(error) if isinstance(error, RuntimeError) else type(error).__name__}))
        raise SystemExit(2)
