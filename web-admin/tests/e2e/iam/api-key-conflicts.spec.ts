import { test, expect, type Page } from '@playwright/test';
test.use({ actionTimeout: 15_000 });
const tenant = '00000000-0000-4000-8000-000000000001';
const ecom = 'com.powerx.plugins.ecommerce';
const keys = [
  {
    key_id: '00000000-0000-4000-8000-000000000101',
    name: 'WorkspaceX Key',
    profile_id: 1,
    tenant_uuid: tenant,
    plugin_ids: ['com.powerx.plugins.workspacex'],
    binding_mode: 'key',
    status: 'active',
    key_prefix: 'fixture-prefix',
  },
  {
    key_id: '00000000-0000-4000-8000-000000000102',
    name: 'CRM Key',
    profile_id: 1,
    tenant_uuid: tenant,
    plugin_ids: ['com.powerx.plugins.crm'],
    binding_mode: 'legacy_permissions',
    status: 'active',
    key_prefix: 'fixture-prefix',
  },
  {
    key_id: '00000000-0000-4000-8000-000000000103',
    name: 'Ecommerce Key',
    profile_id: 2,
    tenant_uuid: tenant,
    plugin_ids: [ecom],
    binding_mode: 'key',
    status: 'active',
    key_prefix: 'fixture-prefix',
  },
];
const catalog = [
  {
    id: 1,
    module: 'scheduler',
    resource: 'scheduler_jobs',
    action: 'read',
    meta: {
      label: '定时任务读取',
      capability_id: 'com.corex.scheduler.jobs.read',
    },
  },
  {
    id: 2,
    module: 'ecommerce',
    resource: 'subscription_plans',
    action: 'read',
    meta: {
      label: '电商套餐读取',
      api_key_explicit: true,
      api_key: {
        plugin_id: ecom,
        scope: '_scope.ecommerce.subscription_plans.read',
      },
      capability_id: ecom + '.product.spu.subscription.plan.read',
    },
  },
];
const conflict = (key: (typeof keys)[number], cleared = false) => ({
  key_id: key.key_id,
  key_name: key.name,
  binding_mode: key.binding_mode,
  owner_plugin_ids: cleared ? [] : key.plugin_ids,
  permission_owner_plugin_ids:
    cleared || key.binding_mode === 'legacy_permissions' ? [] : key.plugin_ids,
  permission_id: 2,
  permission_name: '电商套餐读取',
  capability_id: ecom + '.product.spu.subscription.plan.read',
  required_plugin_id: ecom,
  scope: '_scope.ecommerce.subscription_plans.read',
  action: 'read',
  resource_type: 'capability',
  resource_pattern: 'subscription_plans',
});
const failure = (items: ReturnType<typeof conflict>[], profile = 1) => ({
  code: 400,
  error_code: 'API_KEY_PLUGIN_OWNER_CONFLICT',
  reason_code: 'API_KEY_PLUGIN_OWNER_CONFLICT',
  message: 'Profile 权限保存失败',
  error: '插件归属冲突',
  details: {
    profile_id: profile,
    profile_name: profile === 1 ? 'Shared Profile' : 'Ecommerce Profile',
    conflict_count: items.length,
    conflicting_key_count: new Set(items.map((c) => c.key_id)).size,
    conflicts: items,
    truncated: false,
    trace_id: 'fixture-owner-conflict-trace',
  },
});
async function prepare(page: Page) {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const writes: any[] = [];
  let delaySave = false;
  let finishSave: (() => void) | undefined;
  await page.addInitScript(
    ({ tenant }) => {
      const token = `eyJhbGciOiJub25lIn0.${btoa(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + 3600, tid: tenant, sub: 'conflict-test' }))}.test`;
      localStorage.setItem(
        'powerx.session',
        JSON.stringify({
          version: 1,
          status: 'active',
          sessionId: 'conflict-test',
          revision: 1,
          accessToken: token,
          refreshToken: 'fixture-refresh',
          tokenType: 'Bearer',
          expiresAt: Date.now() + 3600000,
          scope: 'access',
          tenantUuid: tenant,
        }),
      );
      localStorage.setItem('px_locale', 'zh');
      localStorage.setItem('powerx-color-mode', 'dark');
      localStorage.setItem('welcome_guide_completed', 'true');
    },
    { tenant },
  );
  await page.routeWebSocket(/\/api\/ws/, (socket) => socket.close());
  await page.route(
    (url) => url.pathname.startsWith('/api/'),
    async (route) => {
      const req = route.request(),
        url = new URL(req.url()),
        path = url.pathname;
      if (path.includes('/_nuxt_icon/')) {
        await route.fallback();
        return;
      }
      let data: unknown = {};
      if (path.endsWith('/setup/status'))
        data = {
          configured: true,
          requires_login: true,
          install_status: 'installed',
        };
      else if (path.endsWith('/me/context'))
        data = {
          is_root: true,
          current_tenant_uuid: tenant,
          current_member_id: 1,
          current_member_uuid: '00000000-0000-4000-8000-000000000002',
          user: {
            id: 1,
            uuid: '00000000-0000-4000-8000-000000000003',
            display_name: 'Conflict Tester',
            email: 'tester@example.invalid',
            status: 1,
          },
          members: [
            {
              tenant_uuid: tenant,
              tenant_name: 'Test Tenant',
              member_id: 1,
              is_admin: true,
            },
          ],
        };
      else if (path.endsWith('/integration/api-key-profiles'))
        data = {
          items: [
            {
              id: 1,
              tenant_uuid: tenant,
              name: 'Shared Profile',
              key: 'shared',
              status: 1,
            },
            {
              id: 2,
              tenant_uuid: tenant,
              name: 'Ecommerce Profile',
              key: 'ecommerce',
              status: 1,
            },
          ],
        };
      else if (path.endsWith('/integration/api-keys'))
        data = {
          items: keys,
          pagination: { total: 3, page: 1, page_size: 200, pages: 1 },
        };
      else if (path.endsWith('/integration/permissions/catalog'))
        data = { items: catalog };
      else if (/\/api-key-profiles\/\d+\/permissions$/.test(path)) {
        const id = Number(path.match(/profiles\/(\d+)/)![1]);
        if (req.method() === 'PUT') {
          writes.push({ kind: 'profile', id, body: req.postDataJSON() });
          if (delaySave)
            await new Promise<void>((resolve) => {
              finishSave = resolve;
            });
          if (id === 1 && req.postDataJSON().permission_ids.includes(2)) {
            await route.fulfill({
              status: 400,
              contentType: 'application/json',
              body: JSON.stringify(
                failure(keys.slice(0, 2).map((k) => conflict(k))),
              ),
            });
            return;
          }
          data = {
            profile_id: id,
            permission_ids: req.postDataJSON().permission_ids,
          };
        } else data = { profile_id: id, permission_ids: [2] };
      } else if (/\/api-keys\/[^/]+\/plugin-owners$/.test(path)) {
        const key = keys.find((k) => path.includes(k.key_id))!;
        if (req.method() === 'PUT') {
          writes.push({
            kind: 'owners',
            id: key.key_id,
            body: req.postDataJSON(),
          });
          await route.fulfill({
            status: 400,
            contentType: 'application/json',
            body: JSON.stringify(failure([conflict(key, true)], 2)),
          });
          return;
        }
        data = {
          key_id: key.key_id,
          tenant_uuid: tenant,
          plugin_ids: key.plugin_ids,
          binding_mode: key.binding_mode,
        };
      } else if (path.endsWith('/menus'))
        data = {
          menus: [
            {
              id: 'api-keys',
              title: 'API Key',
              path: '/settings/integration-api-keys',
              visible: true,
              origin: 'system',
            },
          ],
        };
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 200, message: 'success', data }),
      });
    },
  );
  await page.goto('/settings/integration-api-keys');
  await expect(
    page.getByRole('heading', { name: 'API Key 权限管理' }),
  ).toBeVisible({ timeout: 60_000 });
  if ((page.viewportSize()?.width || 1440) < 768)
    await page
      .getByRole('complementary')
      .getByRole('button', { name: '收起', exact: true })
      .first()
      .click();
  await expect(page.getByText('已选 1/2 项权限')).toBeVisible();
  return {
    errors,
    writes,
    delay: () => {
      delaySave = true;
    },
    release: () => {
      finishSave?.();
    },
  };
}

