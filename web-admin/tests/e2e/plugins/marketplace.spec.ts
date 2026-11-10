import { test, expect, type Page } from '@playwright/test';
import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
test.use({ actionTimeout: 10000 });

const tenant = '00000000-0000-4000-8000-000000000001';
const plugin = { id: 'com.powerx.market.acceptance', name: 'Marketplace Acceptance', summary: 'Marketplace integration test package', categories: ['开发者工具'], owner_dev_id: 'test-developer' };
const version = { id: 'version-test', plugin_id: plugin.id, semver: '1.2.0', channel: 'stable', min_core: '1.0.0', os: 'darwin', arch: 'arm64', sha256: 'a'.repeat(64), size: 1048576 };

async function prepare(page: Page, options: { auth?: boolean; versionError?: boolean; origin?: 'local' | 'marketplace' | 'unknown'; theme?: string; locale?: 'zh' | 'en'; otherProvider?: boolean; unconfirmed?: number; instanceState?: string } = {}) {
  const requests: string[] = [];
  let installedMarketVersion = '';
  let unconfirmed = options.unconfirmed ?? 0;
  const provider = { id: 'test-developer', authority: 'http://market.test/api/v1' };
  const localPlugins = [{ id: 'com.test.local', name: 'Local Plugin', version: '2.0.0', state: 'installed', install_origin: { type: 'local', source_id: 'local' } }, { id: 'com.test.legacy', name: 'Legacy Plugin', version: '0.1.0', state: 'disabled', install_origin: { type: 'unknown' } }];
  const installs: unknown[] = [];
  const errors: string[] = [];
  page.on('pageerror', e => errors.push(e.message));
  page.on('console', message => { if (['error', 'warning'].includes(message.type()) && /resolve component|hydration|invalid prop/i.test(message.text())) errors.push(message.text()); });
  await page.addInitScript(({ tenant, theme, locale }) => {
    const token = `eyJhbGciOiJub25lIn0.${btoa(JSON.stringify({ exp: Math.floor(Date.now()/1000)+3600, tid: tenant, sub: 'test-root' }))}.test`;
    localStorage.setItem('powerx.session', JSON.stringify({ version: 1, status: 'active', sessionId: 'marketplace-test', revision: 1, accessToken: token, refreshToken: 'test-refresh', tokenType: 'Bearer', expiresAt: Date.now()+3600000, scope: 'access', tenantUuid: tenant }));
    localStorage.setItem('px_locale', locale);
    localStorage.setItem('welcome_guide_completed', 'true');
    localStorage.setItem('powerx-color-mode', theme);
  }, { tenant, theme: options.theme || 'dark', locale: options.locale || 'zh' });
  await page.routeWebSocket(/\/api\/ws/, socket => socket.close());
  await page.route(url => url.pathname.startsWith('/api/'), async route => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (path.includes('/_nuxt_icon/')) { await route.fallback(); return; }
    let data: unknown = {};
    let status = 200;
    if (path.includes('/marketplace/')) requests.push(path);
    if (path.endsWith('/setup/status')) data = { configured: true, requires_login: true, version: '1.0.0' };
    else if (path.endsWith('/me/context')) data = {
      is_root: true, current_tenant_uuid: tenant, current_member_id: 1,
      user: { id: 1, display_name: 'Marketplace Tester', email: 'market-test@example.invalid', status: 1 },
      members: [{ tenant_uuid: tenant, tenant_name: 'Test tenant', member_id: 1, is_admin: true }],
    };
    else if (path.endsWith('/marketplace/catalog')) {
      const installation = installedMarketVersion ? { status: 'installed', version: installedMarketVersion, origin: { type: 'marketplace', source_id: provider.authority }, provider, installed_provider: provider, revision: 'new-revision', can_replace: true } : options.origin ? { status: options.otherProvider || options.origin === 'unknown' ? 'source_conflict' : 'installed', version: '1.0.0', origin: { type: options.origin }, provider, installed_provider: options.origin === 'unknown' ? {} : options.otherProvider ? { ...provider, id: 'other-developer' } : provider, revision: 'old-revision', can_replace: !options.otherProvider, reason: options.otherProvider ? 'provider_mismatch' : options.origin === 'unknown' ? 'provider_unconfirmed' : undefined } : { status: 'not_installed', origin: { type: 'unknown' }, provider };
      const filter = url.searchParams.get('installation');
      const items = !filter || filter === 'all' || filter === installation.status ? [{ ...plugin, installation }] : [];
      data = { inventory_ready: true, connection: { configured: true, download_ready: options.auth !== false, os: 'darwin', arch: 'arm64', core_version: '1.0.0', source_id: 'http://market.test/api/v1', instance: { state: options.instanceState || 'registered', pending_reports: 0, blocked_reports: 0, unconfirmed_installs: unconfirmed } }, items, total: items.length, page: 1, size: 12 };
    }
    else if (path.endsWith('/versions') && path.includes('/marketplace/')) {
      if (options.versionError) { status = 502; data = null; }
      else data = { items: [version, { ...version, id: 'v2', semver: '1.2.1' }], total: 2, page: 1, size: 20 };
    }
    else if (path.endsWith('/marketplace/install')) {
      installs.push(route.request().postDataJSON()); installedMarketVersion = (installs.at(-1) as {version:string}).version; data = { installed: { id: plugin.id, version: (installs.at(-1) as {version:string}).version, state: 'enabled', provider, revision: 'new-revision', install_origin: { type: 'marketplace', source_id: provider.authority } }, enabled: (installs.at(-1) as {enable:boolean}).enable };
    }
    else if (/\/admin\/plugins\/?$/.test(path)) data = { plugins: [...localPlugins, ...(installedMarketVersion ? [{ id: plugin.id, name: plugin.name, version: installedMarketVersion, state: 'installed', install_origin: { type: 'marketplace', source_id: 'http://market.test/api/v1' } }] : [])] };
    else if (path.endsWith('/menus')) data = { menus: [
      { id: 'market', title: '插件市场', path: '/plugins/market', visible: true, origin: 'system' },
      { id: 'local', title: '本地插件', path: '/plugins/local', visible: true, origin: 'system' },
      { id: 'installed', title: '已安装', path: '/plugins/installed', visible: true, origin: 'system' },
    ] };
    else if (path.endsWith('/marketplace/plugins')) data = { items: [] };
    await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify({ code: status, message: status === 502 ? 'Marketplace 接口返回 HTTP 500' : 'success', data }) });
  });
  return { requests, installs, errors, resolveResults: () => { unconfirmed = 0; } };
}

