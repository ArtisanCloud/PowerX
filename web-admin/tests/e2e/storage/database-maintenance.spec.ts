import { test, expect, type Browser } from '@playwright/test';
import { createServer, request as proxyRequest, type ServerResponse } from 'node:http';
import { maintenanceRun, maintenanceScope, maintenanceSnapshot } from '../../unit/storage/database-maintenance-fixture';

// 真正的 Nuxt/API/SSE 浏览器链路；所有数据及事件均来自本测试，不连接日常数据库。
async function fixture(browser: Browser, baseURL: string, width: number) {
  const expiresAt = Date.now() + 3600000;
  const session = { version: 1, sessionId: 'maintenance-browser', revision: 1, status: 'active', accessToken: `header.${Buffer.from(JSON.stringify({ tid: 'maintenance-fixture', exp: Math.floor(expiresAt / 1000) })).toString('base64url')}.fixture`, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid: 'maintenance-fixture' };
  const streams = new Set<ServerResponse>();
  const server = createServer((req, res) => {
    if (req.url?.includes('/runtime/events')) {
      res.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' }); res.write(': connected\n\n'); streams.add(res); res.on('close', () => streams.delete(res)); return;
    }
    const target = new URL(req.url || '/', baseURL);
    const proxy = proxyRequest(target, { method: req.method, headers: { ...req.headers, host: target.host } }, response => { res.writeHead(response.statusCode || 502, response.headers); response.pipe(res); });
    proxy.on('error', () => { res.writeHead(502); res.end(); }); req.pipe(proxy);
  });
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
  const address = server.address(); if (!address || typeof address === 'string') throw new Error('SSE fixture unavailable');
  const origin = `http://127.0.0.1:${address.port}`;
  const context = await browser.newContext({ viewport: { width, height: 1000 }, colorScheme: width < 640 ? 'dark' : 'light', locale: 'zh-CN', storageState: { cookies: [], origins: [{ origin, localStorage: [{ name: 'powerx.session', value: JSON.stringify(session) }, { name: 'welcome_guide_completed', value: 'true' }, { name: 'powerx-color-mode', value: width < 640 ? 'dark' : 'light' }] }] } });
  const page = await context.newPage();
  const status = { scope_id: maintenanceScope, ready: true, worker_enabled: true, worker_running: true, snapshot: maintenanceSnapshot(), runs: [] as any[], active: undefined as any };
  const calls = { reads: 0, starts: [] as any[], cancels: [] as any[], loseStart: false };
  let preview = maintenanceRun();
  await page.route('**/api/v1/admin/user/auth/me/context', route => route.fulfill({ json: { code: 200, data: { is_root: true, current_tenant_uuid: 'maintenance-fixture', current_member_id: 1, user: { id: 1, email: 'fixture@example.invalid', display_name: '维护验收 Root', status: 1 }, members: [{ tenant_uuid: 'maintenance-fixture', tenant_name: '受控验收', member_id: 1, is_admin: true }] } } }));
  await page.route('**/api/v1/admin/menus**', route => route.fulfill({ json: { code: 200, data: { categories: [{ id: 'ops', title: '监控中心', children: [{ id: 'storage', title: '存储治理', path: '/monitor/storage', origin: 'core', visible: true }] }] } } }));
  await page.route('**/api/v1/admin/ops/storage/**', async route => {
    const request = route.request(), path = new URL(request.url()).pathname.split('/storage/')[1];
    if (path === 'runtime/events') return route.continue();
    let data: any;
    if (path === 'overview') data = { mode: 'observe', cleanup_enabled: false, identity: { environment: 'dev', instance_id: 'fixture-install' }, observed_at: new Date().toISOString(), expires_at: new Date(Date.now() + 300000).toISOString(), filesystem: { id: 'disk', view: 'process_filesystem', state: 'observed', total_bytes: 120 * 2 ** 30, used_bytes: 90 * 2 ** 30, available_bytes: 30 * 2 ** 30, reserved_bytes: 0 }, resources: [], coverage: 'partial', unattributed_bytes: null, limitations: [], alerts: [] };
    else if (path === 'database-maintenance') { calls.reads++; data = status; }
    else if (path === 'database-maintenance/previews') { preview = maintenanceRun(); Object.assign(preview, request.postDataJSON()); data = preview; }
    else if (path === 'database-maintenance/runs') {
      const input = request.postDataJSON(); calls.starts.push(input);
      if (calls.loseStart) return route.abort('failed');
      const run = { ...maintenanceRun('queued', input.request_id), mode: preview.mode, preview_id: input.preview_id, tables: preview.tables };
      status.runs = [run]; status.active = run; data = run;
    } else if (path?.endsWith('/cancellations')) {
      const input = request.postDataJSON(), id = path.split('/')[2]!; calls.cancels.push({ ...input, id });
      data = { ...maintenanceRun('cancelled', id), mode: preview.mode, preview_id: input.preview_id, tables: preview.tables };
      status.runs = [data]; status.active = undefined;
    } else if (path?.startsWith('database-maintenance/runs/')) {
      data = status.runs.find(row => path.endsWith(row.id));
      if (!data) return route.fulfill({ status: 404, json: { code: 404, message: 'DATABASE_MAINTENANCE_RUN_NOT_FOUND' } });
    } else return route.fulfill({ status: 503, json: { code: 503, message: 'CONTROLLED_FIXTURE_NOT_CONFIGURED' } });
    await route.fulfill({ json: { code: 200, data } });
  });
  async function choose(name: string, value: string) {
    const workspace = page.getByTestId('storage-workspace');
    if (width < 640) await workspace.locator('[data-section-select]').selectOption(value);
    else await workspace.getByRole('tab', { name, exact: true }).click();
  }
  async function open(ready = true) {
    await page.goto(`${origin}/monitor/storage`);
    await expect(page.getByTestId('storage-workspace')).toBeVisible();
    if (width < 640) { const collapse = page.getByRole('complementary').getByRole('button', { name: '收起', exact: true }).first(); const box = await collapse.boundingBox(); if (box && box.x >= 0) await collapse.click(); }
    await choose('数据库空间维护', 'databaseMaintenance');
    await expect(page.getByTestId('maintenance-preview')).toBeVisible();
    if (ready) await expect(page.getByTestId('maintenance-preview')).toBeEnabled();
  }
  return { page, context, status, calls, streams, choose, open,
    emit: () => { for (const res of streams) res.write('event: change\ndata: {}\n\n'); },
    disconnect: () => { for (const res of streams) res.end(); },
    close: async () => { await context.close(); for (const res of streams) res.end(); server.closeAllConnections(); await new Promise<void>(resolve => server.close(() => resolve())); },
  };
}

