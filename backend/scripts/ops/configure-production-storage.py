#!/usr/bin/env python3
"""正式环境存储观测的显式计划/应用入口；不接管旧清理器、不启用删除。"""
import argparse
import hashlib
import json
import os
import pwd
from pathlib import Path
import re
import stat
import shlex
import urllib.parse
import ipaddress
import tempfile

import yaml

CONTRACT = 'powerx.production-storage-observation.v1'
ID = re.compile(r'^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$')


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def read_config(path, expected_database):
    if not path.is_absolute() or path.resolve() != path or not path.is_file():
        raise ValueError('STORAGE_CONFIG_PATH_INVALID')
    info = path.stat()
    if info.st_nlink != 1 or info.st_mode & 0o022 or info.st_size > 2 << 20:
        raise ValueError('STORAGE_CONFIG_IDENTITY_INVALID')
    raw = path.read_bytes()
    config = yaml.safe_load(raw)
    if not isinstance(config, dict) or config.get('deployment', {}).get('env') != 'prod':
        raise ValueError('STORAGE_PROD_ENVIRONMENT_REQUIRED')
    database = config.get('database') or {}
    if database.get('driver', 'postgres') not in ('postgres', 'pg', 'PostgreSQL', 'Postgres'):
        raise ValueError('STORAGE_PROD_POSTGRES_REQUIRED')
    dsn = database.get('dsn') or ''
    if dsn.startswith(('postgres://', 'postgresql://')):
        parsed = urllib.parse.urlsplit(dsn)
        host, name = parsed.hostname, parsed.path.lstrip('/')
        overrides = urllib.parse.parse_qs(parsed.query)
        if 'host' in overrides or 'hostaddr' in overrides:
            raise ValueError('STORAGE_PROD_DATABASE_SCOPE_MISMATCH')
    elif dsn:
        values = dict(part.split('=', 1) for part in shlex.split(dsn) if '=' in part)
        host, name = values.get('host'), values.get('dbname')
    else:
        host, name = database.get('host'), database.get('database')
    try:
        local = host == 'localhost' or ipaddress.ip_address(host).is_loopback
    except ValueError:
        local = False
    if not local or not expected_database or name != expected_database:
        raise ValueError('STORAGE_PROD_DATABASE_SCOPE_MISMATCH')
    return raw, config, info


def profile(host, filesystem, cluster, storage):
    if any(not ID.fullmatch(v or '') for v in (host, filesystem, cluster)):
        raise ValueError('STORAGE_OBSERVATION_IDENTITY_REQUIRED')
    return {
        'schema_version': 1, 'mode': 'observe', 'host_id': host,
        'filesystem_id': filesystem, 'filesystem_path': str(storage),
        'postgres_enabled': True, 'postgres_cluster_id': cluster,
        'sampling_enabled': True, 'runtime_directory': str(storage / 'storage-governance'),
        'lifecycle_enabled': True, 'manage_native_retention': False,
        'allow_enforce': False, 'admission_enabled': False,
        'advanced_operations_enabled': False,
        'min_free_bytes': 20 << 30, 'critical_free_bytes': 10 << 30, 'max_concurrent': 1,
    }


def save_private(path, value):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'w') as out:
        json.dump(value, out, ensure_ascii=False, indent=2)
        out.flush()
        os.fsync(out.fileno())


def make_plan(config_path, storage, host, filesystem, cluster, service_user=None, expected_database=None):
    raw, config, info = read_config(config_path, expected_database)
    account = pwd.getpwnam(service_user) if service_user else pwd.getpwuid(info.st_uid)
    if 'storage_governance' in config:
        raise ValueError('STORAGE_EXISTING_CONFIGURATION_REVIEW_REQUIRED')
    if not storage.is_absolute() or storage.resolve() != storage or not storage.is_dir() or storage.stat().st_uid not in (info.st_uid, account.pw_uid):
        raise ValueError('STORAGE_ROOT_IDENTITY_INVALID')
    settings = profile(host, filesystem, cluster, storage)
    runtime = Path(settings['runtime_directory'])
    if runtime.exists():
        raise ValueError('STORAGE_EXISTING_RUNTIME_REVIEW_REQUIRED')
    plan = {'contract': CONTRACT, 'config': str(config_path), 'before_sha256': sha(raw),
            'database': expected_database,
            'owner_uid': info.st_uid, 'owner_gid': info.st_gid,
            'runtime_user': account.pw_name, 'runtime_uid': account.pw_uid, 'runtime_gid': account.pw_gid,
            'mode': stat.S_IMODE(info.st_mode), 'device': storage.stat().st_dev,
            'settings': settings}
    return {**plan, 'seal': sha(json.dumps(plan, sort_keys=True).encode())}


