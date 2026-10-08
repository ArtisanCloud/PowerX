"""Generate a private deployment on the Docker host via the backend image."""
import base64
import json
import os
from pathlib import Path
import secrets
import sys
from urllib.parse import urlsplit
import yaml

def initialize(directory=Path('/etc/powerx'), data=Path('/data'), template=Path('/app/container/config.example.yaml')):
    origin=os.environ.get('PUBLIC_ORIGIN','http://127.0.0.1:18080').rstrip('/')
    parsed=urlsplit(origin)
    if parsed.scheme not in ('http','https') or not parsed.hostname or parsed.username or parsed.path not in ('','/') or parsed.query or parsed.fragment:
        raise ValueError('PUBLIC_ORIGIN must be an http(s) origin, without credentials or path')
    environment=os.environ.get('DEPLOYMENT_ENV','dev')
    if environment not in ('dev','test','staging','prod'):
        raise ValueError('DEPLOYMENT_ENV must be dev/test/staging/prod')
    directory.mkdir(parents=True,exist_ok=True,mode=0o700)
    directory.chmod(0o700)
    if (directory/'config.yaml').exists():
        print('Existing configuration preserved; credentials and keys were not regenerated.')
        return
    # Refuse a partially initialized directory rather than silently rotate keys.
    if any(directory.iterdir()):
        raise ValueError('Incomplete configuration directory; restore its backup before retrying')
    data.mkdir(parents=True,exist_ok=True)
    for name in ('uploads','plugins/installed','plugins/market_cache','logs/audit','reports','runtime'):
        (data/name).mkdir(parents=True,exist_ok=True)
    password=secrets.token_urlsafe(32)
    redis_password=secrets.token_urlsafe(32)
    administrator={'username':os.environ.get('ADMIN_USERNAME','admin'),
                   'email':os.environ.get('ADMIN_EMAIL','admin@example.com'),
                   'display_name':'Administrator','phone':'','password':secrets.token_urlsafe(24)}
    cfg=yaml.safe_load(template.read_text())
    cfg['deployment']={'env':environment}
    cfg['install']={'status':'installed','lock_mode':'strict','allow_without_db':False}
    cfg['server'].update(host='0.0.0.0',port=8080,api_prefix='/api/v1',mode='release',read_timeout_seconds=30,write_timeout_seconds=0,
                         secret_key=base64.b64encode(secrets.token_bytes(32)).decode())
    cfg['server']['grpc'].update(host='0.0.0.0',port=9010,reflection=False)
    cfg['database'].update(host='postgres',port=5432,username='powerx',password=password,database='powerx',dsn='')
    cfg['cache'].update(host='redis',port=6379,password=redis_password)
    def redis_settings(value):
        if isinstance(value,dict):
            for key,child in list(value.items()):
                if key=='redis_addr':value[key]='redis:6379'
                elif key=='redis_password':value[key]=redis_password
                elif key=='redis' and isinstance(child,dict):child.update(addr='redis:6379',password=redis_password)
                redis_settings(child)
        elif isinstance(value,list):
            for child in value:redis_settings(child)
    redis_settings(cfg)
    cfg['auth'].update(jwt_secret=secrets.token_urlsafe(48),access_ttl='2h',refresh_ttl='168h')
    cfg['http_security']={'web_admin_origins':[origin],'frame_ancestors':["'self'",origin]}
    cfg['storage']['default_driver']='local'
    cfg['storage']['local'].update(base_path='/data/uploads',public_base_url=origin+'/media',
                                   upload_token_secret=secrets.token_urlsafe(32),public_token_secret=secrets.token_urlsafe(32))
    cfg['plugin'].update(registry_file='/data/plugins/registry.json',installed_dir='/data/plugins/installed',market_cache_dir='/data/plugins/market_cache')
    cfg['log'].update(level='info',console=True,http_debug=False,debug=False)
    cfg['log']['file'].update(enable=True,info_file_path='/data/logs/info.log',error_file_path='/data/logs/error.log')
    cfg['log']['loki']['enable']=False
    cfg.setdefault('audit',{}).setdefault('file',{}).update(dir='/data/logs/audit')
    for name,value in {'postgres-password':password,'redis-password':redis_password,
                        'initial-admin.json':json.dumps(administrator),
                        'setup.wizard.config.json':json.dumps({'admin':administrator})}.items():
        (directory/name).write_text(value+'\n')
        (directory/name).chmod(0o600)
    (directory/'config.yaml').write_text(yaml.safe_dump(cfg,allow_unicode=True,sort_keys=False))
    (directory/'config.yaml').chmod(0o600)
    print('Private config generated. Run bootstrap before starting the application.')

if __name__=='__main__':
    try:initialize()
    except Exception as error:
        print(str(error),file=sys.stderr)
        raise SystemExit(1)
