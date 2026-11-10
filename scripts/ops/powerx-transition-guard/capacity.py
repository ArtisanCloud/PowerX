#!/usr/bin/env python3
"""旧版 Core 过渡期容量检查。仅暂停自己接管的固定计划，升级或外部变更即停止接管。"""
import argparse
import datetime as dt
import fcntl
import hashlib
import json
import math
import os
from pathlib import Path
import subprocess
import time
import urllib.request
import re
import yaml
from notify import send_personal

GIB = 1024 ** 3
STATE = Path('/var/lib/powerx-ops/transition-20261109')
TOOLS = Path('/opt/powerx-ops/transition-20261109')
SNAPSHOT = Path('/opt/powerx/storage/system-snapshots')
EXPECTED_COMMIT = 'dc8583939bfb524dbd349fe645652e746deffc74'


def atomic_json(path, obj):
    temp = path.with_name(path.name + '.next')
    with temp.open('w') as stream:
        json.dump(obj, stream, ensure_ascii=False, indent=2)
        stream.write('\n'); stream.flush(); os.fsync(stream.fileno())
    temp.chmod(0o600); os.replace(temp, path)


def legacy_runtime():
    manifest = Path('/opt/powerx/backend').resolve().parent / 'manifest.txt'
    values = dict(line.split('=', 1) for line in manifest.read_text().splitlines() if '=' in line)
    return (values.get('commit') == EXPECTED_COMMIT
            and not (SNAPSHOT / 'policy-control-binding.json').exists())


def filesystem():
    info = os.statvfs('/')
    return {'total_bytes': info.f_blocks * info.f_frsize,
            'available_bytes': info.f_bavail * info.f_frsize,
            'used_ratio': (info.f_blocks - info.f_bfree) / info.f_blocks}


def sql(database, statement):
    result = subprocess.run(['runuser', '-u', 'postgres', '--', 'psql', '-X', '-A', '-t',
                             '-v', 'ON_ERROR_STOP=1', '-d', database, '-c',
                             "BEGIN READ ONLY; SET LOCAL statement_timeout='4s'; " + statement + '; COMMIT;'],
                            capture_output=True, text=True, timeout=8)
    if result.returncode:
        raise RuntimeError('capacity_metadata_query_failed')
    return [json.loads(line) for line in result.stdout.splitlines() if line.startswith('{')]


def policies():
    raw = json.loads((SNAPSHOT / 'policies.json').read_text())
    current = next(p for p in raw['policies'] if p['id'] == raw['default_policy_id'])
    if current['retention_count'] != 3 or current['interval_minutes'] != 1440:
        raise RuntimeError('snapshot_policy_changed_stop_guard')
    rows = {'snapshot': {'enabled': current['enabled'], 'marker': str((SNAPSHOT / 'policies.json').stat().st_mtime_ns)}}
    size = 0
    for path in (SNAPSHOT / 'snapshots').glob('*.json'):
        point = json.loads(path.read_text())
        if point.get('status') == 'ready':
            size = max(size, sum(c.get('size_bytes', 0) for c in point.get('components', [])))
    rows['snapshot']['peak_bytes'] = math.ceil(size * 1.5) + GIB
    for target, database, count in [('prod-backup', 'powerx_pro', 3), ('dev-backup', 'powerx_dev', 24)]:
        data = sql(database, """SELECT json_build_object('enabled',p.enabled,'marker',p.updated_at::text,
             'retention_count',p.retention_count,'retention_mode',p.retention_mode,
             'peak_source_bytes',(SELECT COALESCE(max(a.size_bytes),0) FROM public.ops_backup_artifacts a
               JOIN public.ops_backup_jobs j ON j.id=a.job_id WHERE j.policy_id=1 AND a.deleted_at IS NULL))
             FROM public.ops_backup_policies p WHERE p.id=1 AND p.deleted_at IS NULL"""
        )[0]
        if data['retention_count'] != count or data['retention_mode'] != 'count':
            raise RuntimeError('backup_policy_changed_stop_guard')
        rows[target] = {'enabled': data['enabled'], 'marker': data['marker'],
                        'peak_bytes': math.ceil(data['peak_source_bytes'] * 1.5) + GIB}
    return rows