def apply_plan(plan, backup):
    if plan.get('contract') != CONTRACT or plan.get('seal') != sha(json.dumps({k: v for k, v in plan.items() if k != 'seal'}, sort_keys=True).encode()):
        raise ValueError('STORAGE_PLAN_INVALID')
    settings = plan['settings']
    account = pwd.getpwnam(plan['runtime_user'])
    if (account.pw_uid, account.pw_gid) != (plan['runtime_uid'], plan['runtime_gid']):
        raise ValueError('STORAGE_RUNTIME_USER_CHANGED')
    storage = Path(settings['filesystem_path'])
    if settings != profile(settings['host_id'], settings['filesystem_id'], settings['postgres_cluster_id'], storage):
        raise ValueError('STORAGE_PLAN_SETTINGS_INVALID')
    config_path = Path(plan['config'])
    raw, config, info = read_config(config_path, plan['database'])
    if sha(raw) != plan['before_sha256'] or 'storage_governance' in config:
        raise ValueError('STORAGE_CONFIG_CHANGED_REPLAN_REQUIRED')
    if (info.st_uid, info.st_gid, stat.S_IMODE(info.st_mode)) != (plan['owner_uid'], plan['owner_gid'], plan['mode']):
        raise ValueError('STORAGE_CONFIG_IDENTITY_CHANGED')
    if storage.resolve() != storage or storage.stat().st_uid not in (info.st_uid, account.pw_uid) or storage.stat().st_dev != plan['device']:
        raise ValueError('STORAGE_ROOT_IDENTITY_CHANGED')
    runtime = Path(settings['runtime_directory'])
    if runtime.exists():
        raise ValueError('STORAGE_EXISTING_RUNTIME_REVIEW_REQUIRED')
    if not backup.is_absolute() or not backup.parent.is_dir() or backup.parent.stat().st_mode & 0o077:
        raise ValueError('STORAGE_PRIVATE_BACKUP_REQUIRED')
    # 原文追加，保留其他模块的注释、凭据及序列化形式。
    after = raw.rstrip() + b'\n\n' + yaml.safe_dump({'storage_governance': settings}, sort_keys=False, allow_unicode=True).encode()
    parsed = yaml.safe_load(after)
    if {k: v for k, v in parsed.items() if k != 'storage_governance'} != config:
        raise ValueError('STORAGE_UNRELATED_CONFIGURATION_CHANGED')
    fd = os.open(backup, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'wb') as out:
        out.write(raw); out.flush(); os.fsync(out.fileno())
    temporary = None
    created = False
    try:
        runtime.mkdir(mode=0o700)
        created = True
        os.chown(runtime, account.pw_uid, account.pw_gid)
        if runtime.stat().st_dev != storage.stat().st_dev:
            raise ValueError('STORAGE_RUNTIME_FILESYSTEM_CHANGED')
        fd, temporary = tempfile.mkstemp(prefix='.config.storage.', dir=config_path.parent)
        with os.fdopen(fd, 'wb') as out:
            os.fchown(out.fileno(), info.st_uid, info.st_gid)
            os.fchmod(out.fileno(), stat.S_IMODE(info.st_mode))
            out.write(after); out.flush(); os.fsync(out.fileno())
        if config_path.read_bytes() != raw:
            raise ValueError('STORAGE_CONFIG_CHANGED_REPLAN_REQUIRED')
        os.replace(temporary, config_path)
        temporary = None
    except BaseException:
        if created:
            runtime.rmdir()  # 仅清理本次创建且仍为空的目录。
        raise
    finally:
        if temporary is not None:
            os.unlink(temporary)
    return {'applied': True, 'config': str(config_path), 'after_sha256': sha(after),
            'backup': str(backup), 'restart_required': True,
            'automatic_cleanup_enabled': False, 'legacy_handoff': False}


def main():
    parser = argparse.ArgumentParser(description=CONTRACT)
    action = parser.add_mutually_exclusive_group(required=True)
    action.add_argument('--config', type=Path)
    action.add_argument('--apply-plan', type=Path)
    parser.add_argument('--storage-root', type=Path)
    parser.add_argument('--confirm-database')
    parser.add_argument('--host-id')
    parser.add_argument('--filesystem-id')
    parser.add_argument('--postgres-cluster-id')
    parser.add_argument('--service-user')
    parser.add_argument('--output-plan', type=Path)
    parser.add_argument('--backup', type=Path)
    args = parser.parse_args()
    if args.config:
        if not args.storage_root or not args.output_plan or not args.service_user:
            raise ValueError('STORAGE_PLAN_ARGUMENTS_REQUIRED')
        plan = make_plan(args.config, args.storage_root, args.host_id, args.filesystem_id, args.postgres_cluster_id, args.service_user, args.confirm_database)
        save_private(args.output_plan, plan)
        print(json.dumps({'contract': CONTRACT, 'seal': plan['seal'], 'settings': plan['settings'], 'applied': False}))
    else:
        if not args.backup:
            raise ValueError('STORAGE_PRIVATE_BACKUP_REQUIRED')
        print(json.dumps(apply_plan(json.loads(args.apply_plan.read_text()), args.backup)))


if __name__ == '__main__':
    main()
