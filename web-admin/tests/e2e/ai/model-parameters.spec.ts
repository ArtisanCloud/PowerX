import { test, expect } from '@playwright/test';

for (const narrow of [false, true]) {
  test(`per-model parameters retain drafts and saved overrides (${narrow ? 'narrow' : 'desktop'})`, async ({ page }, info) => {
    test.setTimeout(180_000);
    await page.setViewportSize(narrow ? { width: 390, height: 844 } : { width: 1440, height: 1000 });
    const tenant = '00000000-0000-4000-8000-000000000001';
    await page.addInitScript(tenant => {
      const expiresAt = Date.now() + 3600_000;
      const token = `header.${btoa(JSON.stringify({ exp: Math.floor(expiresAt / 1000), tid: tenant }))}.fixture`;
      localStorage.setItem('powerx.session', JSON.stringify({ version: 1, status: 'active', sessionId: 'model-parameter-proof', revision: 1, accessToken: token, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid: tenant }));
      localStorage.setItem('welcome_guide_completed','true'); localStorage.setItem('px_locale','zh');
    }, tenant);
    const profiles: any[] = [{ modality:'llm',provider:'openai',model:'gpt-4o',defaults:{temperature:.25,maxTokens:1536,topP:.8} }];
    let active = profiles[0], syncs = 0;
    const saves: any[] = [], errors: string[] = [];
    const entries = () => [
      {id:'gpt-4o',parameter_profile:{version:1,provider:'openai',model:'gpt-4o',source:'official_docs',checked_at:'2026-11-02',defaults:{temperature:1,top_p:1},sampling:'supported'}},
      {id:'gpt-6-luna',parameter_profile:{version:1,provider:'openai',model:'gpt-6-luna',source:'official_docs',checked_at:'2026-11-02',defaults:{reasoning_effort:'medium'},sampling:'reasoning_none',reasoning_efforts:['none','low','medium','high','xhigh','max']}},
      {id:'unknown-native',parameter_profile:{version:1,provider:'openai',model:'unknown-native',source:'unknown',defaults:{},sampling:'unknown'}},
    ];
    page.on('pageerror', error => errors.push(error.message));
    await page.routeWebSocket(/\/api\/ws/, socket => socket.close());
    await page.route(url => url.pathname.startsWith('/api/'),async route => {
      const url = new URL(route.request().url()), path = url.pathname;
      if(path.includes('/_nuxt_icon/')) return route.fallback();
      let data: any = {};
      if(path.endsWith('/setup/status')) data={configured:true,requires_login:true,install_status:'installed'};
      else if(path.endsWith('/me/context')) data={is_root:true,current_tenant_uuid:tenant,current_member_id:1,user:{id:1,display_name:'参数验收',status:1,is_root:true},members:[{tenant_uuid:tenant,tenant_name:'验收',member_id:1,is_admin:true}]};
      else if(path.endsWith('/menus')) data={menus:[{id:'ai',title:'模型配置',path:'/settings/ai',visible:true,origin:'system'}]};
      else if(path.endsWith('/agents/providers')) data={providers:[{id:'openai',name:'OpenAI',configured:true,auth:{scheme:'bearer',fields:['api_key','base_url'],defaults:{base_url:'https://api.openai.com/v1'}}}]};
      else if(path.endsWith('/agents/settings/profiles')) data={env:'dev',profiles};
      else if(path.endsWith('/agents/settings/credentials')) data={env:'dev',credentials:[{provider:'openai',data:{base_url:''}}]};
      else if(path.endsWith('/agents/settings/active')) { if(route.request().method()==='POST') active=profiles.find(p=>p.model===route.request().postDataJSON().model)||active; data={profile:active}; }
      else if(path.endsWith('/agents/models/sync')) {syncs++;data={provider:'openai',env:'dev',app:'',protocol:'openai',source:'https://api.openai.com/v1/models',updated_at:'2026-11-02',revision:`revision-${syncs}`,models:entries()};}
      else if(path.endsWith('/agents/models')) data={models:entries().map(p=>p.id),catalog_source:'online',items:entries()};
      else if(path.endsWith('/agents/settings/save')) {
        const body=route.request().postDataJSON(); saves.push(body);
        const p=body.llm, defaults={...p,reasoning_effort:p.reasoningEffort};
        const existing=profiles.find(pf=>pf.model===p.model);
        if(existing) existing.defaults=defaults; else profiles.push({modality:'llm',provider:p.provider,model:p.model,defaults});
        data={ok:true};
      }
      else if(path.endsWith('/skills/source-policy')) data={allowlist:['builtin','plugin'],effective_source:'default'};
      return route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({code:200,data})});
    });
    await page.goto('/settings/ai');
    const model = page.getByTestId('ai-model-select'), budget = page.getByTestId('llm-max-tokens');
    await expect(model).toContainText('gpt-4o',{timeout:90_000}); await expect(model).toBeEnabled();
    if(narrow) {
      const backdrop=page.locator('[class~="fixed"][class~="inset-0"][class~="z-40"][class~="md:hidden"]');
      if(await backdrop.isVisible()) await backdrop.click({position:{x:380,y:100}});
    }
    await expect(budget).toHaveValue('1536');
    const select = async (name: string) => {
      await model.scrollIntoViewIfNeeded(); await model.click();
      const option = page.getByRole('option',{name:new RegExp(`^${name}`)});
      await expect(option).toBeVisible();
      await option.click();
      await expect(page.getByRole('listbox')).toBeHidden();
    };
    await budget.fill('2048');
    await select('gpt-6-luna'); await expect(budget).toHaveValue('4096');
    const reasoning=page.getByTestId('llm-reasoning'); await expect(reasoning).toContainText('medium');
    await expect(page.getByText('当前型号或推理强度不支持采样设置，温度和 Top P 不会发送。')).toBeVisible();
    await budget.fill('777');
    await select('gpt-4o'); await expect(budget).toHaveValue('2048');
    await select('gpt-6-luna'); await expect(budget).toHaveValue('777');
    await page.getByTestId('ai-online-models').click();
    const dialog=page.getByRole('dialog'); await expect(dialog.getByTestId('online-model-summary')).toBeVisible();
    await expect(dialog.getByText('参数来源：官方文档').first()).toBeVisible();
    await expect(dialog.getByText('供应商未提供参数信息；使用平台预设')).toBeVisible();
    const bounds=await dialog.boundingBox();expect(bounds!.x).toBeGreaterThanOrEqual(0);expect(bounds!.x+bounds!.width).toBeLessThanOrEqual(narrow?390:1440);
    await dialog.getByRole('button',{name:'完成',exact:true}).click();await expect(budget).toHaveValue('777');
    await page.getByTestId('ai-save-settings').click(); await expect.poll(()=>saves.length).toBe(1);
    expect(saves[0].llm).toMatchObject({model:'gpt-6-luna',maxTokens:777,reasoningEffort:'medium'});
    expect(saves[0].llm).not.toHaveProperty('temperature');expect(saves[0].llm).not.toHaveProperty('topP');
    await expect(page.getByText('LLM 文本 配置已更新',{exact:true})).toBeVisible();
    await page.reload();await expect(model).toContainText('gpt-6-luna',{timeout:90_000});await expect(budget).toHaveValue('777');
    if(narrow) { const backdrop=page.locator('[class~="fixed"][class~="inset-0"][class~="z-40"][class~="md:hidden"]');if(await backdrop.isVisible()) await backdrop.click({position:{x:380,y:100}}); }
    await budget.scrollIntoViewIfNeeded();
    await page.screenshot({path:info.outputPath(`model-parameters-${narrow?'narrow':'desktop'}.png`),fullPage:true,animations:'disabled'});
    expect(errors).toEqual([]);expect(syncs).toBe(1);
  });
}