def decision(available, peak, owned_pause):
    # 常驻服务和 WAL 的底线在预估任务峰值之外；恢复多留 2 GiB 防止反复切换。
    required = 12 * GIB + peak
    if not owned_pause and available < required:
        return 'pause'
    if owned_pause and available >= required + 2 * GIB:
        return 'resume'
    return None


def set_enabled(target, enabled):
    config = '/etc/powerx-dev/config.yaml' if target == 'dev-backup' else '/etc/powerx/config.yaml'
    env = os.environ.copy()
    env['PX_TRANSITION_DSN'] = yaml.safe_load(Path(config).read_text())['database']['dsn']
    action = ('resume-' if enabled else 'pause-') + target
    result = subprocess.run([str(TOOLS / 'transition-maintain'), '-action', action],
                            env=env, capture_output=True, text=True, timeout=35)
    if result.returncode:
        raise RuntimeError('schedule_state_change_failed_' + target)


def runtime_health(prior):
    if time.time() - prior.get('health_checked_at', 0) < 300:
        return prior.get('runtime_health', {}), prior.get('health_checked_at', 0)
    result = {}
    start = time.monotonic()
    try:
        with urllib.request.urlopen('http://127.0.0.1:8080/api/v1/admin/setup/status', timeout=5) as response:
            result['core_status'] = response.status
        result['core_seconds'] = round(time.monotonic() - start, 4)
    except Exception:
        result['core_status'] = 0
    try:
        payload = {'streams': [{'stream': {'service': 'powerx-ops-diagnostic', 'module': 'runtime'},
                                'values': [[str(time.time_ns()), 'Temporary capacity guard health probe']]}]}
        request = urllib.request.Request('http://127.0.0.1:3100/loki/api/v1/push',
                                        data=json.dumps(payload).encode(), headers={'Content-Type': 'application/json'})
        with urllib.request.urlopen(request, timeout=5) as response: result['loki_push_status'] = response.status
    except Exception:
        result['loki_push_status'] = 0
    result['healthy'] = result['core_status'] == 200 and result.get('core_seconds', 99) < 2 and result['loki_push_status'] == 204
    return result, time.time()


def retention_status():
    root = Path('/var/lib/powerx-baseline-retention')
    file = root / 'supervisor.json'
    if not file.exists(): return {'state': 'not_started'}
    value = json.loads(file.read_text())
    result = {'last_outcome': value.get('last_outcome'), 'cursor': value.get('cursor'),
              'highwater': value.get('highwater')}
    name = value.get('last_run')
    if name:
        if not re.fullmatch(r'run-\d+', name): raise RuntimeError('retention_run_identity_invalid')
        progress = root / name / 'progress.json'
        if progress.exists(): result['state'] = json.loads(progress.read_text()).get('state')
        else: result['state'] = 'starting'
    else: result['state'] = 'idle'
    status = subprocess.run(['systemctl', 'show', 'powerx-baseline-retention.service', '-p', 'Result', '--value'],
                            capture_output=True, text=True, timeout=4)
    result['service_result'] = status.stdout.strip() if status.returncode == 0 else 'unknown'
    return result


