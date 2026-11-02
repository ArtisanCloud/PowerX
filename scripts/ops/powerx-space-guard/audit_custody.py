#!/usr/bin/env python3
"""Root-only fixed-source custody RPC over authenticated SSH. No SQL payloads in reports."""
import argparse
import contextlib
import fcntl
import hashlib
import hmac
import json
import os
from pathlib import Path
import re
import stat
import sys
import time
import uuid

import guard
import custody_policy

CONTROL_PROTOCOL = 2

STATE = Path('/var/lib/powerx-space-guard')
MAX_INPUT = 65536
SNAPSHOT_FIELDS = {'source', 'name', 'root_dev', 'root_ino', 'dev', 'ino', 'size',
                   'mtime_ns', 'ctime_ns', 'uid', 'id'}


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=True).encode()


def private_json(path):
    s = path.lstat()
    if not stat.S_ISREG(s.st_mode) or s.st_nlink != 1 or s.st_uid != os.geteuid() or s.st_mode & 0o077 or s.st_size > MAX_INPUT:
        raise RuntimeError('private_configuration_unavailable')
    return json.loads(path.read_text())


def sources():
    return {name: (Path(directory).resolve(strict=True), r'audit_event-.*\.log(?:\.gz)?' if name in ['prod-audit', 'dev-audit'] else pattern)
            for name, directory, pattern, _ in guard.RULES
            if name in guard.AUDIT_SOURCES and Path(directory).exists()}


def identity(s):
    return {k: getattr(s, 'st_' + k) for k in ['dev', 'ino', 'size', 'mtime_ns', 'ctime_ns', 'uid']}


def file_id(item):
    # Rotation may rename an immutable inode; its content identity remains useful.
    return hashlib.sha256(canonical({k: item[k] for k in ['source', 'root_dev', 'root_ino', 'dev', 'ino', 'size', 'mtime_ns']})).hexdigest()


def snapshot(source, root, name, s):
    out = {'source': source, 'name': name, 'root_dev': root.stat().st_dev,
           'root_ino': root.stat().st_ino, **identity(s)}
    out['id'] = file_id(out)
    return out


def manifest():
    opened, result, eligible_ids = guard.open_inodes(), [], []
    enabled = False
    budgets = {rule[0]: rule[3] for rule in guard.RULES}
    for source, (root, pattern) in sources().items():
        policy = custody_policy.state_for(root)
        if not policy['allow_archive']:
            continue
        enabled = True
        count, rows, by_name = 0, [], {}
        for p in root.iterdir():
            count += 1
            if count > 8192:
                raise RuntimeError('audit_inventory_limit')
            s = p.lstat()
            if re.fullmatch(pattern, p.name) and stat.S_ISREG(s.st_mode) and s.st_nlink == 1 and (s.st_dev, s.st_ino) not in opened:
                item = snapshot(source, root, p.name, s)
                result.append(item)
                rows.append((s.st_mtime, p, s))
                by_name[p.name] = item['id']
            if len(result) > 4096:
                raise RuntimeError('audit_inventory_limit')
        for p, _, _ in guard.select_expired(sorted(rows, key=lambda row: row[0]), time.time(), 7, budgets[source]):
            eligible_ids.append(by_name[p.name])
    return {'version': 1, 'archive_enabled': enabled, 'control_protocol': 2,
            'files': sorted(result, key=lambda f: (f['mtime_ns'], f['id'])),
            'minimum_local_days': 7, 'eligible_ids': eligible_ids, 'complete': True}


@contextlib.contextmanager
def source_fd(item):
    if not isinstance(item, dict) or set(item) != SNAPSHOT_FIELDS or item.get('id') != file_id(item):
        raise RuntimeError('snapshot_invalid')
    source = sources().get(item['source'])
    if not source or not isinstance(item['name'], str) or Path(item['name']).name != item['name'] or not re.fullmatch(source[1], item['name']):
        raise RuntimeError('source_not_allowed')
    root = source[0]
    if not custody_policy.state_for(root)['allow_archive']:
        raise RuntimeError('archive_disabled_or_managed_by_powerx')
    d = os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        s = os.fstat(d)
        if (s.st_dev, s.st_ino) != (item['root_dev'], item['root_ino']):
            raise RuntimeError('source_replaced')
        current = os.stat(item['name'], dir_fd=d, follow_symlinks=False)
        if not stat.S_ISREG(current.st_mode) or current.st_nlink != 1 or identity(current) != {k: item[k] for k in identity(current)}:
            raise RuntimeError('file_changed')
        if (current.st_dev, current.st_ino) in guard.open_inodes():
            raise RuntimeError('file_open')
        f = os.open(item['name'], os.O_RDONLY | os.O_NOFOLLOW, dir_fd=d)
        try:
            if identity(os.fstat(f)) != identity(current):
                raise RuntimeError('file_changed')
            yield root, d, f
            if identity(os.fstat(f)) != identity(current):
                raise RuntimeError('file_changed')
        finally:
            os.close(f)
    finally:
        os.close(d)


