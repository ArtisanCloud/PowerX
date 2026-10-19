import { test, expect, type Browser } from "@playwright/test";

// UI fixture coverage only; Go tests exercise real encryption, JWT and native PostgreSQL.
async function open(browser: Browser, baseURL: string, mobile = false) {
  const expiresAt = Date.now() + 3600_000;
  const tenantUuid = "snapshot-browser-fixture";
  const session = {
    version: 1,
    sessionId: "snapshot-browser-session",
    revision: 1,
    status: "active",
    accessToken: `header.${Buffer.from(JSON.stringify({ tid: tenantUuid, exp: Math.floor(expiresAt / 1000) })).toString("base64url")}.fixture`,
    refreshToken: "fixture",
    tokenType: "Bearer",
    expiresAt,
    scope: "access",
    tenantUuid,
  };
  const context = await browser.newContext({
    baseURL,
    locale: "zh-CN",
    viewport: mobile
      ? { width: 390, height: 844 }
      : { width: 1440, height: 1000 },
    storageState: {
      cookies: [],
      origins: [
        {
          origin: baseURL,
          localStorage: [
            { name: "welcome_guide_completed", value: "true" },
            { name: "powerx.session", value: JSON.stringify(session) },
            { name: "nuxt-color-mode", value: mobile ? "dark" : "light" },
          ],
        },
      ],
    },
  });
  const page = await context.newPage();
  await page.goto("/ops/backup");
  const skip = page.getByRole("button", { name: "跳过引导", exact: true });
  if (await skip.isVisible()) await skip.click();
  await expect(page.getByTestId("system-snapshots-panel")).toBeVisible();
  if (mobile) {
    const collapse = page
      .getByRole("complementary")
      .getByRole("button", { name: "收起", exact: true })
      .first();
    const box = await collapse.boundingBox();
    if (box && box.x >= 0) await collapse.click();
  }
  return { context, page };
}
test.beforeEach(async ({ request }) => {
  await request.delete("/api/v1/__snapshot_browser/state");
});

for (const mobile of [false, true]) {
  test(`permanently delete one snapshot with confirmation on ${mobile ? 'narrow' : 'desktop'} screen`, async ({ browser, baseURL, request }, info) => {
    const { context, page } = await open(browser, baseURL!, mobile);
    const panel = page.getByTestId('system-snapshots-panel');
    try {
      const row = panel.getByTestId('system-snapshot').first();
      await row.getByTestId('snapshot-delete').click();
      let dialog = page.getByRole('dialog');
      await expect(dialog.getByRole('button', { name: '确认永久删除', exact: true })).toBeDisabled();
      await dialog.getByRole('button', { name: '取消', exact: true }).click();
      let state = await (await request.get('/api/v1/__snapshot_browser/state')).json();
      expect(state.requests.filter((r:any) => r.method === 'DELETE' && r.path.includes('/system-snapshots/'))).toHaveLength(0);
      await row.getByTestId('snapshot-delete').click();dialog=page.getByRole('dialog');
      await expect(dialog).toContainText('数据库、媒体、加密配置和程序副本');
      await expect(dialog).toContainText('操作审计和恢复历史保留');
      await dialog.getByRole('checkbox').check();
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
      await page.screenshot({ path: info.outputPath(`snapshot-delete-${mobile ? 'narrow' : 'desktop'}.png`), fullPage: true, animations: 'disabled' });
      await dialog.getByRole('button', { name: '确认永久删除', exact: true }).click();
      await expect(panel.getByTestId('system-snapshot')).toHaveCount(0);
      await expect(panel.getByTestId('snapshot-operation-success')).toContainText('对应文件已永久删除');
      await expect(panel.getByTestId('snapshot-policy-table')).toBeVisible();
      state = await (await request.get('/api/v1/__snapshot_browser/state')).json();
      const deletion = state.requests.find((r:any) => r.method === 'DELETE' && r.path.includes('/system-snapshots/'));
      expect(deletion.confirmation).toBe('9282f978-33e4-4d7e-a36f-8e8f39536251');
      expect(deletion.deleteProtected).toBe('false');
      await page.reload();
      await expect(page.getByTestId('system-snapshot')).toHaveCount(0);
    } finally { await context.close(); }
  });
}

test('protected snapshot deletion explains the rollback loss and supports a failed cleanup retry', async ({ browser, baseURL, request }) => {
  await request.post('/api/v1/__snapshot_browser/push',{data:{protected:true}});
  await request.post('/api/v1/__snapshot_browser/delete-failure',{data:{enabled:true}});
  const { context, page } = await open(browser, baseURL!);
  try {
    const panel = page.getByTestId('system-snapshots-panel');
    await panel.getByTestId('snapshot-delete').click();
    const dialog=page.getByRole('dialog');
    await expect(dialog).toContainText('这是一份受保护快照');
    await dialog.getByRole('checkbox').check();
    await dialog.getByRole('button',{name:'确认永久删除',exact:true}).click();
    await expect(dialog).toContainText('快照文件清理未完成');
    await expect(panel.getByTestId('system-snapshot')).toHaveCount(1);
    await request.post('/api/v1/__snapshot_browser/delete-failure',{data:{enabled:false}});
    await dialog.getByRole('button',{name:'确认永久删除',exact:true}).click();
    await expect(panel.getByTestId('system-snapshot')).toHaveCount(0);
    const state = await (await request.get('/api/v1/__snapshot_browser/state')).json();
    expect(state.requests.filter((r:any)=>r.method==='DELETE'&&r.path.includes('/system-snapshots/')).every((r:any)=>r.deleteProtected==='true')).toBe(true);
  } finally { await context.close(); }
});