for (const scenario of [{width:1440,theme:'light',locale:'zh',state:'connecting'}, {width:390,theme:'dark',locale:'zh',state:'credential_rejected'}, {width:1440,theme:'dark',locale:'en',state:'connecting'}] as const) {
  test(`automatic registration notice at ${scenario.width}px in ${scenario.locale}`, async ({page},testInfo)=>{
    await page.setViewportSize({width:scenario.width,height:1000});
    const result=await prepare(page,{...scenario,auth:false,instanceState:scenario.state});
    await page.goto('/plugins/market');
    const notice=page.getByTestId('marketplace-registration-notice');
    await expect(notice).toBeVisible({timeout:30000});
    await expect(page.getByRole('dialog', {name:'...',exact:true})).toBeHidden({timeout:10000});
    if(scenario.width===390) await page.getByRole('button',{name:'收起',exact:true}).first().click();
    await notice.click({trial:true});
    await expect(notice).toContainText(scenario.state==='credential_rejected'?'市场已拒绝此实例凭证':scenario.locale==='en'?'No enrollment code is required':'无需手工配置接入码');
    const bounds=await notice.boundingBox();expect(bounds!.x).toBeGreaterThanOrEqual(0);expect(bounds!.x+bounds!.width).toBeLessThanOrEqual(scenario.width);
    await page.screenshot({animations:'disabled',path:testInfo.outputPath('automatic-registration.png')});
    expect(result.installs).toHaveLength(0);expect(result.errors).toEqual([]);
  });
}

