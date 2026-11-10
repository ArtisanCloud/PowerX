import { test, expect, type Page, type APIRequestContext } from '@playwright/test';
const tenant = '11111111-1111-4111-8111-111111111111';
test.beforeEach(async ({ page, request }) => {
  await request.delete('/api/v1/__client_apps_test/state');
  await page.addInitScript(({ tenant }) => {
    localStorage.setItem('powerx.session', JSON.stringify({ version: 1, sessionId: 'client-apps-test', revision: 1, status: 'active', accessToken: `header.${btoa(JSON.stringify({ tid: tenant, exp: Math.floor(Date.now() / 1000) + 3600 }))}.fixture`, refreshToken: 'fixture-refresh', tokenType: 'Bearer', expiresAt: Date.now() + 3600000, scope: 'access', tenantUuid: tenant }));
    localStorage.setItem('locale', 'zh');
    localStorage.setItem('welcome_guide_completed', 'true');
  }, { tenant });
});
async function openRegistration(page: Page, width: number, height = 900) {
  await page.setViewportSize({ width, height });
  await page.goto('/settings/client-apps');
  await expect(page.getByTestId('client-app-new')).toBeEnabled();
  if (width < 768) await page.getByRole('button', { name: '收起', exact: true }).first().click();
  await page.getByTestId('client-app-new').click();
}
async function chooseWeChat(page: Page) {
  await page.getByTestId('client-app-provider').click();
  await page.getByRole('option', { name: '微信', exact: true }).click();
}
async function step(page: Page, name: string, number: number) {
  await expect(page.getByTestId(`client-app-section-${name}`)).toHaveAttribute('aria-current', 'step');
  await expect(page.getByTestId(`client-app-section-${name}`)).toBeInViewport({ ratio: 1 });
  await expect(page.getByTestId('client-app-step-progress')).toContainText(`第 ${number}/3 步`);
  if (number < 3) await expect(page.getByTestId('client-app-save')).toHaveCount(0);
}
async function writes(request: APIRequestContext) {
  const state = await (await request.get('/api/v1/__client_apps_test/state')).json();
  return state.requests.filter((r: any) => r.method === 'POST' || r.method === 'PATCH');
}

for (const width of [1440, 390]) {
test(`registration moves forward and back without losing credentials at ${width}px`, async ({ page, request }, info) => {
  await openRegistration(page, width);
  await step(page, 'overview', 1);
  await expect(page.getByTestId('client-app-next-step')).toHaveText('下一步：平台凭证');
  await page.getByTestId('client-app-name').fill('租户微信应用');
  await chooseWeChat(page);
  await page.getByTestId('client-app-next-step').click();
  await step(page, 'credentials', 2);
  await expect(page.getByTestId('client-app-next-step')).toHaveText('下一步：授权插件');
  await expect(page.getByTestId('client-app-name')).toBeHidden();
  await page.getByTestId('client-app-id').fill('wx-browser-fixture');
  await page.getByTestId('client-app-secret').fill('dummy-browser-secret');
  await expect(page.getByTestId('client-app-secret')).toHaveAttribute('type', 'password');
  await page.getByTestId('client-app-previous-step').click();
  await step(page, 'overview', 1);
  await expect(page.getByTestId('client-app-name')).toHaveValue('租户微信应用');
  await expect(page.getByTestId('client-app-provider')).toContainText('微信');
  await page.getByTestId('client-app-next-step').click();
  await expect(page.getByTestId('client-app-secret')).toHaveValue('dummy-browser-secret');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath('credentials-step.png'), fullPage: true, animations: 'disabled' });
  await page.getByTestId('client-app-next-step').click();
  await step(page, 'plugins', 3);
  await expect(page.getByTestId('client-app-next-step')).toHaveCount(0);
  await page.getByRole('checkbox', { name: 'com.example.shop' }).check();
  expect(await writes(request)).toHaveLength(0);
  await page.getByTestId('client-app-save').click();
  await expect(page.getByRole('dialog')).toBeHidden();
  await expect(page.getByText('租户微信应用', { exact: true })).toBeVisible();
  await page.getByTestId('client-app-edit').click();
  await page.getByTestId('client-app-name').fill('更新后的应用');
  await page.getByTestId('client-app-next-step').click();
  await expect(page.getByTestId('client-app-secret')).toHaveValue('');
  await expect(page.getByTestId('client-app-id')).toBeDisabled();
  await page.getByTestId('client-app-next-step').click();
  await request.put('/api/v1/__client_apps_test/state', { data: { conflict: true } });
  await page.getByTestId('client-app-save').click();
  await expect(page.getByTestId('client-app-form-error')).toContainText('应用已被修改');
  await page.getByTestId('client-app-previous-step').click();
  await page.getByTestId('client-app-previous-step').click();
  await expect(page.getByTestId('client-app-name')).toHaveValue('更新后的应用');
  const posted = await writes(request);
  expect(posted).toHaveLength(2);
  expect(posted[0].body).not.toHaveProperty('tenant_uuid');
  expect(posted[1].body.connections[0]).not.toHaveProperty('app_secret');
  expect(await page.evaluate(() => JSON.stringify(localStorage))).not.toContain('dummy-browser-secret');
  await page.getByRole('button', { name: '关闭', exact: true }).last().click();
  await page.getByTestId('client-app-new').click();
  await expect(page.getByTestId('client-app-name')).toHaveValue('');
  await page.getByTestId('client-app-name').fill('新的应用');
  await chooseWeChat(page);
  await page.getByTestId('client-app-next-step').click();
  await expect(page.getByTestId('client-app-secret')).toHaveValue('');
});