for (const mobile of [false,true]) {
  test(`ended failed recovery history does not prevent deleting protected, ready or failed snapshots on ${mobile?'narrow':'desktop'} screen`,async({browser,baseURL,request},info)=>{
    await request.post('/api/v1/__snapshot_browser/managed');
    const statuses=['failed','failed','failed','failed','complete'];
    await request.post('/api/v1/__snapshot_browser/recovery-history',{data:{statuses,extraSnapshots:true}});
    const {context,page}=await open(browser,baseURL!,mobile);
    try {
      const panel=page.getByTestId('system-snapshots-panel');
      const buttons=panel.getByTestId('snapshot-delete');
      await expect(buttons).toHaveCount(3);
      for(const button of await buttons.all())await expect(button).toBeEnabled();
      for(const status of ['queued','running','attention_required']) {
        await request.post('/api/v1/__snapshot_browser/recovery-history',{data:{statuses:[...statuses,status]}});
        for(const button of await buttons.all())await expect(button).toBeDisabled();
        await request.post('/api/v1/__snapshot_browser/recovery-history',{data:{statuses}});
        for(const button of await buttons.all())await expect(button).toBeEnabled();
      }
      await panel.getByTestId('system-snapshot').filter({hasText:'历史快照采集失败'}).getByTestId('snapshot-delete').click();
      let dialog=page.getByRole('dialog');await dialog.getByRole('checkbox').check();
      await dialog.getByRole('button',{name:'确认永久删除',exact:true}).click();
      await expect(panel.getByTestId('system-snapshot')).toHaveCount(2);
      await panel.getByTestId('system-snapshot').filter({hasText:'受保护'}).getByTestId('snapshot-delete').click();
      dialog=page.getByRole('dialog');
      await expect(dialog).toContainText('不参与自动清理，仍可手动确认删除');
      await expect(dialog.getByRole('button',{name:'确认永久删除',exact:true})).toBeDisabled();
      await dialog.getByRole('checkbox').check();
      await page.screenshot({path:info.outputPath(`protected-delete-after-history-${mobile?'narrow':'desktop'}.png`),fullPage:true,animations:'disabled'});
      await dialog.getByRole('button',{name:'确认永久删除',exact:true}).click();
      await expect(panel.getByTestId('system-snapshot')).toHaveCount(1);
      await panel.getByTestId('snapshot-delete').click();dialog=page.getByRole('dialog');
      await dialog.getByRole('checkbox').check();await dialog.getByRole('button',{name:'确认永久删除',exact:true}).click();
      await expect(panel.getByTestId('system-snapshot')).toHaveCount(0);
      const state=await(await request.get('/api/v1/__snapshot_browser/state')).json();
      const deletions=state.requests.filter((r:any)=>r.method==='DELETE'&&r.path.includes('/system-snapshots/'));
      expect(deletions).toHaveLength(3);
      expect(deletions.map((r:any)=>r.deleteProtected)).toEqual(['false','true','false']);
      expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
    }finally{await context.close();}
  });
}

test("active snapshot progress uses push without periodic status requests and closes on departure", async ({browser,baseURL,request}) => {
  await request.post('/api/v1/__snapshot_browser/push',{data:{status:'capturing',phase:'database_and_media'}});
  const {context,page}=await open(browser,baseURL!);
  const panel=page.getByTestId('system-snapshots-panel');
  try {
    await expect(panel.getByText('正在采集',{exact:true})).toBeVisible();
    await expect.poll(async()=> (await (await request.get('/api/v1/__snapshot_browser/state')).json()).eventConnections).toBe(1);
    await page.waitForTimeout(300);
    const before=await(await request.get('/api/v1/__snapshot_browser/state')).json();
    const count=(state:any)=>state.requests.filter((item:any)=>item.method==='GET'&&item.path==='/api/v1/admin/ops/backup/system-snapshots').length;
    await page.waitForTimeout(5500);
    const quiet=await(await request.get('/api/v1/__snapshot_browser/state')).json();
    expect(count(quiet)).toBe(count(before));
    await panel.getByRole('tab',{name:'概览',exact:true}).click();
    await request.post('/api/v1/__snapshot_browser/push',{data:{status:'ready',phase:'complete'}});
    await panel.getByRole('tab',{name:'快照备份',exact:true}).click();
    await expect(panel.getByText('快照包可用',{exact:true})).toBeVisible();
    const changed=await(await request.get('/api/v1/__snapshot_browser/state')).json();
    expect(count(changed)).toBeGreaterThan(count(quiet));
    expect(changed.requests.filter((item:any)=>item.path==='/api/v1/admin/ops/backup/events')).toHaveLength(1);
  } finally { await context.close(); }
  await expect.poll(async()=> (await (await request.get('/api/v1/__snapshot_browser/state')).json()).eventConnections).toBe(0);
});