for (const scenario of [{ width: 1440, theme: 'dark', locale: 'zh' }, { width: 390, theme: 'light', locale: 'zh' }, { width: 1440, theme: 'light', locale: 'en' }] as const) {
  test(`unconfirmed result notice at ${scenario.width}px in ${scenario.locale}`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width: scenario.width, height: scenario.width === 390 ? 844 : 1000 });
    const result = await prepare(page, { ...scenario, unconfirmed: 1 });
    await page.goto('/plugins/market');
    const notice = page.getByTestId('marketplace-result-unconfirmed');
    await expect(notice).toBeVisible({ timeout: 30000 });
    if (scenario.width === 390) await page.getByRole('button', { name: '收起', exact: true }).first().click();
    await expect(notice).toContainText(scenario.locale === 'en' ? 'Installation result unconfirmed' : '安装结果未确认');
    await expect(notice).toContainText(scenario.locale === 'en' ? 'do not reinstall' : '无需重复安装');
    const bounds = await notice.boundingBox(); expect(bounds!.x).toBeGreaterThanOrEqual(0); expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(scenario.width);
    expect(result.requests.filter(path => path.endsWith('/marketplace/catalog'))).toHaveLength(1);
    await page.screenshot({ animations: 'disabled', path: testInfo.outputPath('result-unconfirmed.png') });
    result.resolveResults();
    await page.getByRole('button', { name: scenario.locale === 'en' ? 'Refresh' : '刷新', exact: true }).click();
    await expect(notice).not.toBeVisible();
    expect(result.requests.filter(path => path.endsWith('/marketplace/catalog'))).toHaveLength(2);
    expect(result.installs).toHaveLength(0); expect(result.errors).toEqual([]);
  });
}

test('desktop: select version, install and return to installed management', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  const result = await prepare(page);
  await page.goto('/plugins/market');
  await expect(page.getByRole('heading', { name: '插件市场', exact: true })).toBeVisible({ timeout: 30000 });
  await expect(page.getByRole('navigation', { name: '插件管理', exact: true }).getByRole('link', { name: '本地插件', exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: plugin.name })).toBeVisible();
  await page.getByRole('button', { name: '查看版本' }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByText('darwin/arm64', { exact: true })).toBeVisible();
  await dialog.getByRole('combobox').click();
  await page.getByRole('option', { name: '1.2.1 · stable' }).click();
  await expect(dialog.getByRole('combobox')).toContainText('1.2.1');
  await page.screenshot({ animations: 'disabled', path: testInfo.outputPath('desktop-version-selection.png') });
  await dialog.getByRole('button', { name: '安装此版本' }).click();
  await expect(page.getByText(`${plugin.name} 1.2.1 已安装并启用`)).toBeVisible();
  expect(result.installs).toEqual([{ plugin_id: plugin.id, version: '1.2.1', enable: true }]);
  await expect(dialog).not.toBeVisible();
  await expect(page.getByRole('link', { name: '管理已安装插件' })).toBeVisible();
  await page.getByRole('link', { name: '管理已安装插件' }).click();
  await expect(page.getByRole('columnheader', { name: '安装来源' })).toBeVisible();
  const installedRow = page.getByRole('row').filter({ hasText: plugin.name });
  await expect(installedRow.getByText('插件市场', { exact: true })).toBeVisible();
  await expect(installedRow.getByText('http://market.test/api/v1', { exact: true })).toBeVisible();
  await page.getByRole('navigation', { name: '筛选安装来源' }).getByRole('button', { name: '本地安装 (1)', exact: true }).click();
  await expect(page.getByRole('row').filter({ hasText: 'Local Plugin' })).toBeVisible();
  await expect(page.getByRole('row').filter({ hasText: plugin.name })).toHaveCount(0);
  expect(result.errors).toEqual([]);
});

