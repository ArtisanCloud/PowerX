import {test,expect} from '@playwright/test';
import {mkdtemp,writeFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
const tenant='00000000-0000-4000-8000-000000000001';
for(const width of [1440,390])test(`durable installation, push and reopen at ${width}px`,async({page,request},info)=>{
 await request.post('http://127.0.0.1:39582/__fixture/reset');await page.setViewportSize({width,height:1000});const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 await page.addInitScript(({tenant})=>{const expiresAt=Date.now()+3600000;const token=`header.${btoa(JSON.stringify({tid:tenant,exp:Math.floor(expiresAt/1000)}))}.fixture`;localStorage.setItem('powerx.session',JSON.stringify({version:1,status:'active',sessionId:'install-tasks-browser',revision:1,accessToken:token,refreshToken:'fixture',tokenType:'Bearer',expiresAt,scope:'access',tenantUuid:tenant}));localStorage.setItem('welcome_guide_completed','true');localStorage.setItem('px_locale','zh');},{tenant});
 await page.routeWebSocket(/\/api\/ws/,socket=>socket.close());
 const source=await mkdtemp(join(tmpdir(),'powerx-install-tasks-'));await writeFile(join(source,'plugin.yaml'),'id: com.powerx.plugins.fixture\nversion: 1.0.0\n');
 const snapshot=async()=> (await(await request.get('http://127.0.0.1:39582/__fixture/status')).json()).data;
 try{
  await page.goto('/plugins/local');
  if(width<640){const collapse=page.getByRole('complementary').getByRole('button',{name:'收起',exact:true}).first();await expect(collapse).toBeVisible();await collapse.click();}
  await page.getByRole('button',{name:'本地安装',exact:true}).click();let dialog=page.getByRole('dialog');await expect(dialog).toBeVisible();
  await dialog.getByRole('combobox').first().click();await page.getByRole('option',{name:'本地上传',exact:true}).click();const chooser=page.waitForEvent('filechooser');await dialog.getByRole('button',{name:'选择本地来源'}).click();await(await chooser).setFiles(source);await dialog.getByRole('button',{name:'安装',exact:true}).click();
  await expect(dialog.getByTestId('install-task')).toContainText('正在后台安装');await expect(dialog.getByTestId('install-task')).toContainText('实时连接正常');
  const before=await snapshot();await page.waitForTimeout(1500);const after=await snapshot();expect(after.requests.filter((r:any)=>r.path.includes('/installations')).length).toBe(before.requests.filter((r:any)=>r.path.includes('/installations')).length);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await page.screenshot({path:info.outputPath(`task-${width}.png`),animations:'disabled'});
  const taskID=after.task.task_id;await dialog.getByRole('button',{name:'关闭，稍后查看'}).click();await expect(dialog).toBeHidden();await expect.poll(async()=> (await snapshot()).subscribers).toBe(0);
  await page.reload();if(width<640)await page.getByRole('complementary').getByRole('button',{name:'收起',exact:true}).first().click();await page.getByRole('button',{name:'本地安装',exact:true}).click();dialog=page.getByRole('dialog');await expect(dialog.getByTestId('install-task')).toContainText('正在后台安装');expect((await snapshot()).task.task_id).toBe(taskID);
  await request.post('http://127.0.0.1:39582/__fixture/complete');await expect(dialog.getByTestId('install-task')).toContainText('安装已完成');
  const final=await snapshot();expect(final.requests.filter((r:any)=>r.method==='POST'&&r.path.endsWith('/installations'))).toHaveLength(1);expect(final.requests.filter((r:any)=>r.method==='PUT'&&r.path.endsWith('/package'))).toHaveLength(1);expect(errors).toEqual([]);
  await page.screenshot({path:info.outputPath(`completed-${width}.png`),animations:'disabled'});await dialog.getByRole('button',{name:'完成',exact:true}).click();await expect(dialog).toBeHidden();
 }finally{await rm(source,{recursive:true,force:true});}
});