test("snapshot storage location and configuration help are visible on desktop and mobile", async ({browser,baseURL,request},info) => {
 for(const mobile of [false,true]){
  await request.delete('/api/v1/__snapshot_browser/state');
  const {context,page}=await open(browser,baseURL!,mobile);
  try {
   const panel=page.getByTestId('system-snapshots-panel');
   const storage=panel.getByTestId('snapshot-storage');
   await expect(storage.getByTestId('snapshot-storage-path')).toHaveText('/var/lib/powerx/snapshot fixture/store');
   await expect(storage.getByText('来源：YAML 配置',{exact:true})).toBeVisible();
   await expect(page.getByText('启动中…',{exact:true})).toBeHidden();
   const box=await storage.boundingBox();expect(box).not.toBeNull();expect(box!.x).toBeGreaterThanOrEqual(0);expect(box!.x+box!.width).toBeLessThanOrEqual(mobile?390:1440);
   await storage.scrollIntoViewIfNeeded();
   await page.screenshot({path:info.outputPath(`snapshot-storage-${mobile?'mobile':'desktop'}.png`),fullPage:true});
   await storage.getByRole('button',{name:'配置说明',exact:true}).click();
   const dialog=page.getByRole('dialog');
   await expect(dialog.getByTestId('snapshot-storage-help')).toContainText('system_snapshots:');
   await expect(dialog).toContainText('POWERX_SNAPSHOT_DIRECTORY');
   await expect(dialog).toContainText('snapshot_directory');
   await expect(dialog).toContainText('不会自动搬迁旧快照');
   await dialog.evaluate(async node=>{await Promise.all(node.getAnimations({subtree:true}).map(a=>a.finished.catch(()=>{})));});
   await page.screenshot({path:info.outputPath(`snapshot-storage-help-${mobile?'mobile':'desktop'}.png`),fullPage:true});
  }finally{await context.close();}
 }
});

test("queued capture failure updates the open details from push", async ({browser,baseURL,request},info) => {
 await request.post('/api/v1/__snapshot_browser/push',{data:{status:'queued',phase:'queued'}});
 const {context,page}=await open(browser,baseURL!,true);
 try {
  const panel=page.getByTestId('system-snapshots-panel');
  await expect(panel.getByText('等待执行',{exact:true})).toBeVisible();
  await panel.getByRole('button',{name:'查看详情',exact:true}).first().click();
  const dialog=page.getByRole('dialog');
  await expect(dialog.getByText('等待执行',{exact:true})).toBeVisible();
  const error='程序产物采集失败（plugins/example/backend/bin）: 程序产物软链接越界（escape）';
  await request.post('/api/v1/__snapshot_browser/push',{data:{status:'failed',phase:'failed',error}});
  await expect(dialog.getByText('失败',{exact:true})).toBeVisible();
  await expect(dialog.getByText(error,{exact:true})).toBeVisible();
  await expect(panel.getByTestId('system-snapshot')).toContainText(error);
  await dialog.evaluate(async node=>{await Promise.all(node.getAnimations({subtree:true}).map(a=>a.finished.catch(()=>{})));});
  await page.screenshot({path:info.outputPath('snapshot-pushed-failure-mobile.png'),fullPage:true});
 } finally {await context.close();}
});

