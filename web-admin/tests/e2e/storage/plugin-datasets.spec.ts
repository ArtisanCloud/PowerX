import { test, expect } from '@playwright/test';

const dataset = 'daabd209-f4e7-499d-a6a7-9b38facd01fa';
const tenant = 'c12ad30c-f94a-4aed-8b44-348064dab78e';

for (const width of [1440, 390]) {
  test(`dataset catalog and event-driven preview at ${width}px`, async ({ browser, baseURL, request }) => {
    const expiresAt = Date.now() + 3600000;
    const payload = Buffer.from(JSON.stringify({ tid: 'snapshot-browser-fixture', exp: Math.floor(expiresAt / 1000) })).toString('base64url');
    const session = { version: 1, sessionId: 'dataset-browser-fixture', revision: 1, status: 'active', accessToken: `header.${payload}.fixture`,
      refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid: 'snapshot-browser-fixture' };
    const context = await browser.newContext({
      viewport: { width, height: 1000 }, locale: 'zh-CN', colorScheme: width === 1440 ? 'dark' : 'light',
      storageState: { cookies: [], origins: [{ origin: baseURL!, localStorage: [
        { name: 'powerx.session', value: JSON.stringify(session) },
        { name: 'welcome_guide_completed', value: 'true' },
        { name: 'nuxt-color-mode', value: width === 1440 ? 'light' : 'dark' },
      ] }] },
    });
    const page = await context.newPage();
    let receiptReads = 0, listReads = 0, complete = false, requestID = '';
    let policyEnabled = false, batchID = '', batchCompleted = false;
    const policy = () => ({ revision: policyEnabled ? 1 : 0, acceptance_revision: policyEnabled ? 1 : 0, enabled: policyEnabled,
      retention_days: 45, action: 'delete', state: policyEnabled ? 'enabled' : 'paused',
      active_request_uuid: batchID && !batchCompleted ? batchID : undefined });
    const batch = () => ({ request_uuid: batchID, state: batchCompleted ? 'committed' : 'queued',
      affected_count: batchCompleted ? 1 : undefined });
    const data = (value: unknown) => ({ json: { code: 200, data: value } });
    await page.route('**/api/v1/admin/menus**', route => route.fulfill(data({ categories: [{ id: 'ops', title: '监控中心',
      children: [{ id: 'storage', title: '存储治理', path: '/monitor/storage', origin: 'core', visible: true }] }] })));
    await page.route('**/api/v1/admin/tenants?**', route => route.fulfill(data({ items: [{ uuid: tenant, name: '隔离租户' }] })));
    await page.route('**/api/v1/admin/ops/storage/datasets?**', route => {
      listReads++;
      return route.fulfill(data({ next_cursor: null, page_physical_bytes: 1024, items: [
        { dataset_uuid: dataset, dataset_key: 'fixture_report', name: '合成报告', owner_kind: 'plugin', plugin_id: 'com.powerx.plugins.storage-fixture',
          source_state: 'ready', acceptance_revision: 1, observation: { total_bytes: 1024 }, available_actions: ['observe', 'preview', 'delete'],
          retention_bounds: { minimum_days: 7, maximum_days: 3650, suggested_days: 30 } },
      ] }));
    });
    await page.route(`**/api/v1/admin/ops/storage/datasets/${dataset}/previews`, route => {
      const input = route.request().postDataJSON();
      expect(input.tenant_uuid).toBe(tenant);
      expect(input.retention_days).toBe(45);
      requestID = input.request_uuid;
      return route.fulfill({ ...data({ request_uuid: requestID, dataset_uuid: dataset, state: 'queued', cutoff_at: '2026-10-01T00:00:00Z' }), status: 202 });
    });
    await page.route(`**/api/v1/admin/ops/storage/datasets/${dataset}/previews/*`, route => {
      receiptReads++;
      expect(route.request().url()).toContain(requestID);
      return route.fulfill(data({ request_uuid: requestID, dataset_uuid: dataset, state: complete ? 'completed' : 'queued',
        cutoff_at: '2026-10-01T00:00:00Z', result: complete ? { coverage: 'complete', candidate_count: 1,
          examined_count: 2, protected_by_reason: [{ reason: 'referenced', count: 1 }] } : undefined }));
    });
    await page.route(`**/api/v1/admin/ops/storage/datasets/${dataset}/policies/${tenant}`, route => {
      if (route.request().method() === 'PUT') {
        expect(route.request().postDataJSON()).toEqual({
          expected_revision: 0, acceptance_revision: 1, enabled: true, action: 'delete', retention_days: 45,
        });
        policyEnabled = true;
      }
      return route.fulfill(data(policy()));
    });
    await page.route(`**/api/v1/admin/ops/storage/datasets/${dataset}/runs`, route => {
      const input = route.request().postDataJSON();
      expect(input.preview_request_uuid).toBe(requestID);
      expect(input.tenant_uuid).toBe(tenant);
      expect(input.expected_policy_revision).toBe(1);
      batchID = input.request_uuid;
      return route.fulfill({ ...data(batch()), status: 202 });
    });
    await page.route(`**/api/v1/admin/ops/storage/datasets/${dataset}/runs/*`, route => {
      expect(route.request().url()).toContain(batchID);
      return route.fulfill(data(batch()));
    });
    try {
      await page.goto(`${baseURL}/monitor/storage`);
      const skip = page.getByRole('button', { name: '跳过引导', exact: true });
      if (await skip.isVisible()) await skip.click();
      const w = page.getByTestId('storage-workspace');
      await expect(w).toBeVisible();
      if (width < 640) {
        const collapse = page.getByRole('complementary').getByRole('button', { name: '收起', exact: true }).first();
        const box = await collapse.boundingBox();
        if (box && box.x >= 0) await collapse.click();
      }
      const section = (name: string, value: string) => width < 640
        ? w.locator('[data-section-select]').selectOption(value) : w.getByRole('tab', { name, exact: true }).click();
      expect(listReads).toBe(0);
      await section('数据集目录', 'datasets');
      const panel = page.getByTestId('storage-datasets');
      await expect(panel).toContainText('1 KiB');
      await panel.getByRole('button', { name: '保留预览' }).click();
      const dialog = page.getByRole('dialog');
      await dialog.getByRole('combobox').click();
      await page.getByRole('option', { name: '隔离租户' }).click();
      await dialog.getByRole('spinbutton').fill('45');
      await page.keyboard.press('Escape');
      await section('容量概览', 'overview');
      await section('数据集目录', 'datasets');
      await panel.getByRole('button', { name: '保留预览' }).click();
      await expect(dialog.getByRole('spinbutton')).toHaveValue('45');
      await dialog.getByRole('button', { name: '保留预览' }).click();
      await expect(dialog.getByText('等待执行', { exact: true })).toBeVisible();
      const before = receiptReads;
      await page.waitForTimeout(1050);
      expect(receiptReads).toBe(before);
      complete = true;
      const catalogBefore = listReads;
      const push = await page.request.post(`${baseURL}/api/v1/__snapshot_browser/storage-push?reason=dataset_preview_changed`);
      expect((await push.json()).connections).toBeGreaterThan(0);
      await expect(dialog.getByTestId('dataset-preview-counts')).toContainText('候选记录 1');
      await expect(dialog.getByText('仍被引用')).toBeVisible();
      expect(receiptReads).toBe(before + 1);
      expect(listReads).toBe(catalogBefore);
      await dialog.getByRole('button', { name: '执行策略与批次' }).click();
      const executor = page.getByTestId('dataset-execution-panel');
      await expect(executor.getByText('策略已暂停')).toBeVisible();
      await executor.getByRole('button', { name: '启用策略' }).click();
      const launch = executor.getByRole('button', { name: '运行一个批次' });
      await expect(launch).toBeDisabled();
      await executor.getByRole('checkbox', { name: '确认按当前预览派发一个删除批次' }).check();
      await launch.click();
      await expect(executor.getByText('批次等待执行')).toBeVisible();
      batchCompleted = true;
      await page.request.post(`${baseURL}/api/v1/__snapshot_browser/storage-push?reason=dataset_execution_changed`);
      await expect(executor.getByText('批次已完成')).toBeVisible();
      await expect(executor).toContainText('影响记录数 1');
      await page.keyboard.press('Escape');
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    } finally { await context.close(); }
    await expect.poll(async () => {
      const state = await request.get('/api/v1/__snapshot_browser/state');
      return (await state.json()).storageEventConnections;
    }).toBe(0);
  });
}
