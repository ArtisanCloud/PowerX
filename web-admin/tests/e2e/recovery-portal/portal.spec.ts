import { test, expect } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

// Real embedded Go HTTP service, session store and filesystem notifications;
// controlled restore driver. Native PG/media/config restoration is tested in Go.
for (const narrow of [false, true]) {
  test(`offline restore survives refresh and reauthentication on ${narrow ? 'narrow' : 'desktop'} screen`, async ({ browser, request, baseURL }, info) => {
    await request.post('/__fixture/reset');
    const record = JSON.parse(readFileSync(resolve('../test-results/recovery-portal-browser-process.json'), 'utf8'));
    const key = readFileSync(record.key_file, 'utf8').trim();
    const context = await browser.newContext({ baseURL, viewport: narrow ? { width: 390, height: 844 } : { width: 1440, height: 1000 } });
    const page = await context.newPage();
    const errors: string[] = [];
    page.on('pageerror', error => errors.push(error.message));
    const catalogReads: string[] = [];
    page.on('request', event => { if (new URL(event.url()).pathname === '/__powerx_recovery/catalog') catalogReads.push(event.url()); });
    const login = async () => {
      await page.getByLabel('恢复密钥', { exact: true }).fill(key);
      await page.getByRole('button', { name: '验证并进入', exact: true }).click();
      await expect(page.getByRole('button', { name: '退出恢复会话', exact: true })).toBeVisible();
      await expect(page.getByRole('heading', { name: '验证恢复管理员身份' })).not.toBeVisible();
    };
    try {
      // No Core or Nuxt process serves this page.
      await page.goto('/ops/backup');
      await expect(page.getByRole('heading', { name: '验证恢复管理员身份' })).toBeVisible();
      await login();
      const cookie = (await context.cookies()).find(item => item.name === 'powerx_recovery_admin');
      expect(cookie?.httpOnly).toBe(true);
      await expect(page.getByText('业务已停止', { exact: true })).toBeVisible();
      const downloadEvent = page.waitForEvent('download');
      await page.getByRole('button', { name: '保存恢复密钥', exact: true }).click();
      expect((await downloadEvent).suggestedFilename()).toBe('powerx-recovery-administrator.key');
      await page.getByRole('button', { name: '恢复当前实例', exact: true }).click();
      const dialog = page.getByRole('dialog');
      // Explicit logout in another tab closes the old confirmation and streams.
      const session = await (await context.request.get('/__powerx_recovery/session')).json();
      await context.request.delete('/__powerx_recovery/session', { headers: { 'X-PowerX-Recovery-CSRF': session.csrf } });
      await page.evaluate(() => { const channel = new BroadcastChannel('powerx-recovery-auth');channel.postMessage({ event: 'logout' });channel.close(); });
      await expect(dialog).not.toBeVisible();
      await expect(page.getByRole('heading', { name: '验证恢复管理员身份' })).toBeVisible();
      await login();
      await page.getByRole('button', { name: '恢复当前实例', exact: true }).click();
      await dialog.getByRole('checkbox').check();
      await dialog.getByRole('button', { name: '确认执行', exact: true }).click();
      await expect(page.getByText('进入维护', { exact: true })).toBeVisible();
      await page.reload();
      await expect(page.getByText('进入维护', { exact: true })).toBeVisible();
      await page.getByRole('button', { name: '退出恢复会话', exact: true }).click();
      await expect(page.getByRole('heading', { name: '验证恢复管理员身份' })).toBeVisible();
      await login();
      await expect(page.getByText('进入维护', { exact: true })).toBeVisible();
      await page.getByText('查看报告', { exact: true }).click();
      await page.getByRole('button', { name: '刷新', exact: true }).click();
      await expect(page.locator('#jobs details').first()).toHaveAttribute('open', '');
      await page.waitForTimeout(300);
      const before = catalogReads.length;
      await page.waitForTimeout(5500);
      expect(catalogReads.length).toBe(before);
      expect(await page.evaluate(() => ({ local: localStorage.length, session: sessionStorage.length, cookieVisible: document.cookie.includes('powerx_recovery_admin') }))).toEqual({ local: 0, session: 0, cookieVisible: false });
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
      await page.screenshot({ path: info.outputPath(`offline-maintenance-${narrow ? 'narrow' : 'desktop'}.png`), fullPage: true });
      await request.post('/__fixture/continue');
      await expect(page.locator('#business-ready')).toHaveText('业务恢复完成', { timeout: 12_000 });
      await expect(page).toHaveURL(/\/ops\/backup$/);
      expect(errors).toEqual([]);
    } finally { await context.close(); }
  });
}