test('on-demand database and media exports work without policies on desktop and mobile', async ({browser,baseURL,request},info)=>{
 for(const mobile of [false,true]){
  await request.delete('/api/v1/__snapshot_browser/state');
  await request.post('/api/v1/__snapshot_browser/empty-policies');
  const {context,page}=await open(browser,baseURL!,mobile);
  const choose=async(scope:any,value:string,label:string)=>mobile?scope.locator('[data-section-select]').selectOption(value):scope.getByRole('tab',{name:label,exact:true}).click();
  try{
   await page.getByRole('tab',{name:'数据库备份',exact:true}).click();
   const database=page.getByTestId('backup-operations');
   await expect(database.getByTestId('database-on-demand')).toBeVisible();
   await expect(database.locator('[data-section="policies"]')).not.toBeVisible();
   await database.getByRole('button',{name:'备份一次',exact:true}).click();
   await page.getByRole('dialog').getByRole('button',{name:'确认执行',exact:true}).click();
   const row=database.getByRole('row').filter({hasText:'on-demand-database.dump'});
   await expect(row).toBeVisible();await expect(row.getByText('按需执行',{exact:true})).toBeVisible();
   const dbDownload=page.waitForEvent('download');await row.getByRole('button',{name:'下载数据库',exact:true}).click();
   expect((await dbDownload).suggestedFilename()).toContain('9001');
   await choose(database,'policies','高级设置');await expect(database.getByText('暂无策略；点击“新建策略”开始配置。',{exact:true})).toBeVisible();
   await page.getByRole('tab',{name:'媒体与日志',exact:true}).click();
   const media=page.getByTestId('resource-archives');
   await expect(media.getByTestId('media-on-demand')).toBeVisible();
   await media.getByRole('button',{name:'生成媒体导出包',exact:true}).click();
   await page.getByRole('dialog').getByRole('button',{name:'确认执行',exact:true}).click();
   const artifact=media.getByTestId('archive-job');await expect(artifact).toBeVisible();await expect(artifact.getByRole('button',{name:'恢复到新目录',exact:true})).toBeVisible();await expect(artifact.getByText('仅媒体文件',{exact:false})).toBeVisible();
   const mediaDownload=page.waitForEvent('download');await artifact.getByRole('button',{name:'下载 .tar.gz',exact:true}).click();
   expect((await mediaDownload).suggestedFilename()).toMatch(/\.tar\.gz$/);
   await page.screenshot({path:info.outputPath(mobile?'on-demand-mobile.png':'on-demand-desktop.png'),fullPage:true});
   const state=await(await request.get('/api/v1/__snapshot_browser/state')).json();
   expect(state.requests.find((r:any)=>r.path.endsWith('/jobs/run'))?.body).toEqual({one_time:true});
   expect(state.requests.find((r:any)=>r.path.endsWith('/archives/jobs')&&r.method==='POST')?.body).toEqual({source_id:'media'});
   expect(state.requests.filter((r:any)=>r.method==='POST'&&r.path.endsWith('/policies'))).toHaveLength(0);
  }finally{await context.close();}
 }
});

test("database backups default to registered files and retain unavailable executions in history", async ({browser, baseURL}, info) => {
  for (const mobile of [false, true]) {
    const {context, page} = await open(browser, baseURL!, mobile);
    try {
      await page.getByRole('tab', {name: '数据库备份', exact: true}).click();
      if(mobile) await page.getByTestId('backup-operations').locator('[data-section-select]').selectOption('files'); else await page.getByRole('tab',{name:'备份文件',exact:true}).click();
      const scope = page.getByTestId('backup-job-views');
      await expect(page.getByText('legacy-available.dump', {exact: true})).toBeVisible();
      await expect(page.getByRole('row').filter({hasText: '#799'})).toHaveCount(0);
      await expect(page.getByRole('row').filter({hasText: '#798'})).toHaveCount(0);
      await scope.getByRole('button', {name:'执行历史', exact:true}).click();
      const history = page.getByRole('row').filter({hasText:'#799'});
      await expect(history).toBeVisible();
      await expect(history.getByText('无文件记录', {exact:true})).toBeVisible();
      await expect(history.getByRole('button', {name:'恢复到新库', exact:true})).toHaveCount(0);
      await expect(page.getByRole('row').filter({hasText:'#798'})).toContainText('fixture backup failure');
      await scope.getByRole('button', {name:'可恢复备份', exact:true}).click();
      await expect(page.getByRole('row').filter({hasText:'#799'})).toHaveCount(0);
      await expect(page.getByText('legacy-available.dump', {exact:true})).toBeVisible();
      await expect(scope.getByRole('button', {name:'可恢复备份', exact:true})).toBeEnabled();
      await page.screenshot({path:info.outputPath(mobile?'database-backups-mobile.png':'database-backups-desktop.png'), fullPage:true});
    } finally { await context.close(); }
  }
});

