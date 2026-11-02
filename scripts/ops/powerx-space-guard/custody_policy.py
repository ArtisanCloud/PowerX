#!/usr/bin/env python3
"""读取部署方固定的 Core 控制绑定；缺失、过期或身份不明时禁止传输。"""
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import stat
import time

CONTROL_PROTOCOL = 2
BINDING = Path('/etc/powerx-space-guard/core-binding.json')


def read_owned_json(path, uid, private=False, limit=65536):
    path = Path(path)
    s = path.lstat()
    if (not stat.S_ISREG(s.st_mode) or s.st_nlink != 1 or s.st_uid != uid
            or s.st_mode & (0o077 if private else 0o022) or s.st_size > limit
            or path.resolve(strict=True) != path):
        raise RuntimeError('control_identity_invalid')
    return json.loads(path.read_text())


def load_binding(path=None):
    binding = read_owned_json(path or BINDING, os.geteuid(), limit=16384)
    base = Path(__file__).resolve().parent
    if (binding.get('protocol') != 2 or not isinstance(binding.get('core_uid'), int)
            or not binding.get('node_id') or not isinstance(binding.get('sources'), list)
            or len(binding['sources']) > 64 or not Path(binding.get('control_file', '')).is_absolute()):
        raise RuntimeError('control_binding_invalid')
    for field, name in [('guard', 'guard.py'), ('helper', 'audit_custody.py'), ('policy', 'custody_policy.py')]:
        script = Path(binding.get(field + '_script', ''))
        if script != base / name:
            raise RuntimeError('control_program_binding_invalid')
        s = script.lstat()
        if not stat.S_ISREG(s.st_mode) or s.st_uid != os.geteuid() or s.st_nlink != 1 or s.st_mode & 0o022:
            raise RuntimeError('control_program_identity_invalid')
        if hashlib.sha256(script.read_bytes()).hexdigest() != binding.get(field + '_sha256'):
            raise RuntimeError('control_program_changed')
    return binding


def state_for(directory, now=None):
    """Core 原生执行器拥有来源后，旧 SSH 接收器不能再读取／确认回收。"""
    result = {'managed': False, 'allow_archive': False, 'reason': 'ui_control_required'}
    try:
        binding = load_binding()
        root = str(Path(directory).resolve(strict=True))
        if root not in binding['sources']:
            return dict(result, reason='source_not_bound')
        # 即使 Core 暂停，旧守护也不能重新夺回来源的删除／压缩权。
        result['managed'] = True
        control = read_owned_json(binding['control_file'], binding['core_uid'], private=True)
        if control.get('protocol') != 2 or control.get('node_id') != binding['node_id']:
            return dict(result, reason='control_node_mismatch')
        stamp = time.time() if now is None else now
        until = dt.datetime.fromisoformat(control['valid_until']).timestamp()
        published = dt.datetime.fromisoformat(control['published_at']).timestamp()
        if not published <= stamp + 30 or not stamp < until <= published + 300:
            return dict(result, reason='control_expired')
        rows = control.get('sources', [])
        if not isinstance(rows, list) or len(rows) > 64:
            return dict(result, reason='control_sources_invalid')
        matched = [s for s in rows if s.get('directory') == root]
        if len(matched) != 1:
            return dict(result, reason='control_source_missing')
        source = matched[0]
        if source.get('executor') == 'powerx':
            return dict(result, reason='managed_by_powerx', source_id=source.get('source_id'),
                        archive_enabled=source.get('archive_enabled') is True)
        # 兼容受权的过渡接收器。当前 Core 只发布 powerx，不会隐式启用此模式。
        allowed = source.get('executor') == 'temporary_receiver' and source.get('archive_enabled') is True
        return dict(result, allow_archive=allowed, reason='enabled' if allowed else 'archive_disabled')
    except (OSError, ValueError, KeyError, TypeError, RuntimeError):
        return dict(result, reason='control_unavailable')


def managed_tree(directory):
    """全局 journal vacuum 不得越过 Core 已接管的子目录。"""
    try:
        root = str(Path(directory).resolve(strict=True)).rstrip('/')
        return any(p == root or p.startswith(root + '/') for p in load_binding()['sources'])
    except (OSError, ValueError, KeyError, TypeError, RuntimeError):
        return False