test('narrow screen: platform selection stays visible and download auth blocks installation', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const result = await prepare(page, { auth: false, theme: 'light' });
  await page.goto('/plugins/market');
  await expect(page.getByText('市场实例正在连接')).toBeVisible({ timeout: 30000 });
  await page.getByRole('button', { name: '收起', exact: true }).first().click();
  await page.getByRole('button', { name: '查看版本' }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByText('darwin/arm64', { exact: true })).toBeVisible();
  await expect(dialog.getByRole('button', { name: '安装此版本' })).toBeDisabled();
  await expect(dialog.getByText('市场实例正在连接')).toBeVisible();
  await page.screenshot({ animations: 'disabled', path: testInfo.outputPath('narrow-version-selection.png') });
  const bounds = await dialog.boundingBox(); expect(bounds!.width).toBeLessThanOrEqual(390);
  await dialog.getByRole('button', { name: '关闭', exact: true }).click();
  await expect(page.getByRole('heading', { name: plugin.name })).toBeVisible();
  expect(result.installs).toEqual([]); expect(result.errors).toEqual([]);
});

test('upstream version failure is visible and never becomes a local catalog fallback', async ({ page }) => {
  const result = await prepare(page, { versionError: true });
  await page.goto('/plugins/market');
  await expect(page.getByRole('heading', { name: '插件市场', exact: true })).toBeVisible({ timeout: 30000 });
  await page.getByRole('button', { name: '查看版本' }).click();
  await expect(page.getByRole('dialog').getByText('Marketplace 接口返回 HTTP 500')).toBeVisible();
  await expect(page.getByRole('dialog').getByRole('button', { name: '安装此版本' })).toBeDisabled();
  expect(result.installs).toEqual([]); expect(result.requests.some(path => path.endsWith('/marketplace/plugins'))).toBe(false);
});


test('a known different publisher cannot replace a same-ID local plugin', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  const result = await prepare(page, { origin: 'local', otherProvider: true });
  await page.goto('/plugins/market');
  await expect(page.getByRole('heading', { name: '插件市场', exact: true })).toBeVisible({ timeout: 30000 });
  await expect(page.getByText('发布者不一致', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '查看版本' }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByText('当前安装与市场包属于不同发布者，不能覆盖。', { exact: true })).toBeVisible();
  await expect(dialog.getByRole('button', { name: '安装此版本' })).toHaveCount(0);
  await expect(dialog.getByRole('link', { name: '管理已安装插件' })).toBeVisible();
  await page.screenshot({ animations: 'disabled', path: testInfo.outputPath('desktop-source-conflict.png') });
  await dialog.getByRole('button', { name: '关闭', exact: true }).click();
  await page.getByRole('button', { name: '已安装', exact: true }).click();
  await expect(page.getByRole('heading', { name: plugin.name })).toHaveCount(0);
  await page.getByRole('button', { name: '身份待处理', exact: true }).click();
  await expect(page.getByRole('heading', { name: plugin.name })).toBeVisible();
  expect(result.installs).toEqual([]); expect(result.errors).toEqual([]);
});

