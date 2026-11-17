import { test, expect } from '@playwright/test';

// 真实 Nuxt 页面和 API client；受控 Root/来源数据，不连接线上或本地日常数据库。
for (const width of [1440, 390]) test(`recovery archive suggestion stays a retained draft at ${width}px`, async ({ browser, baseURL }, info) => {
  const expiresAt = Date.now() + 3600000;
  const session = { version: 1, sessionId: 'recovery-policy-fixture', revision: 1, status: 'active', accessToken: `header.${Buffer.from(JSON.stringify({ tid: 'recovery-fixture', exp: Math.floor(expiresAt / 1000) })).toString('base64url')}.fixture`, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid: 'recovery-fixture' };
  const context = await browser.newContext({ viewport: { width, height: 1000 }, locale: 'zh-CN', colorScheme: width < 640 ? 'dark' : 'light', storageState: { cookies: [], origins: [{ origin: baseURL!, localStorage: [{ name: 'powerx.session', value: JSON.stringify(session) }, { name: 'welcome_guide_completed', value: 'true' }] }] } });
  const page = await context.newPage();
  let writes = 0;
  const configuration = { authority: 'database', ready: true, state: 'applied', desired_revision: 1, applied_revision: 1, snapshot_revision: 1, legacy_digest: 'a'.repeat(64), legacy_count: 0, policies: [] };
  await page.route('**/api/v1/**', route => route.fulfill({ status: 503, json: { code: 503, message: 'CONTROLLED_FIXTURE_NOT_CONFIGURED' } }));
  await page.route('**/api/v1/admin/user/auth/me/context', route => route.fulfill({ json: { code: 200, data: { is_root: true, current_tenant_uuid: 'recovery-fixture', current_member_id: 1, user: { id: 1, email: 'fixture@example.invalid', display_name: '归档验收 Root', status: 1 }, members: [{ tenant_uuid: 'recovery-fixture', tenant_name: '受控验收', member_id: 1, is_admin: true }] } } }));
  await page.route('**/api/v1/admin/menus**', route => route.fulfill({ json: { code: 200, data: { categories: [{ id: 'monitor', title: '监控中心', children: [{ id: 'storage', title: '存储治理', path: '/monitor/storage', origin: 'core', visible: true }] }] } } }));
  await page.route('**/api/v1/admin/ops/storage/overview', route => route.fulfill({ json: { code: 200, data: { mode: 'observe', cleanup_enabled: false, identity: { environment: 'dev', instance_id: 'recovery-fixture' }, observed_at: new Date().toISOString(), expires_at: new Date(Date.now() + 300000).toISOString(), filesystem: { id: 'disk', view: 'process_filesystem', state: 'observed', total_bytes: 128 * 2 ** 30, used_bytes: 64 * 2 ** 30, available_bytes: 60 * 2 ** 30, reserved_bytes: 0 }, resources: [], alerts: [], coverage: 'partial', unattributed_bytes: null, limitations: [] } } }));
  await page.route('**/api/v1/admin/ops/backup/archives**', route => {
    if (route.request().method() !== 'GET') { writes++; return route.fulfill({ status: 409, json: { code: 409, message: 'NO_WRITES_EXPECTED' } }); }
    const path = new URL(route.request().url()).pathname;
    const data = path.endsWith('/catalog') ? { items: [], pagination: { total: 0, page: 1, pages: 1 } } : { overview: { instance: 'fixture', sources: [{ id: 'audit', name: '普通审计日志', kind: 'audit_logs', ready: true }, { id: 'recovery', name: 'dev 空消费恢复副本', kind: 'database_files', ready: true, audit_recovery_database: 'powerx_dev' }], policies: [], targets: [], credentials: [], s3_credentials: [], pending_transfers: 0, jobs: [], restores: [], policy_configuration: configuration } };
    return route.fulfill({ json: { code: 200, data } });
  });
  try {
    await page.goto('/monitor/storage');
    const workspace = page.getByTestId('storage-workspace');
    await expect(workspace).toBeVisible();
    if (width < 640) {
      const collapse = page.getByRole('complementary').getByRole('button', { name: '收起', exact: true }).first();
      const box = await collapse.boundingBox(); if (box && box.x >= 0) await collapse.click();
      await workspace.locator('[data-section-select]').selectOption('archives');
    } else await workspace.getByRole('tab', { name: '日志归档与追溯', exact: true }).click();
    const archive = workspace.locator('[data-section="archives"]');
    if (width < 640) await archive.locator('[data-section-select]').selectOption('policies');
    else await archive.getByRole('tab', { name: '归档策略', exact: true }).click();
    await archive.getByTestId('policy-new').click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await expect(page.getByTestId('audit-recovery-policy-hint')).toHaveCount(0);
    await page.getByTestId('policy-field-source_id').click();
    await page.getByRole('option', { name: 'dev 空消费恢复副本', exact: true }).click();
    await expect(page.getByTestId('audit-recovery-policy-hint')).toContainText('原始来源及操作机副本不会被删除');
    await page.getByTestId('policy-field-name').fill('dev 恢复副本保留');
    await page.getByTestId('audit-recovery-policy-defaults').click();
    await expect(page.getByTestId('policy-field-retention_days')).toHaveValue('30');
    await expect(page.getByTestId('policy-field-remote_retention_days')).toHaveValue('30');
    expect(writes).toBe(0);
    await page.screenshot({ path: info.outputPath(`audit-recovery-policy-${width}.png`), fullPage: true });
    await page.keyboard.press('Escape'); await expect(dialog).toBeHidden();
    if (width < 640) {
      await workspace.locator('[data-section-select]').first().selectOption('overview');
      await workspace.locator('[data-section-select]').first().selectOption('archives');
    } else {
      await workspace.getByRole('tab', { name: '容量概览', exact: true }).click();
      await workspace.getByRole('tab', { name: '日志归档与追溯', exact: true }).click();
    }
    await archive.getByTestId('policy-new').click();
    await expect(page.getByTestId('policy-field-name')).toHaveValue('dev 恢复副本保留');
    await expect(page.getByTestId('policy-field-retention_days')).toHaveValue('30');
    await expect(page.getByTestId('policy-field-remote_retention_days')).toHaveValue('30');
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    await page.getByTestId('policy-field-source_id').click();
    await page.getByRole('option', { name: '普通审计日志', exact: true }).click();
    await expect(page.getByTestId('audit-recovery-policy-hint')).toHaveCount(0);
    await page.waitForTimeout(1200); expect(writes).toBe(0);
  } finally { await context.close(); }
});
