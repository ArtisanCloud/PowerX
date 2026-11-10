import { test, expect } from '@playwright/test';
for (const width of [1440, 390, 320]) {
  test(`automatic storage policy, push and preserved edits at ${width}px`, async ({ browser, baseURL }, info) => {
    const GiB = 1024 ** 3, expiresAt = Date.now() + 3600000;
    const identity = { host_id: 'host', filesystem_id: 'disk', instance_id: 'installed', environment: 'dev' };
    const session = { version: 1, sessionId: 'storage-runtime-browser', revision: 1, status: 'active', accessToken: `header.${Buffer.from(JSON.stringify({ tid: 'snapshot-browser-fixture', exp: Math.floor(expiresAt / 1000) })).toString('base64url')}.fixture`, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid: 'snapshot-browser-fixture' };
    const context = await browser.newContext({ viewport: { width, height: 1000 }, locale: 'zh-CN', storageState: { cookies: [], origins: [{ origin: baseURL!, localStorage: [{ name: 'powerx.session', value: JSON.stringify(session) }, { name: 'welcome_guide_completed', value: 'true' }] }] } });
    const page = await context.newPage(); let runtimeReads = 0, executes = 0, saves = 0;
    const state: any = { configured: true, worker_enabled: true, admission_enabled: true, notifications_configured: false, database_cleanup_enabled: false, budget_policy: { revision: 1, min_free_bytes: 20 * GiB, critical_free_bytes: 10 * GiB, max_used_bytes: 90 * GiB, max_concurrent: 1 }, budget: { available_bytes: 30 * GiB, used_bytes: 70 * GiB, reserved_bytes: 0, min_free_bytes: 20 * GiB, critical_free_bytes: 10 * GiB, active: [] }, lifecycle: { configured: true, allow_enforce: true, revision: 1, database_cleanup_enabled: false, sources: [{ id: 'logs', name: '开发文件日志', kind: 'application_logs', scope: 'dev', instance_id: 'installed', format: 'numeric', supported: true, require_archive: false }], policies: [], runs: [] } };
    await page.route('**/api/v1/admin/menus**', route => route.fulfill({ json: { code: 200, data: { categories: [{ id: 'ops', title: '监控中心', children: [{ id: 'storage', title: '存储治理', path: '/monitor/storage', origin: 'core', visible: true }] }] } } }));
    await page.route('**/api/v1/admin/ops/storage/**', async route => {
      const path = new URL(route.request().url()).pathname.split('/storage/')[1]; if (path === 'runtime/events') return route.continue(); let data: any;
      if (path === 'overview') data = { mode: state.lifecycle.policies[0]?.enabled ? 'enforce' : 'observe', cleanup_enabled: !!state.lifecycle.policies[0]?.enabled, identity, observed_at: new Date().toISOString(), expires_at: new Date(Date.now() + 300000).toISOString(), filesystem: { id: 'disk', view: 'process_filesystem', state: 'observed', total_bytes: 120 * GiB, used_bytes: 70 * GiB, available_bytes: 30 * GiB, reserved_bytes: 0 }, resources: [], coverage: 'partial', unattributed_bytes: null, limitations: [], alerts: [] };
      else if (path === 'runtime') { runtimeReads++; data = state; }
      else if (path === 'runtime/policies/logs') { const input = route.request().postDataJSON(); expect(input.confirm).toBe(true); expect(input.expected_revision).toBe(0); saves++; state.lifecycle.policies = [{ source_id: 'logs', revision: 1, enabled: input.enabled, retention_days: input.retention_days, max_bytes: input.max_bytes, require_archive: input.require_archive, owner_instance: 'installed', operator_id: 1, updated_at: new Date().toISOString() }]; data = state.lifecycle.policies[0]; }
      else if (path === 'runtime/previews') data = { source_id: 'logs', policy_revision: state.lifecycle.policies[0]?.revision || 0, candidate_names: ['app.log.9.gz'], candidate_bytes: 4096, protected: { latest_rotation: 1, within_retention: 3 }, observed_bytes: 8192, over_budget: true, complete: true };
      else if (path === 'runtime/runs') { executes++; const input = route.request().postDataJSON(); state.lifecycle.runs = [{ id: 'run-fixture', request_key: input.request_id, source_id: 'logs', policy_revision: 1, instance_id: 'installed', state: 'partial', started_at: new Date().toISOString(), finished_at: new Date().toISOString(), deleted_files: 1, removed_bytes: 4096, removed_allocated_bytes: 4096, available_before: 30 * GiB, available_after: null, problem: 'protected_budget_exceeded' }]; data = state.lifecycle.runs[0]; }
      else throw new Error(`unexpected runtime fixture ${path}`);
      await route.fulfill({ json: { code: 200, data } });
    });
    try {
      await page.goto(`${baseURL}/monitor/storage`); const skip = page.getByRole('button', { name: '跳过引导', exact: true }); if (await skip.isVisible()) await skip.click();
      const w = page.getByTestId('storage-workspace'); await expect(w).toBeVisible(); await expect(w.getByText('存储观测加载失败', { exact: true })).toHaveCount(0);
      if (width < 640) { const collapse = page.getByRole('complementary').getByRole('button', { name: '收起', exact: true }).first(); const box = await collapse.boundingBox(); if (box && box.x >= 0) await collapse.click(); }
      async function section(name: string, value: string) { if (width < 640) await w.locator('[data-section-select]').selectOption(value); else await w.getByRole('tab', { name, exact: true }).click(); }
      await section('自动治理', 'automation'); await expect(w.getByTestId('storage-runtime')).toContainText('开发文件日志'); await w.getByRole('button', { name: '配置策略', exact: true }).click();
      const policy = page.getByTestId('storage-runtime-policy'); await expect(policy.getByTestId('runtime-save')).toBeDisabled();
      await policy.getByTestId('runtime-enabled').click(); await policy.getByTestId('runtime-confirm').click(); await policy.getByTestId('runtime-save').click(); await expect(policy).toContainText('已从运行账本读回并确认保存'); expect(saves).toBe(1);
      await policy.getByRole('button', { name: '预览日志清理', exact: true }).click(); await expect(policy.getByTestId('runtime-preview')).toContainText('来源超预算');
      await policy.getByTestId('runtime-run').click(); await expect(policy).toContainText('部分完成'); await expect(policy.getByTestId('runtime-run')).toBeDisabled(); expect(executes).toBe(1);
      await policy.getByTestId('runtime-days').fill('14'); const reads = runtimeReads;
      const push = await page.request.post(`${baseURL}/api/v1/__snapshot_browser/storage-push`); expect((await push.json()).connections).toBeGreaterThan(0);
      await expect.poll(() => runtimeReads).toBeGreaterThan(reads); await expect(policy.getByTestId('runtime-days')).toHaveValue('14'); await expect(w.getByText('存储观测加载失败', { exact: true })).toHaveCount(0);
      const stable = runtimeReads; await page.waitForTimeout(1200); expect(runtimeReads).toBe(stable); expect(executes).toBe(1);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
      await page.screenshot({ path: info.outputPath(`runtime-${width}.png`), fullPage: true }); await page.keyboard.press('Escape');
      await section('容量概览', 'overview'); await section('自动治理', 'automation'); await w.getByRole('button', { name: '配置策略', exact: true }).click(); await expect(policy.getByTestId('runtime-days')).toHaveValue('14');
    } finally { await context.close(); }
  });
}
