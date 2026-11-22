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
import uuid
import yaml
from notify import send_personal
import log_output_health

GIB = 1024 ** 3
STATE = Path('/var/lib/powerx-ops/transition-20261109')
TOOLS = Path('/opt/powerx-ops/transition-20261109')
SNAPSHOT = Path('/opt/powerx/storage/system-snapshots')
EXPECTED_COMMIT = 'dc8583939bfb524dbd349fe645652e746deffc74'
MONITOR_KIND = 'monitor_only_no_schedule_changes_or_deletion'
CRITICAL_LABELS = {
    'disk': '磁盘容量告急',
    'core': 'Core 健康检查失败',
    'log_outputs': '无可用日志出口',
    'idle_retention': '空消费维护异常',
    'monitor': '临时监控检查失败',
}
RECOVERY_SAMPLES = 3
NOTIFICATION_RETRY_SECONDS = 300


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
    # Always re-read process identity and active configuration, even inside the
    # HTTP probe cache window. A deploy must not reuse old logging health.
    outputs = log_output_health.check_services()
    old = prior.get('runtime_health', {})
    old_pids = [r.get('pid') for r in old.get('log_outputs', {}).get('services', [])]
    pids = [r.get('pid') for r in outputs.get('services', [])]
    fresh = (time.time() - prior.get('health_checked_at', 0) < 300
             and old.get('log_outputs', {}).get('version') == log_output_health.VERSION and old_pids == pids)
    result = dict(old) if fresh else {}
    checked = prior.get('health_checked_at', 0) if fresh else time.time()
    if not fresh:
        for prefix, port in [('', 8080), ('dev_', 8081)]:
            start = time.monotonic()
            try:
                with urllib.request.urlopen('http://127.0.0.1:%d/api/v1/health' % port, timeout=5) as response:
                    result[prefix + 'core_status'] = response.status
                result[prefix + 'core_seconds'] = round(time.monotonic() - start, 4)
            except Exception:
                result[prefix + 'core_status'] = 0
                result[prefix + 'core_seconds'] = None
    result['log_outputs'] = outputs
    enabled = [s for row in outputs.get('services', []) for name, s in row.get('sinks', {}).items() if name == 'loki' and s['enabled']]
    result['loki_push_status'] = (204 if all(s['available'] for s in enabled) else 0) if enabled else None
    result['healthy'] = http_healthy(result) and outputs['healthy']
    return result, checked


def http_healthy(health):
    return all(health.get(prefix + 'core_status') == 200
               and isinstance(health.get(prefix + 'core_seconds'), (float, int))
               and health[prefix + 'core_seconds'] < 2 for prefix in ('', 'dev_'))


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


def idle_retention_status():
    """仅汇总独立空消费维护回执；监控进程自身不删表或重写 PostgreSQL。"""
    path = Path('/var/lib/powerx-idle-retention/latest.json')
    if not path.exists():
        return {'state': 'not_installed'}
    try:
        if path.is_symlink():
            raise ValueError('symlink')
        value = json.loads(path.read_text())
        at = dt.datetime.fromisoformat(value['at'])
        stale = (dt.datetime.now(dt.timezone.utc)-at).total_seconds() > 600
        databases = value['databases']
        return {'state': 'stale' if stale else 'reported', 'at': value['at'],
                'retention_days': value['retention_days'], 'databases': databases,
                'automatic_physical_compaction': False}
    except Exception:
        return {'state': 'receipt_unavailable'}


def idle_retention_failed(value):
    if value.get('state') in ('stale', 'receipt_unavailable'):
        return True
    return any(item.get('state') == 'stopped' or item.get('helper_problem')
               for item in value.get('databases', {}).values())


def critical_reasons(fs, health, idle_failed):
    active = set()
    if fs.get('available_bytes', 20 * GIB) < 12 * GIB or fs.get('used_ratio', 0) >= .88:
        active.add('disk')
    core_healthy = http_healthy(health) if 'core_status' in health else health.get('healthy', True)
    if not core_healthy:
        active.add('core')
    if health.get('log_outputs', {}).get('healthy') is False:
        active.add('log_outputs')
    if idle_failed:
        active.add('idle_retention')
    return active


