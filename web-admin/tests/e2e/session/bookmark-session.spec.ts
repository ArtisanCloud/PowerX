import { test, expect, type Browser, type Page } from '@playwright/test';

const pluginId = 'com.powerx.plugins.ai-craft';
const path = `/_p/${pluginId}/admin/operations/ai-craft/tracks`;
const bookmark = path;
const tenantUuid = 'session-browser-fixture';

function session(expired = false) {
  const expiresAt = Date.now() + (expired ? -60_000 : 3600_000);
  return {
    version: 1, sessionId: 'bookmarked-host-session', revision: 4, status: 'active',
    accessToken: `header.${Buffer.from(JSON.stringify({ tid: tenantUuid, exp: Math.floor(expiresAt / 1000) })).toString('base64url')}.fixture`,
    refreshToken: 'fixture-refresh', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid,
  };
}
async function openContext(browser: Browser, baseURL: string, snapshot: ReturnType<typeof session> | null) {
  return browser.newContext({ baseURL, locale: 'zh-CN', storageState: {
    cookies: [], origins: [{ origin: baseURL, localStorage: snapshot ? [{ name: 'powerx.session', value: JSON.stringify(snapshot) }] : [] }],
  } });
}
async function ready(page: Page) {
  await expect(page.frameLocator('iframe').getByTestId('plugin-ready')).toBeVisible();
  await expect(page.locator('iframe')).toBeVisible();
}
async function documents(page: Page) {
  const response = await page.request.get('/api/v1/__session_test/state');
  return (await response.json()).requests.filter((item: { path: string }) => item.path.startsWith(`/_p/${pluginId}/admin`));
}

test.beforeEach(async ({ request }) => { await request.delete('/api/v1/__session_test/state'); });

for (const [description, entry] of [
  ['the exact saved tracks address', bookmark],
  ['a saved tracks address with query and hash', `${bookmark}?view=sample#saved`],
] as const) {
test(`${description} restores missing cookies before its first iframe request`, async ({ browser, baseURL }, testInfo) => {
  testInfo.annotations.push({ type: 'native-browser', description: `${process.platform}; ${testInfo.project.name}; ${browser.version()}` });
  const snapshot = session();
  const context = await openContext(browser, baseURL!, snapshot);
  try {
    const page = await context.newPage();
    const response = await page.goto(entry);
    expect(response?.request().headers().referer).toBeUndefined();
    await ready(page);
    await expect(page).toHaveURL(`${baseURL}${entry}`);
    expect(await page.evaluate(() => JSON.parse(localStorage.getItem('powerx.session')!))).toEqual(snapshot);
    expect((await documents(page)).map((item: { hasBearer: boolean }) => item.hasBearer)).toEqual([true]);
    const frameURL = new URL((await page.locator('iframe').getAttribute('src'))!, baseURL);
    expect(frameURL.searchParams.get('__px_iframe')).toBe('1');
    frameURL.searchParams.delete('__px_iframe');
    expect(frameURL.pathname + frameURL.search + frameURL.hash).toBe(entry);
  } finally { await context.close(); }
});
}

test('refreshing the bookmark after removing session cookies restores credentials again', async ({ browser, baseURL }) => {
  const context = await openContext(browser, baseURL!, session());
  try {
    const page = await context.newPage();
    await page.goto(bookmark); await ready(page);
    await context.clearCookies();
    await page.reload(); await ready(page);
    expect((await documents(page)).map((item: { hasBearer: boolean }) => item.hasBearer)).toEqual([true, true]);
  } finally { await context.close(); }
});

test('an expired bookmarked host session renews before loading its iframe', async ({ browser, baseURL }) => {
  const snapshot = session(true);
  const context = await openContext(browser, baseURL!, snapshot);
  try {
    const page = await context.newPage();
    await page.goto(bookmark); await ready(page);
    const restored = await page.evaluate(() => JSON.parse(localStorage.getItem('powerx.session')!));
    expect(restored.sessionId).toBe(snapshot.sessionId);
    expect(restored.revision).toBe(snapshot.revision + 1);
    expect(restored.expiresAt).toBeGreaterThan(Date.now());
    expect((await documents(page)).map((item: { hasBearer: boolean }) => item.hasBearer)).toEqual([true]);
  } finally { await context.close(); }
});

test('a bookmark with no host login goes to login without requesting a plugin document', async ({ browser, baseURL }) => {
  const context = await openContext(browser, baseURL!, null);
  try {
    const page = await context.newPage();
    await page.goto(bookmark);
    await expect(page).toHaveURL(/\/users\/login\?/);
    expect(await documents(page)).toEqual([]);
    await expect(page.locator('iframe')).toHaveCount(0);
  } finally { await context.close(); }
});

test('a rejected plugin document reports authentication instead of a bridge timeout', async ({ browser, baseURL, request }) => {
  await request.put('/api/v1/__session_test/state', { data: { rejectDocuments: true } });
  const context = await openContext(browser, baseURL!, session());
  try {
    const page = await context.newPage();
    await page.goto(bookmark);
    await expect(page.getByText('插件页面未获得有效登录凭证')).toBeVisible();
    await expect(page.getByText('插件页面会话连接超时')).toHaveCount(0);
    expect(await documents(page)).toHaveLength(1);
  } finally { await context.close(); }
});
