import { test, expect } from '@playwright/test';
import { catalog, resources } from '../../unit/storage/fixtures';

// 真实 Nuxt 产物／真实 API client，受控 API fixtures；JWT 与只读文件系统在 Go 测试中验证。
for (const width of [1440, 390, 320]) {
  test(`read-only observation and capacity calculator at ${width}px`, async ({ browser, baseURL }, info) => {
    const expiresAt = Date.now() + 3600000;
    const session = { version: 1, sessionId: 'storage-browser-session', revision: 1, status: 'active', accessToken: `header.${Buffer.from(JSON.stringify({ tid: 'snapshot-browser-fixture', exp: Math.floor(expiresAt / 1000) })).toString('base64url')}.fixture`, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid: 'snapshot-browser-fixture' };
    const context = await browser.newContext({ viewport: { width, height: 1000 }, locale: 'zh-CN', storageState: { cookies: [], origins: [{ origin: baseURL!, localStorage: [{ name: 'powerx.session', value: JSON.stringify(session) }, { name: 'welcome_guide_completed', value: 'true' }, { name: 'nuxt-color-mode', value: width < 500 ? 'dark' : 'light' }] }] } });
    let reads = 0, historyReads = 0, policyReads = 0, fail = false;
    const identity = { host_id: 'fixture-host', filesystem_id: 'fixture-disk', instance_id: 'dev-install', environment: 'dev' };
    let policy: any = { schema_version: 1, revision: 0, state: 'draft', identity, budget: { max_managed_bytes: 0, min_free_bytes: 0, critical_free_bytes: 0 }, resources: [], updated_at: null, updated_by: 0, cleanup_enabled: false, legacy_policies_changed: false, catalog_version: 1, catalog: catalog() };
    const page = await context.newPage();
    await page.route('**/api/v1/admin/ops/storage/policy', async route => {
      if (route.request().method() === 'PUT') {
        const input = route.request().postDataJSON();
        expect(input.expected_revision).toBe(policy.revision); expect(input.state).toBe('draft'); expect(input).not.toHaveProperty('path');
        policy = { ...policy, budget: input.budget, resources: input.resources.map((r: any) => ({ retention_mode: r.retention_mode, ...r })), revision: policy.revision + 1, updated_at: new Date().toISOString(), updated_by: 1 };
      } else policyReads++;
      return route.fulfill({ json: { code: 200, data: policy } });
    });
    await page.route('**/api/v1/admin/ops/storage/history', route => {
      historyReads++;
      return route.fulfill({ json: { code: 200, data: { metric_version: 'allocated-occupancy/v1', window_days: 14, sample_count: 1, expected_samples: 4032, missing_samples: 4031, metric_changes: 0, unavailable_samples: 0, state: 'occupancy_only', rate_basis: 'ingress_not_instrumented', daily_increment_bytes: null, sampling: { configured: true, worker_enabled: true, interval_seconds: 300, metadata_retention_days: 15 }, points: [{ observed_at: new Date().toISOString(), used_bytes: Math.round(87.74 * 1024 ** 3), available_bytes: 25 * 1024 ** 3, database_bytes: 335 * 1024 ** 2, coverage: 'partial' }] } } });
    });
    await page.route('**/api/v1/admin/menus**', route => route.fulfill({ json: { code: 200, data: { categories: [{ id: 'ops', title: '监控中心', order: 1, children: [{ id: 'storage', title: '存储治理', path: '/monitor/storage', origin: 'core', visible: true }, { id: 'backup', title: '备份中心', path: '/ops/backup', origin: 'core', visible: true }] }] } } }));
    await page.route('**/api/v1/admin/ops/storage/overview', route => {
      reads++;
      if (fail) return route.fulfill({ status: 503, json: { code: 503, message: 'fixture observation unavailable' } });
      return route.fulfill({ json: { code: 200, data: { mode: 'observe', cleanup_enabled: false, identity, observed_at: new Date().toISOString(), expires_at: new Date(Date.now() + 300000).toISOString(), filesystem: { id: 'fixture-fs', view: 'process_filesystem', state: 'observed', total_bytes: Math.round(117.74 * 1024 ** 3), used_bytes: Math.round(87.74 * 1024 ** 3), available_bytes: 25 * 1024 ** 3, reserved_bytes: 5 * 1024 ** 3 }, resources: resources().map(r => r.id === 'shared-journal' ? { ...r, state: 'unavailable', physical_bytes: null, logical_bytes: null, attributed_bytes: null, problem: 'source_unreadable_or_symlink' } : r), coverage: 'partial', unattributed_bytes: 89 * 1024 ** 3, limitations: ['cleanup_not_enabled'], alerts: [{ code: 'database_partial', level: 'unknown' }], database: { state: 'partial', instance_id: 'storage-fixture', environment: 'dev', cluster_id: 'shared-fixture-pg', database_bytes: 4 * 1024 ** 3, wal_bytes: null, wal_state: 'unavailable', reusable_bytes: null, relations: [{ id: 'traces', state: 'observed', heap_bytes: 1024 ** 3, indexes_bytes: 1024 ** 2, toast_bytes: 0, total_bytes: 1024 ** 3 + 1024 ** 2 }] } } } });
    });
    await page.route('**/api/v1/admin/ops/storage/estimates', async route => {
      const body = route.request().postDataJSON();
      const resource = catalog().find(o => o.resource.id === body.resource_id)!.resource;
      const base = { basis: 'unavailable', required_bytes: null, deficit_bytes: null, supported_days: null, safety_factor: 1.3, can_enable: false, resource, retention_mode: body.retention_mode, retention_count: body.retention_count };
      if (body.retention_mode === 'count') return route.fulfill({ json: { code: 200, data: { ...base, state: 'artifact_inventory_required' } } });
      expect(body.resource_id).toBe('prod-logs'); expect(body.target_days).toBe(14); expect(body.max_bytes).toBe(3 * 1024 ** 3);
      if (body.daily_increment_bytes === undefined) return route.fulfill({ json: { code: 200, data: { ...base, state: 'sampling_required' } } });
      await route.fulfill({ json: { code: 200, data: { ...base, state: 'budget_insufficient', basis: 'manual_assumption', required_bytes: Math.ceil(5.46 * 1024 ** 3), deficit_bytes: Math.ceil(2.46 * 1024 ** 3), supported_days: 7.7 } } });
    });
    try {
      await page.goto(`${baseURL}/monitor/storage`);
      const skip = page.getByRole('button', { name: '跳过引导', exact: true }); if (await skip.isVisible()) await skip.click();
      const workspace = page.getByTestId('storage-workspace');
      await expect(workspace.getByText('本页自动清理未启用 · 应用日志支持手动批次')).toBeVisible();
      if (width < 640) {
        const collapse = page.getByRole('complementary').getByRole('button', { name: '收起', exact: true }).first();
        const box = await collapse.boundingBox();
        if (box && box.x >= 0) await collapse.click();
      }
      await expect(workspace.getByTestId('storage-powerx-summary')).toContainText('PowerX 总占用：尚未完整统计');
      await expect(workspace.getByTestId('storage-resources')).not.toContainText('共享 journal');
      await expect(workspace.getByTestId('storage-disk-metrics')).toHaveCount(0);
      await workspace.getByTestId('storage-overview-disk').click();
      await expect(workspace.getByTestId('storage-disk-metrics')).toContainText('整盘已用（所有软件与文件）');
      await expect(workspace.getByTestId('storage-resources')).toContainText('共享 journal');
      await expect(workspace.getByTestId('storage-resources')).toContainText('未知');
      await workspace.getByTestId('storage-overview-powerx').click();
      await workspace.getByTestId('storage-database-open').click();
      await expect(page.getByTestId('storage-database')).toContainText('共享 WAL');
      await expect(page.getByTestId('storage-database')).toContainText('—');
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
      await page.keyboard.press('Escape');
      const selectSection = async (name: string, value: string) => {
        if (width < 640) await workspace.locator('[data-section-select]').selectOption(value);
        else await workspace.getByRole('tab', { name, exact: true }).click();
      };
      await selectSection('资源保留策略', 'policies');
      const openDraft = async () => { await workspace.getByTestId('storage-advanced-draft').click(); await page.getByTestId('storage-resource-draft').waitFor(); };
      const changeEnvironment = async (value: string) => { await page.keyboard.press('Escape'); await workspace.getByTestId('storage-policy-environment').selectOption(value); await openDraft(); };
      const changeResource = async (id: string) => { await page.keyboard.press('Escape'); if (width < 1024) await workspace.getByTestId('storage-policy-source').selectOption(id); else await workspace.locator(`[data-resource-tab="${id}"]`).click(); await openDraft(); };
      await openDraft();
      await expect(workspace.getByTestId('storage-policy-environment')).toHaveValue('dev:dev-install');
      await expect(page.getByTestId('storage-policy-count')).toHaveValue('24');
      await expect(page.getByTestId('storage-policy-budget')).toHaveValue('1');
      await expect(page.getByTestId('storage-policy-days')).toHaveCount(0);
      await page.getByTestId('storage-host-open').click();
      await expect(page.getByTestId('storage-host-budget')).toHaveValue('');
      await page.getByTestId('storage-host-suggest').click();
      await expect(page.getByTestId('storage-host-budget')).toHaveValue('90');
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
      await page.getByTestId('storage-host-save').click();
      await expect(page.getByTestId('storage-host-panel')).toContainText('主机预算已保存并读回确认');
      expect(policy.resources).toEqual([]); await page.keyboard.press('Escape');
      await page.getByTestId('storage-policy-count').fill('30');
      await changeEnvironment('prod:prod-install');
      await expect(page.getByTestId('storage-policy-count')).toHaveValue('2');
      await expect(page.getByTestId('storage-policy-budget')).toHaveValue('6');
      await changeResource('prod-logs');
      await page.getByTestId('storage-policy-days').fill('14');
      await page.getByTestId('storage-policy-budget').fill('3');
      await page.getByTestId('storage-calculate').click();
      await page.getByTestId('storage-estimate-submit').click();
      await expect(page.getByTestId('storage-estimate')).toContainText('待采样');
      await page.getByTestId('storage-calculate').click();
      await page.getByTestId('storage-daily').fill('0.3');
      await page.getByTestId('storage-estimate-submit').click();
      await expect(page.getByTestId('storage-estimate')).toContainText('正式应用日志');
      await expect(page.getByTestId('storage-estimate')).toContainText('5.46');
      await page.keyboard.press('Escape'); await selectSection('容量概览', 'overview'); await selectSection('资源保留策略', 'policies'); await openDraft();
      await expect(page.getByTestId('storage-policy-days')).toHaveValue('14');
      await changeEnvironment('dev:dev-install');
      await expect(page.getByTestId('storage-policy-count')).toHaveValue('30');
      await page.getByTestId('storage-policy-save').click();
      await expect(page.getByTestId('storage-resource-draft')).toContainText('其他未保存资源未提交');
      expect(policyReads).toBe(3); expect(policy.resources).toHaveLength(1);
      expect(policy.resources[0]).toMatchObject({ resource_id: 'dev-backup', retention_mode: 'count', target_days: 0, retention_count: 30 });
      expect(policy.cleanup_enabled).toBe(false);
      await changeEnvironment('prod:prod-install');
      await changeResource('prod-logs');
      await expect(page.getByTestId('storage-policy-days')).toHaveValue('14');
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
      await page.screenshot({ path: info.outputPath(`storage-${width}.png`), fullPage: true, animations: 'disabled' });
      await page.keyboard.press('Escape');
      await selectSection('用量历史', 'history');
      await expect(workspace.getByTestId('storage-history-table')).toContainText('335 MiB');
      await expect(workspace.getByTestId('storage-history-table')).not.toContainText('87.74 GiB');
      await expect(workspace.getByTestId('storage-history-scope')).toContainText('不是 PowerX 总占用');
      await expect(workspace.getByTestId('storage-sampling-status')).toContainText('已配置每 5 分钟');
      await page.screenshot({ path: info.outputPath(`storage-powerx-history-${width}.png`), fullPage: true, animations: 'disabled' });
      await workspace.getByTestId('storage-history-disk').click();
      await expect(workspace.getByTestId('storage-history-table')).toContainText('87.74 GiB');
      await expect(workspace.getByTestId('storage-history-table')).not.toContainText('335 MiB');
      await expect(workspace.getByTestId('storage-history-scope')).toContainText('不代表 PowerX 占用');
      await workspace.getByTestId('storage-sampling-details').locator('summary').click();
      await expect(workspace.getByTestId('storage-sampling-details')).toContainText('4031');
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
      await page.screenshot({ path: info.outputPath(`storage-disk-history-${width}.png`), fullPage: true, animations: 'disabled' });
      await selectSection('容量概览', 'overview'); await selectSection('用量历史', 'history');
      await expect(workspace.getByTestId('storage-history-disk')).toHaveAttribute('aria-pressed', 'true');
      expect(historyReads).toBe(1);
      const before = reads; await page.waitForTimeout(1500); expect(reads).toBe(before); expect(historyReads).toBe(1);
      await selectSection('容量概览', 'overview'); fail = true;
      await workspace.getByRole('button', { name: '刷新观测', exact: true }).click();
      await expect(workspace.getByTestId('storage-error')).toBeVisible();
      await expect(workspace.getByTestId('storage-resources')).toContainText('开发备份');
    } finally { await context.close(); }
  });
}
