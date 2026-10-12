"""Verify first boot stays in Setup, then exercise Setup only in disposable CI."""
import json
import secrets
import subprocess
import time
import urllib.request
import urllib.error
from upgrade_smoke import wait_for_setup_page

base='http://127.0.0.1:18080'
def api(path,body=None,method=None,expected_status=200):
    request=urllib.request.Request(base+path,data=json.dumps(body).encode() if body is not None else None,
        headers={'Content-Type':'application/json'},method=method)
    try:
        with urllib.request.urlopen(request,timeout=120) as response:
            assert response.status==expected_status,(path,response.status,expected_status)
            return json.load(response)
    except urllib.error.HTTPError as error:
        payload=error.read().decode(errors='replace')
        if error.code==expected_status:
            return json.loads(payload)
        for value in sensitive_values:
            if value:payload=payload.replace(value,'[REDACTED]')
        raise AssertionError(f'{path}: HTTP {error.code}: '+payload) from None

sensitive_values=[]

def sql(query):
    return subprocess.check_output(['docker','compose','exec','-T','postgres','psql','-v','ON_ERROR_STOP=1',
        '-U','powerx','-d','powerx','-Atc',query],text=True).strip()

def assert_preserved():
    assert sql('SELECT value FROM public.setup_smoke_preservation WHERE id=1')=='preserve-me', \
        'Setup retries must preserve pre-existing data'

wait_for_setup_page()
status=api('/api/v1/admin/setup/status')['data']
assert status['install_status']=='uninstalled' and not status['configured'] and not status['requires_login'],status
rows=subprocess.check_output(['docker','compose','exec','-T','postgres','psql','-U','powerx','-d','powerx','-Atc',"SELECT count(*) FROM information_schema.tables WHERE table_schema='public'"],text=True).strip()
assert rows=='0', 'First boot must not migrate or seed application tables'
print('First boot serves Setup with an empty database and no generated administrator.')
values=json.loads(subprocess.check_output(['docker','compose','run','--rm','--no-deps','init','setup-values'],text=True))
sensitive_values.extend([values['database']['password'],values['cache']['password']])
config=api('/api/v1/admin/setup/config')['data']['config']
config['deployment']={'env':'dev'}
config['domain']={'domain':'127.0.0.1','enable_cdn':False}
config['https']={'mode':'disable'}
config['storage']={'type':'local','local_path':'/data/uploads','public_url':base+'/media'}
config['database']={'type':'postgresql','host':'postgres','port':5432,'name':'powerx','username':'powerx','password':values['database']['password'],'ssl_mode':'disable'}
config['cache']={'type':'redis','redis_host':'redis','redis_port':6379,'redis_db':0,'redis_password':values['cache']['password']}
password=secrets.token_urlsafe(24)
sensitive_values.append(password)
config['admin'].update(username='ci-admin',email='ci-admin@example.com',password=password,display_name='CI Admin')
config['llm']={'enabled':False}
config['ports']={'backend_port':8080,'web_admin_port':3000}
# Reproduce the browser order: database provisioning precedes the admin step.
database_stage=json.loads(json.dumps(config))
database_stage['admin']['password']=''
assert api('/api/v1/admin/setup/config',database_stage,'PUT')['code']==200
assert api('/api/v1/admin/setup/complete',{},expected_status=400)['message']=='setup.errors.adminPasswordRequired'
assert sql("SELECT count(*) FROM information_schema.tables WHERE table_schema='public'")=='0', \
    'Incomplete administrator must be rejected before database initialization'
sql("CREATE TABLE public.setup_smoke_preservation (id integer PRIMARY KEY, value text NOT NULL); "
    "INSERT INTO public.setup_smoke_preservation VALUES (1, 'preserve-me')")
assert api('/api/v1/admin/setup/provision',{})['code']==200
assert_preserved()
assert api('/api/v1/admin/setup/provision',{})['code']==200
assert_preserved()
users=subprocess.check_output(['docker','compose','exec','-T','postgres','psql','-U','powerx','-d','powerx','-Atc',
    'SELECT count(*) FROM public.iam_user'],text=True).strip()
assert users=='0', 'Database step must not create a default/root administrator'
assert api('/api/v1/admin/setup/complete',{},expected_status=400)['message']=='setup.errors.adminPasswordRequired'
assert not api('/api/v1/admin/setup/status')['data']['configured']
assert sql('SELECT count(*) FROM public.iam_user')=='0'
assert_preserved()
print('Database-first Setup step and retry passed with no administrator password.')
assert api('/api/v1/admin/setup/config',config,'PUT')['code']==200
assert api('/api/v1/admin/setup/complete',{})['code']==200
assert_preserved()
# 直接验收迁移后的列宽，不能依赖当前目录碰巧存在某个长度的能力 ID。
for table,column,width in [
    ('capability_registry_records','capability_id',256),
    ('capability_registry_adapter_endpoints','adapter_id',512),
    ('capability_registry_health_probe_results','adapter_id',512),
]:
    actual=sql(f"SELECT character_maximum_length FROM information_schema.columns "
        f"WHERE table_schema='public' AND table_name='{table}' AND column_name='{column}'")
    assert actual==str(width),(table,column,actual,width)
assert int(sql('SELECT count(*) FROM public.capability_registry_records'))>0, 'Capability catalog must be published'
for attempt in range(90):
    try:
        status=api('/api/v1/admin/setup/status')['data']
        if status['configured'] and status['requires_login'] and not status['restart_required']:break
    except Exception:pass
    time.sleep(1)
else:raise AssertionError('Docker restart did not load completed Setup')
login=api('/api/v1/admin/user/auth/login',{'identifier':'ci-admin@example.com','password':password})
assert login['code']==200 and login['data'].get('access_token')
assert sql('SELECT count(*) FROM public.iam_user WHERE is_root=true')=='1'
assert_preserved()
print('Explicit Setup provisioning, completion, Docker restart and selected administrator login passed.')