def notify_critical(report, active, text, prior_active=(), unknown=()):
    """只通知新出现的严重问题，并保留同一通知编号用于失败重试。"""
    incidents = report.setdefault('critical_incidents', {})
    if (prior_active and not incidents and report.get('severity') == 'critical'
            and report.get('notification', {}).get('status') == 'api_accepted'):
        # 沿用旧监控已发送的告警，部署时不补发同一故障。
        incidents['previously_notified'] = {
            'reasons': dict.fromkeys(prior_active, 0), 'accepted': True,
        }
    known = set()
    for key, incident in list(incidents.items()):
        reasons = incident['reasons']
        for reason in list(reasons):
            if reason in unknown:
                continue
            if reason in active:
                reasons[reason] = 0
            else:
                reasons[reason] += 1
                if reasons[reason] >= RECOVERY_SAMPLES:
                    del reasons[reason]
        if not reasons:
            del incidents[key]
        known.update(reasons)
    new = active - known
    if new:
        incidents[uuid.uuid4().hex] = {'reasons': dict.fromkeys(sorted(new), 0)}

    now = time.time()
    for key, incident in incidents.items():
        present = active.intersection(incident['reasons'])
        if not present or incident.get('accepted') or now - incident.get('attempted_at', 0) < NOTIFICATION_RETRY_SECONDS:
            continue
        incident['attempted_at'] = now
        report['event_id'] = key
        state = STATE / 'capacity-latest.json'
        # 提交通知前持久化编号，进程中断后仍用同一个飞书去重编号。
        atomic_json(state, report)
        summary = '、'.join(CRITICAL_LABELS[reason] for reason in sorted(present))
        try:
            report['notification'] = send_personal(
                'PowerX 严重告警\n' + summary + '\n' + text, 'powerx-monitor-critical-' + key,
            )
            if report['notification']['status'] == 'api_accepted':
                report['notification_at'] = time.time()
                incident['accepted'] = True
        except Exception as error:
            report['notification'] = {'status': 'failed', 'error_type': type(error).__name__}
        atomic_json(state, report)


def observe(apply):
    """升级后仅观测与通知；不调用旧计划开关、期限删除或接管入口。"""
    STATE.mkdir(parents=True, exist_ok=True, mode=0o700)
    with (STATE / 'capacity.lock').open('a') as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            print(json.dumps({'status': 'skipped', 'reason': 'already_running'}))
            return
        return observe_locked(apply)


def observe_locked(apply):
    prior = json.loads((STATE / 'capacity-latest.json').read_text()) if (STATE / 'capacity-latest.json').exists() else {}
    fs = filesystem()
    health, health_at = runtime_health(prior)
    retention = retention_status()
    idle = idle_retention_status()
    idle_failed = idle_retention_failed(idle)
    active = critical_reasons(fs, health, idle_failed)
    severity = ('critical' if active else 'warning'
                if fs['available_bytes'] < 20 * GIB or health.get('log_outputs', {}).get('degraded') else 'ok')
    # 完整保留旧调度所有权、水位及未知结果；只读监控不处理这些写入职责。
    report = {**prior, 'at': dt.datetime.now(dt.timezone.utc).isoformat(),
              'severity': severity, 'filesystem': fs, 'runtime_health': health,
              'health_checked_at': health_at, 'retention_executor': retention,
              'idle_taskqueue_retention': idle,
              'idle_retention_failed': idle_failed,
              'guard_kind': MONITOR_KIND,
              'automatic_cleanup_active': False, 'handoff_complete': False,
              'changes': [], 'failure': None}
    if apply:
        prior_active = ()
        if ('critical_incidents' not in prior and prior.get('severity') == 'critical'
                and prior.get('guard_kind') == MONITOR_KIND):
            prior_active = critical_reasons(prior.get('filesystem', {}), prior.get('runtime_health', {}),
                                            prior.get('idle_retention_failed', False))
        display = ('服务器：47.116.10.200\n可用空间：%.2f GiB\n物理使用率：%.2f%%'
                   % (fs['available_bytes'] / GIB, fs['used_ratio'] * 100))
        if 'idle_retention' in active:
            display += '\n空消费维护：dev=%s，prod=%s。' % tuple(
                idle.get('databases', {}).get(env, {}).get('state', idle['state']) for env in ('dev', 'prod')
            )
        notify_critical(report, active, display + log_output_health.alert_text(health), prior_active)
        atomic_json(STATE / 'capacity-latest.json', report)
        history = STATE / 'capacity-observations'
        history.mkdir(exist_ok=True, mode=0o700)
        atomic_json(history / (str(time.time_ns()) + '.json'), report)
        for path in sorted(history.glob('*.json'))[:-2160]:
            if re.fullmatch(r'\d+\.json', path.name) and not path.is_symlink():
                path.unlink()
    print(json.dumps(report, ensure_ascii=False))
    return report