test('multiple snapshot policies use a table with one enabled default on desktop and mobile', async ({browser,baseURL,request},info) => {
 for (const mobile of [false,true]) {
  await request.delete('/api/v1/__snapshot_browser/state');
  const {context,page}=await open(browser,baseURL!,mobile);
  const panel=page.getByTestId('system-snapshots-panel');
  const choose=async(value:string,label:string)=>mobile?panel.locator('[data-section-select]').selectOption(value):panel.getByRole('tab',{name:label,exact:true}).click();
  try {
   await expect(panel.locator('[data-section="policy"]')).toHaveCount(0);
   await expect(panel.getByTestId('snapshot-policy-table')).toBeVisible();
   await expect(panel.getByTestId('snapshot-records-table')).toBeVisible();
   await expect(panel.getByTestId('system-snapshot')).toContainText('每日快照');
   const table=page.getByTestId('snapshot-policy-table');
   const existing=table.getByRole('row').filter({hasText:'每日快照'});
   await expect(existing).toBeVisible();
   await existing.getByRole('button',{name:'编辑策略',exact:true}).click();
   const dialog=page.getByRole('dialog');
   await expect(dialog.getByLabel('执行间隔（分钟）')).toHaveValue('1440');
   await dialog.getByLabel('最多保留份数').fill('3');
   await dialog.getByRole('button',{name:'保存策略',exact:true}).click();
   await expect(table).toBeVisible();
   await expect(dialog).not.toBeVisible();
   await expect(panel.getByRole('button',{name:'新建策略',exact:true})).toBeVisible();
   await expect(existing.getByText('3 份',{exact:true})).toBeVisible();
   await panel.getByRole('button',{name:'新建策略',exact:true}).click();
   await dialog.getByLabel('策略名称',{exact:true}).fill('升级前快照');
   await dialog.getByRole('button',{name:'保存策略',exact:true}).click();
   await expect(table).toBeVisible();
   await expect(dialog).not.toBeVisible();
   await expect(page.getByTestId('snapshot-policy-saved')).toBeVisible();
   await expect(table.locator('tbody tr')).toHaveCount(2);
   const alternate=table.getByRole('row').filter({hasText:'升级前快照'});
   await existing.getByRole('button',{name:'开启定时',exact:true}).click();
   await expect(existing.getByText('已开启',{exact:true})).toBeVisible();
   await alternate.getByRole('button',{name:'设为系统默认',exact:true}).click();
   await expect(alternate.getByText('未开启',{exact:true})).toBeVisible();
   await alternate.getByRole('button',{name:'开启定时',exact:true}).click();
   await expect(alternate.getByText('已开启',{exact:true})).toBeVisible();
   await expect(alternate.getByText('系统默认',{exact:true})).toBeVisible();
   await expect(existing.getByText('未开启',{exact:true})).toBeVisible();
   await expect(existing.getByText('系统默认',{exact:true})).toHaveCount(0);
   await expect(table.getByText('已开启',{exact:true})).toHaveCount(1);

   await table.scrollIntoViewIfNeeded();
   if (mobile) {
    const name=await alternate.getByText('升级前快照',{exact:true}).boundingBox();
    const action=await alternate.getByRole('button',{name:'关闭定时',exact:true}).boundingBox();
    const region=await table.locator('..').boundingBox();
    expect(name!.x).toBeGreaterThanOrEqual(region!.x);
    expect(name!.x+name!.width).toBeLessThanOrEqual(action!.x);
   }
   await page.screenshot({path:info.outputPath(mobile?'snapshot-policy-mobile.png':'snapshot-policy-desktop.png'),fullPage:true});
   await expect(dialog).not.toBeVisible();
   await expect(panel.locator('[data-section="points"]')).toBeVisible();
   await expect(panel.getByTestId('system-snapshot')).toContainText('每日快照');
   await expect(panel.getByTestId('system-snapshot')).not.toContainText('升级前快照');
   await expect(panel.getByTestId('system-snapshot')).toHaveCount(1);
   await panel.getByTestId('system-snapshot').scrollIntoViewIfNeeded();
   await page.screenshot({path:info.outputPath(mobile?'snapshot-workspace-mobile.png':'snapshot-workspace-desktop.png'),fullPage:true});
   await choose('overview','概览');
   await choose('points','快照备份');
   await page.reload();
   if(mobile)await page.getByRole('complementary').getByRole('button',{name:'收起',exact:true}).first().click();
   await expect(table.locator('tbody tr')).toHaveCount(2);
   await expect(alternate.getByText('已开启',{exact:true})).toBeVisible();
   await expect(existing.getByText('3 份',{exact:true})).toBeVisible();
   await alternate.getByRole('button',{name:'关闭定时',exact:true}).click();
   await expect(table.getByText('已开启',{exact:true})).toHaveCount(0);
   const state=await(await request.get('/api/v1/__snapshot_browser/state')).json();
   expect(state.overview.policies).toHaveLength(2);
   expect(state.requests.filter((r:any)=>r.method==='POST'&&r.path==='/api/v1/admin/ops/backup/system-snapshots')).toHaveLength(0);
   await request.post('/api/v1/__snapshot_browser/empty-snapshots');
   await expect(panel.getByTestId('system-snapshot')).toHaveCount(0);
   await expect(table.locator('tbody tr')).toHaveCount(2);
   await expect(table.getByText('升级前快照',{exact:true})).toBeVisible();
   await expect(panel.getByTestId('snapshot-records-table')).toContainText('第一份快照');
   await panel.getByTestId('snapshot-records').scrollIntoViewIfNeeded();
   await page.screenshot({path:info.outputPath(mobile?'snapshot-empty-mobile.png':'snapshot-empty-desktop.png'),fullPage:true});
  } finally { await context.close(); }
 }
});