def read_file(item, output):
    with source_fd(item) as (_, _, fd):
        remaining = item['size']
        while remaining:
            chunk = os.read(fd, min(remaining, 1 << 20))
            if not chunk:
                raise RuntimeError('source_short_read')
            output.write(chunk)
            remaining -= len(chunk)
        if os.read(fd, 1):
            raise RuntimeError('source_grew_during_transfer')
        output.flush()


def digest_fd(fd):
    h = hashlib.sha256()
    os.lseek(fd, 0, os.SEEK_SET)
    while True:
        chunk = os.read(fd, 1 << 20)
        if not chunk:
            break
        h.update(chunk)
    return h.hexdigest()


def verify_receipt(receipt, receiver, now=None):
    now = time.time() if now is None else now
    if not isinstance(receipt, dict) or set(receipt) != {'proof', 'signature'}:
        raise RuntimeError('receipt_invalid')
    proof = receipt['proof']
    if not isinstance(proof, dict) or set(proof) != {'version', 'receiver_id', 'verified_at', 'snapshot', 'object'} or proof['version'] != 1:
        raise RuntimeError('receipt_invalid')
    signature = hmac.new(bytes.fromhex(receiver['key_hex']), canonical(proof), hashlib.sha256).hexdigest()
    if not isinstance(receipt['signature'], str) or not hmac.compare_digest(signature, receipt['signature']):
        raise RuntimeError('receipt_signature_invalid')
    if proof['receiver_id'] != receiver['receiver_id'] or not isinstance(proof['verified_at'], (int, float)) or not -10 <= now - proof['verified_at'] <= 120:
        raise RuntimeError('receipt_stale_or_wrong_receiver')
    obj = proof['object']
    if set(obj) != {'endpoint_id', 'bucket', 'key', 'version_id', 'sha256', 'bytes', 'manifest_key', 'manifest_version_id', 'manifest_sha256'}:
        raise RuntimeError('object_proof_invalid')
    digest = obj['sha256']
    if not isinstance(digest, str) or not re.fullmatch('[a-f0-9]{64}', digest):
        raise RuntimeError('object_proof_invalid')
    expected_key = receiver['prefix'].rstrip('/') + '/objects/' + digest[:2] + '/' + digest
    if obj['bucket'] != receiver['bucket'] or obj['endpoint_id'] != receiver['endpoint_id'] or obj['key'] != expected_key or not obj['version_id'] or obj['version_id'] == 'null' or obj['bytes'] != proof['snapshot']['size']:
        raise RuntimeError('object_proof_invalid')
    if obj['manifest_key'] != receiver['prefix'].rstrip('/') + '/manifests/' + proof['snapshot']['id'] + '.json' or not obj['manifest_version_id'] or obj['manifest_version_id'] == 'null' or not re.fullmatch('[a-f0-9]{64}', obj['manifest_sha256']):
        raise RuntimeError('manifest_proof_invalid')
    return proof


