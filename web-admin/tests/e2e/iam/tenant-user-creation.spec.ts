import { test, expect, type Page } from '@playwright/test';

const tenant = '00000000-0000-4000-8000-000000000001';
const member = '00000000-0000-4000-8000-000000000101';
const role = '00000000-0000-4000-8000-000000000301';

async function prepare(page: Page, failFirst: boolean) {
  const requests: { method: string; path: string; body?: any }[] = [];
  const rows: any[] = [];
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.addInitScript(({ tenant }) => {
    const token = `eyJhbGciOiJub25lIn0.${btoa(JSON.stringify({ exp: Math.floor(Date.now()/1000)+3600, tid: tenant }))}.fixture`;
    localStorage.setItem('powerx.session', JSON.stringify({ version: 1, status: 'active', sessionId: 'member-fixture', revision: 1, accessToken: token, refreshToken: 'fixture-refresh', tokenType: 'Bearer', expiresAt: Date.now()+3600000, scope: 'access', tenantUuid: tenant }));
    localStorage.setItem('px_locale', 'zh');
    localStorage.setItem('welcome_guide_completed', 'true');
  }, { tenant });
  await page.routeWebSocket(/\/api\/ws/, socket => socket.close());
  await page.route(url => url.pathname.startsWith('/api/'), async route => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (path.includes('/_nuxt_icon/')) { await route.fallback(); return; }
    const method = route.request().method();
    let data: any = {};
    if (path.includes('/admin/system/users')) {
      requests.push({ method, path });
      await route.fulfill({ status: 403, json: { code: 403, message: 'root only' } }); return;
    }
    if (path.endsWith('/setup/status')) data = { configured: true, requires_login: true, install_status: 'installed' };
    else if (path.endsWith('/me/context')) data = {
      is_root: false, current_tenant_uuid: tenant, current_member_id: 1, current_member_uuid: member,
      user: { id: 1, uuid: '00000000-0000-4000-8000-000000000201', display_name: 'Tenant Admin', email: 'admin@example.invalid', status: 1, is_root: false },
      members: [{ tenant_uuid: tenant, tenant_name: 'Fixture Tenant', member_id: 1, member_uuid: member, is_admin: true }],
    };
    else if (path.endsWith('/iam/roles')) data = { items: [{ id: 1, uuid: role, scope: 'tenant', tenant_uuid: tenant, code: 'role_user', name: 'Tenant User' }], pagination: { total: 1, page: 1, page_size: 200, pages: 1 } };
    else if (path.endsWith('/iam/members') && method === 'POST') {
      const body = route.request().postDataJSON(); requests.push({ method, path, body });
      if (failFirst) { failFirst = false; await route.fulfill({ status: 400, json: { code: 400, message: 'Fixture 创建失败，请重试' } }); return; }
      rows.push({ Member: { ...body.member, uuid: member, tenant_uuid: tenant }, User: { uuid: '00000000-0000-4000-8000-000000000202', ...body.user, status: 1, is_root: false }, DeptIDs: [] });
      data = { id: 2 };
    }
    else if (path.endsWith('/iam/members') && method === 'GET') data = { items: rows, pagination: { total: rows.length, page: 1, page_size: 10, pages: Math.ceil(rows.length/10) } };
    else if (path.endsWith('/roles') && path.includes('/iam/members/')) { requests.push({ method, path }); data = { role_uuids: [role] }; }
    else if (path.endsWith('/iam/members/'+member) && method === 'PATCH') {
      const body = route.request().postDataJSON(); requests.push({ method, path, body });
      Object.assign(rows[0].Member, body.member); Object.assign(rows[0].User, body.contact); data = { ok: true };
    }
    else if (path.includes('/departments')) data = { items: [], tree: [] };
    else if (path.endsWith('/menus')) data = { menus: [{ id: 'users', title: 'Users', path: '/settings/users', visible: true, origin: 'system' }] };
    await route.fulfill({ status: 200, json: { code: 200, message: 'success', data } });
  });
  await page.goto('/settings/users');
  if ((page.viewportSize()?.width || 1440) < 768) {
    await page.getByRole('complementary').getByRole('button', { name: '收起', exact: true }).first().click();
  }
  await page.locator('main').getByRole('button', { name: '用户', exact: true }).click();
  await page.getByRole('button', { name: '新增用户', exact: true }).click();
  return { requests, errors };
}

for (const width of [1440, 390]) {
  test(`non-root tenant Admin creates and edits a member at ${width}px`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 1000 });
    const { requests, errors } = await prepare(page, width === 1440);
    await page.locator('input[name="px-user-display-name"]').fill('Created Person');
    await page.locator('input[name="px-tenant-username"]').fill('createdperson');
    await page.locator('input[name="px-user-email"]').fill('createdperson@example.invalid');
    await page.locator('input[name="px-user-initial-password"]').fill('Fixture-secret-123');
    await page.locator('input[name="px-user-confirm-password"]').fill('Fixture-secret-123');
    await page.screenshot({ path: testInfo.outputPath(`tenant-member-form-${width}.png`), animations: 'disabled' });
    await page.getByRole('dialog').getByRole('button', { name: '保存', exact: true }).click();
    if (width === 1440) {
      await expect(page.getByRole('dialog')).toBeVisible();
      await expect(page.locator('input[name="px-tenant-username"]')).toHaveValue('createdperson');
      await page.getByRole('dialog').getByRole('button', { name: '保存', exact: true }).click();
    }
    await expect(page.getByRole('dialog')).not.toBeVisible();
    await expect(page.getByRole('cell', { name: 'createdperson', exact: true })).toBeVisible();
    const create = requests.filter(r => r.method === 'POST').at(-1)!;
    expect(create.path).toBe('/api/v1/admin/iam/members');
    expect(create.body).toMatchObject({ member: { username: 'createdperson', display_name: 'Created Person' }, user: { email: 'createdperson@example.invalid' }, role_uuids: [role] });
    expect(create.body).not.toHaveProperty('tenant_uuid');
    await page.getByRole('button', { name: '编辑', exact: true }).click();
    await expect(page.locator('input[name="px-tenant-username"]')).toHaveValue('createdperson');
    await page.locator('input[name="px-user-display-name"]').fill('Edited Person');
    await page.getByRole('dialog').getByRole('button', { name: '保存', exact: true }).click();
    await expect(page.getByRole('dialog')).not.toBeVisible();
    await expect(page.getByText('Edited Person', { exact: true })).toBeVisible();
    const edit = requests.find(r => r.method === 'PATCH')!;
    expect(edit.path).toBe('/api/v1/admin/iam/members/'+member);
    expect(edit.body.role_uuids).toEqual([role]);
    expect(requests.some(r => r.path.includes('/admin/system/users'))).toBe(false);
    expect(errors).toEqual([]);
    await page.screenshot({ path: testInfo.outputPath(`tenant-member-${width}.png`), animations: 'disabled' });
  });
}
