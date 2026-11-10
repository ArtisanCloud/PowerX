import { test, expect } from '@playwright/test';
// 真实 Nuxt 产物与 API client；API fixtures 验证界面。MinIO/PG 单独使用真实隔离实例验收。
for (const width of [1440, 390, 320]) test(`archive tasks, retained forms and safe defaults at ${width}px`, async ({ browser, baseURL }, info) => {
 const expiresAt = Date.now() + 3600000;
 const session = { version: 1, sessionId: 'archive-ui-fixture', revision: 1, status: 'active', accessToken: `header.${Buffer.from(JSON.stringify({ tid: 'snapshot-browser-fixture', exp: Math.floor(expiresAt / 1000) })).toString('base64url')}.fixture`, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid: 'snapshot-browser-fixture' };
 const context = await browser.newContext({ viewport: { width, height: 1000 }, locale: 'zh-CN', storageState: { cookies: [], origins: [{ origin: baseURL!, localStorage: [{ name: 'powerx.session', value: JSON.stringify(session) }, { name: 'welcome_guide_completed', value: 'true' }] }] } });
 const page = await context.newPage(); let reads = 0; let writes = 0;
 await page.route('**/api/v1/admin/menus**',route=>route.fulfill({json:{code:200,data:{categories:[{id:'monitor',title:'监控中心',children:[{id:'storage',title:'存储治理',path:'/monitor/storage',origin:'core',visible:true},{id:'backup',title:'备份中心',path:'/ops/backup',origin:'core',visible:true}]}]}}}));
 await page.route('**/api/v1/admin/ops/storage/overview', route => route.fulfill({ json: { code: 200, data: { mode: 'observe', cleanup_enabled: false, identity: { environment: 'dev', instance_id: 'archive-fixture' }, observed_at: new Date().toISOString(), expires_at: new Date(Date.now() + 300000).toISOString(), filesystem: { id: 'test-fs', view: 'process_filesystem', state: 'observed', total_bytes: 128 * 1024 ** 3, used_bytes: 64 * 1024 ** 3, available_bytes: 60 * 1024 ** 3, reserved_bytes: 4 * 1024 ** 3 }, unattributed_bytes: null, resources: [], alerts: [], coverage: 'partial', limitations: [] } } }));
 let databasePolicy = { revision: 1, owner_instance: 'archive-fixture', enabled: false, success_days: 30, failure_days: 90, pending_hours: 24, quarantine_days: 30, batch_limit: 200, max_bytes: 8 * 1024 ** 3, adopt_legacy: false, reject_new_when_full: false, actor: 1, updated_at: new Date().toISOString() };
 let databaseReady = false;
 await page.route('**/api/v1/admin/ops/storage/database-retention', async route => {
   if (route.request().method() === 'PUT') { const input = route.request().postDataJSON(); expect(input.confirm).toBe(true); expect(input.expected_revision).toBe(databasePolicy.revision); expect(input.policy.success_days).toBe(14); expect(input.policy.enabled).toBe(false); expect(input.policy.reject_new_when_full).toBe(true); databasePolicy = { ...input.policy, revision: databasePolicy.revision + 1 }; return route.fulfill({ json: { code: 200, data: databasePolicy } }); }
   return route.fulfill({ json: { code: 200, data: { policy: databasePolicy, state_configured: databaseReady, enforcement_allowed: databaseReady, migration_ready: databaseReady, ready: databaseReady, worker_enabled: databaseReady, problem: databaseReady ? undefined : 'database_retention_runtime_not_configured', migration_problem: databaseReady ? undefined : 'database_retention_migration_required', relation_bytes: 1024 ** 3, runs: [], scope: 'instance_registered_tenants_invocation_audit_only' } } });
 });
 const target = { id: 'target', driver: 's3', name: '私有日志存储', endpoint: 'https://archive.example.invalid', bucket: 'logs', prefix: 'host/test', region: 'us-east-1', credential_ref: 'independent', force_path_style: true, revision: 1, verification_valid: false, executor: 'powerx:test-node' };
 const policy = { id: 'policy', name: '审计归档', source_id: 'audit', target_id: 'target', revision: 1, enabled: false, remote_cleanup_enabled: false, interval_minutes: 5, retention_days: 7, retention_count: 10, remote_retention_days: 180, remote_retention_count: 100, remote_max_bytes: 1024 ** 3 };
 await page.route('**/api/v1/admin/ops/backup/archives**', async route => {
  const url = new URL(route.request().url()); if (route.request().method() !== 'GET') { writes++; return route.fulfill({ status: 409, json: { code: 409, message: 'target has not been verified' } }); }
  reads++;
  if (url.pathname.endsWith('/preview')) return route.fulfill({ json: { code: 200, data: { name: 'audit_logs/audit_event-2026-10-01.jsonl.gz', text: 'isolated browser canary preview', verified: true, truncated: false } } });
  if (url.pathname.endsWith('/download')) return route.fulfill({ contentType: 'application/gzip', body: Buffer.from('isolated browser download canary') });
  if (url.pathname.endsWith('/jobs/old')) return route.fulfill({ json: { code: 200, data: { job: { id: 'old', files: [{ path: 'audit_logs/audit_event-2026-10-01.jsonl.gz' }] } } } });
  const data = url.pathname.endsWith('/catalog') ? { items: [{ id: 'old', source_id: 'legacy-prod-audit', source_file_name: 'audit_event-2026-10-01.jsonl.gz', kind: 'audit_logs', policy_id: 'legacy', status: 'pruned', remote_state: 'success', imported: true, provenance_verified: false, protected: true, size_bytes: 1024, file_count: 1, created_at: new Date().toISOString() }], pagination: { total: 1, page: 1, pages: 1 } } : { overview: { instance: 'test', sources: [{ id: 'audit', name: '审计文件', kind: 'audit_logs', ready: true }], policies: [policy], targets: [target], s3_credentials: ['independent'], credentials: [], pending_transfers: 0, jobs: [], restores: [], custody: { node_id: 'test-node', legacy_detected: true, managed: false } } };
  return route.fulfill({ json: { code: 200, data } });
 });
 try {
  await page.goto('/monitor/storage'); const skip = page.getByRole('button', { name: '跳过引导', exact: true }); if (await skip.isVisible()) await skip.click();
  if (width < 640) { const collapse = page.getByRole('complementary').getByRole('button', { name: '收起', exact: true }).first(); const box = await collapse.boundingBox(); if (box && box.x >= 0) await collapse.click(); }
  const workspace = page.getByTestId('storage-workspace');
  if (width < 640) await workspace.locator('[data-section-select]').first().selectOption('archives'); else await workspace.getByRole('tab', { name: '日志归档与追溯', exact: true }).click();
  const archive = workspace.locator('[data-section="archives"]');
  await expect(archive.getByText('audit_event-2026-10-01.jsonl.gz', { exact: true })).toBeVisible(); await expect(archive.getByText(/旧守护脚本尚未完成协议接管/)).toBeVisible(); expect(writes).toBe(0);
  await archive.getByRole('textbox', { name: '搜索原文件名、来源或归档编号', exact: true }).fill('audit_event');
  const choose = async (value: string, label: string) => { if (width < 640) { await archive.locator('[data-section-select]').selectOption(value); await expect(archive.locator('[data-section-select]')).toHaveValue(value); } else { const tab = archive.getByRole('tab', { name: label, exact: true }); await tab.click(); await expect(tab).toHaveAttribute('aria-selected', 'true'); } };
  await choose('targets', '存储目标'); await expect(archive.getByText('私有日志存储', { exact: true })).toBeVisible(); await choose('catalog', '检索与取回');
  await expect(archive.getByRole('textbox', { name: '搜索原文件名、来源或归档编号', exact: true })).toHaveValue('audit_event');
  await archive.getByRole('button', { name: '查看日志', exact: true }).click();
  await expect(page.getByRole('dialog')).toBeVisible(); await page.getByRole('dialog').getByRole('button', { name: '核验并读取', exact: true }).click();
  await expect(page.getByTestId('archive-log-preview')).toContainText('isolated browser canary preview');
  await page.keyboard.press('Escape'); await expect(page.getByRole('dialog')).not.toBeVisible();
  const [download] = await Promise.all([page.waitForEvent('download'), archive.getByRole('button', { name: '下载并校验', exact: true }).click()]); await download.saveAs(info.outputPath(`browser-canary-${width}.tar.gz`));
  await expect(archive.getByRole('button', { name: '下载并校验', exact: true })).toBeEnabled();
  const before = reads; await page.waitForTimeout(1300); expect(reads).toBe(before);
  await choose('policies', '归档策略'); await archive.getByRole('button', { name: '开启归档', exact: true }).click(); expect(writes).toBe(0);
  await expect(page.getByRole('dialog')).toBeVisible(); await page.getByRole('dialog').getByRole('button', { name: '确认执行', exact: true }).click(); await expect(page.getByRole('dialog')).toContainText('target has not been verified'); expect(writes).toBe(1);
  await page.screenshot({ path: info.outputPath(`archive-error-${width}.png`) });
  await page.keyboard.press('Escape'); await expect(page.getByRole('dialog')).not.toBeVisible(); await choose('catalog', '检索与取回');
  await archive.getByRole('button', { name: '刷新观测', exact: true }).click(); await expect(archive.getByText('target has not been verified', { exact: true })).toHaveCount(0);
  await workspace.evaluate(e => e.scrollIntoView({ block: 'start' })); await page.waitForTimeout(180);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: info.outputPath(`archive-${width}.png`), fullPage: true });
  if (width < 640) await workspace.locator('[data-section-select]').first().selectOption('databaseRetention'); else await workspace.getByRole('tab', { name: '数据库期限', exact: true }).click();
  const database = page.getByTestId('database-retention');
  await expect(database).toContainText('治理运行环境未配置'); await expect(database).toContainText('数据库结构尚未就绪');
  await database.getByRole('button', { name: '查看准备步骤', exact: true }).click();
  await expect(page.getByRole('dialog')).toContainText('当前为观察模式'); await expect(page.getByRole('dialog')).toContainText('尚缺期限治理字段');
  await page.keyboard.press('Escape'); await expect(page.getByRole('dialog')).not.toBeVisible();
  await expect(database.getByRole('button', { name: '配置策略', exact: true })).toBeDisabled();
  await expect(database.getByRole('button', { name: '检查本批候选', exact: true })).toBeDisabled();
  await database.evaluate(e => e.scrollIntoView({ block: 'start' }));
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: info.outputPath(`database-readiness-${width}.png`), fullPage: true });
  databaseReady = true;
  await database.getByRole('button', { name: '刷新观测', exact: true }).click();
  await expect(database).not.toContainText('治理运行环境未配置');
  await database.getByRole('button', { name: '配置策略', exact: true }).click();
  const editor = page.getByRole('dialog'); await editor.getByRole('spinbutton', { name: '成功终态保留天数', exact: true }).fill('14');
  await editor.getByRole('checkbox', { name: /达到预算.*拒绝新的能力调用/ }).check(); await editor.getByRole('checkbox', { name: /^已核对范围/ }).check();
  await editor.locator('button[form="database-retention-form"]').click(); await expect(editor).not.toBeVisible();
  await expect(database).toContainText('调用容量保护：已开启'); await expect(database).toContainText('成功终态至少 14 天'); expect(databasePolicy.enabled).toBe(false);
  await database.evaluate(e => e.scrollIntoView({ block: 'start' })); await page.waitForTimeout(180); await page.screenshot({ path: info.outputPath(`database-${width}.png`), fullPage: true });

 } finally { await context.close(); }
});
