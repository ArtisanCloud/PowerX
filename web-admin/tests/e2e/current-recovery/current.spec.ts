import { expect, test } from '@playwright/test';
import { readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
const readyFile = resolve('../test-results/current-browser-ready.json');
const results: boolean[] = [];
test.afterEach(async ({ page, request }, info) => {
  results.push(info.status === 'passed');
  if(info.status !== 'passed'){
    const responses: Record<string,unknown> = {};
    for(const path of ['/__powerx_recovery/health','/__powerx_recovery/catalog']){
      try{const response=await page.context().request.get(path,{timeout:4000});const data=await response.json();responses[path]={status:response.status(),jobs:data.jobs?.map((j:any)=>({id:j.id,status:j.status,phase:j.phase,error:j.error})),executor_error:data.executor_error};}catch(error){responses[path]=String(error)}
    }
    writeFileSync(info.outputPath('diagnostic.json'),JSON.stringify(responses,null,2));
  }
});
test.afterAll(() => {
  const ready = JSON.parse(readFileSync(readyFile, 'utf8'));
  writeFileSync(ready.done_file, JSON.stringify({ finished: true, passed: results.length === 3 && results.every(Boolean) }));
});
for (const narrow of [false, true]) {
  test(`real ordinary Core recovers at the original web address on ${narrow ? 'narrow' : 'desktop'} screen`, async ({ browser, baseURL }, info) => {
    const ready = JSON.parse(readFileSync(readyFile, 'utf8'));
    expect(baseURL).toBe(ready.url);
    const key = readFileSync(ready.key_file, 'utf8').trim();
    const context = await browser.newContext({ baseURL, viewport: narrow ? { width: 390, height: 844 } : { width: 1440, height: 1000 } });
    const page = await context.newPage();
    await page.addInitScript(()=>{
      const original=window.fetch.bind(window);const requests:any[]=[];(window as any).__recoveryRequests=requests;
      window.fetch=async(input,init)=>{const url=String(input);try{const response=await original(input,init);requests.push({path:new URL(url,location.href).pathname,status:response.status,type:response.headers.get('content-type'),at:Date.now()});return response}catch(error){requests.push({path:new URL(url,location.href).pathname,error:String(error),at:Date.now()});throw error}};
    });
    const errors: string[] = [];page.on('pageerror', error => errors.push(error.message));
    try {
      await page.goto('/__powerx_recovery/');
      await page.getByLabel('恢复密钥', { exact: true }).fill(key);
      await page.getByRole('button', { name: '验证并进入', exact: true }).click();
      await expect(page.getByRole('button', { name: '退出恢复会话', exact: true })).toBeVisible();
      await page.getByRole('tab', { name: '恢复快照', exact: true }).click();
      const row = page.locator('#snapshots tr').filter({ hasText: ready.snapshot_id });
      await row.getByRole('button', { name: '恢复当前实例', exact: true }).click();
      const dialog = page.getByRole('dialog');await dialog.getByRole('checkbox').check();
      await dialog.getByRole('button', { name: '确认执行', exact: true }).click();
      await page.getByLabel('完成后返回站点', { exact: true }).uncheck();
      await expect(page.locator('#jobs article').first().getByText('完成', { exact: true })).toBeVisible({ timeout: 100_000 });
      await page.reload();
      await expect(page.getByRole('button', { name: '退出恢复会话', exact: true })).toBeVisible();
      await page.getByRole('tab', { name: '恢复记录', exact: true }).click();
      await expect(page.locator('#jobs article').first().getByText('完成', { exact: true })).toBeVisible();
      await expect(page.locator('#message')).not.toHaveText('Bad Gateway');
      expect(new URL(page.url()).origin).toBe(baseURL);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
      expect(await page.evaluate(() => ({ local: localStorage.length, session: sessionStorage.length }))).toEqual({ local: 0, session: 0 });
      await page.screenshot({ path: info.outputPath(`same-address-${narrow ? 'narrow' : 'desktop'}.png`), fullPage: true });
      const reads: string[] = [];page.on('request', request => { if (new URL(request.url()).pathname.endsWith('/catalog')) reads.push(request.url()); });
      await page.waitForTimeout(3500);expect(reads).toEqual([]);
      expect(errors).toEqual([]);
    } catch(error) {
      const diagnostic:Record<string,unknown>={};
      diagnostic.requests=await page.evaluate(()=>(window as any).__recoveryRequests);
      for(const path of ['/__powerx_recovery/health','/__powerx_recovery/catalog']){
        try{const response=await context.request.get(path,{timeout:4000});const data=await response.json();diagnostic[path]={status:response.status(),jobs:data.jobs?.map((j:any)=>({id:j.id,status:j.status,phase:j.phase,error:j.error})),executor_error:data.executor_error};}catch(cause){diagnostic[path]=String(cause)}
      }
      writeFileSync(info.outputPath('authenticated-diagnostic.json'),JSON.stringify(diagnostic,null,2));
      throw error;
    } finally { await context.close(); }
  });
}

test('a temporary gateway error is translated and cleared by the next pushed synchronization', async ({ browser, baseURL }) => {
  const ready=JSON.parse(readFileSync(readyFile,'utf8'));
  const context=await browser.newContext({baseURL});const page=await context.newPage();let injected=false;
  await page.addInitScript(()=>{
    (window as any).__notices=[];
    document.addEventListener('DOMContentLoaded',()=>{const message=document.getElementById('message');if(message)new MutationObserver(()=>{(window as any).__notices.push(message.textContent)}).observe(message,{childList:true,subtree:true,characterData:true})});
  });
  await page.route('**/__powerx_recovery/catalog',async route=>{
    if(!injected){injected=true;await route.fulfill({status:502,contentType:'application/json',body:JSON.stringify({message:'Bad Gateway'})});}
    else await route.continue();
  });
  try{
    await page.goto('/__powerx_recovery/');
    await page.getByLabel('恢复密钥',{exact:true}).fill(readFileSync(ready.key_file,'utf8').trim());
    await page.getByRole('button',{name:'验证并进入',exact:true}).click();
    await expect(page.locator('#snapshots tr').filter({hasText:ready.snapshot_id})).toBeVisible();
    await expect(page.locator('#message')).toBeHidden();
    const notices=await page.evaluate(()=>(window as any).__notices as string[]);
    expect(notices.some(value=>value.includes('恢复服务正在重启'))).toBe(true);
    expect(notices.some(value=>value.includes('Bad Gateway'))).toBe(false);
    await page.getByRole('tab',{name:'恢复记录',exact:true}).click();
    await expect(page.locator('#jobs article > p').filter({hasText:/^failed$/})).toHaveCount(0);
  }finally{await context.close();}
});
