#!/usr/bin/env python3
"""临时空消费审计维护；默认七天，显式 dev 历史范围可保留 24 小时。"""
import datetime as dt
import fcntl
import json
import os
from pathlib import Path
import re
import subprocess
import time

GIB = 1024 ** 3
STATE = Path('/var/lib/powerx-idle-retention')
ARCHIVES = Path('/var/lib/powerx-idle-retention-archives')
CONFIG = Path('/etc/powerx-idle-retention.json')
BINARY = '/opt/powerx-ops/taskqueue-idle-20261115/idle-taskqueue-retention'
DEV_BINARY = '/opt/powerx-ops/dev-reclaim-20261116/dev-idle-retention-controlled'
TARGETS = {'dev': ('powerx_dev', '/opt/powerx-dev'), 'prod': ('powerx_pro', '/opt/powerx')}


def atomic_json(path, value):
    temp = path.with_name(path.name + '.next')
    with temp.open('w') as stream:
        json.dump(value, stream, ensure_ascii=False, indent=2)
        stream.write('\n'); stream.flush(); os.fsync(stream.fileno())
    temp.chmod(0o600); os.replace(temp, path)
    directory = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
    try: os.fsync(directory)
    finally: os.close(directory)


def confirmed(progress):
    if progress.get('state') not in ('complete', 'bounded_complete'):
        raise RuntimeError('cleanup_result_unconfirmed_manual_review_required')
    return int(progress['cursor'])


def configuration(path=CONFIG):
    stat = path.lstat()
    if path.is_symlink() or stat.st_uid != 0 or stat.st_mode & 0o022:
        raise RuntimeError('idle_retention_configuration_not_private')
    value = json.loads(path.read_text())
    if (value.get('version') != 1 or value.get('retention_days') != 7
            or not isinstance(value.get('apply'), bool)
            or set(value.get('targets', {})) != set(TARGETS)):
        raise RuntimeError('idle_retention_configuration_invalid')
    for name, item in value['targets'].items():
        development_binding(name, item)
        if (not re.fullmatch(r'[0-9a-f]{40}', item.get('source_commit', ''))
                or not item.get('native_last_success_at')):
            raise RuntimeError('idle_retention_runtime_binding_invalid')
    return value


def development_binding(name, binding):
    hours = binding.get('retention_hours', 168)
    if hours == 168:
        return False
    if name != 'dev' or hours != 24 or not isinstance(binding.get('highwater'), int) or binding['highwater'] <= 0 or not re.fullmatch(r'[0-9a-f]{64}', binding.get('scope_id','')):
        raise RuntimeError('development_retention_scope_invalid')
    return True


def runtime_guard(name, binding):
    _, installation = TARGETS[name]
    if development_binding(name, binding):
        manifest = json.loads(Path(installation, 'backend/powerx.build.json').read_text())
        if manifest.get('source_commit') != binding['source_commit']:
            raise RuntimeError('runtime_changed_handoff_review_required')
        health = json.loads(Path(installation, 'storage/storage-governance/cleanup-health.json').read_text())
        if health.get('state') not in ('completed', 'running') or not health.get('last_success_at'):
            raise RuntimeError('native_cleanup_unhealthy_review_required')
        return
    manifest = Path(installation, 'backend').resolve().parent / 'manifest.txt'
    values = dict(line.split('=', 1) for line in manifest.read_text().splitlines() if '=' in line)
    if values.get('commit') != binding['source_commit']:
        raise RuntimeError('runtime_changed_handoff_review_required')
    health = json.loads(Path(installation, 'storage/storage-governance/cleanup-health.json').read_text())
    if (health.get('state') != 'failed'
            or health.get('last_success_at') != binding['native_last_success_at']):
        raise RuntimeError('native_cleanup_changed_handoff_review_required')


def allocated_bytes(root):
    total = 0
    for directory, dirs, files in os.walk(root, followlinks=False):
        for name in dirs + files:
            path = Path(directory, name)
            if path.is_symlink():
                raise RuntimeError('archive_symlink_requires_review')
            total += path.stat().st_blocks * 512
    return total


def metadata(database):
    # 固定数据库名及两张表；不输出负载、凭据、SQL 正文或业务数据。
    if database not in ('powerx_dev', 'powerx_pro'):
        raise RuntimeError('unexpected_database')
    sql = """BEGIN READ ONLY; SET LOCAL statement_timeout='4s';
      SELECT json_build_object('highwater',(SELECT COALESCE(max(id),0) FROM public.capability_registry_invocation_traces),
      'relation_bytes',pg_total_relation_size('public.capability_registry_invocation_traces')+pg_total_relation_size('public.capability_registry_event_publications'),
      'dead_rows',(SELECT COALESCE(sum(n_dead_tup),0) FROM pg_stat_user_tables WHERE schemaname='public' AND relname IN ('capability_registry_invocation_traces','capability_registry_event_publications')));
      COMMIT;"""
    result = subprocess.run(['runuser', '-u', 'postgres', '--', 'psql', '-X', '-A', '-t',
                             '-v', 'ON_ERROR_STOP=1', '-d', database, '-c', sql],
                            capture_output=True, text=True, timeout=8, cwd='/')
    if result.returncode:
        raise RuntimeError('idle_retention_metadata_unavailable')
    return next(json.loads(line) for line in result.stdout.splitlines() if line.startswith('{'))