test('manual capture with all schedules off binds the selected system policy', async ({browser,baseURL,request},info) => {
 const {context,page}=await open(browser,baseURL!,true);
 const panel=page.getByTestId('system-snapshots-panel');
 try {
  const table=panel.getByTestId('snapshot-policy-table');
  await expect(panel.getByRole('button',{name:'创建快照',exact:true})).toHaveCount(1);
  const initial=(await (await request.get('/api/v1/__snapshot_browser/state')).json()).overview;
  await table.getByRole('button',{name:'创建快照',exact:true}).click();
  const dialog=page.getByRole('dialog');
  await expect(dialog.getByTestId('snapshot-capture-policy')).toContainText('使用策略：每日快照');
  const createdPolicy=await request.post('/api/v1/admin/ops/backup/system-snapshots/policies',{data:{name:'升级前策略',enabled:false,interval_minutes:1440,retention_days:7,retention_count:3}});
  const alternateID=(await createdPolicy.json()).data.policy.id;
  await request.patch(`/api/v1/admin/ops/backup/system-snapshots/policies/${alternateID}/status`,{data:{enabled:false,make_default:true}});
  await expect(dialog.getByText('系统默认策略已切换，请关闭弹窗并从当前系统默认重新创建。',{exact:true})).toBeVisible();
  await expect(dialog.getByRole('button',{name:'确认执行',exact:true})).toBeDisabled();
  await dialog.getByRole('button',{name:'取消',exact:true}).click();
  const active=table.getByRole('row').filter({hasText:'升级前策略'});
  await expect(active.getByRole('button',{name:'创建快照',exact:true})).toBeVisible();
  await active.getByRole('button',{name:'创建快照',exact:true}).click();
  await expect(dialog.getByTestId('snapshot-capture-policy')).toContainText('使用策略：升级前策略');
  await page.getByRole('dialog').evaluate(async node => { await Promise.all(node.getAnimations({subtree:true}).map(animation => animation.finished.catch(() => {}))); });
  await page.screenshot({path:info.outputPath('snapshot-capture-policy-mobile.png'),fullPage:true});
  await dialog.getByRole('button',{name:'确认执行',exact:true}).click();
  await expect(panel.getByTestId('system-snapshot')).toHaveCount(2);
  const state=await(await request.get('/api/v1/__snapshot_browser/state')).json();
  const writes=state.requests.filter((r:any)=>r.method==='POST'&&r.path==='/api/v1/admin/ops/backup/system-snapshots');
  expect(writes).toHaveLength(1);
  expect(writes[0].body.policy_id).toBe(state.overview.default_policy_id);
  expect(writes[0].body.policy_id).not.toBe(initial.default_policy_id);
  expect(state.overview.snapshots[0].policy_id).toBe(state.overview.default_policy_id);
  expect(state.overview.policies.every((p:any)=>!p.enabled)).toBe(true);
  expect(state.requests.filter((r:any)=>r.path.endsWith('/status')).every((r:any)=>r.body?.enabled!==true)).toBe(true);
  await expect(panel.getByTestId('system-snapshot').first()).toContainText('升级前策略');
 } finally {await context.close();}
});

test("all backup categories show one task panel with a vertical sidebar", async ({browser,baseURL},info) => {
 for (const mobile of [false,true]) {
  const {context,page}=await open(browser,baseURL!,mobile);
  try {
   const snapshots=page.getByTestId('system-snapshots-panel');
   await expect(snapshots.locator('[data-section="points"]')).toBeVisible();
   await expect(snapshots.locator('[data-section="overview"]')).not.toBeVisible();
   await page.getByRole('tab',{name:'数据库备份',exact:true}).click();
   const database=page.getByTestId('backup-operations');
   await expect(database.locator('[data-section="export"]')).toBeVisible();
   await expect(database.locator('[data-section="policies"]')).not.toBeVisible();
   const nav=database.getByRole('tablist',{includeHidden:true});
   const choose=async(scope:any,value:string,label:string)=> mobile ? scope.locator('[data-section-select]').selectOption(value) : scope.getByRole('tab',{name:label,exact:true}).click();
   await expect(nav).toHaveAttribute('aria-orientation','vertical');
   if (!mobile) { const left=await nav.boundingBox(),right=await database.locator('[data-section="export"]').boundingBox();expect(left!.x+left!.width).toBeLessThan(right!.x);expect(Math.abs(left!.y-right!.y)).toBeLessThan(5); }
   await page.screenshot({path:info.outputPath(mobile?'vertical-database-mobile.png':'vertical-database-desktop.png'),fullPage:true});
   await choose(database,'policies','高级设置');
   await expect(database.locator('[data-section="policies"]')).toBeVisible();
   await expect(database.locator('[data-section="files"]')).not.toBeVisible();
   await page.getByRole('tab',{name:'媒体与日志',exact:true}).click();
   const archives=page.getByTestId('resource-archives');
   await expect(archives.locator('[data-section="export"]')).toBeVisible();
   await expect(archives.locator('[data-section="targets"]')).not.toBeVisible();
   await choose(archives,'targets','异地目标');
   await expect(archives.locator('[data-section="targets"]')).toBeVisible();
   await expect(archives.locator('[data-section="jobs"]')).not.toBeVisible();
   await page.getByRole('tab',{name:'运行配置',exact:true}).click();
   const config=page.getByTestId('backup-configuration');
   await expect(config.locator('[data-section="source"]')).toBeVisible();
   await expect(config.locator('[data-section="files"]')).not.toBeVisible();
   await choose(config,'files','文件存储');
   await expect(config.locator('[data-section="files"]')).toBeVisible();
   await expect(config.locator('[data-section="source"]')).not.toBeVisible();
   await page.getByRole('tab',{name:'使用说明',exact:true}).click();
   const guide=page.getByTestId('backup-guide');
   await expect(guide.locator('[data-section="start"]')).toBeVisible();
   await expect(guide.locator('[data-section="recovery"]')).not.toBeVisible();
   await choose(guide,'recovery','正式数据如何还原？');
   await expect(guide.locator('[data-section="recovery"]')).toBeVisible();
   await expect(guide.locator('[data-section="start"]')).not.toBeVisible();
  } finally { await context.close(); }
 }
});

