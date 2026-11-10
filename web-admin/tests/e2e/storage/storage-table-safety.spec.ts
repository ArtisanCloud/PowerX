import { test, expect } from '@playwright/test';
import { tableInventory } from '../../unit/storage/database-tables-fixture';

// 生产 Nuxt 产物与真实 SSE；HTTP 数据为受控夹具，不据此宣称日常实例或正式站验收。
for (const width of [1440, 390, 320]) test(`table safety controls preserve context at ${width}px`, async ({ browser, baseURL }, info) => {
  const expiresAt = Date.now() + 3600000;
  const session = { version: 1, sessionId: 'storage-safety-browser', revision: 1, status: 'active', accessToken: `header.${Buffer.from(JSON.stringify({ tid: 'snapshot-browser-fixture', exp: Math.floor(expiresAt / 1000) })).toString('base64url')}.fixture`, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid: 'snapshot-browser-fixture' };
  const context = await browser.newContext({ viewport: { width, height: 1000 }, locale: 'zh-CN', storageState: { cookies: [], origins: [{ origin: baseURL!, localStorage: [{ name: 'powerx.session', value: JSON.stringify(session) }, { name: 'welcome_guide_completed', value: 'true' }, { name: 'nuxt-color-mode', value: width === 1440 ? 'light' : 'dark' }] }] } });
  const page = await context.newPage(), inventory = tableInventory(), id = 'capability_registry_health_probe_results';
  inventory.items.push({ ...inventory.items[0]!, id, cleanup_mechanism: 'typed_policy', time_column: 'probe_window_start', protection: 'keep_latest_probe' });
  const monitor = inventory.monitoring; monitor.state = 'observed'; monitor.policy.enabled = true; monitor.policy.revision = 7; monitor.sampled_at = '2026-11-04T01:00:00Z';
  monitor.alerts = [{ code: 'table_growth_abnormal', level: 'warning', resource_id: inventory.items[0]!.id }];
  monitor.tables = [{ id: inventory.items[0]!.id, state: 'observed', bytes: inventory.items[0]!.total_bytes, observed_at: monitor.sampled_at, baseline_at: '2026-11-04T00:45:00Z', net_change_bytes: 512 * 2 ** 20, level: 'warning' }];
  let reads = 0, historyReads = 0, policyReads = 0, monitorSaves = 0, cleanupSaves = 0, previews = 0, runs = 0;
  let retention = { id, revision: 0, policy: { enabled: false, retention_days: 30, batch_limit: 200 }, state_configured: true, enforcement_allowed: true, worker_enabled: true, ready: true, runs: [] as any[] };
  await page.route('**/api/v1/admin/menus**', route => route.fulfill({ json: { code: 200, data: { categories: [{ id: 'ops', title: '监控中心', children: [{ id: 'storage', title: '存储治理', path: '/monitor/storage', origin: 'core', visible: true }] }] } } }));
  await page.route('**/api/v1/admin/ops/storage/**', async route => {
    const path = new URL(route.request().url()).pathname.split('/storage/')[1], method = route.request().method();
    if (path === 'runtime/events') return route.continue();
    let data: any;
    if (path === 'overview') data = { mode: 'observe', cleanup_enabled: false, identity: { environment: 'test', instance_id: 'fixture-install' }, observed_at: new Date().toISOString(), expires_at: new Date(Date.now() + 300000).toISOString(), filesystem: { id: 'disk', view: 'process_filesystem', state: 'observed', total_bytes: 120 * 2 ** 30, used_bytes: 70 * 2 ** 30, available_bytes: 30 * 2 ** 30, reserved_bytes: 0 }, resources: [], coverage: 'partial', unattributed_bytes: null, limitations: [], alerts: monitor.alerts };
    else if (path === 'database-tables') { reads++; data = inventory; }
    else if (path === 'database-tables/monitoring' && method === 'PUT') { monitorSaves++; const body = route.request().postDataJSON(); expect(body.expected_revision).toBe(7); expect(body.confirm).toBe(true); expect(body.policy.growth_bytes).toBe(128 * 2 ** 20); monitor.policy = { ...body.policy, revision: 8 }; data = monitor.policy; }
    else if (path?.endsWith('/history')) { historyReads++; data = { id: path.split('/')[1], window_hours: 24, metric: 'allocated_bytes_net_change_not_ingress', points: [{ observed_at: '2026-11-04T00:45:00Z', state: 'observed', bytes: 512 * 2 ** 20 }, { observed_at: '2026-11-04T01:00:00Z', state: 'observed', bytes: 2 ** 30 }] }; }
    else if (path === `database-tables/${id}/retention` && method === 'GET') { policyReads++; data = retention; }
    else if (path === `database-tables/${id}/retention` && method === 'PUT') { cleanupSaves++; const body = route.request().postDataJSON(); expect(body.expected_revision).toBe(0); expect(body.confirm).toBe(true); retention = { ...retention, revision: 1, policy: body.policy }; data = retention; }
    else if (path === `database-tables/${id}/retention/previews`) { previews++; data = { id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', table_id: id, revision: 1, state: 'preview', rows: 2, at: '2026-11-04T01:00:00Z', cutoff: '2026-10-21T01:00:00Z', actor: 1, policy_snapshot: retention.policy }; }
    else if (path === `database-tables/${id}/retention/runs`) { runs++; const body = route.request().postDataJSON(); expect(body.preview_id).toBe('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'); expect(body.confirm).toBe(true); retention.runs = [{ id: body.request_id, table_id: id, revision: 1, state: 'unconfirmed', rows: 0, at: '2026-11-04T01:00:00Z', cutoff: '2026-10-21T01:00:00Z', actor: 1, policy_snapshot: retention.policy }]; return route.fulfill({ status: 503, json: { code: 503, message: 'fixture response lost' } }); }
    else throw new Error(`Unexpected storage request ${method} ${path}`);
    await route.fulfill({ json: { code: 200, data } });
  });
  async function push() { const response = await page.request.post(`${baseURL}/api/v1/__snapshot_browser/storage-push`); expect((await response.json()).connections).toBeGreaterThan(0); }
  async function choose(container: any, label: string, value: string) { if (width < 640) await container.locator('[data-section-select]').selectOption(value); else await container.getByRole('tab', { name: label, exact: true }).click(); }
  try {
    await page.goto(`${baseURL}/monitor/storage`);
    const skip = page.getByRole('button', { name: '跳过引导', exact: true }); if (await skip.isVisible()) await skip.click();
    const workspace = page.getByTestId('storage-workspace'); await expect(workspace).toBeVisible();
    if (width < 640) { const button = page.getByRole('complementary').getByRole('button', { name: '收起', exact: true }).first(), box = await button.boundingBox(); if (box && box.x >= 0) await button.click(); }
    await expect(workspace).toContainText('存储监控或清理状态需要处理');
    await choose(workspace, '系统表清单', 'databaseTables'); const panel = page.getByTestId('system-database-tables'); await expect(panel).toContainText('净变化 +512 MiB');
    expect(policyReads).toBe(0); expect(cleanupSaves + runs + previews + monitorSaves).toBe(0);
    await panel.getByRole('button', { name: '监控与告警', exact: true }).click(); let dialog = page.getByRole('dialog'); await choose(dialog, '告警阈值', 'settings');
    await page.getByTestId('table-growth-mib').fill('128'); const beforePush = reads; await push(); await expect.poll(() => reads).toBeGreaterThan(beforePush); await expect(page.getByTestId('table-growth-mib')).toHaveValue('128');
    await choose(dialog, '运行状态', 'status'); await choose(dialog, '告警阈值', 'settings'); await expect(page.getByTestId('table-growth-mib')).toHaveValue('128');
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true); await page.screenshot({ path: info.outputPath(`monitor-${width}.png`), fullPage: true });
    await dialog.getByRole('button', { name: '确认并保存', exact: true }).click(); await expect.poll(() => monitorSaves).toBe(1); await expect(dialog).not.toBeVisible();
    await panel.getByTestId('system-tables-search').fill(id); await panel.getByRole('button', { name: `查看 ${id} 详情` }).click(); dialog = page.getByRole('dialog');
    await choose(dialog, '24 小时历史', 'history'); await expect(page.getByTestId('table-history')).toContainText('512 MiB'); expect(historyReads).toBe(1);
    await choose(dialog, '安全清理', 'cleanup'); const control = page.getByTestId('typed-table-retention'); await expect(control).toContainText('保留每个适配器'); await expect.poll(() => policyReads).toBe(1);
    await control.locator('input[type="number"]').first().fill('14'); await control.locator('input[type="checkbox"]').first().check();
    await choose(dialog, '合计占用', 'capacity'); await choose(dialog, '安全清理', 'cleanup'); await expect(control.locator('input[type="number"]').first()).toHaveValue('14');
    const priorPolicy = policyReads; await push(); await expect.poll(() => policyReads).toBeGreaterThan(priorPolicy); await expect(control.locator('input[type="number"]').first()).toHaveValue('14');
    await control.getByRole('button', { name: '确认并保存规则', exact: true }).click(); await expect.poll(() => cleanupSaves).toBe(1);
    await control.getByRole('button', { name: '预览可清理记录', exact: true }).click(); await expect(control.getByTestId('typed-cleanup-preview')).toContainText('2 行');
    const execute = control.getByRole('button', { name: '确认删除本批', exact: true }); await expect(execute).toBeDisabled(); await control.locator('input[type="checkbox"]').nth(1).check(); await execute.click(); await expect.poll(() => runs).toBe(1); await expect(control).toContainText('已阻止自动重试'); await expect(execute).toBeDisabled(); await expect(page.getByTestId('typed-cleanup-unconfirmed')).toBeInViewport();
    await push(); await page.waitForTimeout(1200); const stable = { reads, historyReads, policyReads, runs }; await page.waitForTimeout(1500); expect({ reads, historyReads, policyReads, runs }).toEqual(stable);
    expect(previews).toBe(1); expect(runs).toBe(1); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    await page.screenshot({ path: info.outputPath(`cleanup-unknown-${width}.png`), fullPage: true });
  } finally { await context.close(); }
});