def run(apply):
    STATE.mkdir(parents=True, exist_ok=True, mode=0o700)
    lock = (STATE / 'capacity.lock').open('a')
    try: fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        print(json.dumps({'status': 'skipped', 'reason': 'already_running'})); return
    if not legacy_runtime():
        raise RuntimeError('runtime_upgraded_handoff_required')
    prior = json.loads((STATE / 'capacity-latest.json').read_text()) if (STATE / 'capacity-latest.json').exists() else {}
    owned = dict(prior.get('owned_pauses', {}))
    fs = filesystem(); rows = policies(); changes = []
    health, health_at = runtime_health(prior)
    retention = retention_status()
    for target, row in rows.items():
        marker = owned.get(target)
        if marker and (row['enabled'] or marker != row['marker']):
            # 用户或其他执行器写过配置，不擅自重新启用。
            owned.pop(target); changes.append({'target': target, 'state': 'external_change_released'})
            continue
        choice = decision(fs['available_bytes'], row['peak_bytes'], bool(marker))
        if choice == 'pause' and not row['enabled']:
            continue
        if choice and apply:
            set_enabled(target, choice == 'resume')
            updated = policies()[target]
            if choice == 'pause': owned[target] = updated['marker']
            else: owned.pop(target, None)
            # 每个服务调用后立即记录所有权，后续阶段失败也不丢失暂缓来源。
            atomic_json(STATE / 'capacity-latest.json', {**prior, 'owned_pauses': owned, 'pending_verification': True})
        if choice: changes.append({'target': target, 'state': choice, 'applied': apply})
    executor_failed = retention.get('state') in ('stopped_error', 'commit_unknown') or retention.get('service_result') not in ('success', 'unknown')
    severity = 'critical' if fs['available_bytes'] < 12*GIB or fs['used_ratio'] >= .88 or not health.get('healthy') or executor_failed else ('warning' if fs['available_bytes'] < 20*GIB or owned else 'ok')
    report = {'at': dt.datetime.now(dt.timezone.utc).isoformat(), 'severity': severity,
              'filesystem': fs, 'owned_pauses': owned, 'changes': changes,
              'per_job_peak_bytes': {key: row['peak_bytes'] for key, row in rows.items()},
              'runtime_health': health, 'health_checked_at': health_at, 'retention_executor': retention,
              'guard_kind': 'legacy_schedule_pause_with_headroom_not_atomic_core_admission',
              'notification': prior.get('notification', {'status': 'not_configured'})}
    changed = severity != prior.get('severity') or bool(changes)
    due = time.time() - prior.get('notification_at', 0) > 3600
    if apply and (changed or severity != 'ok' and due):
        identity = prior.get('event_id') if not changed else None
        report['event_id'] = identity or str(time.time_ns())
        text = ('PowerX 运维%s\n服务器：47.116.10.200\n可用空间：%.2f GiB\n物理使用率：%.2f%%\n暂停计划：%s'
                % ('恢复' if severity == 'ok' else '告警', fs['available_bytes']/GIB,
                   fs['used_ratio']*100, ', '.join(owned) or '无'))
        try:
            report['notification'] = send_personal(text, 'powerx-capacity-' + report['event_id'])
            if report['notification']['status'] == 'api_accepted': report['notification_at'] = time.time()
        except Exception as error:
            report['notification'] = {'status': 'failed', 'error_type': type(error).__name__}
    else:
        report['event_id'] = prior.get('event_id'); report['notification_at'] = prior.get('notification_at', 0)
    if apply:
        atomic_json(STATE / 'capacity-latest.json', report)
        history = STATE / 'capacity-observations'
        history.mkdir(exist_ok=True, mode=0o700)
        atomic_json(history / (str(time.time_ns()) + '.json'), report)
        # 约 36 小时的有界样本，用于后续实际备份/快照周期和 24 小时增长复核。
        for path in sorted(history.glob('*.json'))[:-2160]:
            if re.fullmatch(r'\d+\.json', path.name) and not path.is_symlink(): path.unlink()
    print(json.dumps(report, ensure_ascii=False))
    return report


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__); parser.add_argument('--apply', action='store_true')
    args = parser.parse_args()
    try: run(args.apply)
    except Exception as error:
        failed = {'status': 'failed', 'error_type': type(error).__name__,
                  'reason': str(error) if isinstance(error, RuntimeError) else 'operation_failed'}
        if args.apply:
            try:
                old = json.loads((STATE / 'capacity-latest.json').read_text()) if (STATE / 'capacity-latest.json').exists() else {}
                old.update({'severity': 'critical', 'failure': failed, 'at': dt.datetime.now(dt.timezone.utc).isoformat()})
                identity = 'powerx-guard-failed-' + failed['reason'] + '-' + str(int(time.time()//3600))
                old['notification'] = send_personal('PowerX 运维告警\n47.116.10.200 临时容量执行器停止，需要核对服务器回执。原因：' + failed['reason'], identity)
                atomic_json(STATE / 'capacity-latest.json', old)
            except Exception:
                failed['notification'] = 'not_delivered'
        print(json.dumps(failed))
        raise SystemExit(1)
