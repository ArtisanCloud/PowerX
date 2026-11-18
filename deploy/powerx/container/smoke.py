"""Verify first boot stays in Setup, then exercise Setup only in disposable CI."""
import json
import secrets
import subprocess
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
        if error.code==expected_status:
            return json.load(error)
        raise AssertionError(f'{path}: unexpected HTTP {error.code}') from None

def sql(query):
    return subprocess.check_output(['docker','compose','exec','-T','postgres','psql','-v','ON_ERROR_STOP=1',
        '-U','powerx','-d','powerx','-Atc',query],text=True).strip()

def assert_preserved():
    assert sql('SELECT value FROM public.setup_smoke_preservation WHERE id=1')=='preserve-me', \
        'Setup retries must preserve pre-existing data'


def main():
    from playwright.sync_api import sync_playwright
    from setup_browser_smoke import check_instance_defaults, check_mode_switch, finish_in_browser, open_setup

    wait_for_setup_page()
    status=api('/api/v1/admin/setup/status')['data']
    assert status['install_status']=='uninstalled' and not status['configured'] and not status['requires_login'],status
    assert sql("SELECT count(*) FROM information_schema.tables WHERE table_schema='public'") == '0'
    print('First boot serves Setup with an empty database and no generated administrator.')
    response = api('/api/v1/admin/setup/config')['data']
    config = response['config']
    assert config['database'] == {
        'source':'instance', 'password_configured':True, 'type':'postgresql',
        'host':'postgres', 'port':5432, 'name':'powerx', 'username':'powerx', 'ssl_mode':'disable',
    }
    assert config['cache'] == {
        'source':'instance', 'password_configured':True, 'type':'redis',
        'redis_host':'redis', 'redis_port':6379,
    }
    assert response['instance_connections'] == {'database':config['database'], 'cache':config['cache']}
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch()
        desktop = open_setup(browser, base, {"width": 1440, "height": 900})
        check_instance_defaults(desktop, base)
        mobile = open_setup(browser, base, {"width": 390, "height": 844})
        check_instance_defaults(mobile, base)
        check_mode_switch(mobile)
        mobile.close()
        check_provision_retries(config)
        password = secrets.token_urlsafe(24)
        finish_in_browser(desktop, base, password)
        status = api('/api/v1/admin/setup/status')['data']
        assert status['configured'] and status['requires_login'] and not status['restart_required']
        browser.close()
    check_installed_database()
    print('Fresh desktop and narrow Setup, secret-free connection tests, install, restart and real browser login passed.')


def check_provision_retries(config):
    # Reproduce the browser order: database provisioning precedes the admin step.
    database_stage=json.loads(json.dumps(config))
    database_stage['admin'].update(username='ci-admin', email='ci-admin@example.com', password='')
    database_stage['domain']={'domain':'127.0.0.1','enable_cdn':False}
    database_stage['database'] = {'source':'instance'}
    database_stage['cache'] = {'source':'instance'}
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
    users=sql('SELECT count(*) FROM public.iam_user')
    assert users=='0', 'Database step must not create a default/root administrator'
    assert api('/api/v1/admin/setup/complete',{},expected_status=400)['message']=='setup.errors.adminPasswordRequired'
    assert not api('/api/v1/admin/setup/status')['data']['configured']
    assert sql('SELECT count(*) FROM public.iam_user')=='0'
    assert_preserved()
    print('Database-first Setup step and retry passed with no administrator password.')



def check_installed_database():
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
    assert sql('SELECT count(*) FROM public.iam_user WHERE is_root=true')=='1'
    assert_preserved()


if __name__ == '__main__':
    main()