test('local workflow on narrow screen never requests Marketplace and keeps group search context', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const result = await prepare(page, { theme: 'light' });
  await page.goto('/plugins/local');
  await expect(page.getByRole('heading', { name: '本地插件', exact: true })).toBeVisible({ timeout: 30000 });
  await page.getByRole('button', { name: '收起', exact: true }).first().click();
  await expect(page.getByRole('navigation', { name: '插件管理', exact: true }).getByRole('link', { name: '插件市场', exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Local Plugin', exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Legacy Plugin', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: '全部 (2)', exact: true })).toHaveAttribute('aria-pressed', 'true');
  const search = page.getByPlaceholder('搜索插件名称或 ID');
  await search.fill('Plugin');
  await page.getByRole('button', { name: '来源未知 (1)', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Legacy Plugin', exact: true })).toBeVisible();
  await expect(search).toHaveValue('Plugin');
  const manage = page.getByTestId('plugin-section-content').getByRole('link', { name: '管理', exact: true });
  await manage.scrollIntoViewIfNeeded();
  await expect(manage).toBeInViewport();
  await page.screenshot({ animations: 'disabled', path: testInfo.outputPath('narrow-local-unknown-source.png') });
  await page.getByRole('button', { name: '本地安装 (1)', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Local Plugin', exact: true })).toBeVisible();
  await expect(search).toHaveValue('Plugin');
  const width = await page.evaluate(() => ({ content: document.documentElement.scrollWidth, viewport: innerWidth }));
  expect(width.content).toBeLessThanOrEqual(width.viewport);
  expect(result.requests).toEqual([]); expect(result.errors).toEqual([]);
});


test('Chinese pages share navigation geometry, show all first, and keep origin labels horizontal', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  const result = await prepare(page, { origin: 'marketplace' });
  const geometry: { x: number; y: number; width: number; contentX: number }[] = [];
  for (const [route, title] of [['market', '插件市场'], ['local', '本地插件'], ['installed', '已安装插件']]) {
    await page.goto(`/plugins/${route}`);
    await expect(page.getByRole('heading', { name: title, exact: true }).first()).toBeVisible({ timeout: 30000 });
    const navigation = page.getByRole('navigation', { name: '插件管理', exact: true });
    await expect(navigation).not.toContainText('Marketplace');
    await expect(navigation.getByRole('link', { name: '插件市场', exact: true })).toBeVisible();
    const rail = page.getByTestId('plugin-section-rail');
    await expect(rail.getByRole('button').first()).toHaveAttribute('aria-pressed', 'true');
    const bounds = await rail.boundingBox();
    const contentBounds = await page.getByTestId('plugin-section-content').boundingBox();
    geometry.push({ x: bounds!.x, y: bounds!.y, width: bounds!.width, contentX: contentBounds!.x });
    if (route === 'local') {
      await expect(page.getByRole('heading', { name: 'Legacy Plugin' })).toBeVisible();
      await expect(page.getByRole('heading', { name: 'Local Plugin' })).toBeVisible();
    }
    if (route === 'installed') {
      const unknown = page.getByRole('row').filter({ hasText: 'Legacy Plugin' }).getByText('来源未知', { exact: true });
      const badgeBounds = await unknown.boundingBox();
      expect(badgeBounds!.width).toBeGreaterThan(badgeBounds!.height);
      await expect(page.getByRole('row').filter({ hasText: 'Legacy Plugin' })).not.toContainText('disabled');
      const content = await page.getByTestId('plugin-section-content').boundingBox();
      const manage = await page.getByRole('row').filter({ hasText: 'Legacy Plugin' }).getByRole('link', { name: '管理', exact: true }).boundingBox();
      expect(manage!.x + manage!.width).toBeLessThanOrEqual(content!.x + content!.width);
      const tableScroll = await page.locator('.installed-table').evaluate(el => ({ width: el.clientWidth, scroll: el.scrollWidth }));
      expect(tableScroll.scroll).toBeLessThanOrEqual(tableScroll.width + 1);

    }
    await page.screenshot({ animations: 'disabled', path: testInfo.outputPath(`desktop-${route}-redesign.png`) });
  }
  for (const item of geometry.slice(1)) {
    expect(item.x).toBe(geometry[0]!.x); expect(item.width).toBe(208);
    expect(item.contentX).toBe(geometry[0]!.contentX);
    expect(Math.abs(item.y - geometry[0]!.y)).toBeLessThanOrEqual(1);
  }
  expect(result.errors).toEqual([]);
});

test('English locale translates source navigation and headings', async ({ page }) => {
  const result = await prepare(page, { locale: 'en', theme: 'light' });
  await page.goto('/plugins/local');
  await expect(page.getByRole('heading', { name: 'Local plugins', exact: true })).toBeVisible({ timeout: 30000 });
  const navigation = page.getByRole('navigation', { name: 'Plugin management', exact: true });
  await expect(navigation.getByRole('link', { name: 'Plugin marketplace', exact: true })).toBeVisible();
  await expect(navigation).not.toContainText('插件');
  expect(result.errors).toEqual([]);
});

for (const legacy of [false, true]) {
 test(`same publisher replacement${legacy ? ' with explicit legacy binding' : ''}`, async ({page},testInfo) => {
  await page.setViewportSize({width:1440,height:1000});
  const result = await prepare(page,{origin:legacy ? 'unknown':'local'});
  await page.goto('/plugins/market');
  await expect(page.getByRole('heading',{name:plugin.name})).toBeVisible({timeout:30000});
  await page.getByRole('button',{name:'查看版本'}).click();
  const dialog=page.getByRole('dialog');
  const replace=dialog.getByRole('button',{name:'替换当前安装',exact:true});
  await expect(replace).toBeDisabled();
  await dialog.getByRole('checkbox',{name:'确认替换当前安装及其代码，保留原配置与业务数据'}).check();
  if(legacy) {
   await expect(replace).toBeDisabled();
   await dialog.getByRole('checkbox',{name:'我已核对，当前安装属于上述发布者，同意建立身份绑定'}).check();
  }
  await expect(replace).toBeEnabled();
  await page.screenshot({animations:'disabled',path:testInfo.outputPath(legacy ? 'legacy-provider-binding.png':'same-publisher-replacement.png')});
  await replace.click();
  await expect(dialog).not.toBeVisible();
  expect(result.installs).toEqual([{plugin_id:plugin.id,version:'1.2.0',enable:true,replace:true,expected_revision:'old-revision',...(legacy ? {confirm_legacy_provider:true}:{})}]);
  expect(result.errors).toEqual([]);
 });
}

for (const width of [1440, 390]) {
 test(`manual URL installation previews replacement before confirmation at ${width}px`, async ({page},testInfo)=>{
  await page.setViewportSize({width,height:width===390?844:1000});
  await prepare(page,{theme:width===390?'light':'dark'});
  const requests: Record<string,unknown>[]=[];
  await page.route('**/api/v1/admin/plugins/install/url',async route=>{
   requests.push(route.request().postDataJSON());
   const preview={code:'PLUGIN_REPLACEMENT_REQUIRED',plugin_id:plugin.id,requires_provider_confirmation:true,current:{version:'1.0.0',revision:'old-revision',provider:{},origin:{type:'unknown'}},target:{version:'1.2.0',provider:{id:'test-developer',authority:'http://market.test/api/v1'},origin:{type:'local'}}};
   await route.fulfill({status:requests.length===1?409:200,contentType:'application/json',body:JSON.stringify(requests.length===1?{code:409,message:'请确认替换',details:preview}:{code:200,data:{installed:{id:plugin.id,version:'1.2.0'},enabled:false}})});
  });
  await page.goto('/plugins/local');
  await expect(page.getByRole('heading',{name:'本地插件',exact:true})).toBeVisible({timeout:30000});
  if(width===390) await page.getByRole('button',{name:'收起',exact:true}).first().click();
  await page.getByRole('button',{name:'本地安装',exact:true}).click();
  const dialog=page.getByRole('dialog');
  await dialog.getByRole('combobox').first().click();
  await page.getByRole('option',{name:'远程 URL',exact:true}).click();
  await dialog.getByPlaceholder('https://example.com/plugin.zip').fill('https://packages.test/plugin.pxp');
  const install=dialog.getByRole('button',{name:'安装',exact:true});
  await install.click();
  await expect(dialog.getByText('当前安装',{exact:true})).toBeVisible();
  await expect(install).toBeDisabled();
  await dialog.getByRole('switch').last().check();
  await expect(install).toBeDisabled();
  await dialog.getByRole('checkbox',{name:'我已核对，当前安装属于上述发布者，同意建立身份绑定'}).check();
  await expect(install).toBeEnabled();
  await install.scrollIntoViewIfNeeded();
  await page.screenshot({animations:'disabled',path:testInfo.outputPath(`local-replacement-${width}.png`)});
  await install.click();
  await expect(dialog).not.toBeVisible();
  expect(requests).toHaveLength(2);
  expect(requests[0]).toMatchObject({force:false,confirm_legacy_provider:false});
  expect(requests[1]).toMatchObject({force:true,expected_revision:'old-revision',confirm_legacy_provider:true});
 });
}


for (const width of [1440,390]) {
 for (const archive of [false,true]) {
  test(`local upload keeps original multipart flow at ${width}px (${archive ? 'archive' : 'directory'})`,async({page},testInfo)=>{
   const fixture=await mkdtemp(join(tmpdir(),'powerx-upload-compat-'));
   try {
    if(archive) await writeFile(join(fixture,'plugin.tar.gz'),'controlled archive payload');
    else {
     await mkdir(join(fixture,'backend/bin'),{recursive:true});
     await writeFile(join(fixture,'plugin.yaml'),'id: com.test.upload\nversion: 0.1.0\n');
     await writeFile(join(fixture,'backend/bin/plugin'),'controlled plugin executable');
    }
    await page.setViewportSize({width,height:width===390?844:1000});
    const result=await prepare(page,{theme:width===390?'light':'dark'});
    const uploads:FormData[]=[];
    await page.route('**/api/v1/admin/plugins/install/local',async route=>{
     const body=route.request().postDataBuffer()!;
     uploads.push(await new Response(body,{headers:{'content-type':route.request().headers()['content-type']!}}).formData());
     await route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({code:200,data:{installed:{id:'com.test.upload',version:'0.1.0',state:'enabled'},enabled:true}})});
    });
    await page.goto('/plugins/local');
    await expect(page.getByRole('heading',{name:'本地插件',exact:true})).toBeVisible({timeout:30000});
    if(width===390) await page.getByRole('button',{name:'收起',exact:true}).first().click();
    await page.getByRole('button',{name:'本地安装',exact:true}).click();
    const dialog=page.getByRole('dialog');
    await dialog.getByRole('combobox').first().click();
    await page.getByRole('option',{name:'本地上传',exact:true}).click();
    await expect(dialog.getByRole('switch').last()).toBeChecked();
    await expect(dialog.getByRole('switch').last()).toBeEnabled();
    await dialog.locator('input[type="file"]').setInputFiles(fixture);
    await expect(dialog.getByRole('switch').last()).toBeChecked();
    await expect(dialog.getByRole('switch').first()).toBeChecked();
    await expect(dialog.getByText('当前安装',{exact:true})).toHaveCount(0);
    await expect(dialog.getByRole('checkbox',{name:'我已核对，当前安装属于上述发布者，同意建立身份绑定'})).toHaveCount(0);
    const install=dialog.getByRole('button',{name:'安装',exact:true});
    await expect(install).toBeEnabled();await install.scrollIntoViewIfNeeded();
    await page.screenshot({animations:'disabled',path:testInfo.outputPath(`local-upload-${width}-${archive?'archive':'directory'}.png`)});
    await install.click();await expect(dialog).not.toBeVisible();
    expect(uploads).toHaveLength(1);
    expect(uploads[0]!.get('force')).toBe('true');expect(uploads[0]!.get('enable')).toBe('true');
    expect(uploads[0]!.has('expected_revision')).toBe(false);expect(uploads[0]!.has('confirm_legacy_provider')).toBe(false);
    expect(uploads[0]!.getAll('files')).toHaveLength(archive?1:2);
    expect(uploads[0]!.get('file_paths_json')).toContain(archive?'plugin.tar.gz':'plugin.yaml');
    expect(JSON.parse(String(uploads[0]!.get('metadata')))).toMatchObject({scope:'system',release_channel:'stable',auto_update:true});
    expect(result.requests).toEqual([]);expect(result.errors).toEqual([]);
   } finally {await rm(fixture,{recursive:true,force:true});}
  });
 }
}
