#!/usr/bin/env python3
"""Bounded log-output checks; never edits Core configuration or restarts services."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import time
import urllib.parse
import urllib.request
import yaml

VERSION = 1
UNITS = ('powerx-backend.service', 'powerx-dev-backend.service')


def command(args):
    r = subprocess.run(args, capture_output=True, text=True, timeout=3)
    if r.returncode or len(r.stdout) > 1024 * 1024:
        raise RuntimeError('command_unavailable')
    return r.stdout


def properties(unit):
    return dict(line.split('=', 1) for line in command([
        'systemctl', 'show', unit, '-p', 'MainPID', '-p', 'ActiveState',
        '-p', 'StandardOutput', '-p', 'LogNamespace']).splitlines() if '=' in line)


def process_config(pid):
    proc = Path('/proc') / str(pid)
    cwd = Path(os.readlink(proc / 'cwd'))
    raw = (proc / 'environ').read_bytes()
    args = (proc / 'cmdline').read_bytes().split(b'\0')
    if len(raw) > 1024 * 1024 or sum(map(len, args)) > 1024 * 1024:
        raise RuntimeError('process_metadata_too_large')
    env = dict(s.split('=', 1) for s in raw.decode().split('\0') if '=' in s)
    selected = None
    for i, a in enumerate(args):
        a = a.decode()
        if a in ('-config', '--config') and i + 1 < len(args): selected = args[i + 1].decode()
        elif a.startswith(('-config=', '--config=')): selected = a.split('=', 1)[1]
    selected = selected or env.get('POWERX_CONFIG')
    if not selected:
        raise RuntimeError('runtime_config_path_unverified')
    path = Path(selected)
    if not path.is_absolute(): path = cwd / path
    path = path.resolve(strict=True)
    info = path.stat()
    if not stat.S_ISREG(info.st_mode) or info.st_size > 1024 * 1024:
        raise RuntimeError('runtime_config_invalid')
    # A file edited after startup is desired configuration, not verified active state.
    ticks = int((proc / 'stat').read_text().rsplit(')', 1)[1].split()[19])
    boot = time.time() - float(Path('/proc/uptime').read_text().split()[0])
    started = boot + ticks / os.sysconf('SC_CLK_TCK')
    if info.st_mtime > started + 2:
        raise RuntimeError('configuration_changed_since_process_start')
    cfg = yaml.safe_load(path.read_text())
    status = dict(line.split(':', 1) for line in (proc / 'status').read_text().splitlines() if ':' in line)
    identity = (int(status['Uid'].split()[1]), int(status['Gid'].split()[1]), [int(v) for v in status['Groups'].split()])
    return path, cwd, cfg, identity


def file_ready(path, identity):
    path = Path(path)
    target = path if path.exists() else path.parent
    info = target.stat()
    if path.exists() and not stat.S_ISREG(info.st_mode): return False
    if not path.exists() and not stat.S_ISDIR(info.st_mode): return False
    uid, gid, groups = identity
    if uid == os.geteuid(): writable = os.access(target, os.W_OK)
    elif os.geteuid() == 0:
        # Check with the Core process credentials, not root's broader permissions.
        r = subprocess.run([sys.executable, '-c', 'import os,sys;sys.exit(0 if os.access(sys.argv[1],os.W_OK) else 1)', str(target)],
                           user=uid, group=gid, extra_groups=groups, env={'PATH': '/usr/bin:/bin'},
                           cwd='/', capture_output=True, timeout=3)
        writable = r.returncode == 0
    else: return False
    v = os.statvfs(target)
    return writable and not v.f_flag & getattr(os, 'ST_RDONLY', 1) and v.f_bavail > 0


def console_ready(unit, pid):
    if unit.get('StandardOutput') not in ('journal', 'journal+console') or unit.get('LogNamespace'):
        return False, 'stdout_not_verified_journal'
    if properties('systemd-journald.service').get('ActiveState') != 'active':
        return False, 'journald_inactive'
    if not stat.S_ISSOCK(Path('/proc', str(pid), 'fd', '1').stat().st_mode):
        return False, 'stdout_not_journal_socket'
    settings = command(['systemd-analyze', 'cat-config', 'systemd/journald.conf'])
    storage = 'auto'
    section = ''
    for line in settings.splitlines():
        line = line.strip()
        if line.startswith('['): section = line
        if section == '[Journal]' and '=' in line and not line.startswith(('#', ';')):
            key, value = line.split('=', 1)
            if key.strip() == 'Storage': storage = value.strip() or 'auto'
    root = Path('/var/log/journal')
    if storage not in ('auto', 'persistent') or not any(root.glob('*/*.journal')):
        return False, 'journal_persistence_unverified'
    return True, 'journal_route_ready_not_delivery_receipt'


def loki_ready(url, probe):
    p = urllib.parse.urlsplit(url)
    if p.scheme not in ('http', 'https') or not p.hostname or p.username or p.password or p.query or p.fragment:
        return False, 'loki_endpoint_unsupported', 0
    base = url.rstrip('/')
    if probe:
        data = {'streams': [{'stream': {'service': 'powerx-ops-diagnostic', 'module': 'log-output-health'},
                             'values': [[str(time.time_ns()), 'PowerX log-output health probe']]}]}
        request = urllib.request.Request(base + '/loki/api/v1/push', data=json.dumps(data).encode(), headers={'Content-Type': 'application/json'})
    else: request = urllib.request.Request(base + '/ready')
    # Never forward probe payloads through a redirect to a different destination.
    class NoRedirect(urllib.request.HTTPRedirectHandler):
        def redirect_request(self, *args, **kwargs): return None
    with urllib.request.build_opener(NoRedirect()).open(request, timeout=2) as r:
        code = r.status
    return (code == 204, 'loki_write_accepted' if code == 204 else 'loki_write_not_probed', code)


def check_service(name, probe_loki=True, cache=None):
    result = {'unit': name, 'state': 'unknown', 'healthy': False, 'sinks': {}}
    try:
        unit = properties(name)
        pid = int(unit.get('MainPID', 0))
        if unit.get('ActiveState') != 'active' or pid <= 0:
            result['reason'] = 'core_service_inactive'; return result
        result['pid'] = pid
        path, cwd, cfg, identity = process_config(pid)
        result['config_path'] = str(path)
        log = cfg['log']
        if not isinstance(log, dict): raise RuntimeError('log_config_invalid')
        flags = {'console': log.get('console'), 'file': log.get('file', {}).get('enable'), 'loki': log.get('loki', {}).get('enable')}
        if any(type(v) is not bool for v in flags.values()): raise RuntimeError('log_config_incomplete')
        for kind, enabled in flags.items():
            sink = {'enabled': enabled, 'available': False, 'reason': 'disabled'}
            result['sinks'][kind] = sink
            if not enabled: continue
            try:
                if kind == 'console': sink['available'], sink['reason'] = console_ready(unit, pid)
                elif kind == 'file':
                    paths = [log['file'].get('info_file_path', 'logs/info.log'), log['file'].get('error_file_path', 'logs/error.log')]
                    sink['available'] = all(file_ready(Path(v) if Path(v).is_absolute() else cwd / v, identity) for v in paths)
                    sink['reason'] = 'file_permissions_ready_not_delivery_receipt' if sink['available'] else 'file_not_writable_or_no_space'
                else:
                    url = log['loki'].get('url', '')
                    key = (url, probe_loki)
                    if cache is not None and key in cache: answer = cache[key]
                    else:
                        try: answer = loki_ready(url, probe_loki)
                        except Exception: answer = (False, 'loki_unreachable_or_rejected', 0)
                        if cache is not None: cache[key] = answer
                    sink['available'], sink['reason'], sink['http_status'] = answer
            except Exception: sink['reason'] = 'sink_check_unavailable'
        result['healthy'] = any(s['available'] for s in result['sinks'].values())
        result['state'] = 'ok' if result['healthy'] else 'critical'
        result['reason'] = 'available_output_found' if result['healthy'] else 'no_available_log_output'
        result['degraded'] = any(s['enabled'] and not s['available'] for s in result['sinks'].values())
    except Exception as error:
        # Exceptions from YAML, URLs or subprocesses may contain secrets. Emit only fixed codes.
        result['reason'] = str(error) if isinstance(error, RuntimeError) and str(error) in {
            'process_metadata_too_large', 'runtime_config_path_unverified', 'runtime_config_invalid',
            'configuration_changed_since_process_start', 'log_config_invalid', 'log_config_incomplete'} else 'runtime_config_or_process_unavailable'
    return result


def check_services(probe_loki=True):
    cache = {}
    rows = [check_service(unit, probe_loki, cache) for unit in UNITS]
    safe = [{'unit': r['unit'], 'state': r['state'], 'reason': r.get('reason'), 'sinks': r['sinks']} for r in rows]
    return {'version': VERSION, 'healthy': all(r['healthy'] for r in rows), 'degraded': any(r.get('degraded', False) for r in rows), 'services': rows,
            'fingerprint': hashlib.sha256(json.dumps(safe, sort_keys=True).encode()).hexdigest()}


def alert_text(health):
    value = health.get('log_outputs', {})
    rows = value.get('services', [])
    if not rows: return '\n日志出口：尚未核实'
    return '\n日志出口：' + '；'.join(r['unit'].removesuffix('-backend.service') + '=' +
        ('可用' if r['healthy'] else {'no_available_log_output': '无可用日志出口', 'configuration_changed_since_process_start': '配置已变更，生效状态未核实', 'core_service_inactive': 'Core未运行'}.get(r.get('reason'), '无法核实日志出口')) for r in rows)


def changed(prior, health):
    return prior.get('runtime_health', {}).get('log_outputs', {}).get('fingerprint') != health.get('log_outputs', {}).get('fingerprint')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--probe-loki', action='store_true', help='write one synthetic entry only to an enabled Loki output')
    args = parser.parse_args()
    report = check_services(args.probe_loki)
    print(json.dumps(report, ensure_ascii=False))
    sys.exit(0 if report['healthy'] else 2)