for (const width of [1440, 390, 320]) test(`native maintenance confirmation, push and preserved draft at ${width}px`, async ({ browser, baseURL }, info) => {
  const f = await fixture(browser, baseURL!, width), page = f.page;
  try {
    await f.open();
    await page.getByTestId('maintenance-mode').click(); await page.getByRole('option', { name: '物理缩容（归还磁盘）' }).click();
    await page.getByTestId('maintenance-table-capability-event-publications').uncheck();
    await f.choose('容量概览', 'overview'); await f.choose('数据库空间维护', 'databaseMaintenance');
    await expect(page.getByTestId('maintenance-table-capability-event-publications')).not.toBeChecked();
    await expect(page.getByTestId('maintenance-mode')).toContainText('物理缩容');
    await page.getByTestId('maintenance-preview').click();
    await expect(page.getByTestId('maintenance-start')).toBeDisabled();
    await page.getByTestId('maintenance-confirm').click(); await expect(page.getByTestId('maintenance-start')).toBeDisabled();
    await page.getByTestId('maintenance-confirm-lock').click();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    await page.screenshot({ path: info.outputPath(`maintenance-confirm-${width}.png`), fullPage: true });
    await page.getByTestId('maintenance-start').click();
    await expect(page.getByTestId('database-maintenance-run')).toContainText('等待执行');
    expect(f.calls.starts).toHaveLength(1); expect(f.calls.starts[0].exclusive_lock_confirmed).toBe(true);
    await expect.poll(() => f.streams.size).toBe(1);
    if (width === 1440) {
      const beforeReconnect = f.calls.reads; f.disconnect();
      await expect.poll(() => f.calls.reads).toBe(beforeReconnect + 1);
      await expect(page.getByTestId('database-maintenance-run')).toContainText('等待执行');
    }
    const reads = f.calls.reads; await page.waitForTimeout(1600); expect(f.calls.reads).toBe(reads);
    const done = { ...maintenanceRun('completed', f.calls.starts[0].request_id), tables: ['capability-invocation-traces'], version: 8, preview_id: f.calls.starts[0].preview_id };
    done.steps = done.steps.filter(row => done.tables.includes(row.table_id)); f.status.runs = [done]; f.status.active = undefined; f.status.snapshot = done.after!; f.emit();
    await expect(page.getByTestId('database-maintenance-run')).toContainText('已完成');
    await expect(page.getByTestId('database-maintenance-run')).toContainText('选定表占用变化');
    await expect.poll(() => f.calls.reads).toBe(reads + 1);
    await page.getByTestId('maintenance-history').click(); await expect(page.getByRole('dialog')).toContainText('已完成'); await page.keyboard.press('Escape'); await expect(page.getByRole('dialog')).toBeHidden();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    await page.screenshot({ path: info.outputPath(`maintenance-completed-${width}.png`), fullPage: true });
    if (width === 1440) {
      await page.evaluate(() => { const session = JSON.parse(localStorage.getItem('powerx.session')!); session.sessionId = 'replacement-fixture-session'; localStorage.setItem('powerx.session', JSON.stringify(session)); window.dispatchEvent(new CustomEvent('powerx:auth-session-changed')); });
      await expect.poll(() => f.streams.size).toBe(0);
      await expect(page.getByTestId('database-maintenance-run')).toHaveCount(0);
    } else { await page.goto(`${baseURL}/`); await expect.poll(() => f.streams.size).toBe(0); }
  } finally { await f.close(); }
});

test('lost start response survives reload and cancels the original request', async ({ browser, baseURL }) => {
  const f = await fixture(browser, baseURL!, 1440), page = f.page;
  try {
    await f.open(); f.calls.loseStart = true;
    await page.getByTestId('maintenance-preview').click(); await page.getByTestId('maintenance-confirm').click(); await page.getByTestId('maintenance-start').click();
    await expect(page.getByTestId('maintenance-start')).toBeDisabled(); await expect.poll(() => f.calls.starts.length).toBe(1);
    await expect(page.getByRole('dialog')).toContainText('核对原任务'); await page.keyboard.press('Escape');
    await f.open(false); await expect(page.getByTestId('maintenance-pending')).toBeVisible();
    await expect(page.getByTestId('maintenance-preview')).toBeDisabled();
    await page.getByTestId('maintenance-cancel-pending').click();
    await expect(page.getByTestId('database-maintenance-run')).toContainText('已取消');
    expect(f.calls.starts).toHaveLength(1); expect(f.calls.cancels).toHaveLength(1);
    expect(f.calls.cancels[0]).toMatchObject({ id: f.calls.starts[0].request_id, preview_id: f.calls.starts[0].preview_id, confirm: true });
    await expect(page.getByTestId('maintenance-pending')).toHaveCount(0);
  } finally { await f.close(); }
});