test('unconfigured sites explain why restore is disabled on desktop and narrow screens', async ({browser,baseURL,request},info) => {
 for (const mobile of [false,true]) {
  await request.delete('/api/v1/__snapshot_browser/state');
  const {context,page}=await open(browser,baseURL!,mobile);
  try {
   const snapshots=page.getByTestId('system-snapshots-panel');
   const alert=snapshots.getByTestId('snapshot-recovery-availability');
   await expect(alert).toContainText('当前后端尚未加载维护恢复功能');
   await expect(snapshots.getByTestId('system-snapshot').getByRole('button',{name:'恢复当前实例',exact:true})).toBeDisabled();
   await expect(snapshots.getByRole('button',{name:'导出完整包',exact:true})).toBeEnabled();
   await page.screenshot({path:info.outputPath(`restore-unconfigured-actions-${mobile?'mobile':'desktop'}.png`),fullPage:true});
   await alert.getByRole('button',{name:'查看恢复状态',exact:true}).click();
   await expect(snapshots.locator('[data-section="recovery"]')).toBeVisible();
   await expect(page.getByRole('dialog')).toHaveCount(0);
   await expect(snapshots.locator('[data-section="recovery"]')).toContainText('当前后端尚未加载维护恢复功能');
   const state=await(await request.get('/api/v1/__snapshot_browser/state')).json();
   expect(state.requests.some((item:any)=>item.method==='POST'&&item.path.endsWith('/recovery'))).toBe(false);
   await page.screenshot({path:info.outputPath(`restore-unconfigured-${mobile?'mobile':'desktop'}.png`),fullPage:true});
  } finally { await context.close(); }
 }
});

test("instance restore uses one confirmation and independent push on desktop and narrow screens",async({browser,baseURL,request},info)=>{
 for(const mobile of [false,true]) {
 await request.delete('/api/v1/__snapshot_browser/state');await request.post('/api/v1/__snapshot_browser/managed');
 const {context,page}=await open(browser,baseURL!,mobile);
 try {
  await page.getByTestId('system-snapshots-panel').getByRole('button',{name:'恢复当前实例',exact:true}).click();
  const panel=page.getByTestId('system-recovery-panel');await expect(panel).toBeVisible();
  const dialog=page.getByRole('dialog');await expect(dialog.getByRole('button',{name:'确认执行',exact:true})).toBeDisabled();
  await expect(dialog.getByRole('combobox')).toHaveCount(1);
  await dialog.getByRole('checkbox').check();await dialog.getByRole('button',{name:'确认执行',exact:true}).click();
  await expect(panel.getByText('完成',{exact:true}).first()).toBeVisible({timeout:15000});
  await expect(panel.getByRole('link',{name:'在新窗口检查当前站点',exact:true})).toHaveAttribute('target','_blank');
  for(const name of ['回到原环境','确认正式切换','打开验证站点'])await expect(panel.getByRole('button',{name,exact:true})).toHaveCount(0);
  await expect(panel.getByRole('button',{name:'恢复这份保护快照',exact:true})).toBeVisible();
  const state=await(await page.request.get('/api/v1/__snapshot_browser/state')).json();
  expect(state.requests.some((item:any)=>item.path.startsWith('/__powerx_recovery/jobs/')&&item.path.endsWith('/events')&&item.hasBearer)).toBeTruthy();
  expect(state.requests.some((item:any)=>item.path.startsWith('/__powerx_recovery/jobs/')&&!item.path.endsWith('/events'))).toBeFalsy();
  expect(state.requests.filter((item:any)=>item.method==='POST'&&/\/(cutover|rollback|cleanup)$/.test(item.path))).toHaveLength(0);
  await page.screenshot({path:info.outputPath(`instance-restore-${mobile?'mobile':'desktop'}.png`),fullPage:true});
 } finally {await context.close();}
 }
});