def observe_failure(failed):
    with (STATE / 'capacity.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        state = STATE / 'capacity-latest.json'
        prior = json.loads(state.read_text()) if state.exists() else {}
        prior.update({'severity': 'critical', 'failure': failed, 'at': dt.datetime.now(dt.timezone.utc).isoformat()})
        # 本轮其余检查未知，保留已有事件状态，不把检测失败视作恢复。
        notify_critical(prior, {'monitor'}, '服务器：47.116.10.200\n请核对服务器监控回执。',
                        unknown=set(CRITICAL_LABELS) - {'monitor'})
        atomic_json(state, prior)


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
    severity = 'critical' if fs['available_bytes'] < 12*GIB or fs['used_ratio'] >= .88 or not health.get('healthy') or executor_failed else ('warning' if fs['available_bytes'] < 20*GIB or owned or health.get('log_outputs', {}).get('degraded') else 'ok')
    report = {'at': dt.datetime.now(dt.timezone.utc).isoformat(), 'severity': severity,
              'filesystem': fs, 'owned_pauses': owned, 'changes': changes,
              'per_job_peak_bytes': {key: row['peak_bytes'] for key, row in rows.items()},
              'runtime_health': health, 'health_checked_at': health_at, 'retention_executor': retention,
              'guard_kind': 'legacy_schedule_pause_with_headroom_not_atomic_core_admission',
              'notification': prior.get('notification', {'status': 'not_configured'})}
    changed = severity != prior.get('severity') or bool(changes) or log_output_health.changed(prior, health)
    due = time.time() - prior.get('notification_at', 0) > 3600
    if apply and (changed or severity != 'ok' and due):
        identity = prior.get('event_id') if not changed else None
        report['event_id'] = identity or str(time.time_ns())
        text = ('PowerX 运维%s\n服务器：47.116.10.200\n可用空间：%.2f GiB\n物理使用率：%.2f%%\n暂停计划：%s'
                % ('恢复' if severity == 'ok' else '告警', fs['available_bytes']/GIB,
                   fs['used_ratio']*100, ', '.join(owned) or '无'))
        try:
            report['notification'] = send_personal(text + log_output_health.alert_text(health), 'powerx-capacity-' + report['event_id'])
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
    parser.add_argument('--observe-only', action='store_true', help='仅监控与个人告警，允许新版 Core；不改变计划或删除数据库行')
    args = parser.parse_args()
    try:
        if args.observe_only:
            observe(args.apply)
        else:
            run(args.apply)
    except Exception as error:
        failed = {'status': 'failed', 'error_type': type(error).__name__,
                  'reason': str(error) if isinstance(error, RuntimeError) else 'operation_failed'}
        if args.apply:
            try:
                if args.observe_only:
                    observe_failure(failed)
                else:
                    old = json.loads((STATE / 'capacity-latest.json').read_text()) if (STATE / 'capacity-latest.json').exists() else {}
                    old.update({'severity': 'critical', 'failure': failed, 'at': dt.datetime.now(dt.timezone.utc).isoformat()})
                    identity = 'powerx-guard-failed-' + failed['reason'] + '-' + str(int(time.time()//3600))
                    old['notification'] = send_personal('PowerX 运维告警\n47.116.10.200 临时容量执行器停止，需要核对服务器回执。原因：' + failed['reason'], identity)
                    atomic_json(STATE / 'capacity-latest.json', old)
            except Exception:
                failed['notification'] = 'not_delivered'
        print(json.dumps(failed))
        raise SystemExit(1)
