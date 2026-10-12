"""升级配置预检查：只读，绝不创建配置、目录或密钥。"""
import argparse
import base64
import json
import os
from pathlib import Path
import sys
import yaml


class UpgradeConfigError(ValueError):
    pass


def validate(config_path, data_path):
    path, data = Path(config_path), Path(data_path)
    if path.is_symlink() or not path.is_file() or not os.access(path, os.R_OK):
        raise UpgradeConfigError('UPGRADE_CONFIG_UNREADABLE')
    if path.stat().st_mode & 0o077:
        raise UpgradeConfigError('UPGRADE_CONFIG_PERMISSIONS')
    try:
        cfg = yaml.safe_load(path.read_text())
    except (OSError, UnicodeError, yaml.YAMLError):
        raise UpgradeConfigError('UPGRADE_CONFIG_INVALID') from None
    if not isinstance(cfg, dict) or cfg.get('install', {}).get('status') != 'installed':
        raise UpgradeConfigError('UPGRADE_INSTANCE_NOT_INSTALLED')
    for section, field in [('database', 'driver'), ('auth', 'jwt_secret'),
                           ('server', 'secret_key'), ('deployment', 'env')]:
        if section=='auth' and os.environ.get('POWERX_AUTH_JWT_SECRET'): continue
        if not isinstance(cfg.get(section), dict) or not cfg[section].get(field):
            raise UpgradeConfigError('UPGRADE_CONFIG_REQUIRED_FIELD')
    if cfg['deployment']['env'] not in ('dev', 'test', 'staging', 'prod'):
        raise UpgradeConfigError('UPGRADE_DEPLOYMENT_INVALID')
    if len(str(os.environ.get('POWERX_AUTH_JWT_SECRET') or cfg['auth']['jwt_secret'])) < 32:
        raise UpgradeConfigError('UPGRADE_SECRET_INVALID')
    try:
        if len(base64.b64decode(cfg['server']['secret_key'], validate=True)) != 32:
            raise ValueError()
    except (ValueError, TypeError):
        raise UpgradeConfigError('UPGRADE_SECRET_INVALID') from None
    db = cfg['database']
    fields={'host':'CORE_X_DB_HOST','username':'CORE_X_DB_USERNAME','database':'CORE_X_DB_DATABASE','password':'CORE_X_DB_PASSWORD'}
    if db['driver'] != 'postgres' or not (db.get('dsn') or all(db.get(k) or os.environ.get(v) for k,v in fields.items())):
        raise UpgradeConfigError('UPGRADE_DATABASE_BINDING_INVALID')
    if not data.is_dir() or not os.access(data, os.R_OK | os.X_OK):
        raise UpgradeConfigError('UPGRADE_DATA_MOUNT_MISSING')
    # 目录由首次 init 创建；缺失时不能以 mkdir 掩盖挂载丢失。
    for name in ('uploads', 'plugins/installed', 'runtime'):
        if not (data / name).is_dir():
            raise UpgradeConfigError('UPGRADE_DATA_MOUNT_INCOMPLETE')
    storage = cfg.get('storage', {})
    if storage.get('default_driver') == 'local':
        local = Path(storage.get('local', {}).get('base_path', ''))
        if not local.is_absolute() or not local.is_dir():
            raise UpgradeConfigError('UPGRADE_STORAGE_PATH_UNAVAILABLE')
    return {'schema_version': 'powerx.upgrade-config/v1', 'status': 'ready',
            'configuration_changes': [], 'configuration_written': False}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--config', default=os.environ.get('POWERX_CONFIG', '/etc/powerx/config.yaml'))
    parser.add_argument('--data-dir', default='/data')
    args = parser.parse_args()
    try:
        print(json.dumps(validate(args.config, args.data_dir)))
        return 0
    except UpgradeConfigError as error:
        print(json.dumps({'schema_version': 'powerx.upgrade-config/v1', 'status': 'blocked', 'error_code': str(error)}))
        return 2


if __name__ == '__main__':
    sys.exit(main())