test("root creates a unified package and inspects its components without data-copy actions", async ({
  browser,
  baseURL,
}, info) => {
  const { context, page } = await open(browser, baseURL!);
  try {
    const panel = page.getByTestId("system-snapshots-panel");
    await expect(panel.getByText("系统快照与整站恢复")).toBeVisible();
    const snapshotReads = async () =>
      (
        await (
          await page.request.get("/api/v1/__snapshot_browser/state")
        ).json()
      ).requests.filter(
        (request: any) =>
          request.method === "GET" &&
          request.path.endsWith("/system-snapshots"),
      ).length;
    const beforeRefresh = await snapshotReads();
    await page
      .getByRole("button", { name: "刷新", exact: true })
      .first()
      .click();
    await expect.poll(snapshotReads).toBeGreaterThan(beforeRefresh);
    await expect(panel.getByRole("button", { name: "创建快照", exact: true })).toHaveCount(1);
    await panel.getByRole("button", { name: "创建快照", exact: true }).click();
    await expect(page.getByTestId("snapshot-capture-policy")).toContainText("使用策略：每日快照");
    await page.getByRole('dialog').evaluate(async node => { await Promise.all(node.getAnimations({subtree:true}).map(animation => animation.finished.catch(() => {}))); });
  await page.screenshot({path:info.outputPath("snapshot-capture-policy-desktop.png"),fullPage:true});
    const dialog = page.getByRole("dialog");
    await expect(
      dialog.getByRole("button", { name: "确认执行", exact: true }),
    ).toBeEnabled();
    await dialog.getByRole("button", { name: "确认执行", exact: true }).click();
    await expect(panel.getByTestId("system-snapshot")).toHaveCount(2);
    await panel.getByRole('button',{name:'查看详情',exact:true}).first().click();
    await expect(page.getByRole('dialog').getByRole('button',{name:'仅准备数据副本（高级）',exact:true})).toHaveCount(0);
    await page.keyboard.press('Escape');
    const state = await (
      await page.request.get("/api/v1/__snapshot_browser/state")
    ).json();
    const create = state.requests.find(
      (r: any) => r.method === "POST" && r.path.endsWith("/system-snapshots"),
    );
    expect(create.hasBearer).toBe(true);
    expect(create.idempotencyKey).toMatch(/^[a-f0-9-]{36}$/);
    await expect(page.getByRole("dialog")).not.toBeVisible();
    await page.locator("main").evaluate((element) => {
      element.scrollTop = 0;
    });
    await page.screenshot({
      path: info.outputPath("snapshot-desktop.png"),
      fullPage: true,
    });
  } finally {
    await context.close().catch(() => {});
  }
});

test("mobile opens details and downloads an export without persisting its passphrase", async ({
  browser,
  baseURL,
}, info) => {
  const { context, page } = await open(browser, baseURL!, true);
  try {
    const panel = page.getByTestId("system-snapshots-panel");
    const label = page
      .getByRole("tab", { name: "系统快照", exact: true })
      .locator("span")
      .last();
    expect(
      await label.evaluate(
        (element) => element.scrollWidth <= element.clientWidth,
      ),
    ).toBe(true);
    await panel
      .getByRole("button", { name: "查看详情", exact: true })
      .first()
      .click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByText(/业务恢复验收.*尚未验收/)).toBeVisible();
    await page.keyboard.press("Escape");
    await panel
      .getByRole("button", { name: "导出完整包", exact: true })
      .first()
      .click();
    const passphrase = "browser-private-passphrase";
    await dialog.locator("input[type=password]").fill(passphrase);
    const downloaded = page.waitForEvent("download");
    await dialog.getByRole("button", { name: "确认执行", exact: true }).click();
    expect((await downloaded).suggestedFilename()).toMatch(
      /^powerx-.+\.pxsnapshot$/,
    );
    await expect(dialog).not.toBeVisible();
    expect(
      await page.evaluate(() =>
        JSON.stringify({
          local: { ...localStorage },
          session: { ...sessionStorage },
        }),
      ),
    ).not.toContain(passphrase);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await expect(page.getByRole("dialog")).not.toBeVisible();
    await page.locator("main").evaluate((element) => {
      element.scrollTop = 0;
    });
    await page.screenshot({
      path: info.outputPath("snapshot-mobile.png"),
      fullPage: true,
    });
  } finally {
    await context.close().catch(() => {});
  }
});

test("imports a selected package through the multipart API after passphrase validation", async ({
  browser,
  baseURL,
}) => {
  const { context, page } = await open(browser, baseURL!);
  try {
    await page
      .getByTestId("system-snapshots-panel")
      .getByRole("button", { name: "导入恢复包", exact: true })
      .click();
    const dialog = page.getByRole("dialog");
    const confirm = dialog.getByRole("button", {
      name: "确认执行",
      exact: true,
    });
    await dialog.locator("input[type=file]").setInputFiles({
      name: "fixture.pxsnapshot",
      mimeType: "application/octet-stream",
      buffer: Buffer.from("browser-only-import-fixture"),
    });
    await dialog.locator("input[type=password]").fill("short");
    await expect(confirm).toBeDisabled();
    await dialog
      .locator("input[type=password]")
      .fill("browser-import-private-passphrase");
    await confirm.click();
    await expect(dialog).not.toBeVisible();
    const state = await (
      await page.request.get("/api/v1/__snapshot_browser/state")
    ).json();
    const request = state.requests.find((r: any) =>
      r.path.endsWith("/system-snapshots/imports"),
    );
    expect(request.hasBearer).toBe(true);
    expect(request.multipartFieldsValid).toBe(true);
  } finally {
    await context.close().catch(() => {});
  }
});