test(`Next and Enter validate only the active step and never save early at ${width}px`, async ({ page, request }, info) => {
  await openRegistration(page, width, 800);
  await expect(page.getByTestId('client-app-section-credentials')).toBeDisabled();
  await expect(page.getByTestId('client-app-section-plugins')).toBeDisabled();
  await page.getByTestId('client-app-next-step').click();
  await step(page, 'overview', 1);
  await expect(page.getByTestId('client-app-name')).toHaveAttribute('aria-invalid', 'true');
  await expect(page.getByTestId('client-app-id')).toHaveCount(0);
  await expect(page.getByTestId('client-app-form-error')).toHaveCount(0);
  await page.getByTestId('client-app-name').fill('分步登记验收');
  await chooseWeChat(page);
  await page.getByTestId('client-app-status').click();
  await page.getByRole('option', { name: '已启用', exact: true }).click();
  await page.getByTestId('client-app-name').press('Enter');
  await step(page, 'credentials', 2);
  await expect(page.locator('#app-panel-credentials')).toBeFocused();
  await expect(page.getByTestId('client-app-id')).toHaveAttribute('aria-invalid', 'false');
  expect(await writes(request)).toHaveLength(0);
  await page.getByTestId('client-app-next-step').click();
  await step(page, 'credentials', 2);
  await expect(page.getByTestId('client-app-id')).toHaveAttribute('aria-invalid', 'true');
  await expect(page.getByTestId('client-app-secret')).toHaveAttribute('aria-invalid', 'true');
  await expect(page.getByTestId('client-app-form-error')).toHaveCount(0);
  await page.getByTestId('client-app-id').fill('wx-step-validation');
  await page.getByTestId('client-app-id').press('Enter');
  await step(page, 'credentials', 2);
  expect(await writes(request)).toHaveLength(0);
  await page.getByTestId('client-app-secret').fill('dummy-step-secret');
  await page.getByTestId('client-app-secret').press('Enter');
  await step(page, 'plugins', 3);
  expect(await writes(request)).toHaveLength(0);
  await page.getByTestId('client-app-save').click();
  await expect(page.getByTestId('client-app-plugin-field')).toContainText('请至少选择一个');
  expect(await writes(request)).toHaveLength(0);
  await page.screenshot({ path: info.outputPath('last-step-validation.png'), fullPage: true, animations: 'disabled' });
  await page.getByRole('checkbox', { name: 'com.example.shop' }).check();
  await page.getByTestId('client-app-save').click();
  await expect(page.getByRole('dialog')).toBeHidden();
  expect(await writes(request)).toHaveLength(1);
});
}

