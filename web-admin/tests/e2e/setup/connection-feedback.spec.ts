import { test, expect } from '@playwright/test';
import type { Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
const zh = JSON.parse(readFileSync(new URL('../../../i18n/locales/zh.json', import.meta.url), 'utf8'));

async function openDatabaseStep(page: Page) {
  await page.route('**/api/v1/**', async route => {
    const url = new URL(route.request().url());
    let data: unknown = [];
    if (url.pathname.endsWith('/setup/status')) data = {
      configured: false, requires_login: false, checks: { users: 0, tenants: 0, ai_profiles: 0 },
    };
    else if (url.pathname.endsWith('/setup/config') && route.request().method() === 'GET') data = {
      config: {
        deployment: { env: 'dev' },
        domain: { domain: 'powerx-dev.artisan-cloud.com' },
        https: { mode: 'auto', cert_email: 'administrator@artisan-cloud.com' },
        database: { type: 'postgresql', host: 'localhost', port: 5432, name: 'setup_fixture', username: 'fixture', password: 'fixture' },
        cache: { type: 'redis', redis_host: 'localhost', redis_port: 6379, redis_db: 0 },
        storage: { type: 'local', local_path: '/data/uploads' },
        ports: { backend_port: 8080, web_admin_port: 3000 },
      },
    };
    else if (url.pathname.endsWith('/setup/config') || url.pathname.endsWith('/setup/provision')) data = { ok: true };
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data }) });
  });
  await page.goto('/setup');
  const checks = page.getByRole('checkbox');
  await checks.nth(0).check();
  await checks.nth(1).check();
  await page.getByRole('button', { name: '下一步', exact: true }).click();
  await expect(page.getByRole('button', { name: zh.setup.connection.cache.test, exact: true })).toBeVisible();
}

test('Redis failure remains visible, retry has a clear state, and tests do not reload the document', async ({ page }) => {
  await openDatabaseStep(page);
  let documents = 0;
  page.on('request', request => { if (request.isNavigationRequest() && request.resourceType() === 'document') documents++; });
  await page.route('**/api/v1/admin/setup/test/cache', route => route.fulfill({
    status: 400, contentType: 'application/json',
    body: JSON.stringify({ code: 400, message: 'Redis 连接测试失败', error: 'NOAUTH Authentication required.', request_id: 'setup-browser-trace' }),
  }));
  const button = page.getByRole('button', { name: zh.setup.connection.cache.test, exact: true });
  const result = page.getByTestId('setup-cache-test-result');
  await button.click();
  await expect(result).toContainText(zh.setup.connection.redisAuthRequired);
  await expect(result).toContainText('setup-browser-trace');
  await expect(result.locator('[role="alert"]')).toBeVisible();
  let finish: (() => void) | undefined;
  await page.route('**/api/v1/admin/setup/test/cache', async route => {
    await new Promise<void>(resolve => { finish = resolve; });
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data: { ok: true } }) });
  });
  await button.click();
  await expect(button).toBeDisabled();
  await expect(result).toContainText(zh.setup.connection.cache.testing);
  await expect(result).toContainText(zh.setup.connection.redisAuthRequired);
  await expect.poll(() => !!finish).toBe(true);
  finish!();
  await expect(result).toContainText(zh.setup.connection.cache.success);
  await expect(button).toBeEnabled();
  expect(documents).toBe(0);
});

test('domain and certificate email use the form width without an empty icon gutter', async ({ page }) => {
  await openDatabaseStep(page);
  await page.getByRole('button', { name: '下一步', exact: true }).click();
  const domain = page.getByLabel('系统域名', { exact: true });
  const email = page.getByLabel('邮箱地址', { exact: true });
  await expect(domain).toHaveValue('powerx-dev.artisan-cloud.com');
  await expect(email).toHaveValue('administrator@artisan-cloud.com');
  for (const input of [domain, email]) {
    const layout = await input.evaluate(element => {
      return {
        inputWidth: element.getBoundingClientRect().width,
        wrapperWidth: element.parentElement!.getBoundingClientRect().width,
        padding: Number.parseFloat(getComputedStyle(element).paddingLeft),
        clientWidth: element.clientWidth,
        scrollWidth: element.scrollWidth,
      };
    });
    expect(layout.inputWidth).toBeGreaterThan(400);
    expect(Math.abs(layout.inputWidth - layout.wrapperWidth)).toBeLessThan(2);
    expect(layout.padding).toBeLessThan(20);
    expect(layout.scrollWidth).toBeLessThanOrEqual(layout.clientWidth);
  }
});
