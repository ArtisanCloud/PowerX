import { test, expect, type Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
const zh = JSON.parse(readFileSync(new URL('../../../i18n/locales/zh.json', import.meta.url), 'utf8'));
const streamPermissions = JSON.parse(readFileSync(new URL('../../fixtures/iam/llm-stream-permissions.json', import.meta.url), 'utf8'));
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
        scope: '_scope.ecommerce.subscription_plans.read',
        plugin_id: ecom,
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
async function prepare(page: Page, legacy = false, permissionCatalog: any[] = catalog) {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const writes: any[] = [];
  let delaySave = false;
  let failPreview = false;
  let failCommit = false;
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
      else if (path.endsWith('/integration/api-key-grants/batch')) {
        const body = req.postDataJSON();
        writes.push({ kind: 'batch', id: 1, body });
        if (body.dry_run && failPreview) {
          failPreview = false;
          await route.fulfill({
            status: 409,
            contentType: 'application/json',
            body: JSON.stringify({
              code: 409,
              error_code: 'CAPABILITY_TENANT_NOT_REGISTERED',
              message: 'not registered',
              details: { failed_permission_name: '电商套餐读取' },
            }),
          });
          return;
        }
        if (!body.dry_run && failCommit) {
          failCommit = false;
          await route.abort('failed');
          return;
        }
        data = {
          committed: !body.dry_run,
          key_count: body.key_ids.length,
          added_count: body.key_ids.length,
          plan_hash: 'a'.repeat(64),
          keys: keys
            .filter((key) => body.key_ids.includes(key.key_id))
            .map((key) => ({
              key_id: key.key_id,
              name: key.name,
              owner_plugin_ids: key.plugin_ids,
              added_count: 1,
              existing_count: 0,
            })),
          permissions: [
            {
              permission_id: 2,
              name: '电商套餐读取',
              capability_id: ecom + '.product.spu.subscription.plan.read',
              plugin_id: ecom,
              scope: '_scope.ecommerce.subscription_plans.read',
              action: 'read',
              resource_type: 'capability',
              resource_pattern: 'subscription_plans',
              effect: 'allow',
            },
          ],
        };
      } else if (path.endsWith('/integration/api-key-profiles'))
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
        data = { items: permissionCatalog };
      else if (/\/api-key-profiles\/\d+\/permissions$/.test(path)) {
        const id = Number(path.match(/profiles\/(\d+)/)![1]);
        if (req.method() === 'PUT') {
          writes.push({ kind: 'profile', id, body: req.postDataJSON() });
          if (delaySave)
            await new Promise<void>((resolve) => {
              finishSave = resolve;
            });
          if (
            legacy &&
            id === 1 &&
            req.postDataJSON().permission_ids.includes(2)
          ) {
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
        } else
          data = {
            profile_id: id,
            permission_ids: id === 1 ? (legacy ? [2] : [1]) : [2],
          };
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
    failPreview: () => {
      failPreview = true;
    },
    failCommit: () => {
      failCommit = true;
    },
    delay: () => {
      delaySave = true;
    },
    release: () => {
      finishSave?.();
    },
  };
}

async function openAdvancedBatch(page: Page) {
  await page.getByRole('tab', { name: 'API Key 管理', exact: true }).click();
  await page.getByRole('button', { name: '高级操作', exact: true }).click();
  await page
    .getByRole('menuitem', { name: '按 Key 追加权限', exact: true })
    .click();
}

for (const viewport of [
  { name: 'desktop', width: 1440, height: 1000 },
  { name: 'narrow', width: 390, height: 844 },
])
  test(`${viewport.name}: batch selection, preview retry, review and atomic save`, async ({
    page,
  }, info) => {
    await page.setViewportSize(viewport);
    const state = await prepare(page);
    await openAdvancedBatch(page);
    const dialog = page.getByRole('dialog');
    await expect(
      dialog.getByText('已选 0 个 Key', { exact: true }),
    ).toBeVisible();
    await dialog
      .getByRole('button', { name: '选择当前列表', exact: true })
      .click();
    await expect(
      dialog.getByText('已选 2 个 Key', { exact: true }),
    ).toBeVisible();
    await dialog.getByRole('button', { name: '下一步', exact: true }).click();
    await expect(
      dialog.getByText('选择当前列表', { exact: true }),
    ).toBeHidden();
    await expect(dialog.getByRole('checkbox')).toHaveCount(1);
    await dialog.getByRole('checkbox').check();
    await dialog
      .getByRole('button', { name: '1. 选择 Key', exact: true })
      .click();
    await expect(
      dialog.getByText('已选 2 个 Key', { exact: true }),
    ).toBeVisible();
    await dialog.getByRole('button', { name: '下一步', exact: true }).click();
    await expect(dialog.getByRole('checkbox')).toBeChecked();
    state.failPreview();
    await dialog.getByRole('button', { name: '预览授权', exact: true }).click();
    await expect(dialog).toContainText('电商套餐读取：当前租户尚未注册该能力');
    await expect(dialog.getByRole('checkbox')).toBeChecked();
    await dialog.getByRole('button', { name: '预览授权', exact: true }).click();
    await expect(dialog).toContainText('2 个 Key × 1 项能力');
    await expect(dialog).toContainText('WorkspaceX Key');
    await expect(dialog).toContainText('CRM Key');
    await expect(dialog).toContainText('subscription_plans');
    const box = await dialog.boundingBox();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(viewport.width);
    expect(
      await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth),
    ).toBe(true);
    await page.screenshot({
      path: info.outputPath(`batch-${viewport.name}-review.png`),
      animations: 'disabled',
    });
    await dialog
      .getByRole('button', { name: '确认保存整批授权', exact: true })
      .click();
    await expect(dialog).toContainText('已为 2 个 Key 保存独立授权');
    expect(state.writes).toHaveLength(3);
    expect(state.writes.every((write) => write.kind === 'batch')).toBe(true);
    expect(state.writes.at(-1)!.body).toEqual({
      key_ids: keys.slice(0, 2).map((key) => key.key_id),
      permission_ids: [2],
      dry_run: false,
      expected_plan_hash: 'a'.repeat(64),
    });
    await dialog.getByRole('button', { name: '完成', exact: true }).click();
    await expect(dialog).toBeHidden();
    await page.getByRole('tab', { name: 'API Key 管理', exact: true }).click();
    const row = page.getByRole('row').filter({ hasText: 'WorkspaceX Key' });
    await expect(row).toContainText('com.powerx.plugins.workspacex');
    expect(state.errors).toEqual([]);
  });
test('unknown save result requires a fresh preview and does not automatically submit again', async ({
  page,
}) => {
  const state = await prepare(page);
  await openAdvancedBatch(page);
  const dialog = page.getByRole('dialog');
  await dialog
    .getByRole('button', { name: '选择当前列表', exact: true })
    .click();
  await dialog.getByRole('button', { name: '下一步', exact: true }).click();
  await dialog.getByRole('checkbox').check();
  await dialog.getByRole('button', { name: '预览授权', exact: true }).click();
  await expect(dialog).toContainText('2 个 Key × 1 项能力');
  state.failCommit();
  await dialog
    .getByRole('button', { name: '确认保存整批授权', exact: true })
    .click();
  await expect(dialog).toContainText('保存结果尚未确认');
  expect(state.writes).toHaveLength(2);
  await expect(
    dialog.getByRole('button', { name: '确认保存整批授权', exact: true }),
  ).toHaveCount(0);
  await dialog.getByRole('button', { name: '重新预览', exact: true }).click();
  await expect(dialog).toContainText('2 个 Key × 1 项能力');
  expect(state.writes).toHaveLength(3);
  expect(state.writes.at(-1)!.body.dry_run).toBe(true);
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  await openAdvancedBatch(page);
  await expect(page.getByRole('dialog')).toContainText('已选 0 个 Key');
});

for (const viewport of [
  { name: 'desktop', width: 1440, height: 1000 },
  { name: 'narrow', width: 390, height: 844 },
]) {
  test(`${viewport.name}: one Profile selects Core and Ecommerce permissions and preserves the draft across Key management`, async ({
    page,
  }, info) => {
    await page.setViewportSize(viewport);
    const state = await prepare(page);
    const panel = page.getByRole('tabpanel', { name: '权限配置', exact: true });
    await expect(panel).toBeVisible();
    await expect(panel.getByRole('checkbox')).toHaveCount(2);
    await expect(panel).not.toContainText('共享权限');
    await expect(panel).not.toContainText('独立授权');
    await panel
      .getByRole('button', { name: '全模块清空', exact: true })
      .click();
    await panel.getByRole('button', { name: '保存权限', exact: true }).click();
    await expect
      .poll(
        () => state.writes.filter((write) => write.kind === 'profile').length,
      )
      .toBe(1);
    expect(state.writes.at(-1)!.body.permission_ids).toEqual([]);
    await panel
      .getByRole('button', { name: '全模块全选', exact: true })
      .click();
    await panel.getByRole('button', { name: '保存权限', exact: true }).click();
    await expect
      .poll(
        () => state.writes.filter((write) => write.kind === 'profile').length,
      )
      .toBe(2);
    expect([...state.writes.at(-1)!.body.permission_ids].sort()).toEqual([
      1, 2,
    ]);
    await expect(page.getByTestId('api-key-owner-conflict')).toBeHidden();
    await expect(panel).toContainText('已选 2/2 项权限');
    await panel.getByRole('textbox').fill('电商');
    await panel.getByRole('checkbox').uncheck();
    const configTab = page.getByRole('tab', { name: '权限配置', exact: true }),
      keysTab = page.getByRole('tab', { name: 'API Key 管理', exact: true });
    await keysTab.click();
    await expect(panel).toBeHidden();
    await expect(
      page.getByRole('row').filter({ hasText: 'WorkspaceX Key' }),
    ).toBeVisible();
    await configTab.click();
    await expect(panel.getByRole('checkbox')).not.toBeChecked();
    await expect(panel.getByRole('textbox')).toHaveValue('电商');
    const left = await configTab.boundingBox(),
      right = await keysTab.boundingBox();
    if (viewport.name === 'desktop') {
      expect(Math.abs(left!.x - right!.x)).toBeLessThan(2);
      expect(right!.y).toBeGreaterThan(left!.y);
    } else {
      expect(Math.abs(left!.y - right!.y)).toBeLessThan(2);
      expect(right!.x).toBeGreaterThan(left!.x);
    }
    expect(await panel.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(
      true,
    );
    await panel.scrollIntoViewIfNeeded();
    await page.screenshot({
      path: info.outputPath(`profile-${viewport.name}.png`),
      animations: 'disabled',
    });
    expect(state.writes).toHaveLength(2);
    expect(state.writes.every((write) => write.kind === 'profile')).toBe(true);
    expect(state.errors).toEqual([]);
  });
}


for (const viewport of [
  { name: 'desktop', width: 1440, height: 1000 },
  { name: 'narrow', width: 390, height: 844 },
]) {
  test(`${viewport.name}: distinguish same-name stream permissions and target one Key`, async ({ page }, info) => {
    await page.setViewportSize(viewport);
    const state = await prepare(page, false, streamPermissions);
    const panel = page.getByRole('tabpanel', { name: zh.integrationApiKeyProfile.sharedTab, exact: true });
    await expect(panel.getByText(zh.integrationApiKeyAccess.http, { exact: true })).toBeVisible();
    await expect(panel.getByText(zh.integrationApiKeyAccess.service, { exact: true })).toBeVisible();
    await expect(panel.getByRole('checkbox').first()).toBeChecked();
    await expect(panel.getByRole('checkbox').last()).not.toBeChecked();
    await panel.screenshot({ path: info.outputPath(`stream-types-${viewport.name}.png`), animations: 'disabled' });
    await page.getByRole('tab', { name: zh.integrationApiKeyProfile.keysTab, exact: true }).click();
    const row = page.getByRole('row').filter({ hasText: keys[0].name });
    await row.getByRole('button', { name: zh.integrationApiKeyProfile.appendToKey, exact: true }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('checkbox').first()).toBeChecked();
    await expect(dialog.getByRole('checkbox').nth(1)).not.toBeChecked();
    await dialog.getByRole('button', { name: zh.integrationApiKeyBatch.next, exact: true }).click();
    await expect(dialog.getByRole('checkbox')).toHaveCount(1);
    await expect(dialog).toContainText(zh.integrationApiKeyAccess.service);
    await expect(dialog).not.toContainText(zh.integrationApiKeyAccess.http);
    await expect(dialog).toContainText(streamPermissions[1].meta.core_internal_endpoint);
    const box = await dialog.boundingBox();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(viewport.width);
    expect(await dialog.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true);
    await page.screenshot({ path: info.outputPath(`stream-service-${viewport.name}.png`), animations: 'disabled' });
    expect(state.writes).toEqual([]);
    expect(state.errors).toEqual([]);
  });
}
