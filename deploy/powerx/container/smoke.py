"""Verify first boot stays in Setup, then exercise Setup only in disposable CI."""
import json
import secrets
import subprocess
import urllib.request
import urllib.error

from playwright.sync_api import expect, sync_playwright
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


def open_setup(browser, origin, viewport):
    page = browser.new_page(locale="zh-CN", viewport=viewport)
    page.goto(origin + "/setup")
    page.get_by_role("checkbox").nth(0).check()
    page.get_by_role("checkbox").nth(1).check()
    page.get_by_role("button", name="下一步", exact=True).click()
    return page


def check_instance_defaults(page, origin, database=("postgres", "5432"), cache=("redis", "6379")):
    for field, value in {
        "db-host": database[0], "db-port": database[1], "db-name": "powerx",
        "db-username": "powerx", "redis-host": cache[0], "redis-port": cache[1],
        "db-instance-password": "已配置", "redis-instance-password": "已配置",
    }.items():
        control = page.get_by_test_id("setup-" + field)
        if field.endswith("password"):
            assert control.input_value() == value, f"Incorrect configured-password indicator for {field}"
            assert not control.is_editable(), f"Expected read-only indicator for {field}"
        else:
            expect(control).to_have_value(value)
            expect(control).not_to_be_editable()
    next_button = page.get_by_role("button", name="下一步", exact=True)
    expect(next_button).to_be_disabled()
    for kind, name in [("database", "测试数据库连接"), ("cache", "测试 Redis 连接")]:
        with page.expect_response(origin + "/api/v1/admin/setup/test/" + kind) as result:
            page.get_by_role("button", name=name, exact=True).click()
        response = result.value
        assert response.request.post_data_json == {kind: {"source": "instance"}}, "Instance credentials must stay on the server"
        assert response.ok, f"{kind} instance connection failed with HTTP {response.status}"
        assert response.json()["data"] == {"ok": True}
        expect(page.get_by_test_id("setup-" + kind + "-test-result").get_by_role("status")).to_be_visible()
    expect(next_button).to_be_enabled()


def check_mode_switch(page):
    cache_source = page.get_by_test_id("setup-cache-source")
    cache_source.click()
    page.get_by_role("option", name="手动配置", exact=True).click()
    expect(page.get_by_test_id("setup-redis-host")).to_be_editable()
    assert page.get_by_placeholder("Redis 密码", exact=True).input_value() == "", "Manual password must start empty"
    expect(page.get_by_test_id("setup-database-test-result").get_by_role("status")).to_be_visible()
    expect(page.get_by_role("button", name="下一步", exact=True)).to_be_disabled()
    cache_source.click()
    page.get_by_role("option", name="使用实例配置", exact=True).click()
    assert page.get_by_test_id("setup-redis-instance-password").input_value() == "已配置"
    expect(page.get_by_role("button", name="下一步", exact=True)).to_be_disabled()


def finish_in_browser(page, origin, password):
    next_button = page.get_by_role("button", name="下一步", exact=True)
    with page.expect_response(origin + "/api/v1/admin/setup/provision", timeout=120_000) as provision:
        next_button.click()
    assert provision.value.ok, f"Browser provisioning returned HTTP {provision.value.status}"
    domain = page.get_by_label("系统域名", exact=True)
    expect(domain).to_have_value("127.0.0.1")
    page.get_by_role("button", name="上一步", exact=True).click()
    for kind in ("database", "cache"):
        expect(page.get_by_test_id("setup-" + kind + "-test-result").get_by_role("status")).to_be_visible()
    next_button.click()
    expect(domain).to_have_value("127.0.0.1")
    next_button.click()
    page.get_by_placeholder("admin", exact=True).fill("ci-admin")
    page.get_by_placeholder("admin@example.com").fill("ci-admin@example.com")
    page.get_by_placeholder("请输入强密码").fill(password)
    page.get_by_placeholder("再次输入密码").fill(password)
    next_button.click()
    next_button.click()
    with page.expect_response(origin + "/api/v1/admin/setup/complete", timeout=120_000) as complete:
        page.get_by_role("button", name="完成设置", exact=True).click()
    assert complete.value.ok, f"Browser completion returned HTTP {complete.value.status}"
    page.wait_for_url("**/home", timeout=120_000)
    page.get_by_role("link", name="登录", exact=True).click()
    page.wait_for_url("**/users/login**")
    page.locator("#identifier").fill("ci-admin@example.com")
    page.locator("#password").fill(password)
    with page.expect_response(origin + "/api/v1/admin/user/auth/login") as login:
        page.locator("form").get_by_role("button", name="登录", exact=True).click()
    assert login.value.ok and login.value.json()["data"].get("access_token"), "New administrator login failed"
    page.wait_for_url(lambda url: "/users/login" not in url, timeout=30_000)


def main():
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
