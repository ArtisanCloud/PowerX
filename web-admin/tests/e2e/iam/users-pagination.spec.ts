import { test, expect, type Page } from '@playwright/test';
test.use({ actionTimeout: 10000 });

const tenant = '00000000-0000-4000-8000-000000000001';
const records = Array.from({ length: 25 }, (_, i) => ({
  Member: {
    uuid: `00000000-0000-4000-8000-${String(i + 100).padStart(12, '0')}`,
    username: `person${i + 1}`, display_name: `Person ${i + 1}`, status: 1,
    avatar_url: '/favicon.ico',
  },
  User: {
    uuid: `00000000-0000-4000-8000-${String(i + 200).padStart(12, '0')}`,
    display_name: `Person ${i + 1}`, email: `person${i + 1}@example.invalid`,
    status: 1, is_root: false,
  },
  DeptIDs: [],
}));

async function prepare(page: Page, options: { root?: boolean; locale?: string; theme?: string } = {}) {
  const requests: Record<string, string>[] = [];
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.addInitScript(({ tenant, locale, theme }) => {
    const token = `eyJhbGciOiJub25lIn0.${btoa(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + 3600, tid: tenant, sub: 'pagination-test' }))}.test`;
    localStorage.setItem('powerx.session', JSON.stringify({ version: 1, status: 'active', sessionId: 'pagination-test', revision: 1, accessToken: token, refreshToken: 'test-refresh', tokenType: 'Bearer', expiresAt: Date.now() + 3600000, scope: 'access', tenantUuid: tenant }));
    localStorage.setItem('px_locale', locale);
    localStorage.setItem('powerx-color-mode', theme);
    localStorage.setItem('welcome_guide_completed', 'true');
  }, { tenant, locale: options.locale || 'zh', theme: options.theme || 'dark' });
  await page.routeWebSocket(/\/api\/ws/, socket => socket.close());
  await page.route(url => url.pathname.startsWith('/api/'), async route => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (path.includes('/_nuxt_icon/')) { await route.fallback(); return; }
    let data: unknown = {};
    if (path.endsWith('/setup/status')) data = { configured: true, requires_login: true, install_status: 'installed' };
    else if (path.endsWith('/me/context')) data = {
      is_root: Boolean(options.root), current_tenant_uuid: tenant, current_member_id: 1,
      current_member_uuid: '00000000-0000-4000-8000-000000000002',
      user: { id: 1, uuid: '00000000-0000-4000-8000-000000000003', display_name: 'Pagination Tester', email: 'pagination@example.invalid', status: 1 },
      members: [{ tenant_uuid: tenant, tenant_name: 'Test Tenant', member_id: 1, is_admin: true }],
    };
    else if (path.endsWith(options.root ? '/system/users' : '/iam/members')) {
      requests.push(Object.fromEntries(url.searchParams));
      const q = url.searchParams.get('q')?.toLowerCase();
      const list = q ? records.filter(record => record.User.email.includes(q) || record.Member.username.includes(q)) : records;
      const pageNumber = Number(url.searchParams.get('page') || 1);
      const pageSize = Number(url.searchParams.get('page_size') || 10);
      data = { items: list.slice((pageNumber - 1) * pageSize, pageNumber * pageSize), pagination: { total: list.length, page: pageNumber, page_size: pageSize, pages: Math.ceil(list.length / pageSize) } };
    }
    else if (/\/(?:system\/users|iam\/members)\/[^/]+\/roles$/.test(path)) data = { role_uuids: [] };
    else if (path.endsWith('/iam/roles')) data = { items: [], pagination: { total: 0, page: 1, page_size: 200, pages: 0 } };
    else if (path.includes('/departments')) data = { items: [], tree: [] };
    else if (path.endsWith('/menus')) data = { menus: [{ id: 'users', title: 'Users', path: '/settings/users', visible: true, origin: 'system' }] };
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, message: 'success', data }) });
  });
  await page.goto('/settings/users');
  if ((page.viewportSize()?.width || 1440) < 768) {
    await page.getByRole('complementary').getByRole('button', { name: '收起', exact: true }).first().click();
  }
  await page.locator('main').getByRole('button', { name: options.locale === 'en' ? 'Users' : '用户', exact: true }).click();
  const footer = page.getByTestId('user-pagination');
  await expect(footer).toContainText(options.locale === 'en' ? 'of 25' : '共 25 条');
  return { footer, requests, errors };
}

test('desktop tenant admin can browse every page and reset a search', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  const { footer, requests, errors } = await prepare(page);
  await expect(footer).toContainText('第 1 / 3 页');
  await footer.locator('button').filter({ hasText: /^3$/ }).click();
  await expect(page.getByTestId('user-pagination-summary')).toContainText('21-25 / 共 25 条');
  await expect(page.getByRole('cell', { name: 'person25', exact: true })).toBeVisible();
  expect(requests.at(-1)).toMatchObject({ page: '3', page_size: '10' });
  expect(requests.at(-1)).not.toHaveProperty('tenant_uuid');
  const search = page.getByPlaceholder('搜索用户名、邮箱或手机号…');
  await search.fill('person25@example.invalid');
  await expect(footer).toContainText('1-1 / 共 1 条');
  await expect(footer.getByRole('button', { name: '下一页', exact: true })).toBeDisabled();
  await search.fill('');
  await expect(footer).toContainText('第 1 / 3 页');
  await footer.scrollIntoViewIfNeeded();
  await page.screenshot({ path: testInfo.outputPath('users-pagination-desktop.png'), animations: 'disabled' });
  expect(errors).toEqual([]);
});

test('narrow root view keeps navigation visible and page size changes preserve the query', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const { footer, requests, errors } = await prepare(page, { root: true, theme: 'light' });
  await page.getByPlaceholder('搜索用户名、邮箱或手机号…').fill('person');
  await expect(footer).toContainText('第 1 / 3 页');
  await footer.getByRole('combobox', { name: '每页条数', exact: true }).click();
  await page.getByRole('option', { name: '每页 20 条', exact: true }).click();
  await expect(footer).toContainText('第 1 / 2 页');
  await footer.getByRole('button', { name: '下一页', exact: true }).click();
  await expect(footer).toContainText('21-25 / 共 25 条');
  expect(requests.at(-1)).toMatchObject({ page: '2', page_size: '20', q: 'person' });
  await footer.scrollIntoViewIfNeeded();
  const bounds = await footer.boundingBox();
  expect(bounds).not.toBeNull();
  expect(bounds!.x).toBeGreaterThanOrEqual(0);
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(390);
  await page.screenshot({ path: testInfo.outputPath('users-pagination-narrow.png'), animations: 'disabled' });
  expect(errors).toEqual([]);
});

test('English pagination remains visible when a search has no matches', async ({ page }) => {
  const { footer, errors } = await prepare(page, { locale: 'en', theme: 'light' });
  await page.getByPlaceholder('Search by username, email or phone…').fill('missing@example.invalid');
  await expect(footer).toContainText('Showing 0-0 of 0');
  await expect(footer).toContainText('Page 1 of 1');
  await expect(footer.getByRole('button', { name: 'Previous', exact: true })).toBeDisabled();
  await expect(footer.getByRole('button', { name: 'Next', exact: true })).toBeDisabled();
  expect(errors).toEqual([]);
});