test('localized menu, empty kind and unavailable state remain discoverable', async ({ page, request }) => {
  await page.goto('/settings/client-apps');
  await expect(page.getByTestId('client-app-new')).toBeEnabled();
  await expect(page.getByText('系统管理', { exact: true })).toBeVisible();
  await expect(page.getByText('SETTINGS', { exact: true })).toHaveCount(0);
  await page.getByRole('tab', { name: 'App', exact: true }).click();
  await expect(page.getByText('当前分类下还没有应用')).toBeVisible();
  await request.put('/api/v1/__client_apps_test/state', { data: { unavailable: true } });
  await page.getByRole('button', { name: '刷新', exact: true }).click();
  await expect(page.getByTestId('client-apps-error')).toContainText('数据库迁移');
  await expect(page.getByTestId('client-app-new')).toBeDisabled();
});

for (const viewport of [{ width: 1440, height: 900 }, { width: 1024, height: 580 }, { width: 390, height: 740 }]) {
for (const scheme of ['dark', 'light'] as const) {
test(`step navigation and select menus remain visible at ${viewport.width}px in ${scheme}`, async ({ page, request }, info) => {
  await page.emulateMedia({ colorScheme: scheme });
  await page.addInitScript(mode => localStorage.setItem('powerx-color-mode', mode), scheme);
  await openRegistration(page, viewport.width, viewport.height);
  await expect(page.locator('html')).toHaveClass(new RegExp(scheme));
  await step(page, 'overview', 1);
  await page.getByTestId('client-app-name').fill('平台选择验收');
  await page.getByTestId('client-app-provider').click();
  await expect(page.getByRole('listbox')).toBeVisible();
  await page.screenshot({ path: info.outputPath(`provider-${scheme}.png`), fullPage: true, animations: 'disabled' });
  await page.keyboard.press('Escape');
  await page.getByTestId('client-app-next-step').click();
  await step(page, 'overview', 1);
  await expect(page.getByTestId('client-app-provider')).toHaveAttribute('aria-invalid', 'true');
  await expect(page.getByTestId('client-app-id')).toHaveCount(0);
  for (const [field, count] of [['kind', 3], ['provider', 3], ['status', 2]] as const) {
    await page.getByTestId(`client-app-${field}`).click();
    const menu = page.getByRole('listbox');
    await expect(menu).toBeVisible();
    await expect(menu.getByRole('option')).toHaveCount(count);
    for (const option of await menu.getByRole('option').all()) {
      await expect(option).toBeInViewport({ ratio: 1 });
      await expect.poll(() => option.evaluate(el => {
        const box = el.getBoundingClientRect(), menu = el.closest('[role="listbox"]')!;
        return [box.top + 4, box.y + box.height / 2, box.bottom - 4].every(y => {
          const top = document.elementFromPoint(box.x + box.width / 2, y);
          return !!top && menu.contains(top);
        });
      })).toBe(true);
    }
    if (field === 'kind') {
      await expect(menu.getByRole('option', { name: 'App (暂未支持)', exact: true })).toHaveAttribute('aria-disabled', 'true');
      await page.keyboard.press('Escape');
    } else if (field === 'provider') {
      await expect(menu.getByRole('option', { name: '支付宝 (暂未支持)', exact: true })).toHaveAttribute('aria-disabled', 'true');
      await menu.getByRole('option', { name: '微信', exact: true }).click();
    } else await menu.getByRole('option', { name: '已启用', exact: true }).click();
    await expect(menu).toBeHidden();
  }
  await page.getByTestId('client-app-next-step').click();
  await step(page, 'credentials', 2);
  await page.getByTestId('client-app-id').fill('wx-visible-provider');
  await page.getByTestId('client-app-secret').fill('dummy-popup-credential');
  await page.screenshot({ path: info.outputPath(`credentials-${scheme}.png`), fullPage: true, animations: 'disabled' });
  await page.getByTestId('client-app-next-step').click();
  await step(page, 'plugins', 3);
  await page.getByRole('checkbox', { name: 'com.example.shop' }).check();
  await page.screenshot({ path: info.outputPath(`final-step-${scheme}.png`), fullPage: true, animations: 'disabled' });
  expect(await writes(request)).toHaveLength(0);
  await page.getByTestId('client-app-save').click();
  await expect(page.getByRole('dialog')).toBeHidden();
  const posted = await writes(request);
  expect(posted).toHaveLength(1);
  expect(posted[0].body.connections[0].provider).toBe('wechat');
  expect(posted[0].body.kind).toBe('mini_program');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
}
}
