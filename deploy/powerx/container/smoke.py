"""Verify first boot stays in Setup, then exercise Setup only in disposable CI."""
import json
import secrets
import subprocess
import time
import urllib.request
import urllib.error

base='http://127.0.0.1:18080'
def api(path,body=None,method=None):
    request=urllib.request.Request(base+path,data=json.dumps(body).encode() if body is not None else None,
        headers={'Content-Type':'application/json'},method=method)
    try:
        with urllib.request.urlopen(request,timeout=120) as response:return json.load(response)
    except urllib.error.HTTPError as error:
        payload=error.read().decode(errors='replace')
        for value in sensitive_values:
            if value:payload=payload.replace(value,'[REDACTED]')
        raise AssertionError(f'{path}: HTTP {error.code}: '+payload) from None

sensitive_values=[]

with urllib.request.urlopen(base+'/setup',timeout=30) as response:assert response.status==200
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
assert api('/api/v1/admin/setup/config',config,'PUT')['code']==200
assert api('/api/v1/admin/setup/provision',{})['code']==200
assert api('/api/v1/admin/setup/complete',{})['code']==200
for attempt in range(90):
    try:
        status=api('/api/v1/admin/setup/status')['data']
        if status['configured'] and status['requires_login'] and not status['restart_required']:break
    except Exception:pass
    time.sleep(1)
else:raise AssertionError('Docker restart did not load completed Setup')
login=api('/api/v1/admin/user/auth/login',{'identifier':'ci-admin@example.com','password':password})
assert login['code']==200 and login['data'].get('access_token')
print('Explicit Setup provisioning, completion, Docker restart and selected administrator login passed.')