for (const viewport of [
  { name: 'desktop', width: 1440, height: 1000 },
  { name: 'narrow', width: 390, height: 844 },
]) {
  test(`${viewport.name}: profile conflict details, preserved selections and owner clear rejection`, async ({
    page,
  }, info) => {
    await page.setViewportSize(viewport);
    const state = await prepare(page);
    await page.getByRole('button', { name: '全模块全选', exact: true }).click();
    await page.getByRole('button', { name: '保存权限', exact: true }).click();
    const notice = page.getByTestId('api-key-owner-conflict');
    await expect(notice).toContainText('发现 2 项冲突，涉及 2 个 API Key');
    await expect(notice).toContainText('当前勾选已保留');
    await expect(page.getByText('已选 2/2 项权限')).toBeVisible();
    expect([...state.writes[0].body.permission_ids].sort()).toEqual([1, 2]);
    await notice.getByRole('button', { name: '查看冲突详情' }).click();
    let dialog = page.getByRole('dialog');
    await expect(dialog).toContainText('WorkspaceX Key');
    await expect(dialog).toContainText('CRM Key');
    await expect(dialog).toContainText('电商套餐读取');
    await expect(dialog).toContainText(ecom);
    await expect(dialog).toContainText('此权限原有插件范围');
    await expect(dialog).toContainText('未绑定');
    await expect(dialog).toContainText('清空绑定仍会冲突');
    await expect(dialog).toContainText('fixture-owner-conflict-trace');
    const box = await dialog.boundingBox();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(viewport.width);
    expect(
      await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth),
    ).toBe(true);
    await page.screenshot({
      path: info.outputPath(`conflict-${viewport.name}.png`),
      animations: 'disabled',
    });
    await dialog.getByRole('button', { name: '关闭详情' }).click();
    // 修正所选集合后可以重试；原来的报错不继续伪装为当前结果。
    await page.getByRole('button', { name: '全模块清空', exact: true }).click();
    await page.getByRole('button', { name: '保存权限', exact: true }).click();
    await expect(notice).toBeHidden();
    expect(state.writes.at(-1).body.permission_ids).toEqual([]);
    await page
      .getByRole('button')
      .filter({ hasText: 'Ecommerce Profile' })
      .click();
    await expect(page.getByText('已选 1/2 项权限')).toBeVisible();
    await page.getByRole('tab', { name: 'API Key 管理', exact: true }).click();
    const row = page.getByRole('row').filter({ hasText: 'Ecommerce Key' });
    await row
      .getByRole('button', { name: '配置插件 owner', exact: true })
      .click();
    dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('textbox')).toHaveValue(ecom);
    await dialog.getByRole('textbox').fill('');
    await dialog
      .getByRole('button', { name: '保存 owner 授权', exact: true })
      .click();
    await expect(dialog).toContainText('输入内容已保留');
    await expect(dialog.getByRole('textbox')).toHaveValue('');
    await dialog.getByRole('button', { name: '查看冲突详情' }).click();
    const details = page.getByTestId('api-key-conflict-details');
    await expect(details).toContainText('Ecommerce Key');
    await expect(details).toContainText('未绑定');
    await expect(details).toContainText('电商套餐读取');
    await page.getByRole('button', { name: '关闭详情' }).click();
    await page
      .getByRole('dialog')
      .getByRole('button', { name: '取消', exact: true })
      .click();
    expect(state.writes.at(-1).body.plugin_ids).toEqual([]);
    await expect(row).toContainText(ecom);
    expect(state.errors).toEqual([]);
  });
}

test('ignores a conflict response after the selected Profile changes', async ({
  page,
}) => {
  const state = await prepare(page);
  state.delay();
  await page.getByRole('button', { name: '全模块全选', exact: true }).click();
  await page.getByRole('button', { name: '保存权限', exact: true }).click();
  await expect.poll(() => state.writes.length).toBe(1);
  await page
    .getByRole('button')
    .filter({ hasText: 'Ecommerce Profile' })
    .click();
  state.release();
  await expect(page.getByText('已选 1/2 项权限')).toBeVisible();
  await page.getByRole('button', { name: '全模块全选', exact: true }).click();
  await expect(
    page.getByRole('button', { name: '保存权限', exact: true }),
  ).not.toBeDisabled();
  await expect(page.getByTestId('api-key-owner-conflict')).toBeHidden();
  expect(state.errors).toEqual([]);
});
