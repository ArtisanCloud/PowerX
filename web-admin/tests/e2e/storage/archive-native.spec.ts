import { test, expect } from '@playwright/test';
import { readFileSync, writeFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';

test('actual UI, native archive API, SSE and private versioned S3 retrieval', async ({ browser, baseURL }, info) => {
  test.skip(!process.env.POWERX_ARCHIVE_UI_NATIVE_FIXTURE, 'owned native fixture required');
  const fixture = JSON.parse(readFileSync(process.env.POWERX_ARCHIVE_UI_NATIVE_FIXTURE!, 'utf8'));
  const cfg = JSON.parse(readFileSync(process.env.POWERX_ARCHIVE_UI_NATIVE_CONFIG!, 'utf8'));
  const expiresAt = Date.now() + 3600000;
  const session = { version: 1, sessionId: 'native-archive-ui', revision: 1, status: 'active', accessToken: `header.${Buffer.from(JSON.stringify({ tid: 'snapshot-browser-fixture', exp: Math.floor(expiresAt / 1000) })).toString('base64url')}.fixture`, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid: 'snapshot-browser-fixture' };
  const context = await browser.newContext({ baseURL, viewport: { width: 1440, height: 1000 }, locale: 'zh-CN', storageState: { cookies: [], origins: [{ origin: baseURL!, localStorage: [{ name: 'powerx.session', value: JSON.stringify(session) }, { name: 'welcome_guide_completed', value: 'true' }] }] } });
  const page = await context.newPage();
  const requests: { method: string; path: string }[] = [];
  const pageErrors: string[] = [];
  page.on('pageerror', e => { pageErrors.push(e.message); writeFileSync(info.outputPath('page-errors.json'), JSON.stringify(pageErrors)); });
  let streamConnections = 0;
  page.on('request', r => { const p = new URL(r.url()).pathname; if (p.startsWith('/api/v1/admin/ops/backup/archives')) { requests.push({ method: r.method(), path: p }); writeFileSync(info.outputPath('request-progress.json'), JSON.stringify(requests)); } if (p === '/api/v1/admin/ops/backup/events') streamConnections++; });
  await page.route('**/api/v1/admin/menus**', route => route.fulfill({ json: { code: 200, data: { categories: [{ id: 'monitor', title: '监控中心', children: [{ id: 'storage', title: '存储治理', path: '/monitor/storage', origin: 'core', visible: true }] }] } } }));
  await page.route('**/api/v1/admin/ops/storage/overview', route => route.fulfill({ json: { code: 200, data: { mode: 'observe', cleanup_enabled: false, identity: { environment: 'test', instance_id: 'owned-ui-fixture' }, observed_at: new Date().toISOString(), expires_at: new Date(Date.now() + 300000).toISOString(), filesystem: { id: 'owned-fixture-fs', view: 'process_filesystem', state: 'observed', total_bytes: 128 * 1024 ** 3, used_bytes: 64 * 1024 ** 3, available_bytes: 60 * 1024 ** 3, reserved_bytes: 4 * 1024 ** 3 }, unattributed_bytes: null, resources: [], alerts: [], coverage: 'partial', limitations: [] } } }));
  const archiveBase = '/api/v1/admin/ops/backup/archives';
  const state = async () => { const r = await context.request.get(archiveBase); expect(r.ok()).toBe(true); return (await r.json()).data.overview; };
  const choose = async (dialog: ReturnType<typeof page.getByRole>, label: string, value: string) => {
    const select = dialog.getByRole('combobox', { name: label, exact: true });
    if (await select.evaluate(e => e.tagName) === 'SELECT') await select.selectOption({ label: value });
    else { await select.click(); await page.getByRole('option', { name: value, exact: true }).click(); }
  };
  try {
    await page.goto('/monitor/storage');
    const skip = page.getByRole('button', { name: '跳过引导', exact: true }); if (await skip.isVisible()) await skip.click();
    const workspace = page.getByTestId('storage-workspace');
    await workspace.getByRole('tab', { name: '日志归档与追溯', exact: true }).click();
    const archive = page.getByTestId('archive-governance');
    await expect(archive).toContainText('owned-native-ui');
    const section = async (value: string, label: string, width = 1440) => {
      if (width < 640) await archive.locator('[data-section-select]').selectOption(value);
      else await archive.getByRole('tab', { name: label, exact: true }).click();
    };
    await section('targets', '存储目标');
    await archive.getByRole('button', { name: '配置独立凭据', exact: true }).click();
    let dialog = page.getByRole('dialog');
    await dialog.getByRole('textbox', { name: '独立凭据引用', exact: true }).fill('native-ui-test');
    await dialog.getByLabel('Access Key', { exact: true }).fill(cfg.AccessKey);
    await dialog.getByLabel('Secret Key', { exact: true }).fill(cfg.SecretKey);
    await dialog.locator('button[form="s3-archive-credential"]').click();
    await expect(dialog).not.toBeVisible();
    await archive.getByRole('button', { name: '配置存储目标', exact: true }).click();
    dialog = page.getByRole('dialog');
    for (const [label, value] of Object.entries({ '名称': '独立验收存储', 'S3 服务地址': cfg.Endpoint, '专用存储桶': cfg.Bucket, '专用目录前缀': 'owned-ui', '区域': cfg.Region })) await dialog.getByRole('textbox', { name: label, exact: true }).fill(String(value));
    await choose(dialog, '独立凭据引用', 'native-ui-test');
    await dialog.getByRole('checkbox', { name: '使用路径式寻址（MinIO 常用）', exact: true }).check();
    await dialog.locator('button[form="s3-archive-target"]').click();
    await expect(dialog).not.toBeVisible();
    expect((await state()).policies).toHaveLength(0);
    await section('policies', '归档策略');
    await archive.getByRole('button', { name: '配置归档策略', exact: true }).click();
    dialog = page.getByRole('dialog');
    await dialog.getByRole('textbox', { name: '名称', exact: true }).fill('自有日志归档验收');
    await choose(dialog, '日志来源', '自有测试日志');
    await choose(dialog, '归档目标', '独立验收存储');
    await dialog.locator('button[form="log-archive-policy"]').click();
    await expect(dialog).not.toBeVisible();
    expect((await state()).policies[0].enabled).toBe(false);
    expect((await state()).jobs).toHaveLength(0);
    await archive.getByRole('button', { name: '开启归档', exact: true }).click();
    dialog = page.getByRole('dialog');
    await dialog.getByRole('button', { name: '确认执行', exact: true }).click();
    await expect(dialog.getByRole('alert')).toBeVisible();
    expect((await state()).policies[0].enabled).toBe(false);
    await page.keyboard.press('Escape');
    await section('targets', '存储目标');
    await archive.getByRole('button', { name: '测试写入与取回', exact: true }).click();
    await expect(archive).toContainText('验证有效');
    await section('policies', '归档策略');
    await archive.getByRole('button', { name: '开启归档', exact: true }).click();
    dialog = page.getByRole('dialog');
    await dialog.getByRole('button', { name: '确认执行', exact: true }).click();
    await expect(dialog).not.toBeVisible();
    const [runResponse] = await Promise.all([
      page.waitForResponse(r => r.request().method() === 'POST' && /\/archives\/policies\/[^/]+\/run$/.test(new URL(r.url()).pathname), { timeout: 10000 }),
      archive.getByRole('button', { name: '归档一批', exact: true }).click(),
    ]);
    expect(runResponse.status()).toBe(202);
    await section('catalog', '检索与取回');
    // 后台完成依靠正式 SSE 的 invalidate 更新页面，测试不轮询任务状态。
    await expect(archive.getByText('可取回', { exact: true })).toBeVisible({ timeout: 30000 });
    const completed = await state();
    expect(completed.jobs[0].remote_state).toBe('success');
    expect(completed.jobs[0].remote_manifest_version).toBeTruthy();
    const reclaim = await context.request.post(`${archiveBase}/__owned-fixture/reclaim`, { data: {} });
    expect(reclaim.ok()).toBe(true);
    const reclaimed = await reclaim.json();
    expect(reclaimed.source_deleted).toBe(true);
    expect(reclaimed.cas_removed).toBe(1);
    await archive.getByRole('textbox', { name: '搜索原文件名、来源或归档编号', exact: true }).fill('app.log.2');
    await archive.getByRole('button', { name: '检索', exact: true }).click();
    await expect(archive.locator('[data-testid="archive-catalog-item"]')).toHaveCount(1);
    await archive.getByRole('button', { name: '查看日志', exact: true }).click();
    await page.getByRole('dialog').getByRole('button', { name: '核验并读取', exact: true }).click();
    await expect(page.getByTestId('archive-log-preview')).toContainText(fixture.expected_text.trim());
    await page.keyboard.press('Escape');
    const [download] = await Promise.all([page.waitForEvent('download'), archive.getByRole('button', { name: '下载并校验', exact: true }).click()]);
    const output = info.outputPath('source-gone-native.tar.gz'); await download.saveAs(output);
    const body = execFileSync('tar', ['-xOf', output, 'application_logs/app.log.2']);
    expect(createHash('sha256').update(body).digest('hex')).toBe(fixture.expected_sha256);
    await expect(archive.getByRole('button', { name: '下载并校验', exact: true })).toBeEnabled();
    const idleStart = requests.length; await page.waitForTimeout(1800); expect(requests.length).toBe(idleStart);
    for (const width of [1440, 390, 320]) {
      await page.setViewportSize({ width, height: 1000 });
      await page.waitForTimeout(350); // 等待响应式导航的位移动画结束。
      await section('targets', '存储目标', width); await section('catalog', '检索与取回', width);
      await expect(archive.getByRole('textbox', { name: '搜索原文件名、来源或归档编号', exact: true })).toHaveValue('app.log.2');
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
      await archive.scrollIntoViewIfNeeded(); await page.screenshot({ path: info.outputPath(`native-archive-${width}.png`), fullPage: true });
      if (width === 1440) await page.getByRole('complementary').getByRole('button', { name: '收起', exact: true }).first().click();
    }
    await section('policies', '归档策略', 320);
    await archive.getByRole('button', { name: '关闭归档', exact: true }).click();
    await page.getByRole('dialog').getByRole('button', { name: '确认执行', exact: true }).click();
    await expect(page.getByRole('dialog')).not.toBeVisible();
    expect((await state()).policies[0].enabled).toBe(false);
    await section('catalog', '检索与取回', 320);
    const [afterDisable] = await Promise.all([page.waitForEvent('download'), archive.getByRole('button', { name: '下载并校验', exact: true }).click()]);
    const disabledOutput = info.outputPath('archive-disabled-still-readable.tar.gz'); await afterDisable.saveAs(disabledOutput);
    expect(createHash('sha256').update(execFileSync('tar', ['-xOf', disabledOutput, 'application_logs/app.log.2'])).digest('hex')).toBe(fixture.expected_sha256);
    expect(streamConnections).toBeGreaterThan(0);
    writeFileSync(info.outputPath('native-ui-result.json'), JSON.stringify({ passed: true, identity: 'controlled fixture', native_api: true, native_sse: true, source_gone: true, cas_gone: true, sha256_verified: true, widths: [1440, 390, 320], idle_polling: false, archive_disabled_after_test: true, requests }, null, 2));
  } finally {
    await context.close().catch(() => {});
    writeFileSync(process.env.POWERX_ARCHIVE_UI_NATIVE_FIXTURE! + '.done', 'done\n', { mode: 0o600 });
  }
});
