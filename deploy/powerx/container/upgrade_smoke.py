"""仅在独立 CI Compose 项目中验收真实旧镜像 -> 新镜像 -> 备份恢复回滚。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import sys
import tempfile
import time
import traceback
import urllib.error
import urllib.request
import yaml

from upgrade_release import REQUIRED_CHECKS

BASE = 'http://127.0.0.1:18080'
PLUGIN = 'com.powerx.upgrade.fixture'


class APIResponseError(RuntimeError):
    def __init__(self, status, path):
        self.status = status
        super().__init__('UPGRADE_CI_HTTP_STATUS_FAILED: ' + path.split('?')[0])


def compose(*args, input=None, check=True):
    result = subprocess.run(['docker', 'compose', *args], input=input, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if check and result.returncode:
        raise RuntimeError('UPGRADE_CI_COMPOSE_OPERATION_FAILED')  # 不回显含凭证的子进程输出。
    return result


def sql(query):
    return compose('exec', '-T', 'postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-U', 'powerx', '-d', 'powerx', '-At', input=query.encode()).stdout.decode().strip()


def prepare_ci_mount_access():
    # The initializer and Core run as container root. The GitHub runner must
    # create/copy the fixture inside these two CI-owned mounts, without making
    # private configuration world-readable or following plugin symlinks.
    if os.environ.get('POWERX_UPGRADE_CI') != '1' or not os.environ.get('COMPOSE_PROJECT_NAME', '').startswith('powerx-upgrade-'):
        raise RuntimeError('UPGRADE_CI_ISOLATION_REQUIRED')
    script = '''import os,sys
uid,gid=map(int,sys.argv[1:])
for base in ('/etc/powerx','/data'):
    if os.path.islink(base) or not os.path.isdir(base):
        raise RuntimeError('UPGRADE_CI_MOUNT_INVALID')
    os.chown(base,uid,gid,follow_symlinks=False)
    for root,dirs,files in os.walk(base,followlinks=False):
        for name in dirs+files:
            os.chown(os.path.join(root,name),uid,gid,follow_symlinks=False)
'''
    compose('run','--rm','--no-deps','init','python3','-c',script,str(os.getuid()),str(os.getgid()))


def select_images(pair, platform):
    path = Path('.env')
    values = {}
    if path.exists():
        values = dict(line.split('=', 1) for line in path.read_text().splitlines() if line and not line.startswith('#') and '=' in line)
    values.update(POWERX_BACKEND_IMAGE=pair['images']['backend']['repository']+'@'+pair['images']['backend']['platforms'][platform]['digest'],
                  POWERX_WEB_IMAGE=pair['images']['web_admin']['repository']+'@'+pair['images']['web_admin']['platforms'][platform]['digest'],
                  COMPOSE_PROJECT_NAME=os.environ['COMPOSE_PROJECT_NAME'], DEPLOYMENT_ENV='dev', PUBLIC_ORIGIN=BASE,
                  PUBLIC_WS_ORIGIN='ws://127.0.0.1:18080', HTTP_PORT='18080', HTTP_BIND='127.0.0.1', POWERX_DATA_DIR='./data')
    values['POWERX_UPGRADE_MODE']='0' if pair['source_sha']=='3d792fd7ea2426825970bceceb0d6e9db46211ef' else '1'
    path.write_text('\n'.join(k+'='+v for k,v in values.items())+'\n')


class Acceptance:
    def __init__(self, report):
        self.report = report
        self.token = ''
        self.tenant = ''

    def api(self, path, body=None, method=None, expected=200):
        headers={'Content-Type':'application/json'}
        if self.token: headers['Authorization']='Bearer '+self.token
        if self.tenant: headers['tenant_uuid']=self.tenant;headers['X-Tenant-UUID']=self.tenant
        request=urllib.request.Request(BASE+path, data=json.dumps(body).encode() if body is not None else None, headers=headers, method=method)
        try:
            response=urllib.request.urlopen(request,timeout=180)
        except urllib.error.HTTPError as error:
            response=error
        with response:
            record={'path':path.split('?')[0], 'status':response.status,
                    'request_id':response.headers.get('X-Request-ID')}
            self.report['http'].append(record)
            # Nginx returns HTML while Core restarts. Preserve its HTTP status
            # before decoding JSON so the bounded readiness wait can retry it.
            if response.status!=expected: raise APIResponseError(response.status,path)
            try:
                payload=json.loads(response.read())
            except (ValueError, UnicodeError):
                raise RuntimeError('UPGRADE_CI_HTTP_JSON_INVALID: '+path.split('?')[0]) from None
            if not isinstance(payload,dict):
                raise RuntimeError('UPGRADE_CI_HTTP_ENVELOPE_INVALID: '+path.split('?')[0])
            record['request_id']=record['request_id'] or payload.get('request_id') or payload.get('trace_id')
            return payload.get('data',payload)

    def wait_installed(self):
        for _ in range(150):
            try:
                status=self.api('/api/v1/admin/setup/status')
                if status['configured'] and status['requires_login'] and not status.get('restart_required'): return
            except APIResponseError as error:
                if error.status not in (502,503,504): raise
            except OSError: pass
            time.sleep(1)
        raise RuntimeError('UPGRADE_CI_INSTALLED_STATE_FAILED')

    def login(self,password):
        value=self.api('/api/v1/admin/user/auth/login',{'identifier':'upgrade-admin@example.com','password':password})
        self.token=value['access_token']
        self.tenant=sql('SELECT uuid FROM public.iam_tenant WHERE deleted_at IS NULL ORDER BY id LIMIT 1')

    def mark(self,name): self.report['checks'][name]=True

    def migration(self,action,target,source,expected=None,code=0):
        args=['run','--rm','--no-deps','init','migrate','--action='+action,'--format=json','--target-release='+target,'--source-sha='+source]
        if action=='apply': args+=['--confirm-offline','--expected-plan='+expected]
        result=compose(*args,check=False)
        value=json.loads(result.stdout)
        self.report['migration_results'].append(value)
        if result.returncode!=code: raise RuntimeError('UPGRADE_CI_MIGRATION_EXIT_FAILED')
        return value

    def verify_lock_conflict(self,target,source,expected):
        process=subprocess.Popen(['docker','compose','exec','-T','-e','PGAPPNAME=powerx-upgrade-ci-lock','postgres','psql','-U','powerx','-d','powerx','-Atc',
            "SELECT pg_advisory_lock(hashtextextended(current_database() || 'powerx:upgrade:public',0)); SELECT pg_sleep(120)"],stdout=subprocess.PIPE,stderr=subprocess.PIPE)
        try:
            for _ in range(100):
                if sql("SELECT count(*) FROM pg_stat_activity a JOIN pg_locks l ON a.pid=l.pid WHERE a.application_name='powerx-upgrade-ci-lock' AND l.locktype='advisory' AND l.granted")=='1':break
                time.sleep(.1)
            else:raise RuntimeError('UPGRADE_CI_LOCK_FIXTURE_FAILED')
            result=compose('run','--rm','--no-deps','init','migrate','--action=apply','--target-release='+target,'--source-sha='+source,'--confirm-offline','--expected-plan='+expected,'--lock-timeout=0s',check=False)
            value=json.loads(result.stdout);assert result.returncode==3 and value['error_code']=='UPGRADE_LOCK_BUSY'
            self.report['migration_results'].append(value);self.mark('lock_conflict')
        finally:
            sql("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE application_name='powerx-upgrade-ci-lock'")
            process.communicate(timeout=10)

    def interrupt_catalog(self,target,source,expected):
        name=os.environ['COMPOSE_PROJECT_NAME']+'-interrupted'
        process=subprocess.Popen(['docker','compose','run','--rm','--no-deps','--name',name,'init','migrate','--action=apply','--target-release='+target,'--source-sha='+source,'--confirm-offline','--expected-plan='+expected],stdout=subprocess.PIPE,stderr=subprocess.PIPE)
        try:
            for _ in range(200):
                if sql("SELECT count(*) FROM public.database_upgrade_runs WHERE status='running' AND migration_id LIKE '20261011_catalog_%'")=='1':break
                if process.poll() is not None:raise RuntimeError('UPGRADE_CI_INTERRUPTION_WINDOW_MISSING')
                time.sleep(.1)
            else:raise RuntimeError('UPGRADE_CI_INTERRUPTION_WINDOW_MISSING')
            time.sleep(1) # 等待目录事务进入测试触发器，之后 TERM 取消查询并回滚。
            subprocess.run(['docker','kill','--signal','TERM',name],check=True,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
            stdout,_=process.communicate(timeout=30)
            if stdout:
                value=json.loads(stdout);assert value['status']!='succeeded';self.report['migration_results'].append(value)
            assert sql("SELECT count(*) FROM public.database_migration_records WHERE migration_id LIKE '20261011_catalog_%'")=='0'
        finally:
            if process.poll() is None:
                subprocess.run(['docker','rm','-f',name],stdout=subprocess.PIPE,stderr=subprocess.PIPE)
                process.communicate(timeout=10)


def fixture_plugin():
    root=Path('data/runtime/upgrade-fixture');root.mkdir(parents=True)
    (root/'frontend/admin').mkdir(parents=True)
    (root/'frontend/admin/index.html').write_text('<!doctype html><title>Upgrade fixture</title><p>preserved-plugin</p>')
    manifest={'id':PLUGIN,'version':'1.0.0','name':'Upgrade CI Fixture','corex_version':'*',
        'runtime':{'kind':'process','entry':'probe.py','health':{'http':'/healthz'}},
        'endpoints':{'http_base_path':'/api/v1'},'routes':{'basePath':'/api/v1'},
        'frontend':{'admin':{'kind':'static','static_dir':'frontend/admin','menus':[]}},
        'rbac':{'resources':[{'resource':'probe','actions':['read']}]},
        'permissions':[{'resource':'probe','path':'/probe','actions':['read'],'description':'Upgrade fixture probe'}]}
    (root/'plugin.yaml').write_text(yaml.safe_dump(manifest))
    script=r'''#!/usr/bin/env python3
import json,os,re,subprocess
from http.server import BaseHTTPRequestHandler,HTTPServer
schema=os.environ['POWERX_DB_SCHEMA']
assert re.fullmatch(r'[a-zA-Z_][a-zA-Z0-9_]*',schema)
env=dict(os.environ,PGHOST=os.environ['POWERX_DB_HOST'],PGPORT=os.environ['POWERX_DB_PORT'],PGUSER=os.environ['POWERX_DB_USERNAME'],PGPASSWORD=os.environ['POWERX_DB_PASSWORD'],PGDATABASE=os.environ['POWERX_DB_DATABASE'],PGOPTIONS='-c search_path='+schema)
def sql(query):
    return subprocess.run(['psql','-v','ON_ERROR_STOP=1','-At'],input=query.encode(),stdout=subprocess.PIPE,stderr=subprocess.PIPE,env=env)
assert sql('CREATE TABLE IF NOT EXISTS upgrade_records (id integer primary key,value text not null); INSERT INTO upgrade_records VALUES (1,\'preserved-plugin-data\') ON CONFLICT DO NOTHING;').returncode==0
class Handler(BaseHTTPRequestHandler):
    def log_message(self,*args):pass
    def do_GET(self):
        data={'marker':'preserved-plugin','schema':schema,'role':env['PGUSER'],'value':sql('SELECT value FROM upgrade_records WHERE id=1').stdout.decode().strip(),'core_user_table_denied':sql('SELECT count(*) FROM public.iam_user').returncode!=0}
        body=json.dumps({'code':200,'data':data}).encode();self.send_response(200);self.send_header('Content-Type','application/json');self.end_headers();self.wfile.write(body)
HTTPServer(('127.0.0.1',int(os.environ['PORT'])),Handler).serve_forever()
'''
    (root/'probe.py').write_text(script);(root/'probe.py').chmod(0o755)


def snapshot():
    config=Path('config/config.yaml')
    return {'configuration':hashlib.sha256(config.read_bytes()).hexdigest(),
        'credentials':hashlib.sha256(sql("SELECT user_id,provider,identifier,secret_hash,is_primary FROM public.iam_credential ORDER BY id").encode()).hexdigest(),
        'users':hashlib.sha256(sql('SELECT uuid,email,is_root,status FROM public.iam_user ORDER BY id').encode()).hexdigest(),
        'grants':hashlib.sha256(sql('SELECT profile_id,permission_id FROM public.iam_api_key_profile_permission ORDER BY profile_id,permission_id').encode()).hexdigest(),
        'uploads':{str(p.relative_to('data/runtime/uploads')):hashlib.sha256(p.read_bytes()).hexdigest() for p in Path('data/runtime/uploads').rglob('*') if p.is_file()}}


def main():
    parser=argparse.ArgumentParser();parser.add_argument('--pair',required=True);parser.add_argument('--platform',required=True);parser.add_argument('--report',required=True)
    args=parser.parse_args()
    if os.environ.get('POWERX_UPGRADE_CI')!='1' or not os.environ.get('COMPOSE_PROJECT_NAME','').startswith('powerx-upgrade-'):
        raise RuntimeError('UPGRADE_CI_ISOLATION_REQUIRED')
    baseline=json.loads(Path(__file__).with_name('upgrade-baseline.json').read_text());pair=json.loads(Path(args.pair).read_text())
    report={'schema_version':'powerx.upgrade-acceptance/v1','status':'running','platform':args.platform,'source_sha':baseline['source_sha'],'target_sha':pair['source_sha'],
        'source_images':{k:v['platforms'][args.platform]['digest'] for k,v in baseline['images'].items()},
        'target_images':{k:v['platforms'][args.platform]['digest'] for k,v in pair['images'].items()},
        'checks':{k:False for k in REQUIRED_CHECKS},'http':[],'migration_results':[], 'stage':'baseline_start'}
    acceptance=Acceptance(report)
    output=Path(args.report);output.parent.mkdir(parents=True,exist_ok=True)
    try:
        select_images(baseline,args.platform)
        subprocess.run(['sh','run.sh','start'],check=True,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
        report['stage']='old_setup'
        values=json.loads(compose('run','--rm','--no-deps','init','setup-values').stdout)
        cfg=acceptance.api('/api/v1/admin/setup/config')['config'];password=secrets.token_urlsafe(32)
        cfg.update(deployment={'env':'dev'},domain={'domain':'127.0.0.1','enable_cdn':False},https={'mode':'disable'},llm={'enabled':False},ports={'backend_port':8080,'web_admin_port':3000})
        cfg['storage']={'type':'local','local_path':'/data/uploads','public_url':BASE+'/media'}
        cfg['database']={'type':'postgresql','host':'postgres','port':5432,'name':'powerx','username':'powerx','password':values['database']['password'],'ssl_mode':'disable'}
        cfg['cache']={'type':'redis','redis_host':'redis','redis_port':6379,'redis_db':0,'redis_password':values['cache']['password']}
        cfg['admin'].update(username='upgrade-admin',email='upgrade-admin@example.com',password=password,display_name='Upgrade Admin')
        acceptance.api('/api/v1/admin/setup/config',cfg,'PUT');acceptance.api('/api/v1/admin/setup/provision',{});acceptance.api('/api/v1/admin/setup/complete',{})
        report['stage']='old_setup_restart'
        acceptance.wait_installed()
        report['stage']='old_login'
        acceptance.login(password);acceptance.mark('old_setup')
        report['stage']='business_plugin_media_fixture'
        prepare_ci_mount_access()
        job=acceptance.api('/api/v1/admin/scheduler/jobs',{'owner_type':'core','owner_id':'upgrade-fixture','name':'preserved-job','schedule_type':'cron','schedule_expr':'0 0 1 1 *','timezone':'UTC','payload':{'marker':'preserved-business'}})['job']
        job_id=job['uuid']
        acceptance.api('/api/v1/admin/scheduler/jobs/'+job_id+'/pause',{})
        fixture_plugin();acceptance.api('/api/v1/admin/plugins/install/local',{'src_dir':'/data/upgrade-fixture','enable':True})
        probe_path='/_p/'+PLUGIN+'/api/v1/probe';old_probe=acceptance.api(probe_path)
        assert old_probe['value']=='preserved-plugin-data' and old_probe['core_user_table_denied']
        contents=b'PowerX preserved upload fixture\n';checksum=hashlib.sha256(contents).hexdigest()
        asset=acceptance.api('/api/v1/admin/media/assets',{'name':'upgrade.txt','driver':'local','uploadMethod':'presign_upload','contentSha256':checksum,'sizeBytes':len(contents),'mimeType':'text/plain'})
        asset_id=asset['uuid'];upload=acceptance.api('/api/v1/admin/media/assets/'+asset_id+'/presign',{'action':'upload','method':'PUT','filename':'upgrade.txt','content_type':'text/plain'})
        url=upload['url'];url=BASE+url if url.startswith('/') else url
        with urllib.request.urlopen(urllib.request.Request(url,data=contents,headers=upload.get('headers',{}),method='PUT'),timeout=30) as response: assert 200<=response.status<300
        compose('stop','backend','web-admin','gateway')
        prepare_ci_mount_access()
        before=snapshot()
        report['stage']='offline_upgrade'
        with tempfile.TemporaryDirectory(prefix='powerx-upgrade-backup-') as temp:
            backup=Path(temp);shutil.copytree('config',backup/'config');shutil.copytree('data/runtime',backup/'runtime')
            dump=compose('exec','-T','postgres','pg_dump','-U','powerx','-d','powerx','-Fc').stdout
            select_images(pair,args.platform)
            identity=json.loads(compose('run','--rm','--no-deps','init','/app/database','--version').stdout)
            assert identity['git_commit']==pair['source_sha'];acceptance.mark('binary_identity')
            checked=json.loads(compose('run','--rm','--no-deps','init','upgrade-check').stdout);assert checked['status']=='ready'
            plan=acceptance.migration('plan',pair['release_id'],baseline['source_sha']);assert plan['status']=='ready'
            unknown=acceptance.migration('plan',pair['release_id'],'0'*40,code=2);assert unknown['error_code']=='UPGRADE_SOURCE_UNSUPPORTED';acceptance.mark('unknown_source_rejected')
            config_path=Path('config/config.yaml');original=config_path.read_bytes();config_path.write_text('auth: [broken')
            invalid=acceptance.migration('plan',pair['release_id'],baseline['source_sha'],code=2);assert invalid['error_code']=='UPGRADE_CONFIG_INVALID';config_path.write_bytes(original);acceptance.mark('invalid_config_rejected')
            # 模拟无服务身份的历史会话：只在测试库事务中暂存，拒绝后恢复。
            table=sql("SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_name LIKE '%local_install_sessions' LIMIT 1")
            if not table: raise RuntimeError('UPGRADE_CI_LEGACY_TABLE_MISSING')
            sql('ALTER TABLE public.'+table+' RENAME TO upgrade_saved_sessions; CREATE TABLE public.'+table+' (plugin_id text,service_actor text); INSERT INTO public.'+table+" VALUES ('',NULL)")
            blocked=acceptance.migration('plan',pair['release_id'],baseline['source_sha'],code=2);assert blocked['error_code']=='UPGRADE_LEGACY_PLUGIN_ACTOR_UNRESOLVED'
            sql('DROP TABLE public.'+table+'; ALTER TABLE public.upgrade_saved_sessions RENAME TO '+table);acceptance.mark('legacy_blocker_rejected')
            plan=acceptance.migration('plan',pair['release_id'],baseline['source_sha'])
            sql('CREATE TABLE public.upgrade_drift (id integer)')
            drift=acceptance.migration('apply',pair['release_id'],baseline['source_sha'],plan['plan_hash'],code=3);assert drift['error_code']=='UPGRADE_PLAN_CHANGED'
            sql('DROP TABLE public.upgrade_drift');acceptance.mark('plan_drift_rejected')
            plan=acceptance.migration('plan',pair['release_id'],baseline['source_sha'])
            acceptance.verify_lock_conflict(pair['release_id'],baseline['source_sha'],plan['plan_hash'])
            # 测试库触发器只拒绝 catalog 步骤，不提供生产命令的任意故障注入参数。
            sql("CREATE FUNCTION public.upgrade_ci_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF current_setting('powerx.upgrade_step',true) LIKE '20261011_catalog_%' THEN RAISE EXCEPTION 'ci catalog fault'; END IF; RETURN NEW; END $$; CREATE TRIGGER upgrade_ci_fault BEFORE INSERT OR UPDATE ON public.iam_permission FOR EACH ROW EXECUTE FUNCTION public.upgrade_ci_fault()")
            plan=acceptance.migration('plan',pair['release_id'],baseline['source_sha'])
            failed=acceptance.migration('apply',pair['release_id'],baseline['source_sha'],plan['plan_hash'],code=4)
            assert failed['status']=='failed' and failed['partial'] and failed['retryable'] and not failed['requires_restore']
            assert sql("SELECT count(*) FROM public.database_migration_records WHERE migration_id LIKE '20261011_catalog_%'")=='0'
            sql("CREATE OR REPLACE FUNCTION public.upgrade_ci_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF current_setting('powerx.upgrade_step',true) LIKE '20261011_catalog_%' THEN PERFORM pg_sleep(120); END IF; RETURN NEW; END $$")
            plan=acceptance.migration('plan',pair['release_id'],baseline['source_sha'])
            acceptance.interrupt_catalog(pair['release_id'],baseline['source_sha'],plan['plan_hash'])
            sql('DROP TRIGGER upgrade_ci_fault ON public.iam_permission; DROP FUNCTION public.upgrade_ci_fault()')
            plan=acceptance.migration('plan',pair['release_id'],baseline['source_sha'])
            applied=acceptance.migration('apply',pair['release_id'],baseline['source_sha'],plan['plan_hash']);assert applied['status']=='succeeded' and applied['applied'] and applied['skipped']
            acceptance.mark('partial_failure_retry');acceptance.mark('interrupted_retry')
            assert snapshot()==before;acceptance.mark('configuration_preserved');acceptance.mark('files_preserved')
            repeat_plan=acceptance.migration('plan',pair['release_id'],baseline['source_sha']);assert repeat_plan['status']=='up_to_date'
            repeat=acceptance.migration('apply',pair['release_id'],baseline['source_sha'],repeat_plan['plan_hash']);assert not repeat['applied'];assert snapshot()==before;acceptance.mark('idempotent')
            record=sql("SELECT migration_id || ':' || checksum FROM public.database_migration_records WHERE migration_id LIKE '20261011_core_%' LIMIT 1");migration_id,checksum=record.split(':')
            sql("UPDATE public.database_migration_records SET checksum='"+'0'*64+"' WHERE migration_id='"+migration_id+"'")
            invalid=acceptance.migration('plan',pair['release_id'],baseline['source_sha'],code=2);assert invalid['error_code']=='UPGRADE_MIGRATION_CHECKSUM_MISMATCH'
            sql("UPDATE public.database_migration_records SET checksum='"+checksum+"' WHERE migration_id='"+migration_id+"'");acceptance.mark('checksum_rejected')
            assert sql("SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='database_migration_records' AND column_name='checksum'")=='1';acceptance.mark('new_schema')
            compose('--profile','standalone','up','-d','--wait','--wait-timeout','300','backend','web-admin','gateway')
            report['stage']='target_runtime_preservation'
            acceptance.wait_installed();acceptance.login(password);acceptance.mark('login_preserved')
            with urllib.request.urlopen(BASE+'/users/login',timeout=30) as response:assert response.status==200
            for service,role in [('backend','backend'),('web-admin','web_admin')]:
                container=compose('ps','-q',service).stdout.decode().strip()
                actual=subprocess.check_output(['docker','inspect','--format','{{.Config.Image}}',container],text=True).strip()
                expected=pair['images'][role]['repository']+'@'+pair['images'][role]['platforms'][args.platform]['digest'];assert actual==expected
            acceptance.mark('paired_frontend')
            preserved=acceptance.api('/api/v1/admin/scheduler/jobs/'+job_id)['job'];assert preserved['name']=='preserved-job' and preserved['status']=='paused';acceptance.mark('business_data_preserved')
            new_probe=acceptance.api(probe_path);assert new_probe==old_probe;acceptance.mark('plugin_runtime_preserved')
            assert snapshot()==before, 'Installed credentials, grants, configuration and uploads changed after startup'
            download=acceptance.api('/api/v1/admin/media/assets/'+asset_id+'/presign',{'action':'download'})
            url=download['url'];url=BASE+url if url.startswith('/') else url
            with urllib.request.urlopen(urllib.request.Request(url,headers=download.get('headers',{})),timeout=30) as response:assert response.read()==contents
            compose('stop','backend','web-admin','gateway')
            report['stage']='restore_rollback'
            prepare_ci_mount_access()
            # rollback=restore_required：确实恢复数据库、配置、文件，再启动固定旧镜像。
            compose('exec','-T','postgres','dropdb','-U','powerx','--force','powerx');compose('exec','-T','postgres','createdb','-U','powerx','powerx')
            compose('exec','-T','postgres','pg_restore','-U','powerx','-d','powerx','--exit-on-error',input=dump)
            shutil.rmtree('config');shutil.copytree(backup/'config','config');shutil.rmtree('data/runtime');shutil.copytree(backup/'runtime','data/runtime')
            assert snapshot()==before
            select_images(baseline,args.platform);compose('--profile','standalone','up','-d','--wait','--wait-timeout','300','backend','web-admin','gateway')
            acceptance.wait_installed();acceptance.login(password);assert acceptance.api(probe_path)==old_probe
            assert acceptance.api('/api/v1/admin/scheduler/jobs/'+job_id)['job']['name']=='preserved-job';acceptance.mark('restore_rollback')
        report['status']='passed';report['stage']='complete'
    except Exception as error:
        report['status']='failed';report['error_code']=str(error) if isinstance(error,RuntimeError) else 'UPGRADE_CI_ACCEPTANCE_FAILED'
        frames=traceback.extract_tb(error.__traceback__)
        report['failure']={'stage':report['stage'],'exception_type':type(error).__name__,
                          'location':{'file':Path(frames[-1].filename).name,'line':frames[-1].lineno} if frames else None,
                          'stack':[{'file':Path(f.filename).name,'line':f.lineno,'function':f.name} for f in frames]}
        # No exception repr, HTTP bodies, SQL or subprocess output: those may
        # contain deployment credentials. The location and status are enough
        # to identify the failed acceptance contract.
        print(json.dumps({'error_code':report['error_code'],'failure':report['failure']},sort_keys=True),file=sys.stderr)
        raise RuntimeError(report['error_code']) from None
    finally:
        output.write_text(json.dumps(report,indent=2)+'\n')


if __name__=='__main__':main()
