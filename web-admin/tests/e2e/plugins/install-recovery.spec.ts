import { test, expect } from '@playwright/test';
import { mkdtemp, writeFile, rm } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';

const tenant = '00000000-0000-4000-8000-000000000001';
const id = 'com.powerx.plugins.fixture';

for (const width of [1440, 390]) {
  test(`separate target recovery with A to B to A at ${width}px`, async ({ page }, info) => {
    await page.setViewportSize({ width, height: 1000 });
    const b = 'com.powerx.plugins.other';
    const source = await mkdtemp(join(tmpdir(), 'powerx-install-targets-'));
    const { mkdir } = await import('node:fs/promises');
    for (const [name, target] of [['a', id], ['b', b]]) {
      await mkdir(join(source, name)); await writeFile(join(source, name, 'plugin.yaml'), `id: ${target}\nversion: 1.0.0\n`);
    }
    const phases: Record<string, number> = { [id]: 0, [b]: 0 };
    const uploads: Record<string, number> = { [id]: 0, [b]: 0 };
    let inspections = 0, resumes = 0;
    const errors: string[] = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.addInitScript(({ tenant }) => {
      const expiresAt = Date.now() + 3600000;
      const token = `header.${btoa(JSON.stringify({ tid: tenant, exp: Math.floor(expiresAt / 1000) }))}.fixture`;
      localStorage.setItem('powerx.session', JSON.stringify({ version: 1, status: 'active', sessionId: 'install-targets-browser', revision: 1, accessToken: token, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid: tenant }));
      localStorage.setItem('welcome_guide_completed', 'true'); localStorage.setItem('px_locale', 'zh');
    }, { tenant });
    await page.routeWebSocket(/\/api\/ws/, socket => socket.close());
    await page.route(url => url.pathname.startsWith('/api/'), async route => {
      const path = new URL(route.request().url()).pathname;
      if (path.includes('/_nuxt_icon/')) return route.fallback();
      let data: any = {};
      if (path.endsWith('/setup/status')) data = { configured: true, requires_login: true, version: 'test' };
      else if (path.endsWith('/me/context')) data = { is_root: true, current_tenant_uuid: tenant, current_member_id: 1, user: { id: 1, display_name: 'Target fixture', status: 1 }, members: [{ tenant_uuid: tenant, tenant_name: 'Fixture', member_id: 1, is_admin: true }] };
      else if (path.endsWith('/menus')) data = { menus: [{ id: 'install-fixture', title: '安装恢复验收', path: '/__fixtures/plugin-install-recovery', visible: true, origin: 'system' }] };
      else if (path.endsWith('/tenant_config')) {
        const target = decodeURIComponent(path.split('/').at(-2)!); inspections++;
        data = { plugin_id: target, installation: phases[target] === 0 ? { ready: false, reason: 'PLUGIN_NOT_INSTALLED' } : { version: '1.0.0', revision: `${target}-new`, state: 'enabled', ready: phases[target] === 2, runtime_ready: true, tenant_ready: phases[target] === 2 } };
      } else if (path.endsWith('/install/local')) {
        // Chromium 的拦截正文省略 File 内容；用显式文件路径及备注核对本次目标。
        const body = route.request().postDataBuffer()!.toString();
        const target = body.includes('["b/plugin.yaml"]') ? b : id;
        expect(body).toContain(target === id ? '["a/plugin.yaml"]' : '["b/plugin.yaml"]');
        if (target === b) expect(body).not.toContain('A 的安装备注');
        uploads[target]++; phases[target] = target === id ? 1 : 2;
        if (target === id) return route.abort('timedout');
        data = { ok: true };
      } else if (path.endsWith(`/${id}/enable`)) {
        resumes++; expect(route.request().postDataJSON()).toEqual({ expected_version: '1.0.0', expected_revision: `${id}-new` }); phases[id] = 2; data = { ok: true };
      } else if (/\/admin\/plugins\/?$/.test(path)) data = { plugins: [] };
      await route.fulfill({ json: { code: 200, message: 'success', data } });
    });
    const dialog = page.getByRole('dialog');
    async function choose(name: string) {
      await dialog.getByRole('combobox').first().click(); await page.getByRole('option', { name: '本地上传', exact: true }).click();
      const chooser = page.waitForEvent('filechooser'); await dialog.getByRole('button', { name: '选择本地来源' }).click();
      await (await chooser).setFiles(join(source, name));
    }
    try {
      await page.goto('/__fixtures/plugin-install-recovery');
      if (width < 640) await page.getByRole('complementary').getByRole('button', { name: '收起', exact: true }).first().click();
      await page.getByRole('button', { name: '安装 A', exact: true }).click();
      await choose('a'); await dialog.locator('textarea').fill('A 的安装备注');
      await dialog.getByRole('button', { name: '安装', exact: true }).click();
      await expect(dialog.getByTestId('install-recovery')).toContainText('已安装，待完成启用');
      await expect(dialog.getByRole('button', { name: '继续完成启用', exact: true })).toBeEnabled();
      await page.keyboard.press('Escape'); await expect(dialog).toBeHidden();
      await page.getByRole('button', { name: '安装 B', exact: true }).click();
      await expect(dialog.getByTestId('install-recovery')).toHaveCount(0);
      await choose('b'); await expect(dialog.locator('textarea')).toHaveValue('');
      await dialog.getByRole('button', { name: '安装', exact: true }).click(); await expect(dialog).toBeHidden();
      expect(uploads).toEqual({ [id]: 1, [b]: 1 });
      await page.getByRole('button', { name: '安装 A', exact: true }).click();
      await expect(dialog.getByTestId('install-recovery')).toContainText(id);
      await expect(dialog.getByTestId('install-recovery')).toContainText('已安装，待完成启用');
      await expect(dialog.getByRole('button', { name: '安装', exact: true })).toHaveCount(0);
      await expect(dialog.locator('textarea')).toHaveValue('A 的安装备注');
      const reads = inspections; await page.waitForTimeout(1200); expect(inspections).toBe(reads);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
      await page.screenshot({ path: info.outputPath(`target-recovery-${width}.png`), animations: 'disabled' });
      await dialog.getByRole('button', { name: '继续完成启用', exact: true }).click();
      await expect(dialog.getByTestId('install-recovery')).toContainText('安装已完成');
      await dialog.getByRole('button', { name: '完成', exact: true }).click(); await expect(dialog).toBeHidden();
      expect(uploads).toEqual({ [id]: 1, [b]: 1 }); expect(resumes).toBe(1); expect(errors).toEqual([]);
    } finally { await rm(source, { recursive: true, force: true }); }
  });

  test(`installation reconciliation without repeated upload at ${width}px`, async ({ page }, info) => {
    await page.setViewportSize({ width, height: 1000 });
    const source = await mkdtemp(join(tmpdir(), 'powerx-install-browser-'));
    await writeFile(join(source, 'plugin.yaml'), `id: ${id}\nversion: 1.0.0\n`);
    let phase = 0, uploads = 0, resumes = 0, inspections = 0;
    const errors: string[] = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.addInitScript(({ tenant }) => {
      const expiresAt = Date.now() + 3600000;
      const token = `header.${btoa(JSON.stringify({ tid: tenant, exp: Math.floor(expiresAt / 1000) }))}.fixture`;
      localStorage.setItem('powerx.session', JSON.stringify({ version: 1, status: 'active', sessionId: 'install-recovery-browser', revision: 1, accessToken: token, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid: tenant }));
      localStorage.setItem('welcome_guide_completed', 'true'); localStorage.setItem('px_locale', 'zh');
    }, { tenant });
    await page.routeWebSocket(/\/api\/ws/, socket => socket.close());
    await page.route(url => url.pathname.startsWith('/api/'), async route => {
      const path = new URL(route.request().url()).pathname;
      if (path.includes('/_nuxt_icon/')) return route.fallback();
      let data: any = {};
      if (path.endsWith('/setup/status')) data = { configured: true, requires_login: true, version: 'test' };
      else if (path.endsWith('/me/context')) data = { is_root: true, current_tenant_uuid: tenant, current_member_id: 1, user: { id: 1, display_name: 'Install fixture', status: 1 }, members: [{ tenant_uuid: tenant, tenant_name: 'Fixture', member_id: 1, is_admin: true }] };
      else if (path.endsWith('/menus')) data = { menus: [{ id: 'plugins', title: '本地插件', path: '/plugins/local', visible: true, origin: 'system' }] };
      else if (path.endsWith('/tenant_config')) {
        inspections++;
        data = { plugin_id: id, exists: phase > 0, enabled: phase > 0, installation: phase === 0 ? { ready: false, reason: 'PLUGIN_NOT_INSTALLED' } : { version: '1.0.0', revision: 'new', state: 'enabled', ready: phase === 2, runtime_ready: true, tenant_ready: phase === 2 } };
      } else if (path.endsWith('/install/local')) {
        uploads++; phase = 1; return route.abort('timedout');
      } else if (path.endsWith(`/${id}/enable`)) {
        resumes++; expect(route.request().postDataJSON()).toEqual({ expected_version: '1.0.0', expected_revision: 'new' }); phase = 2; data = { ok: true };
      } else if (/\/admin\/plugins\/?$/.test(path)) data = { plugins: phase ? [{ id, name: 'Fixture', version: '1.0.0', revision: 'new', state: 'enabled', install_origin: { type: 'local' } }] : [] };
      else if (path.endsWith('/tenant-instances')) data = [];
      else if (path.endsWith('/marketplace/plugins')) data = { items: [] };
      await route.fulfill({ json: { code: 200, message: 'success', data } });
    });
    try {
      await page.goto('/plugins/local');
      if (width < 640) await page.getByRole('complementary').getByRole('button', { name: '收起', exact: true }).first().click();
      await page.getByRole('button', { name: '本地安装', exact: true }).click();
      const dialog = page.getByRole('dialog'); await expect(dialog).toBeVisible();
      await dialog.getByRole('combobox').first().click(); await page.getByRole('option', { name: '本地上传', exact: true }).click();
      const chooser = page.waitForEvent('filechooser'); await dialog.getByRole('button', { name: '选择本地来源' }).click();
      await (await chooser).setFiles(source);
      await expect(dialog).toContainText('1 个文件');
      await dialog.getByRole('button', { name: '安装', exact: true }).click();
      await expect(dialog.getByTestId('install-recovery')).toContainText('已安装，待完成启用');
      await expect(dialog.getByRole('button', { name: '安装', exact: true })).toHaveCount(0);
      expect(uploads).toBe(1); expect(resumes).toBe(0);
      const readCount = inspections; await page.waitForTimeout(1200); expect(inspections).toBe(readCount);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
      await page.screenshot({ path: info.outputPath(`recovery-${width}.png`), animations: 'disabled' });
      await dialog.getByRole('button', { name: '继续完成启用', exact: true }).click();
      await expect(dialog.getByTestId('install-recovery')).toContainText('安装已完成');
      expect(uploads).toBe(1); expect(resumes).toBe(1); expect(errors).toEqual([]);
      await dialog.getByRole('button', { name: '完成', exact: true }).click(); await expect(dialog).toBeHidden();
    } finally { await rm(source, { recursive: true, force: true }); }
  });
}