def confirm(receipt, receiver, state=STATE, now=None):
    now = time.time() if now is None else now
    proof = verify_receipt(receipt, receiver, now)
    item, obj = proof['snapshot'], proof['object']
    # Local age/budget policy applies only after a fresh verified offsite receipt.
    rule = next((r for r in guard.RULES if r[0] == item['source']), None)
    source = sources().get(item['source'])
    if rule is None or source is None:
        raise RuntimeError('source_not_allowed')
    if not custody_policy.state_for(source[0])['allow_archive']:
        raise RuntimeError('archive_disabled_or_managed_by_powerx')
    eligible = guard.select_expired(guard.candidates(source[0], source[1], guard.open_inodes()), now, 7, rule[3])
    if not any(p.name == item['name'] for p, _, _ in eligible):
        return {'state': 'retained', 'reason': 'minimum_local_retention', 'id': item['id']}
    state.mkdir(mode=0o700, parents=True, exist_ok=True)
    with (state / 'guard.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        # Also exclude native logrotate renumbering during the final move.
        rotation = Path('/var/lib/logrotate/status')
        with contextlib.ExitStack() as stack:
            if rotation.exists():
                rlock = stack.enter_context(rotation.open('a'))
                fcntl.flock(rlock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            with source_fd(item) as (_, directory, fd):
                if digest_fd(fd) != obj['sha256']:
                    raise RuntimeError('source_digest_mismatch')
            # source_fd closed; the final directory descriptor stays separately pinned.
            root = sources()[item['source']][0]
            directory = os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
            try:
                dstat = os.fstat(directory)
                if (dstat.st_dev, dstat.st_ino) != (item['root_dev'], item['root_ino']):
                    raise RuntimeError('source_replaced')
                current = os.stat(item['name'], dir_fd=directory, follow_symlinks=False)
                if identity(current) != {k: item[k] for k in identity(current)} or current.st_nlink != 1:
                    raise RuntimeError('file_changed')
                if (current.st_dev, current.st_ino) in guard.open_inodes():
                    raise RuntimeError('file_open')
                try:
                    os.mkdir('.powerx-audit-custody', 0o700, dir_fd=directory)
                except FileExistsError:
                    pass
                q = os.open('.powerx-audit-custody', os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=directory)
                try:
                    qs = os.fstat(q)
                    if qs.st_uid != os.geteuid() or qs.st_mode & 0o077:
                        raise RuntimeError('quarantine_unsafe')
                    if os.listdir(q):
                        raise RuntimeError('quarantine_reconciliation_required')
                    held = uuid.uuid4().hex
                    receipts = state / 'archive-receipts'
                    receipts.mkdir(mode=0o700, exist_ok=True)
                    record = {'state': 'intent', **proof, 'quarantine': held}
                    record_path = receipts / (item['id'] + '.json')
                    guard.atomic_json(record_path, record)
                    os.rename(item['name'], held, src_dir_fd=directory, dst_dir_fd=q)
                    moved = os.stat(held, dir_fd=q, follow_symlinks=False)
                    if not stat.S_ISREG(moved.st_mode) or moved.st_nlink != 1 or any(getattr(moved, 'st_' + k) != item[k] for k in ['dev', 'ino', 'size', 'mtime_ns', 'uid']) or (moved.st_dev, moved.st_ino) in guard.open_inodes():
                        try:
                            os.link(held, item['name'], src_dir_fd=q, dst_dir_fd=directory, follow_symlinks=False)
                            os.unlink(held, dir_fd=q)
                        except OSError:
                            pass
                        raise RuntimeError('quarantine_identity_conflict')
                    os.fsync(directory)
                    os.fsync(q)
                    os.unlink(held, dir_fd=q)
                    os.fsync(q)
                    record['state'] = 'deleted'
                    guard.atomic_json(record_path, record)
                finally:
                    os.close(q)
            finally:
                os.close(directory)
    return {'state': 'deleted', 'id': item['id'], 'bytes': item['size'], 'archive_version': obj['version_id']}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('operation', choices=['manifest', 'read', 'confirm'])
    args = parser.parse_args()
    if os.geteuid() != 0:
        parser.error('root SSH operator required')
    try:
        if args.operation == 'manifest':
            print(json.dumps(manifest()))
            return 0
        raw = sys.stdin.buffer.read(MAX_INPUT + 1)
        if len(raw) > MAX_INPUT:
            raise RuntimeError('input_too_large')
        value = json.loads(raw)
        if args.operation == 'read':
            read_file(value, sys.stdout.buffer)
        else:
            print(json.dumps(confirm(value, private_json(STATE / 'archive-receiver.json'))))
        return 0
    except Exception:
        # No raw exception, paths, SQL or proof secret goes into the host journal.
        if args.operation != 'read':
            print(json.dumps({'state': 'blocked', 'reason': 'custody_verification_failed'}))
        return 2


if __name__ == '__main__':
    sys.exit(main())
