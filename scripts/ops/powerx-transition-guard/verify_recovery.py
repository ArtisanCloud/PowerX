#!/usr/bin/env python3
"""临时保留策略预检：只输出恢复点元数据，不输出数据库或配置正文。"""
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import stat
import subprocess

ROOT = Path('/opt/powerx/storage/system-snapshots')
STATE = Path('/var/lib/powerx-ops/transition-20261109')


def sha_file(path, expected_size, expected_hash):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    with os.fdopen(fd, 'rb') as stream:
        before = os.fstat(stream.fileno())
        if not stat.S_ISREG(before.st_mode) or before.st_size != expected_size:
            raise RuntimeError('recovery_file_metadata_mismatch')
        hasher = hashlib.sha256()
        while True:
            chunk = stream.read(4 * 1024 * 1024)
            if not chunk:
                break
            hasher.update(chunk)
        digest = hasher.hexdigest()
        after = os.fstat(stream.fileno())
    if (before.st_ino, before.st_size, before.st_mtime_ns) != (
            after.st_ino, after.st_size, after.st_mtime_ns) or digest != expected_hash:
        raise RuntimeError('recovery_file_hash_or_identity_mismatch')
    return {'path': str(path), 'bytes': before.st_size, 'sha256': digest,
            'device': before.st_dev, 'inode': before.st_ino, 'mtime_ns': before.st_mtime_ns}


def read_sql(database, sql):
    run = subprocess.run(['runuser', '-u', 'postgres', '--', 'psql', '-X', '-A', '-t',
                          '-v', 'ON_ERROR_STOP=1', '-d', database, '-c', sql],
                         capture_output=True, text=True, timeout=20)
    if run.returncode:
        raise RuntimeError('read_only_metadata_query_failed')
    return [json.loads(line) for line in run.stdout.splitlines() if line.startswith('{')]


def main():
    STATE.mkdir(parents=True, exist_ok=True, mode=0o700)
    snapshots = [json.loads(p.read_text()) for p in (ROOT / 'snapshots').glob('*.json')]
    if any(s.get('status') in ('queued', 'running', 'capturing', 'verifying', 'restoring') for s in snapshots):
        raise RuntimeError('snapshot_job_active')
    ready = sorted([s for s in snapshots if s.get('status') == 'ready'
                    and s.get('trigger') == 'scheduled' and not s.get('protected')],
                   key=lambda s: s['created_at'], reverse=True)
    if len(ready) < 3:
        raise RuntimeError('not_enough_recovery_points')
    proof = {'at': dt.datetime.now(dt.timezone.utc).isoformat(), 'state': 'verifying',
             'snapshot_keep': [], 'snapshot_delete': [s['id'] for s in ready[3:]], 'dev_backup_keep': []}
    for point in ready[:3]:
        base = ROOT / 'contents' / point['id']
        manifest = json.loads((base / 'manifest.json').read_text())
        if manifest['id'] != point['id'] or manifest['contract'] != 'powerx.system-snapshot/v1':
            raise RuntimeError('snapshot_manifest_invalid')
        files = []
        for entry in manifest['files']:
            relative = Path(entry['path'])
            path = base / relative
            if relative.is_absolute() or '..' in relative.parts or path.resolve() != path:
                raise RuntimeError('snapshot_path_unsafe')
            files.append(sha_file(path, entry['size'], entry['sha256']))
        decode = subprocess.run(['pg_restore', '--file=/dev/null', str(base / 'database/database.dump')],
                                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=300)
        if decode.returncode:
            raise RuntimeError('retained_snapshot_database_decode_failed')
        proof['snapshot_keep'].append({'id': point['id'], 'created_at': point['created_at'],
                                       'files': files, 'database_full_decode': True})
        print(json.dumps({'verified_snapshot': point['id'], 'files': len(files)}), flush=True)
    rows = read_sql('powerx_dev', """BEGIN READ ONLY; SET LOCAL statement_timeout='5s';
        WITH kept AS (SELECT j.id AS job_id,a.id AS artifact_id,a.storage_uri,a.size_bytes,a.checksum,
           row_number() OVER(ORDER BY j.created_at DESC,j.id DESC) AS rn,j.protected
          FROM public.ops_backup_jobs j JOIN public.ops_backup_artifacts a ON a.job_id=j.id
          WHERE j.policy_id=1 AND j.status='success' AND j.deleted_at IS NULL AND a.deleted_at IS NULL)
        SELECT row_to_json(k) FROM kept k WHERE rn<=24 OR protected ORDER BY rn; COMMIT;"""
    )
    if len(rows) < 24:
        raise RuntimeError('insufficient_dev_backups')
    from urllib.parse import urlparse, unquote
    for row in rows:
        path = Path(unquote(urlparse(row['storage_uri']).path))
        if not str(path).startswith('/opt/powerx-dev/storage/backups/') or path.resolve() != path:
            raise RuntimeError('dev_backup_path_unsafe')
        proof['dev_backup_keep'].append({**row, **sha_file(path, row['size_bytes'], row['checksum'])})
    decode = subprocess.run(['pg_restore', '--file=/dev/null', proof['dev_backup_keep'][0]['path']],
                            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=60)
    if decode.returncode:
        raise RuntimeError('retained_dev_backup_decode_failed')
    proof['state'] = 'verified'
    destination = STATE / 'recovery-proof.json'
    destination.write_text(json.dumps(proof, indent=2) + '\n')
    destination.chmod(0o600)
    print(json.dumps({'state': 'verified', 'retained_snapshots': len(proof['snapshot_keep']),
                      'retained_dev_backups': len(proof['dev_backup_keep']),
                      'snapshot_delete': proof['snapshot_delete']}), flush=True)


if __name__ == '__main__':
    main()
