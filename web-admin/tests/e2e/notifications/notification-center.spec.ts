import { test, expect } from '@playwright/test';

const id = '33333333-3333-4333-8333-333333333333';
const missingID = '55555555-5555-4555-8555-555555555555';
test.beforeEach(async ({request}) => { await request.delete('/api/v1/__notification_fixture'); });
for (const narrow of [false, true]) {
  test(`notification detail and task links on ${narrow ? 'narrow dark' : 'desktop light'} screen`, async ({browser, baseURL, request}, info) => {
    const expiresAt = Date.now() + 3600_000;
    const tenantUuid = '11111111-1111-4111-8111-111111111111';
    const session = {version: 1, sessionId: 'notification-browser-session', revision: 1, status: 'active', accessToken: `header.${Buffer.from(JSON.stringify({tid: tenantUuid, exp: Math.floor(expiresAt/1000)})).toString('base64url')}.fixture`, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid};
    const context = await browser.newContext({baseURL, locale: 'zh-CN', viewport: narrow ? {width: 390, height: 844} : {width: 1440, height: 1000}, storageState: {cookies: [], origins: [{origin: baseURL!, localStorage: [{name: 'welcome_guide_completed', value: 'true'}, {name: 'powerx.session', value: JSON.stringify(session)}, {name: 'powerx-color-mode', value: narrow ? 'dark' : 'light'}]}]}});
    const page = await context.newPage();
    const errors: string[] = []; page.on('pageerror', e => errors.push(e.message));
    const closeMobileSidebar = async () => {
      if (!narrow) return;
      const collapse = page.getByRole('complementary').getByRole('button', {name: '收起', exact: true}).first();
      const box = await collapse.boundingBox();
      if (box && box.x >= 0) await collapse.click();
    };
    try {
      await page.goto(`/notifications?id=${id}`);
      const skip = page.getByRole('button', {name: '跳过引导', exact: true});
      if (await skip.isVisible()) await skip.click();
      await expect(page.getByRole('heading', {name: '数据库备份完成', exact: true, level: 1})).toBeVisible();
      await closeMobileSidebar();
      await expect(page.getByText('备份任务 #803 已完成，可查看备份文件及执行记录。', {exact: true}).last()).toBeVisible();
      await page.screenshot({path: info.outputPath(`notification-detail-${narrow ? 'narrow' : 'desktop'}.png`), fullPage: true});
      let state = await (await request.get('/api/v1/__notification_fixture')).json();
      expect(state.items.find((n: any) => n.id === id).isRead).toBe(true);
      await expect.poll(async () => (await (await request.get('/api/v1/__notification_fixture')).json()).subscribers).toBeGreaterThan(0);
      state = await (await request.get('/api/v1/__notification_fixture')).json();
      await request.post('/api/v1/__notification_fixture');
      const count = state.requests.filter((r: any) => r.path.includes('/admin/notifications')).length;
      await page.waitForTimeout(1500); // observe idle traffic; there must be no state polling
      state = await (await request.get('/api/v1/__notification_fixture')).json();
      expect(state.requests.filter((r: any) => r.path.includes('/admin/notifications')).length).toBe(count);
      await page.locator('header').getByRole('button', {name: '查看所有通知', exact: true}).click();
      await expect(page.getByRole('menuitem', {name: /恢复验证失败/})).toBeVisible();
      await expect(page.getByRole('menuitem', {name: /不应进入铃铛的临时任务/})).toHaveCount(0);
      await page.keyboard.press('Escape');

      await page.goto(`/notifications?id=${missingID}`);
      await expect(page.getByTestId('notification-detail-state')).toContainText('通知不存在、已删除，或不属于当前账号。');
      await closeMobileSidebar();
      await expect(page.getByRole('button', {name: '返回列表', exact: true})).toBeVisible();
      await page.screenshot({path: info.outputPath(`notification-missing-${narrow ? 'narrow' : 'desktop'}.png`), fullPage: true});
      await page.getByRole('button', {name: '返回列表', exact: true}).click();
      await expect(page).toHaveURL(/\/notifications$/);
      await expect(page.getByRole('heading', {name: '消息通知', exact: true})).toBeVisible();

      await page.goto('/ops/backup?job_id=803');
      await expect(page.getByTestId('backup-linked-task')).toContainText('当前显示通知对应的记录');
      await closeMobileSidebar();
      await expect(page.getByText('历史任务失败原因（浏览器验收）')).toBeVisible();
      await expect(page.getByText(/仅展示执行成功且仍登记备份文件的任务/)).toHaveCount(0);
      await expect(page.getByTestId('system-snapshots-panel')).not.toBeVisible();
      await page.screenshot({path: info.outputPath(`backup-link-${narrow ? 'narrow' : 'desktop'}.png`), fullPage: true});
      await page.goto('/notifications?id=44444444-4444-4444-8444-444444444444');
      await expect(page.getByRole('button', {name: '查看恢复验证', exact: true}).last()).toBeVisible();
      await closeMobileSidebar();
      await page.getByRole('button', {name: '查看恢复验证', exact: true}).last().click();
      await expect(page).toHaveURL(/\/ops\/backup\?drill_id=99$/);
      await expect(page.getByText('隔离库验证报告（浏览器验收）')).toBeVisible();
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
      expect(errors).toEqual([]);
    } finally { await context.close(); }
  });
}
