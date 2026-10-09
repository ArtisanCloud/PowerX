"""Image-only acceptance; secrets and tokens are never printed."""
import json
from pathlib import Path
import time
import subprocess
import urllib.request

base='http://127.0.0.1:18080'
def api(path,body=None):
    request=urllib.request.Request(base+path,data=json.dumps(body).encode() if body else None,
        headers={'Content-Type':'application/json'})
    with urllib.request.urlopen(request,timeout=30) as response:
        return json.load(response)

with urllib.request.urlopen(base,timeout=30) as response:
    assert response.status==200
health=api('/api/v1/health')
assert health['data']['install_status']=='installed',health
credentials=json.loads(subprocess.check_output(['docker','compose','run','--rm','--no-deps','init','credentials'],text=True))
login=api('/api/v1/admin/user/auth/login',{'identifier':credentials['email'],'password':credentials['password']})
assert login['code']==200 and login['data'].get('access_token'), 'Login failed'
assert not any('localhost:8077' in str(v) for v in login.values())
print('Frontend HTTP 200, installed health and generated administrator login verified.')
