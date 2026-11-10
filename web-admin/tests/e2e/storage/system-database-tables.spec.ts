import { test, expect } from '@playwright/test';
import { tableInventory } from '../../unit/storage/database-tables-fixture';

// 真实 Nuxt 生产产物、受控 HTTP 数据；真实 PG 边界与 Root JWT 由 Go 验收覆盖。
for (const width of [1440, 390, 320]) {
  test(`system table inventory and preserved context at ${width}px`, async ({ browser, baseURL }, info) => {
    const expiresAt = Date.now() + 3600000;
    const session = { version: 1, sessionId: 'table-inventory-browser', revision: 1, status: 'active', accessToken: `header.${Buffer.from(JSON.stringify({ tid: 'snapshot-browser-fixture', exp: Math.floor(expiresAt / 1000) })).toString('base64url')}.fixture`, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid: 'snapshot-browser-fixture' };
    const context = await browser.newContext({ viewport: { width, height: 1000 }, locale: 'zh-CN', storageState: { cookies: [], origins: [{ origin: baseURL!, localStorage: [{ name: 'powerx.session', value: JSON.stringify(session) }, { name: 'welcome_guide_completed', value: 'true' }, { name: 'nuxt-color-mode', value: width === 1440 ? 'light' : 'dark' }] }] } });
    const page = await context.newPage(); let reads = 0, detailReads = 0, fail = false, writes = 0;
    await page.route('**/api/v1/admin/menus**', route => route.fulfill({ json: { code: 200, data: { categories: [{ id: 'ops', title: '监控中心', children: [{ id: 'storage', title: '存储治理', path: '/monitor/storage', origin: 'core', visible: true }] }] } } }));
    await page.route('**/api/v1/admin/ops/storage/**', async route => {
      const path = new URL(route.request().url()).pathname.split('/storage/')[1];
      if (path === 'runtime/events') return route.continue();
      if (route.request().method() !== 'GET') writes++;
      let data: any;
      if (path === 'overview') data = { mode: 'observe', cleanup_enabled: false, identity: { environment: 'dev', instance_id: 'fixture-install' }, observed_at: new Date().toISOString(), expires_at: new Date(Date.now() + 300000).toISOString(), filesystem: { id: 'disk', view: 'process_filesystem', state: 'observed', total_bytes: 120 * 1024 ** 3, used_bytes: 70 * 1024 ** 3, available_bytes: 30 * 1024 ** 3, reserved_bytes: 0 }, resources: [], coverage: 'partial', unattributed_bytes: null, limitations: [], alerts: [] };
      else if (path === 'database-tables') { reads++; if (fail) return route.fulfill({ status: 503, json: { code: 503, message: 'fixture table observation unavailable' } }); data = tableInventory(); }
      else if (path === 'database-tables/capability_registry_invocation_traces') { detailReads++; data = tableInventory(); data.items = data.items.slice(0, 1); }
      else throw new Error(`Unexpected storage request ${path}`);
      await route.fulfill({ json: { code: 200, data } });
    });
    try {
      await page.goto(`${baseURL}/monitor/storage`);
      const skip = page.getByRole('button', { name: '跳过引导', exact: true }); if (await skip.isVisible()) await skip.click();
      const w = page.getByTestId('storage-workspace'); await expect(w).toBeVisible();
      if (width < 640) { const collapse = page.getByRole('complementary').getByRole('button', { name: '收起', exact: true }).first(); const box = await collapse.boundingBox(); if (box && box.x >= 0) await collapse.click(); }
      async function section(name: string, value: string) { if (width < 640) await w.locator('[data-section-select]').selectOption(value); else await w.getByRole('tab', { name, exact: true }).click(); }
      expect(reads).toBe(0);
      await section('系统表清单', 'databaseTables');
      const panel = w.getByTestId('system-database-tables'); await expect(panel.getByTestId('system-tables-list')).toContainText('能力调用追踪');
      await expect(panel).toContainText('1 GiB'); await expect(panel).toContainText('2026');
      await expect(panel).toContainText('当前数据库未建表');
      if (width >= 640) await expect(w.getByRole('tab', { name: '系统表清单', exact: true })).toHaveAttribute('aria-selected', 'true');
      if (width < 640) await panel.getByTestId('system-tables-scroll').scrollIntoViewIfNeeded();
      await page.screenshot({ path: info.outputPath(`tables-${width}.png`), fullPage: true });
      await panel.getByTestId('system-tables-category').selectOption('protected');
      await panel.getByTestId('system-tables-search').fill('iam_authorization');
      await expect(panel.locator('tbody tr')).toHaveCount(1); await expect(panel).toContainText('受保护');
      const beforeSwitch = reads;
      await section('容量概览', 'overview'); await section('系统表清单', 'databaseTables');
      await expect(panel.getByTestId('system-tables-search')).toHaveValue('iam_authorization');
      expect(reads).toBe(beforeSwitch);
      await panel.getByTestId('system-tables-category').selectOption('all');
      await panel.getByTestId('system-tables-search').fill('capability_registry_invocation_traces');
      await panel.getByRole('button', { name: '查看 capability_registry_invocation_traces 详情' }).click();
      const detail = page.getByTestId('system-table-detail'); await expect(detail).toContainText('4 KiB'); await expect(detail).toContainText('created_at');
      await page.getByRole('button', { name: '重新采集此表', exact: true }).click(); await expect.poll(() => detailReads).toBe(1);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
      await page.screenshot({ path: info.outputPath(`table-detail-${width}.png`), fullPage: true });
      await page.keyboard.press('Escape');
      const push = await page.request.post(`${baseURL}/api/v1/__snapshot_browser/storage-push`); expect((await push.json()).connections).toBeGreaterThan(0);
      await expect.poll(() => reads).toBeGreaterThan(beforeSwitch);
      await expect(panel.getByTestId('system-tables-search')).toHaveValue('capability_registry_invocation_traces');
      const stable = reads; await page.waitForTimeout(1200); expect(reads).toBe(stable);
      fail = true; await panel.getByRole('button', { name: '刷新观测', exact: true }).click();
      await expect(panel.getByTestId('system-tables-error')).toBeVisible(); await expect(panel.locator('tbody')).toContainText('capability_registry_invocation_traces');
      expect(writes).toBe(0); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    } finally { await context.close(); }
  });
}
