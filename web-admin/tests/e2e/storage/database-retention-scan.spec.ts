import { test, expect } from '@playwright/test';

// 真实 Nuxt/API client 的受控页面验收；原生事务、窗口和权限在 PostgreSQL/Go 中验证。
for (const width of [1440, 390, 320]) {
  test(`bounded database preview, timeout and preserved context at ${width}px`, async ({ browser, baseURL }, info) => {
    const expiresAt = Date.now() + 3600000;
    const session = { version: 1, sessionId: 'retention-window-browser', revision: 1, status: 'active', accessToken: `header.${Buffer.from(JSON.stringify({ tid: 'snapshot-browser-fixture', exp: Math.floor(expiresAt / 1000) })).toString('base64url')}.fixture`, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid: 'snapshot-browser-fixture' };
    const context = await browser.newContext({ viewport: { width, height: 1000 }, locale: 'zh-CN', storageState: { cookies: [], origins: [{ origin: baseURL!, localStorage: [{ name: 'powerx.session', value: JSON.stringify(session) }, { name: 'welcome_guide_completed', value: 'true' }] }] } });
    const page = await context.newPage();
    let reads = 0, previews = 0, failPreview = false;
    const scan = { window: { before: { traces: 0, events: 0 }, end: { traces: 2048, events: 2048 }, highwater: { traces: 5000000, events: 5000000 } }, next: { traces: 2048, events: 2048 }, complete: false };
    const policy = { revision: 2, enabled: true, owner_instance: 'fixture-install', success_days: 30, failure_days: 90, pending_hours: 24, quarantine_days: 30, batch_limit: 200, max_bytes: 8 * 2 ** 30, adopt_legacy: false, reject_new_when_full: false };
    const status = { scan, configuration: { authority: 'database', state: 'applied', ready: true, execution_ready: true, desired_revision: 2, applied_revision: 2, snapshot_revision: 2, legacy_digest: 'a'.repeat(64), effective_capacity: policy }, state_configured: true, enforcement_allowed: true, migration_ready: true, worker_enabled: true, ready: true, policy, runs: [], relation_bytes: 12 * 2 ** 30 };
    await page.route('**/api/v1/admin/menus**', route => route.fulfill({ json: { code: 200, data: { categories: [{ id: 'ops', title: '监控中心', children: [{ id: 'storage', title: '存储治理', path: '/monitor/storage', origin: 'core', visible: true }] }] } } }));
    await page.route('**/api/v1/admin/ops/storage/**', async route => {
      const path = new URL(route.request().url()).pathname.split('/storage/')[1];
      let data: any;
      if (path === 'overview') data = { mode: 'observe', cleanup_enabled: false, identity: { environment: 'dev', instance_id: 'fixture-install' }, observed_at: new Date().toISOString(), expires_at: new Date(Date.now() + 300000).toISOString(), filesystem: { id: 'disk', view: 'process_filesystem', state: 'observed', total_bytes: 120 * 2 ** 30, used_bytes: 90 * 2 ** 30, available_bytes: 30 * 2 ** 30, reserved_bytes: 0 }, resources: [], coverage: 'partial', unattributed_bytes: null, limitations: [], alerts: [] };
      else if (path === 'database-retention') { reads++; data = status; }
      else if (path === 'database-retention/previews') {
        previews++;
        if (failPreview) return route.fulfill({ status: 503, json: { code: 503, message: 'DATABASE_RETENTION_QUERY_TIMEOUT', error: 'DATABASE_RETENTION_QUERY_TIMEOUT' } });
        data = { id: crypto.randomUUID(), revision: 2, state: 'preview', actor: 1, at: new Date().toISOString(), expires_at: new Date(Date.now() + 300000).toISOString(), counts: { traces: 0, events: 0, quarantined: 0, classified: 0 }, policy_snapshot: policy, scan };
      } else return route.continue();
      await route.fulfill({ json: { code: 200, data } });
    });
    try {
      await page.goto(`${baseURL}/monitor/storage`);
      const workspace = page.getByTestId('storage-workspace'); await expect(workspace).toBeVisible();
      if (width < 640) { const collapse = page.getByRole('complementary').getByRole('button', { name: '收起', exact: true }).first(); const box = await collapse.boundingBox(); if (box && box.x >= 0) await collapse.click(); }
      async function choose(name: string, value: string) { if (width < 640) await workspace.locator('[data-section-select]').selectOption(value); else await workspace.getByRole('tab', { name, exact: true }).click(); }
      await choose('数据库期限', 'databaseRetention');
      const panel = page.getByTestId('database-retention');
      await expect(panel.getByTestId('db-retention-scan-status')).toContainText('已保存进度');
      await panel.getByTestId('db-retention-preview').click();
      await expect(page.getByTestId('db-retention-scan-hint')).toContainText('本批为 0 不代表全库没有候选');
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
      await page.screenshot({ path: info.outputPath(`retention-preview-${width}.png`), fullPage: true });
      await page.keyboard.press('Escape');
      await panel.getByTestId('db-retention-edit').click();
      const input = page.locator('#database-retention-form input[type="number"]').first(); await input.fill('31');
      await page.keyboard.press('Escape'); await choose('容量概览', 'overview'); await choose('数据库期限', 'databaseRetention');
      await panel.getByTestId('db-retention-edit').click(); await expect(input).toHaveValue('31'); await page.keyboard.press('Escape');
      failPreview = true; await panel.getByTestId('db-retention-preview').click();
      await expect(panel).toContainText('候选检查超时'); await expect(panel).not.toContainText('观测存储未就绪');
      await expect(panel.getByTestId('db-retention-scan-status')).toContainText('已保存进度');
      const stableReads = reads; await page.waitForTimeout(1300); expect(reads).toBe(stableReads); expect(previews).toBe(2);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
      await page.screenshot({ path: info.outputPath(`retention-timeout-${width}.png`), fullPage: true });
    } finally { await context.close(); }
  });
}
