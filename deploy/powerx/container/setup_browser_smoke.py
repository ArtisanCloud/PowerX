"""Run the first-install wizard against the actual container gateway."""
from playwright.sync_api import expect


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