def execute(database, mode, sweep, available, development=None):
    # preview 与 apply 使用独立游标，预览不会被当作已删除进度。
    directory = ARCHIVES / database
    directory.mkdir(mode=0o700, exist_ok=True)
    os.chown(directory, 0, 0)
    # 仅受控 helper 使用 postgres 的本地 peer 连接；归档子树由 postgres 管理。
    import pwd
    postgres = pwd.getpwnam('postgres')
    os.chown(directory, postgres.pw_uid, postgres.pw_gid)
    name = mode + '-' + str(time.time_ns())
    output = directory / name
    sweep['last_pairs'] = 0
    sweep['last_run'] = name
    arguments = ['runuser', '-u', 'postgres', '--', BINARY, '-database', database, '-mode', mode,
                 '-out', str(output), '-cutoff', sweep['cutoff'], '-start-id', str(sweep['cursor']),
                 '-end-id', str(sweep['highwater']), '-batch', '500', '-span', '2000',
                 '-max-pairs', '20000', '-max-seconds', '25', '-min-free', str(8*GIB),
                 '-backup-budget', str(min(128*1024**2, max(1, 4*GIB-allocated_bytes(ARCHIVES))))]
    if development is not None:
        if database != 'powerx_dev':
            raise RuntimeError('development_retention_scope_invalid')
        arguments[4] = DEV_BINARY
        arguments += ['-dev-keep-hours', '24', '-expected-dev-commit', development['source_commit'], '-expected-dev-scope', development['scope_id']]
        for flag, value in [('-batch', '5000'), ('-max-pairs', '100000')]:
            arguments[arguments.index(flag)+1] = value
    return arguments, output


def prune_previews(database, keep=32):
    # 已确认的只读扫描收据有界保留；绝不删除 apply 的恢复副本和删除意图。
    directory = ARCHIVES / database
    previews = sorted(p for p in directory.glob('preview-*') if re.fullmatch(r'preview-\d+', p.name))
    for path in previews[:-keep]:
        if path.is_symlink() or not path.is_dir():
            raise RuntimeError('preview_receipt_path_invalid')
        files = list(path.iterdir())
        if any(p.name not in ('run-contract.json', 'progress.json') or not p.is_file() or p.is_symlink() for p in files):
            raise RuntimeError('preview_receipt_contains_unexpected_files')
        for p in files: p.unlink()
        path.rmdir()


def reconcile(database, mode, sweep):
    name = sweep.get('last_run')
    if not name:
        return
    if not re.fullmatch(mode + r'-\d+', name):
        raise RuntimeError('idle_retention_receipt_identity_invalid')
    path = ARCHIVES / database / name / 'progress.json'
    if not path.is_file():
        if mode == 'preview':
            sweep.pop('last_run', None); return
        raise RuntimeError('cleanup_receipt_missing_manual_review_required')
    progress = json.loads(path.read_text())
    if progress.get('mode') != mode or progress.get('cutoff') != sweep['cutoff']:
        raise RuntimeError('cleanup_receipt_binding_mismatch')
    if mode == 'preview' and progress.get('state') not in ('complete', 'bounded_complete'):
        sweep.pop('last_run', None); return
    cursor = confirmed(progress)
    if not sweep['cursor'] <= cursor <= sweep['highwater']:
        raise RuntimeError('cleanup_cursor_outside_frozen_sweep')
    sweep['cursor'] = cursor
    sweep['pairs'] += int(progress['pairs'])
    sweep['last_pairs'] = int(progress['pairs'])
    sweep['last_run_at'] = progress['at']
    sweep.pop('last_run', None)
    if progress['state'] == 'complete':
        sweep['completed_at'] = time.time()


def vacuum(database, development=False):
    # 正常 VACUUM 供库内复用；不在后台获取 TRUNCATE/FULL 的独占锁。
    if development and database != 'powerx_dev':
        raise RuntimeError('development_retention_scope_invalid')
    # 10ms/500 在 dev 整理后的约 1.6 GiB 上仍会用尽 60 秒；仅 dev 专项提高吞吐。
    delay = '2ms' if development else '10ms'
    settings = "SET lock_timeout='1s'; SET statement_timeout='60s'; SET vacuum_cost_delay='"+delay+"'; SET vacuum_cost_limit=500;"
    result = subprocess.run(['runuser', '-u', 'postgres', '--', 'psql', '-X', '-q',
                             '-v', 'ON_ERROR_STOP=1', '-d', database, '-c', settings,
                             '-c', 'VACUUM (ANALYZE, TRUNCATE FALSE, PARALLEL 0) public.capability_registry_invocation_traces, public.capability_registry_event_publications'],
                            capture_output=True, text=True, timeout=70, cwd='/')
    return {'at': dt.datetime.now(dt.timezone.utc).isoformat(), 'success': result.returncode == 0,
            'kind': 'ordinary_vacuum_internal_reuse_only', 'filesystem_shrink_promised': False}


