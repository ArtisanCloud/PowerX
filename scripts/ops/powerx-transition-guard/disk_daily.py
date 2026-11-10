#!/usr/bin/env python3
"""北京时间磁盘日报；独立于 Core、数据库和旧版容量接管。默认仅预览。"""
import argparse
import datetime as dt
import fcntl
import json
import math
import os
from pathlib import Path
import tempfile
from zoneinfo import ZoneInfo

from notify import send_personal

GIB = 1024 ** 3
SHANGHAI = ZoneInfo('Asia/Shanghai')
STATE = Path('/var/lib/powerx-ops/disk-daily')
GUARD = Path('/var/lib/powerx-ops/transition-20261109/capacity-latest.json')


def atomic_json(path, value):
    fd, temporary = tempfile.mkstemp(prefix=path.name + '.', dir=path.parent)
    try:
        os.fchmod(fd, 0o600)
        with os.fdopen(fd, 'w') as stream:
            json.dump(value, stream, ensure_ascii=False, indent=2)
            stream.write('\n'); stream.flush(); os.fsync(stream.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def filesystem():
    value = os.statvfs('/')
    total = value.f_blocks * value.f_frsize
    used = (value.f_blocks - value.f_bfree) * value.f_frsize
    available = value.f_bavail * value.f_frsize
    severity = ('critical' if available < 12 * GIB or used / total >= .88
                else 'warning' if available < 20 * GIB else 'ok')
    return {'total_bytes': total, 'used_bytes': used, 'available_bytes': available,
            'reserved_bytes': total - used - available,
            'used_percent_df': math.ceil(100 * used / (used + available)),
            'severity': severity}


def guard_status(now):
    try:
        value = json.loads(GUARD.read_text())
        age = (now - dt.datetime.fromisoformat(value['at'])).total_seconds()
        if not -60 <= age <= 600:
            return {'state': 'stale'}
        return {'state': 'fresh', 'severity': value['severity'],
                'paused_plans': sorted(value.get('owned_pauses', {}))}
    except (OSError, ValueError, KeyError, TypeError):
        return {'state': 'unavailable'}


def collect(now, test=False):
    local = now.astimezone(SHANGHAI)
    disk = filesystem()
    guard = guard_status(now)
    labels = {'ok': '正常', 'warning': '需关注', 'critical': '紧张'}
    guard_labels = {'ok': '正常', 'warning': '告警', 'critical': '严重告警'}
    protection = ('状态过期，需检查' if guard['state'] == 'stale' else '状态未确认，需检查')
    if guard['state'] == 'fresh':
        protection = guard_labels.get(guard['severity'], '状态未确认，需检查')
        protection += '，暂停计划 %d 项' % len(guard['paused_plans'])
    lines = [
        '【磁盘日报%s】' % ('｜部署测试' if test else ''),
        '服务器：47.116.10.200',
        '采集时间：' + local.strftime('%Y-%m-%d %H:%M:%S') + '（北京时间）',
        '磁盘状态：' + labels[disk['severity']],
        '总容量：%.2f GiB' % (disk['total_bytes'] / GIB),
        '已使用：%.2f GiB（%d%%）' % (disk['used_bytes'] / GIB, disk['used_percent_df']),
        '可用空间：%.2f GiB' % (disk['available_bytes'] / GIB),
        '系统预留：%.2f GiB' % (disk['reserved_bytes'] / GIB),
        '容量保护：' + protection,
    ]
    if test:
        lines.append('这是部署验证消息；正式日报每天 00:05 发送。')
    return {'at': now.isoformat(), 'report_date': local.date().isoformat(),
            'kind': 'deployment_test' if test else 'daily_report',
            'filesystem': disk, 'capacity_guard': guard, 'text': '\n'.join(lines)}


def run(send=False, test=False, now=None):
    now = now or dt.datetime.now(dt.timezone.utc)
    if not send:
        return collect(now, test)
    STATE.mkdir(parents=True, exist_ok=True, mode=0o700)
    with (STATE / 'daily.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        path = STATE / ('disk-daily-test.json' if test else 'disk-daily-latest.json')
        prior = json.loads(path.read_text()) if path.exists() else {}
        date = now.astimezone(SHANGHAI).date().isoformat()
        if prior.get('report_date') == date and prior.get('notification', {}).get('status') == 'api_accepted':
            return {'status': 'already_accepted', 'report_date': date, 'test': test}
        report = collect(now, test)
        event_id = 'powerx-disk-daily-47.116.10.200-' + date + ('-deployment-test' if test else '')
        result = send_personal(report['text'], event_id)
        if result.get('status') != 'api_accepted':
            raise RuntimeError('daily_notification_not_accepted')
        report['notification'] = result
        atomic_json(path, report)
        return report


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--send', action='store_true')
    parser.add_argument('--test', action='store_true')
    args = parser.parse_args()
    try:
        result = run(args.send, args.test)
        # 日志仅记录摘要；通知回执在 root 私有目录中保存。
        print(json.dumps({key: result[key] for key in ('status', 'report_date', 'kind', 'filesystem', 'capacity_guard')
                          if key in result}, ensure_ascii=False))
        if not args.send:
            print(result['text'])
    except Exception as error:
        print(json.dumps({'status': 'failed', 'error_type': type(error).__name__}))
        raise SystemExit(1)