def maintain(name, config, report):
    database, _ = TARGETS[name]
    binding = config['targets'][name]
    development = development_binding(name, binding)
    runtime_guard(name, binding)
    path = STATE / (name + ('-24h' if development else '') + '.json')
    state = json.loads(path.read_text()) if path.exists() else {}
    if state.get('stopped'):
        raise RuntimeError(state['stopped'])
    before = metadata(database)
    if development:
        before['highwater'] = min(before['highwater'], binding['highwater'])
    report.update({'retention_hours': 24 if development else 168, 'frozen_source_highwater': binding.get('highwater')})
    volume = os.statvfs('/')
    available = volume.f_bavail * volume.f_frsize
    remaining = 4*GIB - allocated_bytes(ARCHIVES)
    mode = 'apply' if config['apply'] and available >= 8*GIB and remaining >= 128*1024**2 else 'preview'
    report.update({'state': 'active' if mode == 'apply' else 'preview_only', 'mode': mode,
                   'reason': None if mode == 'apply' else 'minimum_8_gib_headroom_or_archive_budget',
                   'relation_bytes_before': before['relation_bytes'], 'dead_rows_estimate': before['dead_rows']})
    # 重启后必须先确认任何 apply 提交，不能用只读扫描覆盖未知结果。
    for old_mode in ('apply', 'preview'):
        if old_mode in state:
            try: reconcile(database, old_mode, state[old_mode])
            except RuntimeError as error:
                state['stopped'] = str(error); atomic_json(path, state); raise
    sweep = state.get(mode)
    if sweep and sweep.get('completed_at') and time.time()-sweep['completed_at'] < 3600:
        report.update({'state': 'sweep_complete', 'pairs_in_completed_sweep': sweep['pairs'],
                       'sweep_cutoff': sweep['cutoff'], 'mode': mode})
        atomic_json(path, state); return
    if not sweep or sweep.get('completed_at'):
        sweep = {'cutoff': (dt.datetime.now(dt.timezone.utc)-dt.timedelta(hours=24 if development else 168,seconds=1)).strftime('%Y-%m-%dT%H:%M:%SZ'),
                 'highwater': before['highwater'], 'cursor': 0, 'pairs': 0}
        state[mode] = sweep
    arguments, output = (execute(database, mode, sweep, available, binding) if development
                         else execute(database, mode, sweep, available))
    atomic_json(path, state)
    if mode == 'preview': prune_previews(database)
    result = subprocess.run(arguments, capture_output=True, text=True, timeout=70, cwd='/')
    try:
        reconcile(database, mode, sweep)
    except RuntimeError as error:
        state['stopped'] = str(error); atomic_json(path, state); raise
    atomic_json(path, state)
    if result.returncode:
        report['helper_problem'] = 'bounded_helper_stopped_check_private_receipts'
    after = metadata(database)
    report.update({'cursor': sweep['cursor'], 'highwater': sweep['highwater'],
                   'pairs_seen_or_deleted_in_sweep': sweep['pairs'], 'pairs_this_run': sweep.get('last_pairs', 0),
                   'sweep_complete': bool(sweep.get('completed_at')), 'sweep_cutoff': sweep['cutoff'],
                   'relation_bytes_after': after['relation_bytes'],
                   'relation_bytes_reclaimed_observed': before['relation_bytes']-after['relation_bytes'],
                   'deleted_pairs_this_run': sweep.get('last_pairs', 0) if mode == 'apply' else 0})
    # 不删除归档；至少一万对已删并且间隔一小时后再做有界普通 VACUUM。
    state['since_vacuum'] = state.get('since_vacuum', 0) + report['deleted_pairs_this_run']
    if mode == 'apply' and state['since_vacuum'] >= 10000 and time.time()-state.get('vacuum_at', 0) >= 3600:
        report['vacuum'] = vacuum(database, development=True) if development else vacuum(database)
        state['vacuum_at'] = time.time()
        if report['vacuum']['success']: state['since_vacuum'] = 0
        atomic_json(path, state)


def run():
    config = configuration()
    STATE.mkdir(mode=0o700, exist_ok=True)
    lock = (STATE / 'worker.lock').open('a')
    try: fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        print(json.dumps({'state': 'already_running'})); return
    report = {'at': dt.datetime.now(dt.timezone.utc).isoformat(), 'retention_days': 7,
              'scope': 'verified_successful_empty_taskqueue_dequeue_and_unique_undelivered_notification',
              'databases': {}, 'automatic_physical_compaction': False}
    for name in ('dev', 'prod'):
        item = {}; report['databases'][name] = item
        try: maintain(name, config, item)
        except Exception as error:
            item.update({'state': 'stopped', 'reason': str(error) if isinstance(error, RuntimeError) else 'operation_failed_check_private_receipts'})
    volume = os.statvfs('/')
    report['available_bytes'] = volume.f_bavail*volume.f_frsize
    atomic_json(STATE / 'latest.json', report)
    print(json.dumps(report, ensure_ascii=False))


if __name__ == '__main__':
    run()
